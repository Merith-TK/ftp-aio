package server

import (
	"context"
	"fmt"
	"sync"

	"github.com/Merith-TK/ftp-aio/internal/auth"
	"github.com/Merith-TK/ftp-aio/internal/config"
	"github.com/Merith-TK/ftp-aio/internal/fs"
	"github.com/Merith-TK/ftp-aio/internal/utils"
)

// Manager handles the lifecycle of all protocol servers
type Manager struct {
	config        *config.Config
	logger        *utils.Logger
	authenticator *auth.Authenticator
	fileSystem    *fs.FileSystem
	servers       []Server
	wg            sync.WaitGroup
}

// Server interface that all protocol servers must implement
type Server interface {
	Start(ctx context.Context) error
	Stop() error
	Name() string
	Port() int
}

// NewManager creates a new server manager
func NewManager(cfg *config.Config, logger *utils.Logger, authenticator *auth.Authenticator, fileSystem *fs.FileSystem) *Manager {
	return &Manager{
		config:        cfg,
		logger:        logger,
		authenticator: authenticator,
		fileSystem:    fileSystem,
		servers:       make([]Server, 0),
	}
}

// Start starts all enabled servers
func (m *Manager) Start(ctx context.Context) error {
	m.logger.Info("Starting server manager...")

	// Create servers based on configuration
	if err := m.createServers(); err != nil {
		return fmt.Errorf("failed to create servers: %w", err)
	}

	// Start all servers
	for _, server := range m.servers {
		m.wg.Add(1)
		go func(s Server) {
			defer m.wg.Done()

			m.logger.Info("Starting %s server on port %d", s.Name(), s.Port())

			if err := s.Start(ctx); err != nil {
				m.logger.Error("Failed to start %s server: %v", s.Name(), err)
			}
		}(server)
	}

	m.logger.Info("All servers started successfully")
	return nil
}

// Stop stops all servers
func (m *Manager) Stop() error {
	m.logger.Info("Stopping all servers...")

	// Stop all servers
	for _, server := range m.servers {
		if err := server.Stop(); err != nil {
			m.logger.Error("Failed to stop %s server: %v", server.Name(), err)
		} else {
			m.logger.Info("Stopped %s server", server.Name())
		}
	}

	// Wait for all goroutines to finish
	m.wg.Wait()

	m.logger.Info("All servers stopped")
	return nil
}

// createServers creates server instances based on configuration
func (m *Manager) createServers() error {
	// FTP Server
	if m.config.Services.FTP.Enabled {
		server := NewFTPServer(m.config, m.logger, m.authenticator, m.fileSystem)
		m.servers = append(m.servers, server)
	}

	// TODO: Add other servers (FTPS, SFTP, HTTP, HTTPS, TFTP) in future phases

	if len(m.servers) == 0 {
		return fmt.Errorf("no servers enabled")
	}

	return nil
}
