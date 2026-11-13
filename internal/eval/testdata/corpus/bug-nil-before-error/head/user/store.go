package user

import "errors"

// ErrNotFound reports a missing user.
var ErrNotFound = errors.New("user not found")

type User struct {
	ID   string
	Name string
}

type Store struct {
	users map[string]*User
}

// Lookup returns the user with the given ID, or ErrNotFound.
func (s *Store) Lookup(id string) (*User, error) {
	u, ok := s.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return u, nil
}

// DisplayName returns the user's name, or "unknown" when the user does not exist.
func (s *Store) DisplayName(id string) string {
	u, err := s.Lookup(id)
	name := u.Name
	if err != nil {
		return "unknown"
	}
	return name
}
