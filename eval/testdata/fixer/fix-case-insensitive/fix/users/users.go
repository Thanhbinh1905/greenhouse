package users

import "strings"

// Directory indexes user names by email address.
type Directory struct {
	byEmail map[string]string
}

// NewDirectory returns an empty Directory.
func NewDirectory() *Directory {
	return &Directory{byEmail: map[string]string{}}
}

// Add records name under email. Email addresses are case-insensitive.
func (d *Directory) Add(email, name string) {
	d.byEmail[strings.ToLower(email)] = name
}

// Find returns the name recorded for email, ignoring case.
func (d *Directory) Find(email string) (string, bool) {
	name, ok := d.byEmail[strings.ToLower(email)]
	return name, ok
}
