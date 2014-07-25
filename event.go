package db

import (
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/simleb/errors"
)

// An Event contains all the information about an event
// including what, when and where it is, who is invited,
// how many participants are required for the event to take place
// and who is participating so far.
type Event struct {
	// doc is the full event document in the database.
	doc event

	// participants is the list of participant documents in the database.
	participants []participant

	// users is the list of user documents corresponding to each participant.
	users []user

	// circles is the list of circle documents invited to the event.
	circles []circle
}

// event is a structure used for JSON serialization
type event struct {
	Id        string    `json:"_id,omitempty"`
	Rev       string    `json:"_rev,omitempty"`
	Type      string    `json:"type"`
	Location  string    `json:"location"`
	Title     string    `json:"title"`
	Info      string    `json:"info"`
	Creator   string    `json:"creator"`
	Date      time.Time `json:"date"`
	Created   time.Time `json:"created"`
	Threshold int       `json:"threshold"`
}

func (Event) Type() string {
	return "Event"
}

// participant is a structure used for JSON serialization
type participant struct {
	Id    string    `json:"_id,omitempty"`
	Rev   string    `json:"_rev,omitempty"`
	Type  string    `json:"type"`
	User  string    `json:"user"`
	Event string    `json:"event"`
	Date  time.Time `json:"date"`
}

type Participant struct {
	Id   string    `json:"id"`
	Name string    `json:"name"`
	Date time.Time `json:"date"`
}

// invitation is a structure used for JSON serialization
type invitation struct {
	Id     string `json:"_id,omitempty"`
	Rev    string `json:"_rev,omitempty"`
	Type   string `json:"type"`
	Circle string `json:"circle"`
	Event  string `json:"event"`
}

// NewEvent creates and initialize an Event with a date, location, description,
// a creator (who will be the first participant), the list of invited circles
// and the number of participants required for the event to take place.
func (db *DB) NewEvent(date time.Time, loc, title, info, creator string, thresh int, circles []string) (*Event, error) {
	// Sanity checks
	if date.Before(time.Now()) {
		return nil, fmt.Errorf("new event: event must take place in the future")
	}
	if loc == "" {
		return nil, fmt.Errorf("new event: location is required")
	}
	if title == "" {
		return nil, fmt.Errorf("new event: title is required")
	}
	if creator == "" {
		return nil, fmt.Errorf("new event: creator is required")
	}
	if thresh < 0 {
		return nil, fmt.Errorf("new event: threshold must be non-negative")
	}
	if len(circles) == 0 {
		return nil, fmt.Errorf("new event: must have at least one circle")
	}

	e := &Event{
		participants: make([]participant, 1),
		users:        make([]user, 1),
		circles:      make([]circle, len(circles)),
	}

	// Check if creator exists and get its user doc
	s, err := db.get(creator, &e.users[0])
	if err != nil {
		return nil, err
	}
	switch s {
	case http.StatusOK: // ok
	case http.StatusNotFound:
		return nil, fmt.Errorf("new event: creator does not exist")
	default:
		return nil, fmt.Errorf("new event: database error")
	}

	// Check that all circles exist
	for i, c := range circles {
		s, err := db.get(c, &e.circles[i])
		if err != nil {
			return nil, err
		}
		switch s {
		case http.StatusOK: // ok
		case http.StatusNotFound:
			return nil, fmt.Errorf("new event: circle does not exist")
		default:
			return nil, fmt.Errorf("new event: database error")
		}
	}

	// Create event document in database
	e.doc.Type = "event"
	e.doc.Title = title
	e.doc.Info = info
	e.doc.Date = date
	e.doc.Location = loc
	e.doc.Threshold = thresh
	var r struct{ Id, Rev string }
	s, err = db.post("", &e.doc, &r)
	if err != nil {
		return nil, err
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new event: database error")
	}
	e.doc.Id = r.Id
	e.doc.Rev = r.Rev

	// Create participant document in database
	e.participants[0].Type = "participant"
	e.participants[0].User = creator
	e.participants[0].Event = e.doc.Id
	e.participants[0].Date = date
	s, err = db.post("", &e.participants[0], &r)
	if err != nil {
		return nil, err
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new event: database error")
	}
	e.participants[0].Id = r.Id
	e.participants[0].Rev = r.Rev

	// Create invitation documents in database
	for _, c := range circles {
		inv := invitation{
			Type:   "invitation",
			Circle: c,
			Event:  e.doc.Id,
		}
		s, err = db.post("", &inv, nil)
		if err != nil {
			return nil, err
		}
		if s != http.StatusCreated {
			return nil, fmt.Errorf("new event: database error")
		}
	}
	return e, nil
}

func (db *DB) GetParticipants(event_id, user_id string) ([]Participant, error) {
	// TODO: check that user is invited to this event
	// Get user's circles and get the event's invited circles
	var v struct {
		Rows []struct {
			Key []string
			Doc user
		}
	}
	s, err := db.get(fmt.Sprintf(`_design/toople/_view/participants?startkey=["%s",{}]&endkey=["%[1]s"]&include_docs=true&descending=true`, url.QueryEscape(event_id)), &v)
	if err != nil {
		return nil, errors.Stack(err, "get participants: error querying participants view")
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get participants: database error")
	}
	p := make([]Participant, len(v.Rows))
	for i, r := range v.Rows {
		p[i].Id = r.Doc.Id
		p[i].Name = r.Doc.Name
		p[i].Date, err = time.Parse(time.RFC3339, r.Key[1])
		if err != nil {
			return nil, fmt.Errorf("get participants: error parsing date")
		}
	}
	return p, nil
}

// Id returns the event's id.
func (e *Event) Id() string {
	return e.doc.Id
}

// Date returns the event's date.
func (e *Event) Date() time.Time {
	return e.doc.Date
}

// PrettyDate returns the formatted event's date.
func (e *Event) PrettyDate() string {
	if e.doc.Date.Year() != time.Now().Year() {
		return e.doc.Date.Format("Monday Jan 2, 2006 — 3:04pm")
	}
	return e.doc.Date.Format("Monday Jan 2 — 3:04pm")
}

// Created returns the event's creation date.
func (e *Event) Created() time.Time {
	return e.doc.Created
}

// Location returns the event's location.
func (e *Event) Location() string {
	return e.doc.Location
}

// Title returns the event's title.
func (e *Event) Title() string {
	return e.doc.Title
}

// Info returns more info about the event.
func (e *Event) Info() string {
	return e.doc.Info
}

// Threshold returns the event's minimum number of participants.
func (e *Event) Threshold() int {
	return e.doc.Threshold
}

// Status returns the event's status.
func (e *Event) Status() string {
	if len(e.participants) < e.doc.Threshold {
		if e.doc.Date.Before(time.Now()) {
			return "Cancelled"
		}
		return "Pending"
	}
	return "Confirmed"
}

// Participants returns the event's list of participants.
func (e *Event) Participants() []User {
	users := make([]User, len(e.users))
	for i, u := range e.users {
		users[i].doc = u
	}
	return users
}
