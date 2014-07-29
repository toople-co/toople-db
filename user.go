package db

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
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
	s, err := db.get(db.view("email", email, false, false), &v)
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
	s, err := db.get(db.view("email", email, true, false), &v)
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
// It can contain either an Event or a Circle.
type FeedEntry struct {
	*Event
	*Circle
}

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
			Creator:   e.Creator,
			Status:    e.Status,
			Date:      e.Date,
			Created:   e.Created,
			Threshold: e.Threshold,
		}
	case "circle":
		var c circle
		if err := json.Unmarshal(b, &c); err != nil {
			return err
		}
		f.Circle = &Circle{
			Id:   c.Id,
			Name: c.Name,
			Slug: c.Slug,
		}
	default:
		return fmt.Errorf("feed entry: not event nor circle")
	}
	return nil
}

// GetFeed returns the feed (a slice of feed entries) of a user.
func (db *DB) GetFeed(userId string) ([]FeedEntry, error) {
	var v struct{ Rows []struct{ Doc FeedEntry } }
	s, err := db.get(fmt.Sprintf(`_design/toople/_view/user?startkey=["%s",{}]&endkey=["%[1]s"]&include_docs=true&descending=true`, url.QueryEscape(userId)), &v)
	if err != nil {
		return nil, errors.Stack(err, "get feed: error querying user view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get feed: database error")
	}
	feed := make([]FeedEntry, 0)
	for _, r := range v.Rows {
		feed = append(feed, r.Doc)
	}
	return feed, nil
}
