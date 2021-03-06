package db

import (
	"fmt"
	"net/http"
	"time"

	"github.com/simleb/errors"
)

// An Event is a proxy for a full event document in the database.
type Event struct {
	Id           string        `json:"id"`
	Location     string        `json:"location"`
	Title        string        `json:"title"`
	Info         string        `json:"info"`
	Date         time.Time     `json:"date"`
	Threshold    int           `json:"threshold"`
	Creator      User          `json:"creator"`
	Created      time.Time     `json:"created"`
	Status       string        `json:"status"`
	Participants []Participant `json:"participants"`
}

// An event is a CouchDB event document.
type event struct {
	Id        string    `json:"_id,omitempty"`
	Rev       string    `json:"_rev,omitempty"`
	Type      string    `json:"type"`
	Location  string    `json:"location"`
	Title     string    `json:"title"`
	Info      string    `json:"info"`
	Date      time.Time `json:"date"`
	Threshold int       `json:"threshold"`
}

// A Participant is a proxy for a full participant document in the database.
type Participant struct {
	Id   string    `json:"id"`
	Name string    `json:"name"`
	Date time.Time `json:"date"`
}

// A participant is a CouchDB participant document.
type participant struct {
	Id    string    `json:"_id,omitempty"`
	Rev   string    `json:"_rev,omitempty"`
	Type  string    `json:"type"`
	User  string    `json:"user"`
	Event string    `json:"event"`
	Date  time.Time `json:"date"`
}

// An invitation is a CouchDB invitation document.
type invitation struct {
	Id     string `json:"_id,omitempty"`
	Rev    string `json:"_rev,omitempty"`
	Type   string `json:"type"`
	Circle string `json:"circle"`
	Event  string `json:"event"`
}

// NewEvent creates a new event in the database with a date, location, description,
// a creator (who will be the first participant), the list of invited circles
// and the number of participants required for the event to take place.
func (db *DB) NewEvent(date time.Time, loc, title, info, creator string, thresh int, circles []string) error {
	// Sanity checks
	if date.Before(time.Now()) {
		return fmt.Errorf("new event: event must take place in the future")
	}
	if loc == "" {
		return fmt.Errorf("new event: location is required")
	}
	if title == "" {
		return fmt.Errorf("new event: title is required")
	}
	if creator == "" {
		return fmt.Errorf("new event: creator is required")
	}
	if thresh < 1 {
		return fmt.Errorf("new event: threshold must be strictly positive")
	}
	if len(circles) == 0 {
		return fmt.Errorf("new event: must invite at least one circle")
	}

	// Check if creator exists
	rev, err := db.rev(creator)
	if err != nil {
		return errors.Stack(err, "new event: cannot check if user %q exists", creator)
	}
	if rev == "" {
		return fmt.Errorf("new event: user %q does not exist", creator)
	}

	// Check that all circles exist
	for _, c := range circles {
		rev, err := db.rev(c)
		if err != nil {
			return errors.Stack(err, "new event: cannot check if circle %q exists", c)
		}
		if rev == "" {
			return fmt.Errorf("new event: circle %q does not exist", c)
		}

	}

	// Create event document in database
	e := event{
		Type:      "event",
		Title:     title,
		Info:      info,
		Date:      date,
		Location:  loc,
		Threshold: thresh,
	}
	var r struct{ Id string }
	s, err := db.post("", &e, &r)
	if err != nil {
		return errors.Stack(err, "new event: cannot create event")
	}
	if s != http.StatusCreated {
		return fmt.Errorf("new event: got status %d trying to create event", s)
	}

	// Create participant document in database
	p := participant{
		Type:  "participant",
		User:  creator,
		Event: r.Id,
		Date:  date,
	}
	s, err = db.post("", &p, nil)
	if err != nil {
		return errors.Stack(err, "new event: cannot create participant")
	}
	if s != http.StatusCreated {
		return fmt.Errorf("new event: got status %d trying to create participant", s)
	}

	// Create invitation documents in database
	for _, c := range circles {
		i := invitation{
			Type:   "invitation",
			Circle: c,
			Event:  r.Id,
		}
		s, err = db.post("", &i, nil)
		if err != nil {
			return errors.Stack(err, "new event: cannot create invitation")
		}
		if s != http.StatusCreated {
			return fmt.Errorf("new event: got status %d trying to create invitation", s)
		}
	}
	return nil
}

// PrettyDate returns the formatted event's date.
func (e *Event) PrettyDate() string {
	if e.Date.Year() != time.Now().Year() {
		return e.Date.Format("Mon Jan 2, 2006 — 3:04pm")
	}
	return e.Date.Format("Mon Jan 2 — 3:04pm")
}

func (db *DB) JoinEvent(event, user string) error {
	// Check if not already participant
	var v struct {
		Rows []struct {
			Value struct {
				User string `json:"_id"`
			}
		}
	}
	s, err := db.get(db.view("participants", event, false), &v)
	if err != nil {
		return errors.Stack(err, "join event: error querying participants view")
	}
	if s != http.StatusOK {
		return fmt.Errorf("join event: database error")
	}
	for _, r := range v.Rows {
		if r.Value.User == user {
			return nil
		}
	}

	p := participant{
		Type:  "participant",
		User:  user,
		Event: event,
		Date:  time.Now(),
	}
	s, err = db.post("", &p, nil)
	if err != nil {
		return errors.Stack(err, "join event: database error")
	}
	if s != http.StatusCreated {
		return fmt.Errorf("join event: got status %d trying to create event", s)
	}
	return nil
}
