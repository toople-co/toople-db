package db

import (
	"encoding/gob"
	"fmt"
	"net/http"
	"unicode"

	"code.google.com/p/go.crypto/bcrypt"
	"github.com/simleb/errors"
)

// A User represents a physical user.
// It has a name, a list of emails, a password and a list of circles.
type User struct {
	doc user
}

// A UserId is a reference to a user document.
type UserId string

// A user is a JSON friendly structure for storing a user document.
type user struct {
	Id       string     `json:"_id,omitempty"`
	Rev      string     `json:"_rev,omitempty"`
	Type     string     `json:"type"`
	Name     string     `json:"name"`
	Emails   []string   `json:"emails"`
	Password string     `json:"password"`
	Circles  []CircleId `json:"circles"`
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
	id, err := db.GetUserIdFromEmail(email)
	if err != nil {
		return nil, errors.Stack(err, "new user: error checking email avaibility")
	}
	if id == UserId("") {
		return nil, fmt.Errorf("new user: email not available")
	}

	// Create document in database
	var r struct{ Id, Rev string }
	s, err := db.post("", u.doc, &r)
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
	u := v.Rows[0].Doc
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return false, &User{u}, nil
	}
	return true, &User{u}, nil
}

// PutUser updates a user in the database while keeping the database consistent.
// If the password is in plain text, it will be hashed. Emails have to be unique.
func (db *DB) PutUser(u *User) error {
	// Check for empty fields
	if u.doc.Name == "" {
		return fmt.Errorf("put user: user has no name")
	}
	if len(u.doc.Emails) == 0 {
		return fmt.Errorf("put user: user has no email")
	}
	if len(u.doc.Password) == 0 {
		return fmt.Errorf("put user: user has no password")
	}

	// Check if all emails are available
	var e map[string]int
	for _, email := range u.doc.Emails {
		var v struct{ Rows []struct{ Id string } }
		s, err := db.get(db.view("email", email, false), &v)
		if err != nil {
			return err
		}
		if s != http.StatusOK {
			return fmt.Errorf("put user: database error")
		}
		for _, r := range v.Rows {
			if r.Id != u.doc.Id {
				return fmt.Errorf("put user: email exists for a different user")
			}
		}
		if e[email]++; e[email] > 2 {
			return fmt.Errorf("put user: duplicate email")
		}
	}

	// Hash password if not hashed already
	if _, err := bcrypt.Cost([]byte(u.doc.Password)); err != nil {
		pwd, err := bcrypt.GenerateFromPassword([]byte(u.doc.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.doc.Password = string(pwd)
	}

	// Put document (return error on conflict)
	s, err := db.put(u.doc.Id, u)
	if err != nil {
		return err
	}
	switch s {
	case http.StatusCreated: // ok
	case http.StatusConflict:
		return fmt.Errorf("put user: conflict, get a fresh revision and try again")
	default:
		return fmt.Errorf("put user: database error")
	}
	return nil
}

// GetUser retrieves an existing user from the database.
func (db *DB) GetUser(id UserId) (*User, error) {
	var u user
	s, err := db.get(string(id), &u)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get user: database error")
	}
	return &User{u}, nil
}

// GetUserIdFromEmail retrieves the id of an user from one of their email addresses.
func (db *DB) GetUserIdFromEmail(email string) (UserId, error) {
	var v struct{ Rows []struct{ Id string } }
	s, err := db.get(db.view("email", email, false), &v)
	if err != nil {
		return UserId(""), errors.Stack(err, "get user id from email: database unreachable")
	}
	if s != http.StatusOK {
		return UserId(""), fmt.Errorf("get user id from email: db view error (status %d)", s)
	}
	if len(v.Rows) == 0 {
		return UserId(""), fmt.Errorf("get user id from email: not found")
	}
	return UserId(v.Rows[0].Id), nil
}

// GetUserFromEmail retrieves an existing user from one of their email addresses.
func (db *DB) GetUserFromEmail(email string) (*User, error) {
	var v struct{ Rows []struct{ Doc user } }
	s, err := db.get(db.view("email", email, true), &v)
	if err != nil {
		return nil, errors.Stack(err, "get user from email: database unreachable")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get user from email: db view error (status %d)", s)
	}
	if len(v.Rows) == 0 {
		return nil, fmt.Errorf("get user from email: not found")
	}
	return &User{doc: v.Rows[0].Doc}, nil
}

// DeleteUser removes a user from the database
// as well as all its event participations and circle memberships.
// The function fails if the user is the only admin of a circle.
func (db *DB) DeleteUser(id UserId) error {
	u, err := db.GetUser(id)
	if err != nil {
		return err
	}

	// Remove user from circles if user is not the only circle admin
	for _, cid := range u.doc.Circles {
		c, err := db.GetCircle(cid)
		if err != nil {
			return err
		}
		if isAdmin(c.doc.Members[id]) {
			for uid, r := range c.doc.Members {
				if uid != id && isAdmin(r) {
					return fmt.Errorf("delete user: user is the only admin of a circle")
				}
			}
		}
		delete(c.doc.Members, id)
		if err := db.PutCircle(c); err != nil {
			return err
		}
	}

	// Remove user from events
	events, err := db.GetEvents(id)
	if err != nil {
		return err
	}
	for _, e := range events {
		for i, p := range e.doc.Participants {
			if p == id {
				e.doc.Participants = append(e.doc.Participants[:i], e.doc.Participants[i+1:]...)
				break
			}
		}
		if err := db.PutEvent(&e); err != nil {
			return err
		}
	}

	// Delete user
	s, err := db.delete(string(id))
	if err != nil {
		return err
	}
	if s != http.StatusOK {
		return fmt.Errorf("delete user: database error")
	}
	return nil
}

// Id returns the user's id.
func (u *User) Id() UserId {
	return UserId(u.doc.Id)
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

// AddEmail adds a non-primary email to the user's list of emails.
func (u *User) AddEmail(email string) error {
	if email == "" {
		return fmt.Errorf("add email: empty email address")
	}
	email = normalizeEmail(email)
	for _, e := range u.doc.Emails {
		if e == email {
			return fmt.Errorf("add email: duplicate") // or nil?
		}
	}
	u.doc.Emails = append(u.doc.Emails, email)
	return nil
}

// RemoveEmail removes an email from the user's list of emails.
// If it is the primary email, then the second email becomes primary.
// Users must always have a primary email.
func (u *User) RemoveEmail(email string) error {
	if email == "" {
		return fmt.Errorf("remove email: empty email address")
	}
	email = normalizeEmail(email)
	for i, e := range u.doc.Emails {
		if e == email {
			if len(u.doc.Emails) < 2 {
				return fmt.Errorf("remove email: cannot remove the last email address")
			}
			u.doc.Emails = append(u.doc.Emails[:i], u.doc.Emails[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("remove email: not found")
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

// AddCircle adds a circle to the user's list of circles.
func (u *User) AddCircle(circle CircleId) error {
	for _, c := range u.doc.Circles {
		if c == circle {
			return fmt.Errorf("add circle: duplicate")
		}
	}
	u.doc.Circles = append(u.doc.Circles, circle)
	return nil
}

// RemoveCircle removes an email from the user's list of emails.
// If it is the primary email, then the second email becomes primary.
// Users must always have a primary email.
func (u *User) RemoveCircle(email string) error {
	for i, e := range u.doc.Emails {
		if normalizeEmail(email) == normalizeEmail(e) {
			if len(u.doc.Emails) < 2 {
				return fmt.Errorf("remove email: cannot remove the last email address")
			}
			u.doc.Emails = append(u.doc.Emails[:i], u.doc.Emails[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("remove email: not found")
}

func init() {
	var u UserId
	gob.Register(u)
}
