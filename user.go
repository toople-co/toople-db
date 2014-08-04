package db

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	// "unicode"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/simleb/errors"
)

// A User is a proxy for a full user document in the database.
type User struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// A user is a CouchDB user document.
type user struct {
	Id       string   `json:"_id,omitempty"`
	Rev      string   `json:"_rev,omitempty"`
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Emails   []string `json:"emails"`
	Password string   `json:"password"`
}

// NewUser creates a new user in the database with a name, email and password.
// The name cannot be empty.
// The email must contain "@".
// The password must have at least 8 characters.
func (db *DB) NewUser(name, email, password string) (*User, error) {
	// Validate fields
	if err := validateName(name); err != nil {
		return nil, errors.Stack(err, "new user: bad name")
	}
	if err := validateEmail(email); err != nil {
		return nil, errors.Stack(err, "new user: bad email")
	}
	if err := validatePassword(password); err != nil {
		return nil, errors.Stack(err, "new user: bad password")
	}

	// Check if email is available
	var v struct{ Rows []struct{} }
	s, err := db.get(db.view("email", email, false), &v)
	if err != nil {
		return nil, errors.Stack(err, "new user: error querying email view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("new user: database error")
	}
	if len(v.Rows) > 0 {
		return nil, fmt.Errorf("new user: email not available")
	}

	// Encrypt password
	pwd, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Stack(err, "new user: failed to encrypt password")
	}

	u := user{
		Type:     "user",
		Name:     name,
		Emails:   []string{email},
		Password: string(pwd),
	}

	// Create document in database
	var r struct{ Id string }
	s, err = db.post("", &u, &r)
	if err != nil {
		return nil, errors.Stack(err, "new user: cannot post document")
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new user: db post error (status %d)", s)
	}

	return &User{Id: r.Id, Name: name}, nil
}

// validateName returns nil when a name is valid.
// Currently, a name is valid if it is not empty.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("validate name: empty")
	}
	return nil
}

// validateEmail returns nil when an email is valid.
// Currently, an email is valid if it contains "@".
func validateEmail(email string) error {
	if !strings.Contains(email, "@") {
		return fmt.Errorf("validate email: @ missing")
	}
	return nil
}

// validatePassword returns nil if a password is valid.
// Currently, a password is valid if it contains at least 8 characters.
func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("validate password: too short (min 8)")
	}
	// lower, upper, digit, punct := false, false, false, false
	// for _, r := range password {
	// 	switch {
	// 	case unicode.IsLower(r):
	// 		lower = true
	// 	case unicode.IsUpper(r):
	// 		upper = true
	// 	case unicode.IsDigit(r):
	// 		digit = true
	// 	case unicode.IsPunct(r):
	// 		punct = true
	// 	}
	// }
	// switch {
	// case !lower:
	// 	return fmt.Errorf("validate password: must contain lowercase")
	// case !upper:
	// 	return fmt.Errorf("validate password: must contain uppercase")
	// case !digit:
	// 	return fmt.Errorf("validate password: must contain digits")
	// case !punct:
	// 	return fmt.Errorf("validate password: must contain punctuation")
	// }
	return nil
}

// AuthUser tries to authenticate a user from an email and a password.
// It returns true only if authentication is successful.
// It returns the user matching the email or nil if none is found, even if authentication fails.
func (db *DB) AuthUser(email, password string) (bool, *User, error) {
	email = normalizeEmail(email)

	// Find user doc from email
	var v struct{ Rows []struct{ Doc user } }
	s, err := db.get(db.view("email", email, true), &v)
	if err != nil {
		return false, nil, errors.Stack(err, "auth user: error querying email view")
	}
	if s != http.StatusOK {
		return false, nil, fmt.Errorf("auth user: database error")
	}
	if len(v.Rows) == 0 {
		return false, nil, nil // User not found
	}

	// Compare hashed passwords
	w := v.Rows[0].Doc
	u := &User{Id: w.Id, Name: w.Name}
	if err := bcrypt.CompareHashAndPassword([]byte(w.Password), []byte(password)); err != nil {
		return false, u, nil
	}
	return true, u, nil
}

// normalizeEmail returns a copy of the email with the server part in lowercase.
// This allows
func normalizeEmail(email string) string {
	if n := strings.LastIndex(email, "@"); n != -1 {
		return email[0:n+1] + strings.ToLower(email[n+1:])
	}
	return email
}

// A FeedEntry is an entry in a user's feed.
// It can contain either an Event or a Member.
type FeedEntry struct {
	*Event
	*Member
}

func (f *FeedEntry) Type() string {
	if f.Event != nil {
		return "event"
	}
	return "member"
}

func (f *FeedEntry) Id() string {
	if f.Event != nil {
		return f.Event.Id
	}
	return f.Member.Id
}

func (f *FeedEntry) Date() time.Time {
	if f.Event != nil {
		return f.Event.Date
	}
	return f.Member.Date
}

type ByDate []FeedEntry

func (b ByDate) Len() int           { return len(b) }
func (b ByDate) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByDate) Less(i, j int) bool { return b[i].Date().After(b[j].Date()) }

// UnmarshalJSON populates a FeedEntry from a JSON blob.
func (f *FeedEntry) UnmarshalJSON(b []byte) error {
	var t struct{ Type string }
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	switch t.Type {
	case "event":
		var e event
		if err := json.Unmarshal(b, &e); err != nil {
			return err
		}
		f.Event = &Event{
			Id:        e.Id,
			Location:  e.Location,
			Title:     e.Title,
			Info:      e.Info,
			Date:      e.Date,
			Threshold: e.Threshold,
		}
	case "user":
		var u user
		if err := json.Unmarshal(b, &u); err != nil {
			return err
		}
		f.Member = &Member{
			User: User{
				Id:   u.Id,
				Name: u.Name,
			},
		}
	default:
		return fmt.Errorf("feed entry: not event nor member")
	}
	return nil
}

// GetFeed returns the feed (a slice of feed entries) of a user.
func (db *DB) GetFeed(userId string) ([]FeedEntry, error) {
	// Get list of circles
	var v struct{ Rows []struct{ Doc circle } }
	s, err := db.get(db.view("circles", userId, true), &v)
	if err != nil {
		return nil, errors.Stack(err, "get feed: error querying circles view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get feed: db get error (status %d)", s)
	}
	if len(v.Rows) == 0 {
		return nil, nil
	}

	// For each circle, get events and members
	fe := make(map[string]FeedEntry)
	for _, r := range v.Rows {
		var v struct {
			Rows []struct {
				Key []string
				Doc FeedEntry
			}
		}
		s, err := db.get(db.dateView("feed", r.Doc.Id, true), &v)
		if err != nil {
			return nil, errors.Stack(err, "get feed: error querying feed view")
		}
		if s != http.StatusOK {
			return nil, fmt.Errorf("get feed: db get feed view error (status %d)", s)
		}
		for _, rf := range v.Rows {
			if rf.Doc.Type() == "member" {
				rf.Doc.Member.Circle = Circle{
					Id:   r.Doc.Id,
					Name: r.Doc.Name,
					Slug: r.Doc.Slug,
				}
				rf.Doc.Member.Id = rf.Key[2]
				rf.Doc.Member.Date, err = time.Parse(time.RFC3339, rf.Key[1])
				if err != nil {
					return nil, fmt.Errorf("get feed: error parsing date")
				}
				rf.Doc.Member.Me = rf.Doc.Member.User.Id == userId
			}
			fe[rf.Doc.Id()] = rf.Doc
		}
	}

	// For each event, get status
	for k, v := range fe {
		if v.Type() == "event" {
			var w struct {
				Rows []struct {
					Doc struct {
						Id   string `json:"_id"`
						Name string
					}
				}
			}
			s, err := db.get(fmt.Sprintf(
				`_design/toople/_view/participants`+
					`?startkey=["%s"]&endkey=["%[1]s","%s"]&include_docs=true`,
				url.QueryEscape(v.Event.Id),
				url.QueryEscape(v.Event.Date.Format(time.RFC3339Nano))), &w)
			if err != nil {
				return nil, errors.Stack(err, "get feed: error querying participants view")
			}
			if s != http.StatusOK {
				return nil, fmt.Errorf("get feed: db get error (status %d)", s)
			}
			n := len(w.Rows)
			if n < 1 {
				return nil, fmt.Errorf("get feed: events must have at least one participant")
			}
			v.Event.Creator.Id = w.Rows[0].Doc.Id
			v.Event.Creator.Name = w.Rows[0].Doc.Name
			if n >= v.Event.Threshold {
				fe[k].Status = "Confirmed"
			} else {
				if v.Event.Date.Before(time.Now()) {
					fe[k].Status = "Cancelled"
				} else {
					fe[k].Status = "Pending"
				}
			}
		}
	}

	// Optionally, group similar notification (Amin and 3 others joined your circle…)

	// Get user's dismissed notifications
	var w struct{ Rows []struct{ Key, Value string } }
	s, err = db.get(db.view("dismiss", userId, false), &w)
	if err != nil {
		return nil, errors.Stack(err, "get feed: error querying dismiss view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get feed: db get dismiss view error (status %d)", s)
	}
	d := make(map[string]struct{})
	for _, r := range w.Rows {
		d[r.Value] = struct{}{}
	}

	// Build FeedEntry slice sorted by date
	feed := make([]FeedEntry, 0)
	for id, f := range fe {
		if _, found := d[id]; !found {
			feed = append(feed, f)
		}
	}
	sort.Sort(ByDate(feed))
	return feed, nil
}

type dismiss struct {
	Id   string `json:"_id,omitempty"`
	Rev  string `json:"_rev,omitempty"`
	Type string `json:"type"`
	User string `json:"user"`
	What string `json:"what"`
}

func (db *DB) DismissFeedEntry(id, userId string) error {
	d := dismiss{
		Type: "dismiss",
		User: userId,
		What: id,
	}
	s, err := db.post("", &d, nil)
	if err != nil {
		return errors.Stack(err, "dismiss: database error")
	}
	if s != http.StatusCreated {
		return fmt.Errorf("dismiss: got status %d trying to create dismiss", s)
	}
	return nil
}
