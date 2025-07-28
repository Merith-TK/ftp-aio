package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Data     string           `yaml:"data"`
	Users    map[string]*User `yaml:"users"`
	Services ServiceConfig    `yaml:"services"`
	Logging  LoggingConfig    `yaml:"logging"`
	TLS      TLSConfig        `yaml:"tls"`
}

// User represents a user configuration
type User struct {
	Pass        string `yaml:"pass"`
	UID         int    `yaml:"uid"`
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions"` // "ro" or "rw"
}

// ServiceConfig contains all service configurations
type ServiceConfig struct {
	FTP   ProtocolConfig `yaml:"ftp"`
	FTPS  FTPSConfig     `yaml:"ftps"`
	SFTP  SFTPConfig     `yaml:"sftp"`
	HTTP  HTTPConfig     `yaml:"http"`
	HTTPS HTTPSConfig    `yaml:"https"`
	TFTP  ProtocolConfig `yaml:"tftp"`
}

// ProtocolConfig is basic protocol configuration
type ProtocolConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// FTPSConfig extends ProtocolConfig with TLS settings
type FTPSConfig struct {
	ProtocolConfig
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// SFTPConfig extends ProtocolConfig with SSH settings
type SFTPConfig struct {
	ProtocolConfig
	HostKey string `yaml:"host_key"`
}

// HTTPConfig extends ProtocolConfig with HTTP-specific settings
type HTTPConfig struct {
	ProtocolConfig
	Upload  bool `yaml:"upload"`
	Listing bool `yaml:"listing"`
}

// HTTPSConfig extends HTTPConfig with TLS settings
type HTTPSConfig struct {
	HTTPConfig
	Cert string `yaml:"cert"`
	Key  string `yaml:"key"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

// TLSConfig contains TLS settings for auto-generated certificates
type TLSConfig struct {
	Hostname     string `yaml:"hostname"`
	Organization string `yaml:"organization"`
}

// DefaultConfig returns a configuration with sane defaults
func DefaultConfig() *Config {
	return &Config{
		Data:  "./data",
		Users: make(map[string]*User),
		Services: ServiceConfig{
			FTP:   ProtocolConfig{Enabled: false, Port: 21},
			FTPS:  FTPSConfig{ProtocolConfig: ProtocolConfig{Enabled: false, Port: 990}},
			SFTP:  SFTPConfig{ProtocolConfig: ProtocolConfig{Enabled: false, Port: 22}},
			HTTP:  HTTPConfig{ProtocolConfig: ProtocolConfig{Enabled: false, Port: 80}, Upload: true, Listing: true},
			HTTPS: HTTPSConfig{HTTPConfig: HTTPConfig{ProtocolConfig: ProtocolConfig{Enabled: false, Port: 443}, Upload: true, Listing: true}},
			TFTP:  ProtocolConfig{Enabled: false, Port: 69},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		TLS: TLSConfig{
			Hostname:     "localhost",
			Organization: "FTP-AIO",
		},
	}
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(filename string) (*Config, error) {
	config := DefaultConfig()

	if filename == "" {
		return config, nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil // Return default config if file doesn't exist
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// ApplyEnvironmentVariables applies environment variables to the configuration
func (c *Config) ApplyEnvironmentVariables() {
	// Data directory
	if val := os.Getenv("AIO_DATA"); val != "" {
		c.Data = val
	}

	// Users from environment
	if val := os.Getenv("AIO_USERS"); val != "" {
		users, err := ParseUserString(val)
		if err == nil {
			// Merge with existing users
			for username, user := range users {
				c.Users[username] = user
			}
		}
	}

	// Protocol settings
	if val := os.Getenv("AIO_FTP"); val == "true" {
		c.Services.FTP.Enabled = true
	}
	if val := os.Getenv("AIO_FTP_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Services.FTP.Port = port
		}
	}

	if val := os.Getenv("AIO_FTPS"); val == "true" {
		c.Services.FTPS.Enabled = true
	}
	if val := os.Getenv("AIO_FTPS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Services.FTPS.Port = port
		}
	}

	if val := os.Getenv("AIO_SFTP"); val == "true" {
		c.Services.SFTP.Enabled = true
	}
	if val := os.Getenv("AIO_SFTP_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Services.SFTP.Port = port
		}
	}

	if val := os.Getenv("AIO_HTTP"); val == "true" {
		c.Services.HTTP.Enabled = true
	}
	if val := os.Getenv("AIO_HTTP_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Services.HTTP.Port = port
		}
	}

	if val := os.Getenv("AIO_HTTPS"); val == "true" {
		c.Services.HTTPS.Enabled = true
	}
	if val := os.Getenv("AIO_HTTPS_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Services.HTTPS.Port = port
		}
	}

	if val := os.Getenv("AIO_TFTP"); val == "true" {
		c.Services.TFTP.Enabled = true
	}
	if val := os.Getenv("AIO_TFTP_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Services.TFTP.Port = port
		}
	}

	// Logging
	if val := os.Getenv("AIO_LOG_LEVEL"); val != "" {
		c.Logging.Level = val
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate data directory
	if c.Data == "" {
		return fmt.Errorf("data directory cannot be empty")
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(c.Data, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Validate users
	if len(c.Users) == 0 {
		return fmt.Errorf("at least one user must be configured")
	}

	for username, user := range c.Users {
		if username == "" {
			return fmt.Errorf("username cannot be empty")
		}
		if user.Pass == "" {
			return fmt.Errorf("password cannot be empty for user %s", username)
		}
		if user.Permissions != "ro" && user.Permissions != "rw" {
			return fmt.Errorf("invalid permissions '%s' for user %s, must be 'ro' or 'rw'", user.Permissions, username)
		}

		// Ensure user path exists
		userPath := filepath.Join(c.Data, strings.TrimPrefix(user.Path, "/"))
		if err := os.MkdirAll(userPath, 0755); err != nil {
			return fmt.Errorf("failed to create user directory for %s: %w", username, err)
		}
	}

	// Validate that at least one service is enabled
	enabled := c.Services.FTP.Enabled || c.Services.FTPS.Enabled || c.Services.SFTP.Enabled ||
		c.Services.HTTP.Enabled || c.Services.HTTPS.Enabled || c.Services.TFTP.Enabled

	if !enabled {
		return fmt.Errorf("at least one service must be enabled")
	}

	// Validate log level
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.Logging.Level] {
		return fmt.Errorf("invalid log level '%s', must be one of: debug, info, warn, error", c.Logging.Level)
	}

	return nil
}
