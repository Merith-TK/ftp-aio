package auth

import (
	"fmt"

	"github.com/Merith-TK/ftp-aio/internal/config"
)

// Authenticator handles user authentication
type Authenticator struct {
	users map[string]*config.User
}

// NewAuthenticator creates a new authenticator with the given users
func NewAuthenticator(users map[string]*config.User) *Authenticator {
	return &Authenticator{
		users: users,
	}
}

// Authenticate verifies user credentials
func (a *Authenticator) Authenticate(username, password string) (*config.User, error) {
	user, exists := a.users[username]
	if !exists {
		return nil, fmt.Errorf("user '%s' not found", username)
	}

	if user.Pass != password {
		return nil, fmt.Errorf("invalid password for user '%s'", username)
	}

	return user, nil
}

// GetUser returns a user by username without authentication
func (a *Authenticator) GetUser(username string) (*config.User, bool) {
	user, exists := a.users[username]
	return user, exists
}

// ListUsers returns all usernames
func (a *Authenticator) ListUsers() []string {
	users := make([]string, 0, len(a.users))
	for username := range a.users {
		users = append(users, username)
	}
	return users
}
