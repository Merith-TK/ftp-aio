package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Merith-TK/ftp-aio/internal/auth"
	"github.com/Merith-TK/ftp-aio/internal/config"
	"github.com/Merith-TK/ftp-aio/internal/fs"
	"github.com/Merith-TK/ftp-aio/internal/utils"
)

// FTPServer implements the FTP protocol server
type FTPServer struct {
	config        *config.Config
	logger        *utils.Logger
	authenticator *auth.Authenticator
	fileSystem    *fs.FileSystem
	listener      net.Listener
	done          chan struct{}
}

// FTPConnection represents a single FTP connection
type FTPConnection struct {
	conn         net.Conn
	server       *FTPServer
	user         *config.User
	username     string
	currentDir   string
	dataConn     net.Conn
	pasvListener net.Listener
}

// NewFTPServer creates a new FTP server
func NewFTPServer(cfg *config.Config, logger *utils.Logger, authenticator *auth.Authenticator, fileSystem *fs.FileSystem) *FTPServer {
	return &FTPServer{
		config:        cfg,
		logger:        logger,
		authenticator: authenticator,
		fileSystem:    fileSystem,
		done:          make(chan struct{}),
	}
}

// Start starts the FTP server
func (s *FTPServer) Start(ctx context.Context) error {
	port := s.config.Services.FTP.Port

	// Start listening
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", port, err)
	}
	s.listener = listener

	s.logger.Info("FTP server listening on port %d", port)

	// Accept connections in a goroutine
	go func() {
		for {
			select {
			case <-s.done:
				return
			default:
				conn, err := listener.Accept()
				if err != nil {
					select {
					case <-s.done:
						return
					default:
						s.logger.Error("Failed to accept FTP connection: %v", err)
						continue
					}
				}

				// Handle connection in a goroutine
				go s.handleConnection(conn)
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// Stop stops the FTP server
func (s *FTPServer) Stop() error {
	close(s.done)
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

// Name returns the server name
func (s *FTPServer) Name() string {
	return "FTP"
}

// Port returns the port the server is listening on
func (s *FTPServer) Port() int {
	return s.config.Services.FTP.Port
}

// handleConnection handles a single FTP connection
func (s *FTPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	s.logger.Debug("New FTP connection from %s", conn.RemoteAddr())

	ftpConn := &FTPConnection{
		conn:       conn,
		server:     s,
		currentDir: "/",
	}

	// Send welcome message
	ftpConn.sendResponse(220, "FTP-AIO Server Ready")

	// Handle commands
	ftpConn.handleCommands()
}

// sendResponse sends an FTP response
func (c *FTPConnection) sendResponse(code int, message string) {
	response := fmt.Sprintf("%d %s\r\n", code, message)
	c.conn.Write([]byte(response))
	c.server.logger.Debug("FTP response: %s", strings.TrimSpace(response))
}

// handleCommands handles FTP commands in a loop
func (c *FTPConnection) handleCommands() {
	scanner := bufio.NewScanner(c.conn)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		c.server.logger.Debug("FTP command: %s", line)

		parts := strings.SplitN(line, " ", 2)
		command := strings.ToUpper(parts[0])
		var args string
		if len(parts) > 1 {
			args = parts[1]
		}

		switch command {
		case "USER":
			c.handleUser(args)
		case "PASS":
			c.handlePass(args)
		case "QUIT":
			c.sendResponse(221, "Goodbye")
			return
		case "SYST":
			c.sendResponse(215, "UNIX Type: L8")
		case "PWD", "XPWD":
			if c.user == nil {
				c.sendResponse(530, "Not logged in")
			} else {
				// Show the current directory relative to the user's root
				displayPath := c.currentDir
				if c.user.Path != "/" && strings.HasPrefix(displayPath, c.user.Path) {
					displayPath = "/" + strings.TrimPrefix(displayPath, c.user.Path)
					displayPath = strings.TrimPrefix(displayPath, "/")
					if displayPath == "" {
						displayPath = "/"
					} else {
						displayPath = "/" + displayPath
					}
				}
				c.sendResponse(257, fmt.Sprintf("\"%s\" is current directory", displayPath))
			}
		case "TYPE":
			c.handleType(args)
		case "PASV":
			c.handlePasv()
		case "LIST", "NLST":
			c.handleList(args)
		case "CWD":
			c.handleCwd(args)
		case "RETR":
			c.handleRetr(args)
		case "STOR":
			c.handleStor(args)
		case "NOOP":
			c.sendResponse(200, "OK")
		default:
			c.sendResponse(502, "Command not implemented")
		}
	}
}

// handleUser handles the USER command
func (c *FTPConnection) handleUser(username string) {
	c.username = username
	c.sendResponse(331, "Password required")
}

// handlePass handles the PASS command
func (c *FTPConnection) handlePass(password string) {
	if c.username == "" {
		c.sendResponse(503, "Send USER first")
		return
	}

	// Authenticate user
	user, err := c.server.authenticator.Authenticate(c.username, password)
	if err != nil {
		c.sendResponse(530, "Login incorrect")
		return
	}

	c.user = user
	// Set initial directory to user's configured path
	c.currentDir = user.Path
	if c.currentDir == "" {
		c.currentDir = "/"
	}
	
	c.sendResponse(230, "Login successful")
}

// handleType handles the TYPE command
func (c *FTPConnection) handleType(args string) {
	c.sendResponse(200, "Type set to binary")
}

// handlePasv handles the PASV command (passive mode)
func (c *FTPConnection) handlePasv() {
	// Create a listener for passive data connection
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		c.sendResponse(425, "Cannot open passive connection")
		return
	}

	c.pasvListener = listener

	// Get the port
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port

	// Convert port to high/low bytes
	p1 := port / 256
	p2 := port % 256

	// Get local IP (simplified - use 127.0.0.1 for local testing)
	c.sendResponse(227, fmt.Sprintf("Entering Passive Mode (127,0,0,1,%d,%d)", p1, p2))
}

// handleList handles the LIST command
func (c *FTPConnection) handleList(args string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if c.pasvListener == nil {
		c.sendResponse(425, "Use PASV first")
		return
	}

	c.sendResponse(150, "Opening data connection for directory listing")

	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.sendResponse(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	// Get actual directory listing from file system
	files, err := c.server.fileSystem.ListDirectory(c.user, c.currentDir)
	if err != nil {
		c.server.logger.Error("Failed to list directory: %v", err)
		c.sendResponse(550, "Failed to list directory")
		return
	}

	// Format listing in standard FTP format with proper ownership
	var listing strings.Builder
	for _, file := range files {
		// Determine permissions based on user access and file type
		var perms string
		if file.IsDir {
			if c.user.CanWrite() {
				perms = "drwxr-xr-x"
			} else {
				perms = "dr-xr-xr-x"
			}
		} else {
			if c.user.CanWrite() {
				perms = "-rw-r--r--"
			} else {
				perms = "-r--r--r--"
			}
		}

		// Format modification time (simplified for Phase 1)
		modTime := file.ModTime.Format("Jan 02 15:04")
		if file.ModTime.Year() != time.Now().Year() {
			modTime = file.ModTime.Format("Jan 02  2006")
		}

		// Show the authenticated user as owner for all files they can see
		username := c.username
		if username == "" {
			username = "ftp"
		}

		if file.IsDir {
			listing.WriteString(fmt.Sprintf("%s   2 %s %s     4096 %s %s\r\n", 
				perms, username, username, modTime, file.Name))
		} else {
			listing.WriteString(fmt.Sprintf("%s   1 %s %s %8d %s %s\r\n", 
				perms, username, username, file.Size, modTime, file.Name))
		}
	}

	// Send listing
	dataConn.Write([]byte(listing.String()))

	c.sendResponse(226, "Directory listing completed")

	// Close passive listener
	c.pasvListener.Close()
	c.pasvListener = nil
}

// handleCwd handles the CWD command
func (c *FTPConnection) handleCwd(path string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	// Normalize the path
	var newPath string
	if strings.HasPrefix(path, "/") {
		newPath = path
	} else {
		if c.currentDir == "/" {
			newPath = "/" + path
		} else {
			newPath = c.currentDir + "/" + path
		}
	}

	// Check if user has permission to access this directory
	if err := auth.CheckPermission(c.user, c.server.config.Data, newPath, auth.PermissionList); err != nil {
		c.server.logger.Debug("CWD permission denied for user %s to path %s: %v", c.username, newPath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Try to list the directory to ensure it exists and is accessible
	_, err := c.server.fileSystem.ListDirectory(c.user, newPath)
	if err != nil {
		c.server.logger.Debug("CWD failed for user %s to path %s: %v", c.username, newPath, err)
		c.sendResponse(550, "Directory not found or access denied")
		return
	}

	// Update current directory
	c.currentDir = newPath
	c.sendResponse(250, fmt.Sprintf("Directory changed to %s", newPath))
}

// handleRetr handles the RETR command
func (c *FTPConnection) handleRetr(filename string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if c.pasvListener == nil {
		c.sendResponse(425, "Use PASV first")
		return
	}

	// Get the file path
	filePath := filename
	if !strings.HasPrefix(filePath, "/") {
		if c.currentDir == "/" {
			filePath = "/" + filename
		} else {
			filePath = c.currentDir + "/" + filename
		}
	}

	// Check read permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, filePath, auth.PermissionRead); err != nil {
		c.server.logger.Debug("RETR permission denied for user %s to file %s: %v", c.username, filePath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Read the file and get a reader
	reader, err := c.server.fileSystem.ReadFile(c.user, filePath)
	if err != nil {
		c.server.logger.Error("Failed to read file %s: %v", filePath, err)
		c.sendResponse(550, "File not found")
		return
	}
	defer reader.Close()

	c.sendResponse(150, "Opening data connection for file transfer")

	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.sendResponse(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	// Copy file content to data connection
	_, err = io.Copy(dataConn, reader)
	if err != nil {
		c.server.logger.Error("Failed to send file: %v", err)
		c.sendResponse(426, "Transfer aborted")
		return
	}

	c.sendResponse(226, "Transfer completed")

	// Close passive listener
	c.pasvListener.Close()
	c.pasvListener = nil
}

// handleStor handles the STOR command
func (c *FTPConnection) handleStor(filename string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if c.pasvListener == nil {
		c.sendResponse(425, "Use PASV first")
		return
	}

	// Check if user has write permissions
	if c.user.IsReadOnly() {
		c.sendResponse(550, "Permission denied: read-only user")
		return
	}

	// Get the file path
	filePath := filename
	if !strings.HasPrefix(filePath, "/") {
		if c.currentDir == "/" {
			filePath = "/" + filename
		} else {
			filePath = c.currentDir + "/" + filename
		}
	}

	// Check write permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, filePath, auth.PermissionWrite); err != nil {
		c.server.logger.Debug("STOR permission denied for user %s to file %s: %v", c.username, filePath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	c.sendResponse(150, "Opening data connection for file upload")

	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.sendResponse(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	// Write the file using the file system
	writer, err := c.server.fileSystem.WriteFile(c.user, filePath)
	if err != nil {
		c.server.logger.Error("Failed to create file %s: %v", filePath, err)
		c.sendResponse(550, "Failed to store file")
		return
	}
	defer writer.Close()

	// Copy data from connection to file
	_, err = io.Copy(writer, dataConn)
	if err != nil {
		c.server.logger.Error("Failed to write file data %s: %v", filePath, err)
		c.sendResponse(550, "Failed to store file")
		return
	}

	c.sendResponse(226, "Transfer completed")

	// Close passive listener
	c.pasvListener.Close()
	c.pasvListener = nil
}
