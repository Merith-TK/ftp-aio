package config

// Default port numbers for different protocols
const (
	DefaultFTPPort   = 21
	DefaultFTPSPort  = 990
	DefaultSFTPPort  = 22
	DefaultHTTPPort  = 80
	DefaultHTTPSPort = 443
	DefaultTFTPPort  = 69
)

// Default configuration values
const (
	DefaultDataDir     = "./data"
	DefaultLogLevel    = "info"
	DefaultLogFormat   = "text"
	DefaultTLSHostname = "localhost"
	DefaultTLSOrgName  = "FTP-AIO"
	DefaultHTTPUpload  = true
	DefaultHTTPListing = true
)

// GetDefaultPortForProtocol returns the default port for a given protocol
func GetDefaultPortForProtocol(protocol string) int {
	switch protocol {
	case "ftp":
		return DefaultFTPPort
	case "ftps":
		return DefaultFTPSPort
	case "sftp":
		return DefaultSFTPPort
	case "http":
		return DefaultHTTPPort
	case "https":
		return DefaultHTTPSPort
	case "tftp":
		return DefaultTFTPPort
	default:
		return 0
	}
}
