package db

import (
	"fmt"
	"net/http"
	"time"
)

// An Event contains all the information about an event
// including what, when and where it is, who is invited,
// how many participants are required for the event to take place
// and who is participating so far.
type Event struct {
	doc event
}

// A EventId is a reference to a circle document.
type EventId string

// event is a structure used for JSON serialization
type event struct {
	Id           string     `json:"_id,omitempty"`
	Rev          string     `json:"_rev,omitempty"`
	Type         string     `json:"type"`
	Date         time.Time  `json:"date"`
	Location     string     `json:"location"`
	Desc         string     `json:"desc"`
	Threshold    int        `json:"threshold"`
	Circles      []CircleId `json:"circles"`
	Participants []UserId   `json:"participants"`
}

// NewEvent creates and initialize an Event with a date, location, description,
// a creator (who will be the first participant), the list of invited circles
// and the number of participants required for the event to take place.
func (db *DB) NewEvent(date time.Time, loc, desc string, creator UserId, thresh int, circles []CircleId) (*Event, error) {
	// Sanity checks
	if date.Before(time.Now()) {
		return nil, fmt.Errorf("new event: event must take place in the future")
	}
	if loc == "" {
		return nil, fmt.Errorf("new event: location is required")
	}
	if desc == "" {
		return nil, fmt.Errorf("new event: description is required")
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

	// Check that creator exists
	rev, err := db.rev(string(creator))
	if err != nil {
		return nil, err
	}
	if rev == "" {
		return nil, fmt.Errorf("new event: creator does not exist")
	}

	// Check that all circles exist
	for _, c := range circles {
		rev, err := db.rev(string(c))
		if err != nil {
			return nil, err
		}
		if rev == "" {
			return nil, fmt.Errorf("new event: one of the circles does not exist")
		}
	}

	e := event{
		Type:         "event",
		Desc:         desc,
		Date:         date,
		Location:     loc,
		Threshold:    thresh,
		Circles:      circles,
		Participants: []UserId{creator},
	}

	// Create document in database
	var r struct{ Id, Rev string }
	s, err := db.post("", &e, &r)
	if err != nil {
		return nil, err
	}
	if s != http.StatusCreated {
		return nil, fmt.Errorf("new event: database error")
	}

	e.Id = r.Id
	e.Rev = r.Rev

	return &Event{e}, nil
}

// PutEvent adds an event to the database or updates it while keeping the database consistent.
// It makes sure that all participants and invited circles do exist.
func (db *DB) PutEvent(e *Event) error {
	// Check for empty fields
	if e.doc.Date.Before(time.Now()) {
		return fmt.Errorf("put event: event must take place in the future")
	}
	if e.doc.Location == "" {
		return fmt.Errorf("put event: event has no location")
	}
	if e.doc.Desc == "" {
		return fmt.Errorf("put event: event has no description")
	}
	if e.doc.Threshold < 0 {
		return fmt.Errorf("put event: event has negative threshold")
	}
	if len(e.doc.Circles) == 0 {
		return fmt.Errorf("put event: event has no circles")
	}

	// Check that all circles exist
	for _, id := range e.doc.Circles {
		rev, err := db.rev(string(id))
		if err != nil {
			return err
		}
		if rev == "" {
			return fmt.Errorf("put event: circle does not exist")
		}
	}

	// Check that all participants exist
	for _, id := range e.doc.Participants {
		rev, err := db.rev(string(id))
		if err != nil {
			return err
		}
		if rev == "" {
			return fmt.Errorf("put event: user does not exist")
		}
	}

	// Put document
	s, err := db.put(e.doc.Id, e)
	if err != nil {
		return err
	}
	switch s {
	case http.StatusCreated: // ok
	case http.StatusConflict:
		return fmt.Errorf("put event: conflict, get a fresh revision and try again")
	default:
		return fmt.Errorf("put event: database error")
	}
	return nil
}

// GetEvent retrieves an event from the database.
func (db *DB) GetEvent(id EventId) (*Event, error) {
	var e event
	s, err := db.get(string(id), &e)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get event: database error")
	}
	return &Event{e}, nil
}

// DeleteEvent removes an event from the database.
func (db *DB) DeleteEvent(id EventId) error {
	s, err := db.delete(string(id))
	if err != nil {
		return err
	}
	if s != http.StatusOK {
		return fmt.Errorf("delete event: database error")
	}
	return nil
}

// GetEvents returns all events for a given user.
func (db *DB) GetEvents(id UserId) ([]Event, error) {
	var v struct{ Rows []struct{ Doc event } }
	s, err := db.get(db.view("events", string(id), true), &v)
	if err != nil {
		return nil, err
	}
	if s != http.StatusOK {
		return nil, fmt.Errorf("get events: database error")
	}
	e := make([]Event, len(v.Rows))
	for i, r := range v.Rows {
		e[i].doc = r.Doc
	}
	return e, nil
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

// Location returns the event's location.
func (e *Event) Location() string {
	return e.doc.Location
}

// Desc returns the event's description.
func (e *Event) Desc() string {
	return e.doc.Desc
}

// Threshold returns the event's minimum number of participants.
func (e *Event) Threshold() int {
	return e.doc.Threshold
}

// Circles returns the event's list of invited circles.
func (e *Event) Circles() []CircleId {
	return e.doc.Circles
}

// Participants returns the event's list of participants.
func (e *Event) Participants() []UserId {
	return e.doc.Participants
}
