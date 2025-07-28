package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Merith-TK/ftp-aio/internal/auth"
	"github.com/Merith-TK/ftp-aio/internal/config"
	"github.com/Merith-TK/ftp-aio/internal/fs"
	"github.com/Merith-TK/ftp-aio/internal/server"
	"github.com/Merith-TK/ftp-aio/internal/utils"
)

var (
	// Global configuration
	cfg *config.Config

	// CLI flags
	configFile string
	dataDir    string
	userString string
	logLevel   string
	certFile   string
	keyFile    string

	// Protocol flags
	enableFTP   bool
	ftpPort     int
	enableFTPS  bool
	ftpsPort    int
	enableSFTP  bool
	sftpPort    int
	enableHTTP  bool
	httpPort    int
	enableHTTPS bool
	httpsPort   int
	enableTFTP  bool
	tftpPort    int
)

var rootCmd = &cobra.Command{
	Use:   "ftp-aio [data-directory]",
	Short: "All-in-One File Transfer Server",
	Long: `A dead simple, all-in-one file transfer server supporting multiple protocols.
	
Examples:
  ftp-aio ./data --user="admin:pass:1000:/:/rw" --ftp
  ftp-aio ./data --user="user:pass:1000:/folder:rw,guest:guest:1001:/public:ro" --ftp --http
  ftp-aio --config=config.yml`,
	Args: cobra.MaximumNArgs(1),
	RunE: runServer,
}

func init() {
	// General flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Configuration file path")
	rootCmd.PersistentFlags().StringVar(&userString, "user", "", "Users in format 'user:pass:uid:path:perm,user2:...'")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&certFile, "cert", "", "SSL certificate file")
	rootCmd.PersistentFlags().StringVar(&keyFile, "key", "", "SSL key file")

	// Protocol flags
	rootCmd.PersistentFlags().BoolVar(&enableFTP, "ftp", false, "Enable FTP server")
	rootCmd.PersistentFlags().IntVar(&ftpPort, "ftp-port", 0, "FTP port (default: 21)")
	rootCmd.PersistentFlags().BoolVar(&enableFTPS, "ftps", false, "Enable FTPS server")
	rootCmd.PersistentFlags().IntVar(&ftpsPort, "ftps-port", 0, "FTPS port (default: 990)")
	rootCmd.PersistentFlags().BoolVar(&enableSFTP, "sftp", false, "Enable SFTP server")
	rootCmd.PersistentFlags().IntVar(&sftpPort, "sftp-port", 0, "SFTP port (default: 22)")
	rootCmd.PersistentFlags().BoolVar(&enableHTTP, "http", false, "Enable HTTP server")
	rootCmd.PersistentFlags().IntVar(&httpPort, "http-port", 0, "HTTP port (default: 80)")
	rootCmd.PersistentFlags().BoolVar(&enableHTTPS, "https", false, "Enable HTTPS server")
	rootCmd.PersistentFlags().IntVar(&httpsPort, "https-port", 0, "HTTPS port (default: 443)")
	rootCmd.PersistentFlags().BoolVar(&enableTFTP, "tftp", false, "Enable TFTP server")
	rootCmd.PersistentFlags().IntVar(&tftpPort, "tftp-port", 0, "TFTP port (default: 69)")
}

func runServer(cmd *cobra.Command, args []string) error {
	var err error

	// Get data directory from args or flag
	if len(args) > 0 {
		dataDir = args[0]
	}

	// Load configuration
	cfg, err = loadConfiguration()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override with CLI flags
	if err := applyCLIFlags(cfg); err != nil {
		return fmt.Errorf("failed to apply CLI flags: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create logger
	logger := utils.NewLogger(cfg.Logging.Level, cfg.Logging.Format)
	logger.Info("Starting FTP-AIO server...")
	logger.Info("Data directory: %s", cfg.Data)
	logger.Info("Users configured: %d", len(cfg.Users))

	// Create authenticator
	authenticator := auth.NewAuthenticator(cfg.Users)

	// Create file system
	fileSystem := fs.NewFileSystem(cfg.Data, authenticator)

	// Create server manager
	manager := server.NewManager(cfg, logger, authenticator, fileSystem)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Start servers
	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start servers: %w", err)
	}

	// Setup graceful shutdown
	utils.GracefulShutdown(ctx, cancel, logger, func() error {
		return manager.Stop()
	})

	return nil
}

func loadConfiguration() (*config.Config, error) {
	// Load from file first
	cfg, err := config.LoadFromFile(configFile)
	if err != nil {
		return nil, err
	}

	// Apply environment variables
	cfg.ApplyEnvironmentVariables()

	return cfg, nil
}

func applyCLIFlags(cfg *config.Config) error {
	// Data directory
	if dataDir != "" {
		cfg.Data = dataDir
	}

	// Users
	if userString != "" {
		users, err := config.ParseUserString(userString)
		if err != nil {
			return fmt.Errorf("failed to parse users: %w", err)
		}
		// Replace existing users with CLI users
		cfg.Users = users
	}

	// Logging
	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}

	// SSL certificates
	if certFile != "" {
		cfg.Services.FTPS.Cert = certFile
		cfg.Services.HTTPS.Cert = certFile
	}
	if keyFile != "" {
		cfg.Services.FTPS.Key = keyFile
		cfg.Services.HTTPS.Key = keyFile
	}

	// Protocol settings
	if enableFTP {
		cfg.Services.FTP.Enabled = true
		if ftpPort > 0 {
			cfg.Services.FTP.Port = ftpPort
		} else if cfg.Services.FTP.Port == 0 {
			cfg.Services.FTP.Port = config.DefaultFTPPort
		}
	}

	if enableFTPS {
		cfg.Services.FTPS.Enabled = true
		if ftpsPort > 0 {
			cfg.Services.FTPS.Port = ftpsPort
		} else if cfg.Services.FTPS.Port == 0 {
			cfg.Services.FTPS.Port = config.DefaultFTPSPort
		}
	}

	if enableSFTP {
		cfg.Services.SFTP.Enabled = true
		if sftpPort > 0 {
			cfg.Services.SFTP.Port = sftpPort
		} else if cfg.Services.SFTP.Port == 0 {
			cfg.Services.SFTP.Port = config.DefaultSFTPPort
		}
	}

	if enableHTTP {
		cfg.Services.HTTP.Enabled = true
		if httpPort > 0 {
			cfg.Services.HTTP.Port = httpPort
		} else if cfg.Services.HTTP.Port == 0 {
			cfg.Services.HTTP.Port = config.DefaultHTTPPort
		}
	}

	if enableHTTPS {
		cfg.Services.HTTPS.Enabled = true
		if httpsPort > 0 {
			cfg.Services.HTTPS.Port = httpsPort
		} else if cfg.Services.HTTPS.Port == 0 {
			cfg.Services.HTTPS.Port = config.DefaultHTTPSPort
		}
	}

	if enableTFTP {
		cfg.Services.TFTP.Enabled = true
		if tftpPort > 0 {
			cfg.Services.TFTP.Port = tftpPort
		} else if cfg.Services.TFTP.Port == 0 {
			cfg.Services.TFTP.Port = config.DefaultTFTPPort
		}
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
