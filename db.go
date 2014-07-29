/*
Package db implements an interface to the Toople database.

Currently the underlying database is CouchDB but this package abstracts this away.
*/
package db

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/simleb/errors"
)

// db is a string containing the URL of the CouchDB database
type DB struct {
	url    string
	client *http.Client
}

// New returns an initialized DB object
func New(host, port, username, password, dbname string) (*DB, error) {
	jar, _ := cookiejar.New(nil) // err is always nil
	db := &DB{
		url:    fmt.Sprintf("http://%s:%s/%s", host, port, dbname),
		client: &http.Client{Jar: jar},
	}

	// Test connection to database
	r, err := db.client.Head(fmt.Sprintf("http://%s:%s/", host, port))
	if err != nil {
		return nil, fmt.Errorf("db: %s:%s: connection refused", host, port)
	}
	if !strings.HasPrefix(r.Header.Get("Server"), "CouchDB") {
		return nil, fmt.Errorf("db: %s:%s is not CouchDB", host, port)
	}
	switch r.StatusCode {
	case 200:
	default:
		return nil, fmt.Errorf("db: %s:%s: status %d", host, port, r.StatusCode)
	}

	// Authenticate
	s := fmt.Sprintf("http://%s:%s/_session", host, port)
	p := fmt.Sprintf("name=%s&password=%s", url.QueryEscape(username), url.QueryEscape(password))
	r, err = db.client.Post(s, "application/x-www-form-urlencoded", strings.NewReader(p))
	if err != nil {
		return nil, fmt.Errorf("db: %s:%s: connection refused", host, port)
	}
	defer r.Body.Close()
	switch r.StatusCode {
	case 200:
	case 401:
		return nil, fmt.Errorf("db: %s:%s: unauthorized", host, port)
	default:
		return nil, fmt.Errorf("db: %s:%s: status %d", host, port, r.StatusCode)
	}

	// Check role (read/write access)
	var v struct{ Roles []string }
	d := json.NewDecoder(r.Body)
	if err = d.Decode(&v); err != nil {
		return nil, fmt.Errorf("db: %s:%s: bad JSON in database response", host, port)
	}
	for _, role := range v.Roles {
		if role == "db" {
			return db, nil
		}
	}
	return nil, fmt.Errorf("db: %s:%s: read or write permission denied", host, port)
}

// view gives the URL of a view including queries for a key and if docs should be returned
func (db *DB) view(view, key string, include_docs, descending bool) string {
	return fmt.Sprintf(`_design/toople/_view/%s?key="%s"&include_docs=%t&descending=%t`, view, url.QueryEscape(key), include_docs, descending)
}

// request performs an http request against the database
func (db *DB) request(method, path string, in, out interface{}) (int, error) {
	body := new(bytes.Buffer)

	// Encode JSON
	if in != nil {
		d := json.NewEncoder(body)
		if err := d.Encode(in); err != nil {
			return http.StatusInternalServerError, errors.Stack(err, "request: error encoding JSON")
		}
	}

	// Request
	req, err := http.NewRequest(method, db.url+"/"+path, body)
	if err != nil {
		return http.StatusInternalServerError, errors.Stack(err, "request: error creating HTTP request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := db.client.Do(req)
	if err != nil {
		return http.StatusInternalServerError, errors.Stack(err, "request: error sending HTTP request")
	}
	defer res.Body.Close()

	// Decode JSON
	if out != nil {
		d := json.NewDecoder(res.Body)
		if err := d.Decode(out); err != nil {
			return res.StatusCode, errors.Stack(err, "request: error decoding JSON")
		}
	}

	return res.StatusCode, nil
}

// rev returns the current revision of a document if it was found
func (db *DB) rev(id string) (string, error) {
	res, err := http.Head(db.url + "/" + id)
	return res.Header.Get("ETag"), errors.Stack(err, "rev: error during HEAD request")
}

// get performs a get request against the database
func (db *DB) get(path string, out interface{}) (int, error) {
	return db.request("GET", path, nil, out)
}

// post performs a post request against the database
func (db *DB) post(path string, in, out interface{}) (int, error) {
	return db.request("POST", path, in, out)
}

// put performs a put request against the database
func (db *DB) put(path string, in interface{}) (int, error) {
	return db.request("PUT", path, in, nil)
}

// delete performs a delete request against the database
func (db *DB) delete(id, rev string) (int, error) {
	return db.request("DELETE", fmt.Sprintf("%s?rev=%s", id, rev), nil, nil)
}
