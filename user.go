package db

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"unicode"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/simleb/errors"
)

// A User represents a physical user.
// It has a name, a list of emails, a password and a list of circles.
type User struct {
	// doc is the full user document in the database
	doc user
}

// A user is a JSON friendly structure for storing a user document.
type user struct {
	Id       string   `json:"_id,omitempty"`
	Rev      string   `json:"_rev,omitempty"`
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Emails   []string `json:"emails"`
	Password string   `json:"password"`
}

// NewUser creates and initialize a User with a name, email and password.
func (db *DB) NewUser(name, email, password string) (*User, error) {
	// Create User
	u := new(User)
	u.doc.Type = "user"
	if err := u.SetName(name); err != nil {
		return nil, errors.Stack(err, "new user: bad name")
	}
	if err := u.SetPrimaryEmail(email); err != nil {
		return nil, errors.Stack(err, "new user: bad email")
	}
	if err := u.SetPassword(password); err != nil {
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

	// Create document in database
	var r struct{ Id, Rev string }
	s, err = db.post("", u.doc, &r)
	if err != nil {
		return nil, errors.Stack(err, "new user: cannot post document")
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new user: db post error (status %d)", s)
	}

	u.doc.Id = r.Id
	u.doc.Rev = r.Rev

	return u, nil
}

// AuthUser finds a user from an email and checks their credentials.
// It returns true only if authentication is successful.
// It returns the user matching the email if one is found, even if authentication fails.
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
	u := v.Rows[0].Doc
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return false, &User{u}, nil
	}
	return true, &User{u}, nil
}

type FeedEntry struct {
	*Event
	*Circle
}

func (f *FeedEntry) UnmarshalJSON(b []byte) error {
	var t struct{ Type string }
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	switch t.Type {
	case "event":
		f.Event = new(Event)
		if err := json.Unmarshal(b, &f.Event.doc); err != nil {
			return err
		}
	case "circle":
		f.Circle = new(Circle)
		if err := json.Unmarshal(b, &f.Circle.doc); err != nil {
			return err
		}
	default:
		return fmt.Errorf("feed entry: not event nor circle")
	}
	return nil
}

// GetFeed
func (db *DB) GetFeed(id string) ([]FeedEntry, error) {
	var v struct{ Rows []struct{ Doc FeedEntry } }
	s, err := db.get(fmt.Sprintf(`_design/toople/_view/user?startkey=["%s",{}]&endkey=["%[1]s"]&include_docs=true&descending=true`, url.QueryEscape(id)), &v)
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

// Id returns the user's id.
func (u *User) Id() string {
	return string(u.doc.Id)
}

// Name returns the user's name.
func (u *User) Name() string {
	return u.doc.Name
}

// SetName sets the user's name.
func (u *User) SetName(name string) error {
	if name == "" {
		return fmt.Errorf("set name: name is empty")
	}
	u.doc.Name = name
	return nil
}

// Emails returns the list of emails of the user.
// The firt one is the primary email.
func (u *User) Emails() []string {
	return u.doc.Emails
}

// SetPrimaryEmail sets the user's primary email.
// If the email exists, it becomes primary.
// Otherwise it is added to the list.
func (u *User) SetPrimaryEmail(email string) error {
	if email == "" {
		return fmt.Errorf("add email: empty email address")
	}
	email = normalizeEmail(email)
	for i, e := range u.doc.Emails {
		if e == email {
			u.doc.Emails = append([]string{email}, append(u.doc.Emails[:i], u.doc.Emails[i+1:]...)...)
			return nil
		}
	}
	u.doc.Emails = append([]string{email}, u.doc.Emails...)
	return nil
}

// SetPassword sets the user's password.
func (u *User) SetPassword(password string) error {
	if err := validatePassword(password); err != nil {
		return errors.Stack(err, "set password: invalid")
	}
	pwd, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	u.doc.Password = string(pwd)
	return nil
}

// TestPassword tests a password against the user's password.
func (u *User) TestPassword(password string) bool {
	if err := bcrypt.CompareHashAndPassword([]byte(u.doc.Password), []byte(password)); err != nil {
		return false
	}
	return true
}

// validatePassword checks if a password satisfies the password policy.
func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("validate password: too short (min 8)")
	}
	lower, upper, digit, punct := false, false, false, false
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r):
			punct = true
		}
	}
	switch {
	case !lower:
		return fmt.Errorf("validate password: must contain lowercase")
	case !upper:
		return fmt.Errorf("validate password: must contain uppercase")
	case !digit:
		return fmt.Errorf("validate password: must contain digits")
	case !punct:
		return fmt.Errorf("validate password: must contain punctuation")
	}
	return nil
}
