package db

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/fiam/gounidecode/unidecode"
)

// A Right is an action a user can perform within or upon a circle.
// The possible rights are:
//  - admin: can update/delete the circle, add/remove members and give/remove them rights
//  - invite: can invite other users to join the circle
//  - post: can create events
type Right string

const (
	Post   Right = "post"
	Invite Right = "invite"
	Admin  Right = "admin"
)

// allRights is the list of all the rights a user can have.
// They are given by default to the first member of a circle.
var allRights []Right = []Right{Post, Invite, Admin}

// A Circle is a named group of users with a unique human-readable identifier (slug)
// where each member has rights.
// Members with no rights can only participate in events.
type Circle struct {
	// doc is the full circle document in the database.
	doc circle

	// members is the list of member documents in the database.
	members []member

	// users is the list of user documents corresponding to each member.
	users []user
}

// circle is a structure used for JSON serialization
type circle struct {
	Id   string `json:"_id,omitempty"`
	Rev  string `json:"_rev,omitempty"`
	Type string `json:"type"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// A Member is a member of a circle.
type Member struct {
	Id   string
	Name string
	Date time.Time
}

func (Circle) Type() string {
	return "Circle"
}

// member is a structure used for JSON serialization
type member struct {
	Id     string    `json:"_id,omitempty"`
	Rev    string    `json:"_rev,omitempty"`
	Type   string    `json:"type"`
	User   string    `json:"user"`
	Circle string    `json:"circle"`
	Rights []Right   `json:"rights"`
	Date   time.Time `json:"date"`
}

// NewCircle creates and initialize a Circle with a name, slug
// and a single member with all rights (circle admin).
// The slug has to be unique. If left blank, a slug will tentatively
// be derived from the name.
func (db *DB) NewCircle(name, slug, creator string) (*Circle, error) {
	// Check for empty fields
	if name == "" {
		return nil, fmt.Errorf("new circle: name is missing")
	}
	if slug == "" {
		slug = Slugify(slug)
	}
	if creator == "" {
		return nil, fmt.Errorf("new circle: initial member is missing")
	}

	// Check if slug is unique
	var v struct{ Rows []struct{ Value string } }
	s, err := db.get(db.view("slug", slug, false, false), &v)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("new circle: database error")
	}
	if len(v.Rows) > 0 {
		return nil, fmt.Errorf("new circle: slug is not unique")
	}

	c := &Circle{
		members: make([]member, 1),
		users:   make([]user, 1),
	}

	// Check if creator exists and get its user doc
	s, err = db.get(creator, &c.users[0])
	if err != nil {
		return nil, err
	}
	switch s {
	case http.StatusOK: // ok
	case http.StatusNotFound:
		return nil, fmt.Errorf("new circle: initial member does not exist")
	default:
		return nil, fmt.Errorf("new circle: database error")
	}

	// Create circle document in database
	c.doc.Type = "circle"
	c.doc.Name = name
	c.doc.Slug = slug
	var r struct{ Id, Rev string }
	s, err = db.post("", &c.doc, &r)
	if err != nil {
		return nil, err
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new circle: database error")
	}
	c.doc.Id = r.Id
	c.doc.Rev = r.Rev

	// Create member document in database
	c.members[0].Type = "member"
	c.members[0].User = creator
	c.members[0].Circle = c.doc.Id
	c.members[0].Rights = allRights
	c.members[0].Date = time.Now()
	s, err = db.post("", &c.members[0], &r)
	if err != nil {
		return nil, err
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new circle: database error")
	}
	c.members[0].Id = r.Id
	c.members[0].Rev = r.Rev

	return c, nil
}

// Name returns the circle's name.
func (c *Circle) Name() string {
	return c.doc.Name
}

// Slug returns the circle's slug.
func (c *Circle) Slug() string {
	return c.doc.Slug
}

// Members returns the circle's list of members.
func (c *Circle) Members() []Member {
	m := make([]Member, len(c.members))
	for i := range m {
		m[i].Id = c.users[i].Id
		m[i].Name = c.users[i].Name
		m[i].Date = c.members[i].Date
	}
	return m
}

var invalidSlugPattern = regexp.MustCompile(`[^a-z0-9 _-]`)
var whiteSpacePattern = regexp.MustCompile(`\s+`)

// Slugify generates a human-readable identifier (slug) from any string.
// Basically it removes accents, weird characters and replaces spaces with dashes.
// It can be used to generate the slug for a circle from its name.
func Slugify(s string) string {
	s = unidecode.Unidecode(s)
	s = strings.ToLower(s)
	s = invalidSlugPattern.ReplaceAllString(s, "")
	s = whiteSpacePattern.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// isAdmin checks if a list of rights contains admin
func isAdmin(rs []Right) bool {
	for _, r := range rs {
		if r == Admin {
			return true
		}
	}
	return false
}
