package db

import (
	"fmt"
	"net/http"
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

// A Notification is an element of the user's home page.
// There are two types of notifications:
// events, which can be pending, confirmed or cancelled, and
// notifications of new memberships to the user's circles.
type Notification struct {
	*Event
	*Member
}

// Date returns the sort date of the notification.
func (n Notification) Date() time.Time {
	if n.Event != nil {
		return n.Event.Date
	}
	return n.Member.Date
}

// ByDate is a wrapper for sorting notifications by date.
type ByDate []Notification

func (b ByDate) Len() int           { return len(b) }
func (b ByDate) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b ByDate) Less(i, j int) bool { return b[i].Date().After(b[j].Date()) }

// GetNotifications returns the notifications of a user sorted by descending date.
func (db *DB) GetNotifications(userId string) ([]Notification, error) {
	// Get list of circles
	var vc struct{ Rows []struct{ Doc circle } }
	s, err := db.get(db.view("circles", userId, true), &vc)
	if err != nil {
		return nil, errors.Stack(err, "get feed: error querying circles view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get feed: db get circles view error (status %d)", s)
	}
	if len(vc.Rows) == 0 {
		return nil, nil
	}

	// Get user's dismissed notifications
	var vd struct{ Rows []struct{ Value string } }
	s, err = db.get(db.view("dismiss", userId, false), &vd)
	if err != nil {
		return nil, errors.Stack(err, "get feed: error querying dismiss view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get feed: db get dismiss view error (status %d)", s)
	}
	skip := make(map[string]struct{})
	for _, r := range vd.Rows {
		skip[r.Value] = struct{}{}
	}

	n := make([]Notification, 0)
	m := make(map[string]event)

	// For each circle
	for _, rc := range vc.Rows {
		// Get list of events
		var ve struct{ Rows []struct{ Doc event } }
		s, err := db.get(db.view("events", rc.Doc.Id, true), &ve)
		if err != nil {
			return nil, errors.Stack(err, "get feed: error querying events view")
		}
		if s != http.StatusOK {
			return nil, fmt.Errorf("get feed: db get events view error (status %d)", s)
		}
		for _, re := range ve.Rows {
			if _, ok := m[re.Doc.Id]; !ok {
				m[re.Doc.Id] = re.Doc
			}
		}

		// Get list of members
		var vu struct {
			Rows []struct {
				Id  string `json:"_id"`
				Key []string
				Doc user
			}
		}
		s, err = db.get(db.dateView("members", rc.Doc.Id, true), &vu)
		if err != nil {
			return nil, errors.Stack(err, "get feed: error querying members view")
		}
		if s != http.StatusOK {
			return nil, fmt.Errorf("get feed: db get members view error (status %d)", s)
		}
		for _, ru := range vu.Rows {
			if _, ok := skip[ru.Id]; ok {
				continue
			}
			date, err := time.Parse(time.RFC3339, ru.Key[1])
			if err != nil {
				return nil, fmt.Errorf("get feed: error parsing date")
			}
			m := Member{
				User: User{
					Id:   ru.Doc.Id,
					Name: ru.Doc.Name,
				},
				Circle: Circle{
					Id:   rc.Doc.Id,
					Name: rc.Doc.Name,
					Slug: rc.Doc.Slug,
				},
				Id:   ru.Id,
				Date: date,
				Me:   ru.Doc.Id == userId,
			}
			n = append(n, Notification{Member: &m})
		}
	}

	// For each event
	for id, e := range m {
		if _, ok := skip[id]; ok {
			continue
		}
		// Get list of participants
		var v struct {
			Rows []struct {
				Key []string
				Doc user
			}
		}
		s, err := db.get(db.dateView("participants", id, true), &v)
		if err != nil {
			return nil, errors.Stack(err, "get feed: error querying participants view")
		}
		if s != http.StatusOK {
			return nil, fmt.Errorf("get feed: db get participants view error (status %d)", s)
		}
		if len(v.Rows) == 0 {
			return nil, fmt.Errorf("get feed: db inconsistent: event with no participants")
		}
		p := make([]Participant, len(v.Rows))
		np := 0
		for i, r := range v.Rows {
			date, err := time.Parse(time.RFC3339, r.Key[1])
			if err != nil {
				return nil, fmt.Errorf("get feed: error parsing date")
			}
			p[i] = Participant{
				Id:   r.Doc.Id,
				Name: r.Doc.Name,
				Date: date,
			}
			if p[i].Date.Before(e.Date) {
				np++
			}
		}
		var status string
		if np >= e.Threshold {
			status = "Confirmed"
		} else {
			if e.Date.Before(time.Now()) {
				status = "Cancelled"
			} else {
				status = "Pending"
			}
		}
		ev := Event{
			Id:        e.Id,
			Location:  e.Location,
			Title:     e.Title,
			Info:      e.Info,
			Date:      e.Date,
			Threshold: e.Threshold,
			Creator: User{
				Id:   p[0].Id,
				Name: p[0].Name,
			},
			Created:      p[0].Date,
			Status:       status,
			Participants: p,
		}
		n = append(n, Notification{Event: &ev})
	}

	// Optionally, group similar notification (Amin and 3 others joined your circle…)

	// Sort by date
	sort.Sort(ByDate(n))

	return n, nil
}

// A dismiss is a CouchDB dismiss document.
type dismiss struct {
	Id   string `json:"_id,omitempty"`
	Rev  string `json:"_rev,omitempty"`
	Type string `json:"type"`
	User string `json:"user"`
	What string `json:"what"`
}

// DismissFeedEntry creates a dismiss document in the database
// so that the notification disappears from the user's home page.
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
