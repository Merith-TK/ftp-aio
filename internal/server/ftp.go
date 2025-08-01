package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"path/filepath"
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
	pasvMinPort   int
	pasvMaxPort   int
}

// FTPConnection represents a single FTP connection
type FTPConnection struct {
	conn         net.Conn
	server       *FTPServer
	user         *config.User
	username     string
	currentDir   string
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
		pasvMinPort:   2122, // Start just above the FTP control port
		pasvMaxPort:   2132, // Small range for better firewall compatibility
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
	var response string
	if code == 0 {
		// Raw message without code (for multi-line responses like FEAT)
		response = message + "\r\n"
	} else {
		response = fmt.Sprintf("%d %s\r\n", code, message)
	}
	c.conn.Write([]byte(response))
	c.server.logger.Debug("FTP response: %s", strings.TrimSpace(response))
}

// normalizePath normalizes a path by resolving . and .. components
func (c *FTPConnection) normalizePath(path string) string {
	// Clean the path to resolve . and .. 
	cleaned := filepath.Clean(path)
	
	// Ensure it starts with /
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	
	// If the cleaned path goes above root, return root
	if cleaned == "." || strings.HasPrefix(cleaned, "../") {
		return "/"
	}
	
	return cleaned
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
		case "FEAT":
			c.handleFeat()
		case "TYPE":
			c.handleType(args)
		case "PASV":
			c.handlePasv()
		case "EPSV":
			c.handleEpsv()
		case "PORT":
			c.handlePort(args)
		case "LIST", "NLST":
			c.handleList(args)
		case "CWD":
			c.handleCwd(args)
		case "RETR":
			c.handleRetr(args)
		case "STOR":
			c.handleStor(args)
		case "DELE":
			c.handleDele(args)
		case "MKD", "XMKD":
			c.handleMkd(args)
		case "RMD", "XRMD":
			c.handleRmd(args)
		case "SIZE":
			c.handleSize(args)
		case "MDTM":
			c.handleMdtm(args)
		case "MLSD":
			c.handleMlsd(args)
		case "OPTS":
			c.handleOpts(args)
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

// handleFeat handles the FEAT command (feature list)
func (c *FTPConnection) handleFeat() {
	features := []string{
		"211-Features:",
		" PASV",
		" EPSV",
		" SIZE",
		" MDTM",
		" MLST type*;size*;modify*;",
		" MLSD",
		" UTF8",
		"211 END",
	}
	
	for _, feature := range features {
		c.sendResponse(0, feature) // Send raw without code
	}
}

// handlePasv handles the PASV command (passive mode)
func (c *FTPConnection) handlePasv() {
	// Close any existing passive listener
	if c.pasvListener != nil {
		c.pasvListener.Close()
	}

	// Try to create a listener within the passive port range
	var listener net.Listener
	var err error
	var port int
	
	for p := c.server.pasvMinPort; p <= c.server.pasvMaxPort; p++ {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			port = p
			break
		}
	}
	
	if listener == nil {
		c.server.logger.Error("Failed to create passive listener in range %d-%d: %v", c.server.pasvMinPort, c.server.pasvMaxPort, err)
		c.sendResponse(425, "Cannot open passive connection")
		return
	}

	c.pasvListener = listener

	// Convert port to high/low bytes
	p1 := port / 256
	p2 := port % 256

	c.server.logger.Debug("PASV: created listener on port %d (p1=%d, p2=%d)", port, p1, p2)

	// Get the local IP address from the control connection
	localAddr := c.conn.LocalAddr().(*net.TCPAddr).IP
	ip := localAddr.To4()
	
	c.server.logger.Debug("PASV: local address detected as %s", localAddr.String())
	
	// For local testing and many firewall scenarios, use 127.0.0.1
	// In production, you'd want to configure the external IP
	if ip == nil || localAddr.IsUnspecified() || localAddr.String() == "0.0.0.0" {
		ip = net.IPv4(127, 0, 0, 1)
		c.server.logger.Debug("PASV: using loopback IP for passive mode")
	} else {
		c.server.logger.Debug("PASV: using detected IP %s for passive mode", ip.String())
	}

	c.sendResponse(227, fmt.Sprintf("Entering Passive Mode (%d,%d,%d,%d,%d,%d)", 
		ip[0], ip[1], ip[2], ip[3], p1, p2))
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

	// Set timeout for passive listener
	deadline := time.Now().Add(60 * time.Second)
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(deadline)
	}

	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.sendResponse(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	// Clear listener deadline
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Time{})
	}

	c.server.logger.Debug("Data connection established for LIST %s from %s", c.currentDir, dataConn.RemoteAddr())

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

// handleEpsv handles the EPSV command (extended passive mode)
func (c *FTPConnection) handleEpsv() {
	// Close any existing passive listener
	if c.pasvListener != nil {
		c.pasvListener.Close()
	}

	// Try to create a listener within the passive port range
	var listener net.Listener
	var err error
	var port int
	
	for p := c.server.pasvMinPort; p <= c.server.pasvMaxPort; p++ {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			port = p
			break
		}
	}
	
	if listener == nil {
		c.server.logger.Error("Failed to create passive listener in range %d-%d: %v", c.server.pasvMinPort, c.server.pasvMaxPort, err)
		c.sendResponse(425, "Cannot open passive connection")
		return
	}

	c.pasvListener = listener

	c.server.logger.Debug("EPSV: created listener on port %d", port)

	// Extended passive mode response format: (|||port|)
	c.sendResponse(229, fmt.Sprintf("Entering Extended Passive Mode (|||%d|)", port))
}

// handlePort handles the PORT command (active mode) 
func (c *FTPConnection) handlePort(args string) {
	// Parse PORT command: PORT h1,h2,h3,h4,p1,p2
	parts := strings.Split(args, ",")
	if len(parts) != 6 {
		c.sendResponse(501, "Invalid PORT command format")
		return
	}

	// For simplicity, just respond that PORT is not supported and suggest PASV
	c.sendResponse(502, "PORT not supported, use PASV")
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

	// Normalize the path to handle .. and . properly
	newPath = c.normalizePath(newPath)

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
	c.server.logger.Debug("CWD successful: user %s changed to directory %s", c.username, newPath)
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

	// Normalize the path to handle .. and . properly
	filePath = c.normalizePath(filePath)

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

	// Set timeout for passive listener
	deadline := time.Now().Add(60 * time.Second)
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(deadline)
	}

	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.server.logger.Error("Failed to accept data connection for RETR: %v", err)
		c.sendResponse(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	// Clear listener deadline and set data connection timeout
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Time{})
	}
	dataConn.SetDeadline(time.Now().Add(10 * time.Minute))

	c.server.logger.Debug("Data connection established for RETR %s from %s", filePath, dataConn.RemoteAddr())

	// Copy file content to data connection
	bytesRead, err := io.Copy(dataConn, reader)
	if err != nil {
		c.server.logger.Error("Failed to send file: %v", err)
		c.sendResponse(426, "Transfer aborted")
		return
	}

	c.server.logger.Debug("RETR completed: sent %d bytes from %s", bytesRead, filePath)
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

	// Normalize the path to handle .. and . properly
	filePath = c.normalizePath(filePath)

	c.server.logger.Debug("STOR: normalized path %s for user %s", filePath, c.username)

	// Check write permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, filePath, auth.PermissionWrite); err != nil {
		c.server.logger.Debug("STOR permission denied for user %s to file %s: %v", c.username, filePath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	c.server.logger.Debug("STOR: permissions OK, sending 150 response")
	c.sendResponse(150, "Opening data connection for file upload")

	c.server.logger.Debug("STOR: waiting for data connection on port range %d-%d...", c.server.pasvMinPort, c.server.pasvMaxPort)
	
	// Set a reasonable timeout for the data connection (increased for GUI clients)
	deadline := time.Now().Add(60 * time.Second)
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(deadline)
	}
	
	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.server.logger.Error("Failed to accept data connection for STOR: %v", err)
		c.sendResponse(425, "Cannot open data connection")
		if c.pasvListener != nil {
			c.pasvListener.Close()
			c.pasvListener = nil
		}
		return
	}
	defer dataConn.Close()

	// Clear the deadline after successful accept and set data timeout
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Time{})
	}
	
	// Set timeout for data transfer
	dataConn.SetDeadline(time.Now().Add(10 * time.Minute))

	c.server.logger.Debug("Data connection established for STOR %s from %s", filePath, dataConn.RemoteAddr())

	// Create the file writer
	writer, err := c.server.fileSystem.WriteFile(c.user, filePath)
	if err != nil {
		c.server.logger.Error("Failed to create file %s: %v", filePath, err)
		c.sendResponse(550, "Failed to store file")
		return
	}
	defer writer.Close()

	c.server.logger.Debug("STOR: file writer created, starting data transfer...")

	// Copy data from connection to file
	bytesWritten, err := io.Copy(writer, dataConn)
	if err != nil {
		c.server.logger.Error("Failed to write file data %s: %v", filePath, err)
		c.sendResponse(550, "Failed to store file")
		return
	}

	c.server.logger.Debug("STOR completed: wrote %d bytes to %s", bytesWritten, filePath)
	c.sendResponse(226, "Transfer completed")

	// Close passive listener
	c.pasvListener.Close()
	c.pasvListener = nil
}

// handleDele handles the DELE command (delete file)
func (c *FTPConnection) handleDele(filename string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
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

	// Normalize the path to handle .. and . properly
	filePath = c.normalizePath(filePath)

	// Check delete permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, filePath, auth.PermissionDelete); err != nil {
		c.server.logger.Debug("DELE permission denied for user %s to file %s: %v", c.username, filePath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Delete the file using the file system
	err := c.server.fileSystem.DeleteFile(c.user, filePath)
	if err != nil {
		c.server.logger.Error("Failed to delete file %s: %v", filePath, err)
		c.sendResponse(550, "Failed to delete file")
		return
	}

	c.server.logger.Debug("DELE completed: deleted %s", filePath)
	c.sendResponse(250, "File deleted")
}

// handleMkd handles the MKD command (make directory)
func (c *FTPConnection) handleMkd(dirname string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if c.user.IsReadOnly() {
		c.sendResponse(550, "Permission denied: read-only user")
		return
	}

	// Get the directory path
	dirPath := dirname
	if !strings.HasPrefix(dirPath, "/") {
		if c.currentDir == "/" {
			dirPath = "/" + dirname
		} else {
			dirPath = c.currentDir + "/" + dirname
		}
	}

	dirPath = c.normalizePath(dirPath)

	// Check write permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, dirPath, auth.PermissionWrite); err != nil {
		c.server.logger.Debug("MKD permission denied for user %s to dir %s: %v", c.username, dirPath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Create directory using file system
	err := c.server.fileSystem.CreateDirectory(c.user, dirPath)
	if err != nil {
		c.server.logger.Error("Failed to create directory %s: %v", dirPath, err)
		c.sendResponse(550, "Failed to create directory")
		return
	}

	c.server.logger.Debug("MKD completed: created %s", dirPath)
	c.sendResponse(257, fmt.Sprintf("\"%s\" directory created", dirPath))
}

// handleRmd handles the RMD command (remove directory)
func (c *FTPConnection) handleRmd(dirname string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if c.user.IsReadOnly() {
		c.sendResponse(550, "Permission denied: read-only user")
		return
	}

	// Get the directory path
	dirPath := dirname
	if !strings.HasPrefix(dirPath, "/") {
		if c.currentDir == "/" {
			dirPath = "/" + dirname
		} else {
			dirPath = c.currentDir + "/" + dirname
		}
	}

	dirPath = c.normalizePath(dirPath)

	// Check delete permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, dirPath, auth.PermissionDelete); err != nil {
		c.server.logger.Debug("RMD permission denied for user %s to dir %s: %v", c.username, dirPath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Remove directory using file system
	err := c.server.fileSystem.RemoveDirectory(c.user, dirPath)
	if err != nil {
		c.server.logger.Error("Failed to remove directory %s: %v", dirPath, err)
		c.sendResponse(550, "Failed to remove directory")
		return
	}

	c.server.logger.Debug("RMD completed: removed %s", dirPath)
	c.sendResponse(250, "Directory removed")
}

// handleSize handles the SIZE command
func (c *FTPConnection) handleSize(filename string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
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

	filePath = c.normalizePath(filePath)

	// Check read permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, filePath, auth.PermissionRead); err != nil {
		c.server.logger.Debug("SIZE permission denied for user %s to file %s: %v", c.username, filePath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Get file size using file system
	size, err := c.server.fileSystem.GetFileSize(c.user, filePath)
	if err != nil {
		c.server.logger.Error("Failed to get size of file %s: %v", filePath, err)
		c.sendResponse(550, "File not found")
		return
	}

	c.sendResponse(213, fmt.Sprintf("%d", size))
}

// handleMlsd handles the MLSD command (machine-readable directory listing)
func (c *FTPConnection) handleMlsd(args string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if c.pasvListener == nil {
		c.sendResponse(425, "Use PASV first")
		return
	}

	c.sendResponse(150, "Opening data connection for MLSD")

	// Set timeout for passive listener
	deadline := time.Now().Add(60 * time.Second)
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(deadline)
	}

	// Accept data connection
	dataConn, err := c.pasvListener.Accept()
	if err != nil {
		c.sendResponse(425, "Cannot open data connection")
		return
	}
	defer dataConn.Close()

	// Clear listener deadline
	if tcpListener, ok := c.pasvListener.(*net.TCPListener); ok {
		tcpListener.SetDeadline(time.Time{})
	}

	c.server.logger.Debug("Data connection established for MLSD %s from %s", c.currentDir, dataConn.RemoteAddr())

	// Get directory listing
	files, err := c.server.fileSystem.ListDirectory(c.user, c.currentDir)
	if err != nil {
		c.server.logger.Error("Failed to list directory for MLSD: %v", err)
		c.sendResponse(550, "Failed to list directory")
		return
	}

	// Format in MLSD format: fact1=value1;fact2=value2; filename
	var listing strings.Builder
	for _, file := range files {
		facts := []string{}
		
		if file.IsDir {
			facts = append(facts, "type=dir")
		} else {
			facts = append(facts, "type=file")
			facts = append(facts, fmt.Sprintf("size=%d", file.Size))
		}
		
		// Add modification time in YYYYMMDDHHMMSS format
		modTime := file.ModTime.Format("20060102150405")
		facts = append(facts, fmt.Sprintf("modify=%s", modTime))
		
		factString := strings.Join(facts, ";") + ";"
		listing.WriteString(fmt.Sprintf("%s %s\r\n", factString, file.Name))
	}

	// Send listing
	dataConn.Write([]byte(listing.String()))
	c.sendResponse(226, "MLSD completed")

	// Close passive listener
	c.pasvListener.Close()
	c.pasvListener = nil
}

// handleOpts handles the OPTS command (set options)
func (c *FTPConnection) handleOpts(args string) {
	// Parse OPTS command - commonly used for UTF8
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 1 {
		c.sendResponse(501, "Invalid OPTS command")
		return
	}
	
	option := strings.ToUpper(parts[0])
	
	switch option {
	case "UTF8":
		// Accept UTF8 option but don't actually change anything
		c.sendResponse(200, "UTF8 set to on")
	default:
		c.sendResponse(502, "OPTS not implemented for " + option)
	}
}

// handleMdtm handles the MDTM command (file modification time)
func (c *FTPConnection) handleMdtm(filename string) {
	if c.user == nil {
		c.sendResponse(530, "Not logged in")
		return
	}

	if filename == "" {
		c.sendResponse(501, "No filename given")
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

	// Normalize the path to handle .. and . properly
	filePath = c.normalizePath(filePath)

	// Check read permission
	if err := auth.CheckPermission(c.user, c.server.config.Data, filePath, auth.PermissionRead); err != nil {
		c.server.logger.Debug("MDTM permission denied for user %s to file %s: %v", c.username, filePath, err)
		c.sendResponse(550, "Permission denied")
		return
	}

	// Get file information
	fileInfo, err := c.server.fileSystem.GetFileInfo(c.user, filePath)
	if err != nil {
		c.server.logger.Debug("MDTM failed to get file info for %s: %v", filePath, err)
		c.sendResponse(550, "File not found")
		return
	}

	// Format time as YYYYMMDDHHMMSS (UTC)
	modTime := fileInfo.ModTime.UTC().Format("20060102150405")
	c.sendResponse(213, modTime)
}
