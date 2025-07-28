package auth

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Merith-TK/ftp-aio/internal/config"
)

// Permission represents different file operation permissions
type Permission int

const (
	PermissionRead Permission = iota
	PermissionWrite
	PermissionDelete
	PermissionList
)

// CheckPermission checks if a user has permission to perform an operation on a path
func CheckPermission(user *config.User, dataDir, requestPath string, perm Permission) error {
	if user == nil {
		return fmt.Errorf("user not authenticated")
	}

	// Normalize the requested path
	requestPath = filepath.Clean(requestPath)
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	// Get user's allowed path
	userPath := user.Path
	if userPath == "" {
		userPath = "/"
	}

	// Check if the requested path is within the user's allowed path
	if !strings.HasPrefix(requestPath, userPath) {
		return fmt.Errorf("access denied: path '%s' is outside user's allowed path '%s'", requestPath, userPath)
	}

	// Check permission based on user's access level
	switch perm {
	case PermissionRead, PermissionList:
		// Read operations are always allowed for authenticated users
		return nil
	case PermissionWrite, PermissionDelete:
		if user.IsReadOnly() {
			return fmt.Errorf("access denied: user '%s' has read-only permissions", "user")
		}
		return nil
	default:
		return fmt.Errorf("unknown permission type")
	}
}

// GetUserRootPath returns the full filesystem path for the user's root directory
func GetUserRootPath(user *config.User, dataDir string) string {
	if user == nil {
		return dataDir
	}
	return user.GetFullPath(dataDir)
}

// NormalizePath normalizes a path relative to the user's root
func NormalizePath(userPath, requestPath string) string {
	// Ensure userPath ends with /
	if !strings.HasSuffix(userPath, "/") {
		userPath += "/"
	}

	// Clean the request path
	requestPath = filepath.Clean(requestPath)
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	// If request is for user's root or above, return user's path
	if requestPath == "/" || strings.HasPrefix(userPath, requestPath) {
		return userPath
	}

	// Combine user path with request path
	if strings.HasPrefix(requestPath, userPath) {
		return requestPath
	}

	// Default: prepend user path
	return userPath + strings.TrimPrefix(requestPath, "/")
}
