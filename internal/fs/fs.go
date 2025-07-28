package fs

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Merith-TK/ftp-aio/internal/auth"
	"github.com/Merith-TK/ftp-aio/internal/config"
)

// FileInfo represents file information
type FileInfo struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool
}

// FileSystem provides file system operations with user isolation
type FileSystem struct {
	dataDir string
	auth    *auth.Authenticator
}

// NewFileSystem creates a new file system instance
func NewFileSystem(dataDir string, authenticator *auth.Authenticator) *FileSystem {
	return &FileSystem{
		dataDir: dataDir,
		auth:    authenticator,
	}
}

// ListDirectory lists files in a directory for a given user
func (fs *FileSystem) ListDirectory(user *config.User, path string) ([]FileInfo, error) {
	// Check read permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionList); err != nil {
		return nil, err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Read directory
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // Skip files we can't stat
		}

		files = append(files, FileInfo{
			Name:    info.Name(),
			Size:    info.Size(),
			Mode:    info.Mode(),
			ModTime: info.ModTime(),
			IsDir:   info.IsDir(),
		})
	}

	return files, nil
}

// ReadFile reads a file for a given user
func (fs *FileSystem) ReadFile(user *config.User, path string) (io.ReadCloser, error) {
	// Check read permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionRead); err != nil {
		return nil, err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Open file
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// WriteFile writes a file for a given user
func (fs *FileSystem) WriteFile(user *config.User, path string) (io.WriteCloser, error) {
	// Check write permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionWrite); err != nil {
		return nil, err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create file
	file, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return file, nil
}

// DeleteFile deletes a file for a given user
func (fs *FileSystem) DeleteFile(user *config.User, path string) error {
	// Check delete permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionDelete); err != nil {
		return err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Delete file
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// CreateDirectory creates a directory for a given user
func (fs *FileSystem) CreateDirectory(user *config.User, path string) error {
	// Check write permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionWrite); err != nil {
		return err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Create directory
	if err := os.MkdirAll(fullPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// RemoveDirectory removes a directory for a given user
func (fs *FileSystem) RemoveDirectory(user *config.User, path string) error {
	// Check delete permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionDelete); err != nil {
		return err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Remove directory
	if err := os.Remove(fullPath); err != nil {
		return fmt.Errorf("failed to remove directory: %w", err)
	}

	return nil
}

// GetFileInfo gets information about a file or directory
func (fs *FileSystem) GetFileInfo(user *config.User, path string) (*FileInfo, error) {
	// Check read permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionRead); err != nil {
		return nil, err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Get file info
	info, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	return &FileInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    info.Mode(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}, nil
}

// getFullPath converts a user-relative path to a full filesystem path
func (fs *FileSystem) getFullPath(user *config.User, path string) string {
	// Get user's root path
	userRoot := auth.GetUserRootPath(user, fs.dataDir)

	// Clean the path
	path = filepath.Clean(path)
	if path == "." || path == "/" {
		return userRoot
	}

	// Remove leading slash and join with user root
	path = strings.TrimPrefix(path, "/")
	return filepath.Join(userRoot, path)
}

// GetFileSize gets the size of a file for a given user
func (fs *FileSystem) GetFileSize(user *config.User, path string) (int64, error) {
	// Check read permission
	if err := auth.CheckPermission(user, fs.dataDir, path, auth.PermissionRead); err != nil {
		return 0, err
	}

	// Get the actual filesystem path
	fullPath := fs.getFullPath(user, path)

	// Get file info
	info, err := os.Stat(fullPath)
	if err != nil {
		return 0, err
	}

	// Return file size, or 0 if it's a directory
	if info.IsDir() {
		return 0, fmt.Errorf("path is a directory")
	}

	return info.Size(), nil
}
