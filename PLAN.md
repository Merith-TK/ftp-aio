# FTP-AIO Project Implementation Plan

## Project Vision
Create a dead simple, all-in-one file transfer server supporting multiple protocols in a single Go binary. Focus on simplicity and ease of deployment - it should "just work" with minimal configuration.

## Supported Protocols

### Core Protocols (MVP)
- **FTP** - File Transfer Protocol (RFC 959)
- **FTPS** - FTP over SSL/TLS (Explicit and Implicit modes)
- **SFTP** - SSH File Transfer Protocol
- **HTTP/HTTPS** - Web file server with directory listing
- **TFTP** - Trivial File Transfer Protocol (RFC 1350)

### Future Protocols (Post-MVP)
- **WebDAV** - Web Distributed Authoring and Versioning
- **SCP** - Secure Copy Protocol (piggybacks on SFTP)

## Core Design Principles

1. **Dead Simple CLI**: One command to rule them all
2. **Sane Defaults**: Works out of the box with minimal config
3. **Single Binary**: No external dependencies
4. **Multiple Config Methods**: CLI args, env vars, or config file
5. **User-Friendly**: Clear error messages and helpful output

## Usage Examples

### Simple CLI Usage
```bash
# Start FTP server on default port (21) with one user
ftp-aio ./data --user="admin:password:1000:/:/rw" --ftp

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
export AIO_DATA_DIR="./data"
export AIO_USERS="admin:password:1000:/:rw"
export AIO_FTP="true"
export AIO_FTP_PORT="2121"
export AIO_SFTP="true"
export AIO_SFTP_PORT="2222"
ftp-aio
```

### Configuration File
```yaml
# config.yml
data: ./data

users:
  admin:
    pass: password
    uid: 1000
    path: /
    permissions: rw
  readonly:
    pass: readonly123
    uid: 1001
    path: /public
    permissions: ro

services:
  ftp:
    enabled: true
    port: 2121
  sftp:
    enabled: true
    port: 2222
  http:
    enabled: true
    port: 8080
    upload: true
  https:
    enabled: false
    port: 8443
    cert: # auto-generated if not specified
    key:
```

## Simplified Architecture

### Core Components
1. **CLI Parser** - Parse flags, env vars, and config files
2. **User Manager** - Handle user authentication and permissions
3. **Protocol Servers** - One server per protocol
4. **File System** - Simple local file operations with user isolation

## Simplified Project Structure

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

## Implementation Phases

### Phase 1: MVP Foundation (Week 1)
**Goal: Get basic FTP working with simple CLI**

- [ ] Project structure setup
- [ ] CLI argument parsing (cobra)
- [ ] User string parsing (`user:pass:uid:path:permissions`)
- [ ] Basic FTP server implementation
- [ ] File system operations with user isolation
- [ ] Simple logging

**Deliverables:**
- Working FTP server
- CLI with `--ftp`, `--ftp-port`, `--user` flags
- Basic user authentication

### Phase 2: Core Protocols (Week 2)
**Goal: Add SFTP and HTTP support**

- [ ] SFTP server implementation
- [ ] HTTP file server with upload/download
- [ ] Unified user authentication across protocols
- [ ] Configuration file support (YAML)
- [ ] Environment variable support

**Deliverables:**
- FTP, SFTP, and HTTP servers working
- Configuration file parsing
- Environment variable support

### Phase 3: Secure Protocols (Week 3)
**Goal: Add FTPS and HTTPS**

- [ ] FTPS implementation (explicit and implicit)
- [ ] HTTPS implementation
- [ ] Auto-generated SSL certificates
- [ ] TFTP server
- [ ] Better error handling and logging

**Deliverables:**
- Secure protocol variants
- Auto-SSL certificate generation
- TFTP support

### Phase 4: Polish & Deployment (Week 4)
**Goal: Production ready**

- [ ] Docker container
- [ ] Multi-architecture builds
- [ ] Better documentation
- [ ] Example configurations
- [ ] Testing and bug fixes

**Deliverables:**
- Production-ready binary
- Docker container
- Complete documentation

## CLI Flags and Environment Variables

### Protocol Flags
```bash
--ftp                   # Enable FTP (AIO_FTP=true)
--ftp-port=21          # FTP port (AIO_FTP_PORT=21)
--ftps                 # Enable FTPS (AIO_FTPS=true)  
--ftps-port=990        # FTPS port (AIO_FTPS_PORT=990)
--sftp                 # Enable SFTP (AIO_SFTP=true)
--sftp-port=22         # SFTP port (AIO_SFTP_PORT=22)
--http                 # Enable HTTP (AIO_HTTP=true)
--http-port=80         # HTTP port (AIO_HTTP_PORT=80)
--https                # Enable HTTPS (AIO_HTTPS=true)
--https-port=443       # HTTPS port (AIO_HTTPS_PORT=443)
--tftp                 # Enable TFTP (AIO_TFTP=true)
--tftp-port=69         # TFTP port (AIO_TFTP_PORT=69)
```

### General Flags
```bash
--config=config.yml    # Config file path (AIO_CONFIG=config.yml)
--data=./data          # Data directory (AIO_DATA=./data)
--user="user:pass:uid:path:perm,..."  # Users (AIO_USERS="...")
--cert=/path/to/cert   # SSL certificate (AIO_CERT=/path/to/cert)
--key=/path/to/key     # SSL key (AIO_KEY=/path/to/key)
--log-level=info       # Log level (AIO_LOG_LEVEL=info)
```

### User Format
```
username:password:uid:path:permissions
```

- `username` - Login username
- `password` - Plain text password
- `uid` - Unix user ID for file ownership
- `path` - User's home directory (relative to data dir)
- `permissions` - `ro` (read-only) or `rw` (read-write)

Examples:
```bash
--user="admin:secret:1000:/:/rw"
--user="guest:guest123:1001:/public:ro"
--user="admin:pass:1000:/:/rw,guest:guest:1001:/public:ro"
```

## Configuration Schema (Simplified)

### YAML Configuration File
```yaml
# config.yml - all fields optional with sane defaults
data: ./data  # Data directory (default: ./data)

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
    cert:
    key:
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

## Dependencies (Minimal)

### Core Dependencies
```go
// CLI and Configuration
github.com/spf13/cobra          // CLI framework
gopkg.in/yaml.v3                // YAML parsing

// FTP/FTPS
github.com/goftp/server         // FTP server library

// SFTP/SSH
golang.org/x/crypto/ssh         // SSH server
github.com/pkg/sftp             // SFTP implementation

// HTTP/HTTPS
net/http                        // Standard library HTTP (no external deps)

// TFTP
github.com/pin/tftp             // TFTP implementation

// TLS/Certificates
crypto/tls                      // Standard library TLS
crypto/x509                     // Certificate generation

// Logging
log/slog                        // Standard library structured logging (Go 1.21+)
```

## File Structure Examples

### Data Directory Layout
```
./data/                         # Root data directory
├── admin/                      # User "admin" home directory
│   ├── documents/
│   ├── uploads/
│   └── private/
├── guest/                      # User "guest" home directory (read-only)
│   └── public/
│       ├── files/
│       └── downloads/
└── shared/                     # Shared directory
    ├── public/
    └── common/
```

### Generated Certificate Structure
```
./certs/                        # Auto-generated if not specified
├── server.crt                  # Server certificate
├── server.key                  # Server private key
└── ca.crt                      # CA certificate (if needed)
```

## Success Criteria (Simplified)

### MVP Requirements
1. **Single Binary**: One executable that "just works"
2. **FTP Support**: Basic FTP server with user authentication
3. **Multi-Protocol**: At least FTP, SFTP, and HTTP working
4. **Simple CLI**: Dead simple command-line interface
5. **Configuration**: CLI args, env vars, and config file support
6. **User Management**: Simple user:pass:uid:path:perm format

### Production Requirements
1. **Security**: FTPS and HTTPS with auto-generated certificates
2. **Reliability**: Handles errors gracefully, doesn't crash
3. **Docker Ready**: Works in containers out of the box
4. **Documentation**: Clear examples and usage instructions
5. **Cross-Platform**: Works on Linux, macOS, Windows

### Future Enhancements
1. **WebDAV Support**: For more advanced file management
2. **Web UI**: Simple web interface for administration
3. **Better Security**: Rate limiting, IP filtering
4. **Performance**: Optimizations for high-load scenarios

This simplified plan focuses on creating a truly simple, easy-to-use FTP server that prioritizes usability over complexity. The goal is to have something that works perfectly for the 80% use case while keeping the door open for future enhancements.
