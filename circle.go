package db

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fiam/gounidecode/unidecode"
	"github.com/simleb/errors"
)

// A Circle is a proxy for a full circle document in the database.
type Circle struct {
	Id   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// A circle is a CouchDB circle document.
type circle struct {
	Id   string `json:"_id,omitempty"`
	Rev  string `json:"_rev,omitempty"`
	Type string `json:"type"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// A Member is a proxy for a full member document in the database.
type Member struct {
	User
	Circle
	Id   string
	Date time.Time
	Me   bool
}

// A member is a CouchDB member document.
type member struct {
	Id     string    `json:"_id,omitempty"`
	Rev    string    `json:"_rev,omitempty"`
	Type   string    `json:"type"`
	User   string    `json:"user"`
	Circle string    `json:"circle"`
	Rights []string  `json:"rights"`
	Date   time.Time `json:"date"`
}

// NewCircle creates a new circle in the database with a name, a slug and a creator.
// The creator gets all rights on the circle (circle admin).
// The slug has to be unique.
// If left blank, a slug will tentatively be derived from the name.
func (db *DB) NewCircle(name, slug, creator string) (string, error) {
	// Check for empty fields
	if name == "" {
		return "", fmt.Errorf("new circle: name is missing")
	}
	if slug == "" {
		slug = Slugify(slug)
	}
	if creator == "" {
		return "", fmt.Errorf("new circle: initial member is missing")
	}

	// Check if slug is unique
	var v struct{ Rows []struct{ Value string } }
	s, err := db.get(db.view("slug", slug, false), &v)
	if err != nil {
		return "", err
	}
	if s != http.StatusOK {
		return "", fmt.Errorf("new circle: database error")
	}
	if len(v.Rows) > 0 {
		return "", fmt.Errorf("new circle: slug is not unique")
	}

	// Check if creator exists
	rev, err := db.rev(creator)
	if err != nil {
		return "", errors.Stack(err, "new circle: database error")
	}
	if rev == "" {
		return "", fmt.Errorf("new circle: initial member does not exist")
	}

	// Create circle document in database
	c := circle{
		Type: "circle",
		Name: name,
		Slug: slug,
	}
	var r struct{ Id string }
	s, err = db.post("", &c, &r)
	if err != nil {
		return "", err
	}
	if s != http.StatusCreated {
		return "", fmt.Errorf("new circle: database error")
	}

	// Create member document in database
	m := member{
		Type:   "member",
		User:   creator,
		Circle: r.Id,
		Rights: []string{"post", "invite", "admin"},
		Date:   time.Now(),
	}
	s, err = db.post("", &m, nil)
	if err != nil {
		return r.Id, err
	}
	if s != http.StatusCreated {
		return r.Id, fmt.Errorf("new circle: database error")
	}

	return r.Id, nil
}

func (db *DB) GetCircles(userId string) ([]Circle, error) {
	var v struct{ Rows []struct{ Doc circle } }
	s, err := db.get(db.view("circles", userId, true), &v)
	if err != nil {
		return nil, errors.Stack(err, "get circles: error querying circles view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get circles: database error")
	}
	c := make([]Circle, len(v.Rows))
	for i, r := range v.Rows {
		c[i].Id = r.Doc.Id
		c[i].Name = r.Doc.Name
		c[i].Slug = r.Doc.Slug
	}
	return c, nil
}

func (db *DB) SendInvitation(circleId, email string) error {
	email = normalizeEmail(email)
	var v struct{ Rows []struct{ Doc user } }
	s, err := db.get(db.view("email", email, true), &v)
	if err != nil {
		return errors.Stack(err, "send invitation: error querying email view")
	}
	if s != http.StatusOK {
		return fmt.Errorf("send invitation: get email view got status %d", s)
	}
	if len(v.Rows) < 1 {
		return fmt.Errorf("send invitation: email not found")
	}
	m := member{
		Type:   "member",
		User:   v.Rows[0].Doc.Id,
		Circle: circleId,
		Rights: []string{"post", "invite", "admin"},
		Date:   time.Now(),
	}
	s, err = db.post("", &m, nil)
	if err != nil {
		return errors.Stack(err, "send invitation: database error")
	}
	if s != http.StatusCreated {
		return fmt.Errorf("send invitation: post member got status %d", s)
	}
	return nil
}

var (
	invalidSlugPattern = regexp.MustCompile(`[^a-z0-9 _-]`)
	whiteSpacePattern  = regexp.MustCompile(`\s+`)
)

// Slugify generates a human-readable identifier (slug) from a string.
// Basically it removes accents, weird characters and replaces spaces with dashes.
func Slugify(s string) string {
	s = unidecode.Unidecode(s)
	s = strings.ToLower(s)
	s = invalidSlugPattern.ReplaceAllString(s, "")
	s = whiteSpacePattern.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
