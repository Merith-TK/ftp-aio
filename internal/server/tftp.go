package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Merith-TK/ftp-aio/internal/auth"
	"github.com/Merith-TK/ftp-aio/internal/config"
	"github.com/Merith-TK/ftp-aio/internal/fs"
	"github.com/Merith-TK/ftp-aio/internal/utils"
)

// TFTP opcodes according to RFC 1350
const (
	OpRRQ   = 1 // Read Request
	OpWRQ   = 2 // Write Request  
	OpDATA  = 3 // Data
	OpACK   = 4 // Acknowledgment
	OpERROR = 5 // Error
)

// TFTP error codes
const (
	ErrNotDefined        = 0
	ErrFileNotFound      = 1
	ErrAccessViolation   = 2
	ErrDiskFull         = 3
	ErrIllegalOperation = 4
	ErrUnknownTID       = 5
	ErrFileExists       = 6
	ErrNoSuchUser       = 7
)

// TFTP packet types
type TFTPPacket struct {
	Opcode uint16
	Data   []byte
}

// transferState represents an active file transfer
type transferState struct {
	user        *config.User
	filename    string
	isUpload    bool        // true for upload (WRQ), false for download (RRQ)
	writer      io.WriteCloser  // for uploads
	reader      io.ReadCloser   // for downloads
	blockNum    uint16
	lastPacket  []byte      // for retransmission
}

// TFTPServer implements the TFTP protocol server
type TFTPServer struct {
	config        *config.Config
	logger        *utils.Logger
	authenticator *auth.Authenticator
	fileSystem    *fs.FileSystem
	conn          *net.UDPConn
	done          chan struct{}
	
	// Active transfers map: clientAddr -> transfer state
	transfers map[string]*transferState
	transfersMutex sync.RWMutex
}

// NewTFTPServer creates a new TFTP server
func NewTFTPServer(cfg *config.Config, logger *utils.Logger, authenticator *auth.Authenticator, fileSystem *fs.FileSystem) *TFTPServer {
	return &TFTPServer{
		config:        cfg,
		logger:        logger,
		authenticator: authenticator,
		fileSystem:    fileSystem,
		done:          make(chan struct{}),
		transfers:     make(map[string]*transferState),
	}
}

// Start starts the TFTP server
func (s *TFTPServer) Start(ctx context.Context) error {
	port := s.config.Services.TFTP.Port

	// Start listening on UDP
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP port %d: %w", port, err)
	}
	s.conn = conn

	s.logger.Info("TFTP server listening on port %d", port)

	// Handle packets in a goroutine
	go func() {
		buffer := make([]byte, 516) // TFTP max packet size

		for {
			select {
			case <-s.done:
				return
			default:
				// Set read timeout to avoid blocking forever
				s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
				
				n, clientAddr, err := s.conn.ReadFromUDP(buffer)
				if err != nil {
					// Check if it's a timeout
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						continue
					}
					select {
					case <-s.done:
						return
					default:
						s.logger.Error("Failed to read UDP packet: %v", err)
						continue
					}
				}

				// Handle packet in a separate goroutine
				go s.handlePacket(buffer[:n], clientAddr)
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// Stop stops the TFTP server
func (s *TFTPServer) Stop() error {
	close(s.done)
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// Name returns the server name
func (s *TFTPServer) Name() string {
	return "TFTP"
}

// Port returns the port the server is listening on
func (s *TFTPServer) Port() int {
	return s.config.Services.TFTP.Port
}

// handlePacket handles a single TFTP packet
func (s *TFTPServer) handlePacket(data []byte, clientAddr *net.UDPAddr) {
	if len(data) < 2 {
		s.sendError(clientAddr, ErrIllegalOperation, "Invalid packet")
		return
	}

	opcode := binary.BigEndian.Uint16(data[:2])
	clientKey := clientAddr.String()
	
	s.logger.Debug("TFTP packet from %s: opcode=%d, size=%d", clientAddr, opcode, len(data))

	switch opcode {
	case OpRRQ:
		s.handleRRQ(data[2:], clientAddr)
	case OpWRQ:
		s.handleWRQ(data[2:], clientAddr)
	case OpDATA:
		s.handleDATA(data, clientAddr, clientKey)
	case OpACK:
		s.handleACK(data, clientAddr, clientKey)
	default:
		s.logger.Debug("Unsupported TFTP opcode: %d", opcode)
		s.sendError(clientAddr, ErrIllegalOperation, "Unsupported operation")
	}
}

// parseRequest parses a RRQ or WRQ packet
func (s *TFTPServer) parseRequest(data []byte) (filename, mode string, err error) {
	// Format: filename\0mode\0
	parts := strings.Split(string(data), "\000")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid request format")
	}
	
	filename = parts[0]
	mode = strings.ToLower(parts[1])
	
	// Only support octet (binary) mode for simplicity
	if mode != "octet" && mode != "binary" {
		return "", "", fmt.Errorf("unsupported mode: %s", mode)
	}
	
	return filename, mode, nil
}

// handleRRQ handles a Read Request
func (s *TFTPServer) handleRRQ(data []byte, clientAddr *net.UDPAddr) {
	filename, mode, err := s.parseRequest(data)
	if err != nil {
		s.logger.Debug("Invalid RRQ: %v", err)
		s.sendError(clientAddr, ErrIllegalOperation, err.Error())
		return
	}
	
	s.logger.Debug("TFTP RRQ: file=%s, mode=%s, client=%s", filename, mode, clientAddr)
	
	// For TFTP, we'll use a default user or anonymous access
	// In a real implementation, you might want to add authentication
	user := s.getDefaultUser()
	if user == nil {
		s.sendError(clientAddr, ErrAccessViolation, "No default user configured")
		return
	}
	
	// Normalize filename
	if !strings.HasPrefix(filename, "/") {
		filename = "/" + filename
	}
	
	// Check read permission
	if err := auth.CheckPermission(user, s.config.Data, filename, auth.PermissionRead); err != nil {
		s.logger.Debug("TFTP RRQ permission denied: %v", err)
		s.sendError(clientAddr, ErrAccessViolation, "Access denied")
		return
	}
	
	// Open file
	reader, err := s.fileSystem.ReadFile(user, filename)
	if err != nil {
		s.logger.Debug("TFTP RRQ file not found: %v", err)
		s.sendError(clientAddr, ErrFileNotFound, "File not found")
		return
	}
	
	// Create transfer state
	clientKey := clientAddr.String()
	s.transfersMutex.Lock()
	s.transfers[clientKey] = &transferState{
		user:     user,
		filename: filename,
		isUpload: false,
		reader:   reader,
		blockNum: 1, // Start with block 1
	}
	s.transfersMutex.Unlock()
	
	// Send first block
	s.sendNextBlock(s.transfers[clientKey], clientAddr, clientKey)
}

// handleWRQ handles a Write Request  
func (s *TFTPServer) handleWRQ(data []byte, clientAddr *net.UDPAddr) {
	filename, mode, err := s.parseRequest(data)
	if err != nil {
		s.logger.Debug("Invalid WRQ: %v", err)
		s.sendError(clientAddr, ErrIllegalOperation, err.Error())
		return
	}
	
	s.logger.Debug("TFTP WRQ: file=%s, mode=%s, client=%s", filename, mode, clientAddr)
	
	// For TFTP, we'll use a default user or anonymous access
	user := s.getDefaultUser()
	if user == nil {
		s.sendError(clientAddr, ErrAccessViolation, "No default user configured")
		return
	}
	
	// Check if user has write permissions
	if user.IsReadOnly() {
		s.sendError(clientAddr, ErrAccessViolation, "Read-only access")
		return
	}
	
	// Normalize filename
	if !strings.HasPrefix(filename, "/") {
		filename = "/" + filename
	}
	
	// Check write permission
	if err := auth.CheckPermission(user, s.config.Data, filename, auth.PermissionWrite); err != nil {
		s.logger.Debug("TFTP WRQ permission denied: %v", err)
		s.sendError(clientAddr, ErrAccessViolation, "Access denied")
		return
	}
	
	// Create file writer
	writer, err := s.fileSystem.WriteFile(user, filename)
	if err != nil {
		s.logger.Debug("TFTP WRQ failed to create file: %v", err)
		s.sendError(clientAddr, ErrAccessViolation, "Cannot create file")
		return
	}
	
	// Create transfer state
	clientKey := clientAddr.String()
	s.transfersMutex.Lock()
	s.transfers[clientKey] = &transferState{
		user:     user,
		filename: filename,
		isUpload: true,
		writer:   writer,
		blockNum: 1, // Expecting block 1 first
	}
	s.transfersMutex.Unlock()
	
	// Send initial ACK (block 0) to start the transfer
	s.sendACK(0, clientAddr)
}

// sendFile sends a file to the client in TFTP DATA packets
func (s *TFTPServer) sendFile(reader io.Reader, clientAddr *net.UDPAddr) {
	blockNum := uint16(1)
	buffer := make([]byte, 512) // TFTP data block size
	
	for {
		n, err := reader.Read(buffer)
		if err != nil && err != io.EOF {
			s.logger.Error("Error reading file: %v", err)
			s.sendError(clientAddr, ErrNotDefined, "Read error")
			return
		}
		
		// Send DATA packet
		dataPacket := make([]byte, 4+n)
		binary.BigEndian.PutUint16(dataPacket[0:2], OpDATA)
		binary.BigEndian.PutUint16(dataPacket[2:4], blockNum)
		copy(dataPacket[4:], buffer[:n])
		
		// Send packet and wait for ACK
		for retries := 0; retries < 3; retries++ {
			_, sendErr := s.conn.WriteToUDP(dataPacket, clientAddr)
			if sendErr != nil {
				s.logger.Error("Failed to send DATA packet: %v", sendErr)
				return
			}
			
			// Wait for ACK
			ackReceived := s.waitForACK(blockNum, clientAddr)
			if ackReceived {
				break
			}
			
			if retries == 2 {
				s.logger.Debug("No ACK received after 3 retries, giving up")
				return
			}
		}
		
		// If this was the last packet (less than 512 bytes), we're done
		if n < 512 || err == io.EOF {
			s.logger.Debug("TFTP file transfer completed")
			break
		}
		
		blockNum++
	}
}

// receiveFile receives a file from the client in TFTP DATA packets
func (s *TFTPServer) receiveFile(writer io.Writer, clientAddr *net.UDPAddr) {
	// Send initial ACK (block 0) to start the transfer
	s.sendACK(0, clientAddr)
	
	expectedBlock := uint16(1)
	
	for {
		// Wait for DATA packet
		dataPacket, err := s.waitForDATA(clientAddr, 5*time.Second)
		if err != nil {
			s.logger.Debug("Error waiting for DATA packet: %v", err)
			s.sendError(clientAddr, ErrNotDefined, "Transfer timeout")
			return
		}
		
		if len(dataPacket) < 4 {
			s.sendError(clientAddr, ErrIllegalOperation, "Invalid DATA packet")
			return
		}
		
		blockNum := binary.BigEndian.Uint16(dataPacket[2:4])
		data := dataPacket[4:]
		
		// Check if this is the expected block
		if blockNum != expectedBlock {
			s.logger.Debug("Unexpected block number: got %d, expected %d", blockNum, expectedBlock)
			// Send ACK for previous block to trigger retransmission
			s.sendACK(expectedBlock-1, clientAddr)
			continue
		}
		
		// Write data to file
		_, err = writer.Write(data)
		if err != nil {
			s.logger.Error("Error writing to file: %v", err)
			s.sendError(clientAddr, ErrDiskFull, "Write error")
			return
		}
		
		// Send ACK
		s.sendACK(blockNum, clientAddr)
		
		// If this was the last packet (less than 512 bytes), we're done
		if len(data) < 512 {
			s.logger.Debug("TFTP file upload completed")
			break
		}
		
		expectedBlock++
	}
}

// sendACK sends an ACK packet
func (s *TFTPServer) sendACK(blockNum uint16, clientAddr *net.UDPAddr) {
	ackPacket := make([]byte, 4)
	binary.BigEndian.PutUint16(ackPacket[0:2], OpACK)
	binary.BigEndian.PutUint16(ackPacket[2:4], blockNum)
	
	s.conn.WriteToUDP(ackPacket, clientAddr)
}

// sendError sends an ERROR packet
func (s *TFTPServer) sendError(clientAddr *net.UDPAddr, errorCode uint16, message string) {
	errorPacket := make([]byte, 4+len(message)+1)
	binary.BigEndian.PutUint16(errorPacket[0:2], OpERROR)
	binary.BigEndian.PutUint16(errorPacket[2:4], errorCode)
	copy(errorPacket[4:], message)
	errorPacket[len(errorPacket)-1] = 0 // Null terminator
	
	s.conn.WriteToUDP(errorPacket, clientAddr)
}

// waitForACK waits for an ACK packet for the specified block number
func (s *TFTPServer) waitForACK(expectedBlock uint16, clientAddr *net.UDPAddr) bool {
	deadline := time.Now().Add(2 * time.Second)
	buffer := make([]byte, 516)
	
	for time.Now().Before(deadline) {
		s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}
		
		// Check if packet is from the expected client
		if addr.String() != clientAddr.String() {
			continue
		}
		
		if n >= 4 {
			opcode := binary.BigEndian.Uint16(buffer[0:2])
			blockNum := binary.BigEndian.Uint16(buffer[2:4])
			
			if opcode == OpACK && blockNum == expectedBlock {
				return true
			}
		}
	}
	
	return false
}

// waitForDATA waits for a DATA packet from the client
func (s *TFTPServer) waitForDATA(clientAddr *net.UDPAddr, timeout time.Duration) ([]byte, error) {
	deadline := time.Now().Add(timeout)
	buffer := make([]byte, 516)
	
	for time.Now().Before(deadline) {
		s.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, addr, err := s.conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}
		
		// Check if packet is from the expected client
		if addr.String() != clientAddr.String() {
			continue
		}
		
		if n >= 4 {
			opcode := binary.BigEndian.Uint16(buffer[0:2])
			if opcode == OpDATA {
				return buffer[:n], nil
			}
		}
	}
	
	return nil, fmt.Errorf("timeout waiting for DATA packet")
}

// getDefaultUser returns a default user for TFTP operations
// In a real implementation, you might want to configure this or use anonymous access
func (s *TFTPServer) getDefaultUser() *config.User {
	// Try to find the first user with write permissions for uploads
	for _, user := range s.config.Users {
		if !user.IsReadOnly() {
			return user // Return the first user with write permissions
		}
	}
	
	// If no write user found, return the first user (for read operations)
	for _, user := range s.config.Users {
		return user
	}
	return nil
}

// handleDATA handles a DATA packet during an upload
func (s *TFTPServer) handleDATA(data []byte, clientAddr *net.UDPAddr, clientKey string) {
	if len(data) < 4 {
		s.sendError(clientAddr, ErrIllegalOperation, "Invalid DATA packet")
		return
	}
	
	blockNum := binary.BigEndian.Uint16(data[2:4])
	fileData := data[4:]
	
	s.logger.Debug("TFTP DATA from %s: block=%d, size=%d", clientAddr, blockNum, len(fileData))
	
	// Get transfer state
	s.transfersMutex.RLock()
	transfer, exists := s.transfers[clientKey]
	s.transfersMutex.RUnlock()
	
	if !exists || !transfer.isUpload {
		s.sendError(clientAddr, ErrUnknownTID, "No active upload")
		return
	}
	
	// Check if this is the expected block
	if blockNum != transfer.blockNum {
		s.logger.Debug("Unexpected block number: got %d, expected %d", blockNum, transfer.blockNum)
		// Send ACK for previous block to trigger retransmission
		if blockNum == transfer.blockNum-1 {
			s.sendACK(blockNum, clientAddr)
		}
		return
	}
	
	// Write data to file
	_, err := transfer.writer.Write(fileData)
	if err != nil {
		s.logger.Error("Error writing to file: %v", err)
		s.sendError(clientAddr, ErrDiskFull, "Write error")
		s.cleanupTransfer(clientKey)
		return
	}
	
	// Send ACK
	s.sendACK(blockNum, clientAddr)
	
	// If this was the last packet (less than 512 bytes), we're done
	if len(fileData) < 512 {
		s.logger.Debug("TFTP file upload completed")
		s.cleanupTransfer(clientKey)
		return
	}
	
	// Update expected block number
	s.transfersMutex.Lock()
	transfer.blockNum++
	s.transfersMutex.Unlock()
}

// handleACK handles an ACK packet during a download
func (s *TFTPServer) handleACK(data []byte, clientAddr *net.UDPAddr, clientKey string) {
	if len(data) < 4 {
		s.sendError(clientAddr, ErrIllegalOperation, "Invalid ACK packet")
		return
	}
	
	blockNum := binary.BigEndian.Uint16(data[2:4])
	
	s.logger.Debug("TFTP ACK from %s: block=%d", clientAddr, blockNum)
	
	// Get transfer state
	s.transfersMutex.RLock()
	transfer, exists := s.transfers[clientKey]
	s.transfersMutex.RUnlock()
	
	if !exists || transfer.isUpload {
		s.sendError(clientAddr, ErrUnknownTID, "No active download")
		return
	}
	
	// Check if this is the expected ACK
	if blockNum != transfer.blockNum-1 {
		s.logger.Debug("Unexpected ACK number: got %d, expected %d", blockNum, transfer.blockNum-1)
		return
	}
	
	// Send next block
	s.sendNextBlock(transfer, clientAddr, clientKey)
}

// cleanupTransfer removes a transfer state and closes resources
func (s *TFTPServer) cleanupTransfer(clientKey string) {
	s.transfersMutex.Lock()
	defer s.transfersMutex.Unlock()
	
	if transfer, exists := s.transfers[clientKey]; exists {
		if transfer.writer != nil {
			transfer.writer.Close()
		}
		if transfer.reader != nil {
			transfer.reader.Close()
		}
		delete(s.transfers, clientKey)
	}
}

// sendNextBlock sends the next block for a download transfer
func (s *TFTPServer) sendNextBlock(transfer *transferState, clientAddr *net.UDPAddr, clientKey string) {
	buffer := make([]byte, 512)
	n, err := transfer.reader.Read(buffer)
	if err != nil && err != io.EOF {
		s.logger.Error("Error reading file: %v", err)
		s.sendError(clientAddr, ErrNotDefined, "Read error")
		s.cleanupTransfer(clientKey)
		return
	}
	
	// Send DATA packet
	dataPacket := make([]byte, 4+n)
	binary.BigEndian.PutUint16(dataPacket[0:2], OpDATA)
	binary.BigEndian.PutUint16(dataPacket[2:4], transfer.blockNum)
	copy(dataPacket[4:], buffer[:n])
	
	s.conn.WriteToUDP(dataPacket, clientAddr)
	
	// If this was the last block (less than 512 bytes), cleanup after ACK
	if n < 512 {
		// We'll cleanup when we receive the final ACK
		return
	}
	
	// Update block number for next packet
	s.transfersMutex.Lock()
	transfer.blockNum++
	s.transfersMutex.Unlock()
}
