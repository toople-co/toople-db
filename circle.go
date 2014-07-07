package db

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

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
	doc circle
}

// A CircleId is a reference to a circle document.
type CircleId string

// circle is a structure used for JSON serialization
type circle struct {
	Id      string             `json:"_id,omitempty"`
	Rev     string             `json:"_rev,omitempty"`
	Type    string             `json:"type"`
	Name    string             `json:"name"`
	Slug    string             `json:"slug"`
	Members map[UserId][]Right `json:"members"`
}

// NewCircle creates and initialize a Circle with a name, slug
// and a single member with all rights (circle admin).
// The slug has to be unique. If left blank, a slug will tentatively
// be derived from the name.
func (db *DB) NewCircle(name, slug string, member UserId) (*Circle, error) {
	// Check for empty fields
	if name == "" {
		return nil, fmt.Errorf("new circle: name is missing")
	}
	if slug == "" {
		slug = Slugify(slug)
	}
	if member == "" {
		return nil, fmt.Errorf("new circle: initial member is missing")
	}

	// Check if user exists
	rev, err := db.rev(string(member))
	if err != nil {
		return nil, err
	}
	if rev == "" {
		return nil, fmt.Errorf("new circle: initial member does not exist")
	}

	// Check if slug is unique
	var v struct{ Rows []struct{ Id string } }
	s, err := db.get(db.view("slug", slug, false), &v)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("new circle: database error")
	}
	if len(v.Rows) > 0 {
		return nil, fmt.Errorf("new circle: slug is not unique")
	}

	c := circle{
		Type:    "circle",
		Name:    name,
		Slug:    slug,
		Members: map[UserId][]Right{member: allRights},
	}

	// Create document in database
	var r struct{ Id, Rev string }
	s, err = db.post("", &c, &r)
	if err != nil {
		return nil, err
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new circle: database error")
	}

	c.Id = r.Id
	c.Rev = r.Rev

	return &Circle{c}, nil
}

// PutCircle updates a circle in the database while keeping the database consistent.
// It makes sure that all and only the members of the circle have a reference to it.
//
// The slug always has to be unique.
// The circle has to have at least one admin (member with all rights).
func (db *DB) PutCircle(c *Circle) error {
	// Check for empty fields
	if c.doc.Name == "" {
		return fmt.Errorf("put circle: circle has no name")
	}
	if c.doc.Slug == "" {
		return fmt.Errorf("put circle: circle has no slug")
	}
	if len(c.doc.Members) == 0 {
		return fmt.Errorf("put circle: circle has no members")
	}

	// Check presence of an admin
	for _, r := range c.doc.Members {
		if isAdmin(r) {
			goto next
		}
	}
	return fmt.Errorf("put circle: circle has no admin")
next:

	// Check if slug is unique
	var v struct{ Rows []struct{ Id string } }
	s, err := db.get(db.view("slug", c.doc.Slug, false), &v)
	if err != nil {
		return err
	}
	if s != http.StatusOK {
		return fmt.Errorf("put circle: database error")
	}
	if len(v.Rows) > 0 && v.Rows[0].Id != c.doc.Id {
		return fmt.Errorf("put circle: slug is not unique")
	}

	// Put document (return error on conflict)
	s, err = db.put(c.doc.Id, c)
	if err != nil {
		return err
	}
	switch s {
	case http.StatusCreated: // ok
	case http.StatusConflict:
		return fmt.Errorf("put circle: conflict, get a fresh revision and try again")
	default:
		return fmt.Errorf("put circle: database error")
	}
	return nil
}

// GetCircle retrieves an existing circle from the database.
func (db *DB) GetCircle(id CircleId) (*Circle, error) {
	var c circle
	s, err := db.get(string(id), &c)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get circle: database error")
	}
	return &Circle{c}, nil
}

// DeleteCircle removes a circle from the database
// as well as all references to this circle from users and events.
func (db *DB) DeleteCircle(id CircleId) error {
	// Delete references to circle from events and users
	var v struct {
		Rows []struct {
			Doc struct {
				Id      string     `json:"_id"`
				Circles []CircleId `json:"circles"`
			}
		}
	}
	s, err := db.get(db.view("circle_refs", string(id), true), &v)
	if err != nil {
		return err
	}
	if s != http.StatusOK {
		return fmt.Errorf("delete circle: database error")
	}
	for _, r := range v.Rows {
		for i, cid := range r.Doc.Circles {
			if cid == id {
				r.Doc.Circles = append(r.Doc.Circles[:i], r.Doc.Circles[i+1:]...)
				break
			}
		}
		if _, err := db.put(r.Doc.Id, &r.Doc); err != nil {
			return err
		}
	}

	// Delete circle
	s, err = db.delete(string(id))
	if err != nil {
		return err
	}
	if s != http.StatusOK {
		return fmt.Errorf("delete circle: database error")
	}
	return nil
}

// GetCircles returns all circles for a given user.
func (db *DB) GetCircles(id CircleId) ([]Circle, error) {
	var v struct{ Rows []struct{ Doc circle } }
	s, err := db.get(db.view("circles", string(id), true), &v)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get circles: database error")
	}
	c := make([]Circle, len(v.Rows))
	for i, r := range v.Rows {
		c[i].doc = r.Doc
	}
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
func (c *Circle) Members() []UserId {
	m := make([]UserId, len(c.doc.Members))
	i := 0
	for k := range c.doc.Members {
		m[i] = k
		i++
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
