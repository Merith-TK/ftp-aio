# FTP-AIO - All-in-One File Transfer Server

A dead simple, all-in-one file transfer server supporting multiple protocols in a single Go binary. Designed to "just work" with minimal configuration.

## Project Overview

This project creates a single binary that serves multiple file transfer protocols with the simplest possible configuration. Perfect for quick deployments, Docker containers, and situations where you just need a file server that works.

## Supported Protocols

### Core Protocols (MVP)
- **FTP** - File Transfer Protocol (RFC 959)
- **FTPS** - FTP over SSL/TLS (Explicit and Implicit modes)
- **SFTP** - SSH File Transfer Protocol
- **HTTP/HTTPS** - Web file server with directory listing
- **TFTP** - Trivial File Transfer Protocol (RFC 1350)

### Future Protocols
- **WebDAV** - Web Distributed Authoring and Versioning
- **SCP** - Secure Copy Protocol (piggybacks on SFTP)

## Usage Examples

### Dead Simple CLI
```bash
# Start FTP server with one user
ftp-aio ./data --user="admin:password:1000:/:rw" --ftp

# Multiple protocols with custom ports
ftp-aio ./data \
  --user="user:pass:1000:/folder/in/data/:rw,user2:pass2:1001:/otherfolder/in/data/:ro" \
  --ftp --ftp-port=2121 \
  --sftp --sftp-port=2222 \
  --http --http-port=8080

# HTTPS with auto-generated certificates
ftp-aio ./data --user="admin:pass:1000:/:rw" --https --https-port=8443
```

### Environment Variables
```bash
export AIO_DATA="./data"
export AIO_USERS="admin:password:1000:/:rw"
export AIO_FTP="true"
export AIO_FTP_PORT="2121"
export AIO_SFTP="true" 
export AIO_SFTP_PORT="2222"
ftp-aio
```

### User Format
```
username:password:uid:path:permissions
```

- **username** - Login username
- **password** - Plain text password  
- **uid** - Unix user ID for file ownership
- **path** - User's home directory (relative to data dir)
- **permissions** - `ro` (read-only) or `rw` (read-write)

Multiple users: `--user="user1:pass1:1000:/:rw,user2:pass2:1001:/public:ro"`

## Core Design Principles

1. **Dead Simple CLI**: One command to rule them all
2. **Sane Defaults**: Works out of the box with minimal config  
3. **Single Binary**: No external dependencies
4. **Multiple Config Methods**: CLI args, env vars, or config file
5. **User-Friendly**: Clear error messages and helpful output

## Project Structure (Simplified)

```
ftp-aio/
├── cmd/
│   └── ftp-aio/
│       └── main.go                 # Main entry point with CLI
├── internal/
│   ├── config/
│   │   ├── config.go              # Configuration struct and parsing
│   │   ├── users.go               # User parsing and validation
│   │   └── defaults.go            # Default values
│   ├── server/
│   │   ├── manager.go             # Server lifecycle management
│   │   ├── ftp.go                 # FTP server
│   │   ├── ftps.go                # FTPS server
│   │   ├── sftp.go                # SFTP server
│   │   ├── http.go                # HTTP file server
│   │   ├── https.go               # HTTPS file server
│   │   └── tftp.go                # TFTP server
│   ├── auth/
│   │   ├── auth.go                # Simple user authentication
│   │   └── permissions.go         # Permission checking (rw/ro)
│   ├── fs/
│   │   ├── fs.go                  # File system operations
│   │   └── chroot.go              # User path isolation
│   └── utils/
│       ├── logger.go              # Simple logging
│       ├── certs.go               # Auto-generate SSL certificates
│       └── signals.go             # Graceful shutdown
├── configs/
│   └── example.yml                # Example configuration file
├── go.mod
├── go.sum
├── Makefile                       # Simple build commands
└── README.md
```

## Configuration Options

### Command Line Flags
```bash
# Protocol flags
--ftp                   # Enable FTP (default port 21)
--ftp-port=2121        # Custom FTP port
--ftps                 # Enable FTPS (default port 990)
--ftps-port=2990       # Custom FTPS port
--sftp                 # Enable SFTP (default port 22)
--sftp-port=2222       # Custom SFTP port
--http                 # Enable HTTP (default port 80)
--http-port=8080       # Custom HTTP port
--https                # Enable HTTPS (default port 443)
--https-port=8443      # Custom HTTPS port
--tftp                 # Enable TFTP (default port 69)
--tftp-port=6969       # Custom TFTP port

# General flags
--config=config.yml    # Config file path
--data=./data          # Data directory
--user="user:pass:uid:path:perm,..." # Users
--cert=/path/to/cert   # SSL certificate
--key=/path/to/key     # SSL key
--log-level=info       # Log level (debug, info, warn, error)
```

### Environment Variables
All CLI flags have corresponding environment variables with `AIO_` prefix:
```bash
AIO_FTP=true
AIO_FTP_PORT=2121
AIO_SFTP=true
AIO_SFTP_PORT=2222
AIO_DATA="./data"
AIO_USERS="admin:password:1000:/:rw"
AIO_LOG_LEVEL=info
```

### Configuration File (YAML)
```yaml
# config.yml - all fields optional with sane defaults
data: ./data  # Data directory

users:
  admin:
    pass: password123
    uid: 1000
    path: /              # relative to data dir
    permissions: rw      # rw or ro
  guest:
    pass: guest123
    uid: 1001
    path: /public
    permissions: ro

services:
  ftp:
    enabled: true
    port: 21
  ftps:
    enabled: false
    port: 990
    cert:                # auto-generated if empty
    key:                 # auto-generated if empty
  sftp:
    enabled: false
    port: 22
    host_key:            # auto-generated if empty
  http:
    enabled: false
    port: 80
    upload: true         # allow uploads
    listing: true        # show directory listings
  https:
    enabled: false
    port: 443
    cert:                # auto-generated if empty
    key:                 # auto-generated if empty
  tftp:
    enabled: false
    port: 69

# Optional settings
logging:
  level: info            # debug, info, warn, error
  format: text           # text or json

# TLS settings (for auto-generated certs)
tls:
  hostname: localhost    # used for auto-generated certs
  organization: FTP-AIO
```

## Development Roadmap

### Phase 1: MVP Foundation (Week 1)
- [x] Project structure setup
- [ ] CLI argument parsing
- [ ] User string parsing (`user:pass:uid:path:permissions`)
- [ ] Basic FTP server implementation
- [ ] File system operations with user isolation
- [ ] Simple logging

### Phase 2: Core Protocols (Week 2)
- [ ] SFTP server implementation
- [ ] HTTP file server with upload/download
- [ ] Configuration file support (YAML)
- [ ] Environment variable support

### Phase 3: Secure Protocols (Week 3)
- [ ] FTPS implementation (explicit and implicit)
- [ ] HTTPS implementation
- [ ] Auto-generated SSL certificates
- [ ] TFTP server

### Phase 4: Polish & Deployment (Week 4)
- [ ] Docker container
- [ ] Multi-architecture builds
- [ ] Documentation and examples
- [ ] Testing and bug fixes

## Dependencies (Minimal)

- `github.com/spf13/cobra` - CLI framework
- `gopkg.in/yaml.v3` - YAML parsing
- `github.com/goftp/server` - FTP server library
- `golang.org/x/crypto/ssh` - SSH server
- `github.com/pkg/sftp` - SFTP implementation
- `github.com/pin/tftp` - TFTP implementation
- Standard library for HTTP/HTTPS, TLS, logging

## Goals

1. **Dead Simple**: One command that just works
2. **Single Binary**: No external dependencies
3. **Multi-Protocol**: FTP, FTPS, SFTP, HTTP/HTTPS, TFTP support
4. **Docker Ready**: Perfect for containerized deployment
5. **Auto-SSL**: Automatic certificate generation for HTTPS/FTPS
6. **User-Friendly**: Clear CLI and helpful error messages

## Quick Start

```bash
# Clone and build
git clone https://github.com/Merith-TK/ftp-aio.git
cd ftp-aio
go build -o ftp-aio cmd/ftp-aio/main.go

# Run with FTP and HTTP
./ftp-aio ./data --user="admin:secret:1000:/:rw" --ftp --http --http-port=8080

# Use config file
./ftp-aio --config=config.yml
```

## License

[To be determined]

## Contributing

[To be determined]
