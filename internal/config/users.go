package config

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseUserString parses a user string in the format:
// "user:pass:uid:path:permissions,user2:pass2:uid2:path2:permissions2"
func ParseUserString(userStr string) (map[string]*User, error) {
	if userStr == "" {
		return make(map[string]*User), nil
	}

	users := make(map[string]*User)
	userEntries := strings.Split(userStr, ",")

	for _, entry := range userEntries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 5 {
			return nil, fmt.Errorf("invalid user format '%s', expected 'username:password:uid:path:permissions'", entry)
		}

		username := strings.TrimSpace(parts[0])
		password := strings.TrimSpace(parts[1])
		uidStr := strings.TrimSpace(parts[2])
		path := strings.TrimSpace(parts[3])
		permissions := strings.TrimSpace(parts[4])

		if username == "" {
			return nil, fmt.Errorf("username cannot be empty in '%s'", entry)
		}

		if password == "" {
			return nil, fmt.Errorf("password cannot be empty for user '%s'", username)
		}

		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			return nil, fmt.Errorf("invalid UID '%s' for user '%s': %w", uidStr, username, err)
		}

		if path == "" {
			path = "/"
		} else if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}

		if permissions != "ro" && permissions != "rw" {
			return nil, fmt.Errorf("invalid permissions '%s' for user '%s', must be 'ro' or 'rw'", permissions, username)
		}

		users[username] = &User{
			Pass:        password,
			UID:         uid,
			Path:        path,
			Permissions: permissions,
		}
	}

	return users, nil
}

// IsReadOnly returns true if the user has read-only permissions
func (u *User) IsReadOnly() bool {
	return u.Permissions == "ro"
}

// CanWrite returns true if the user has write permissions
func (u *User) CanWrite() bool {
	return u.Permissions == "rw"
}

// GetFullPath returns the full filesystem path for the user
func (u *User) GetFullPath(dataDir string) string {
	if u.Path == "/" {
		return dataDir
	}
	return strings.TrimSuffix(dataDir, "/") + "/" + strings.TrimPrefix(u.Path, "/")
}
