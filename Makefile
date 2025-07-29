# FTP-AIO Makefile

.PHONY: build clean run test install deps

# Build the binary
build:
	go build -o ftp-aio cmd/ftp-aio/main.go

# Clean build artifacts
clean:
	rm -f ftp-aio

# Run with example configuration
run: build
	./ftp-aio --config=configs/example.yml

# Run tests
test:
	go test -v ./...

# Install dependencies
deps:
	go mod download
	go mod tidy

# Install binary to GOPATH/bin
install:
	go install cmd/ftp-aio/main.go

# Development run with FTP enabled
dev: build
	./ftp-aio ./data --user="admin:password:1000:/:rw,guest:guest:1001:/public:ro" --ftp --ftp-port=2121 --log-level=debug

# Show help
help:
	@echo "Available targets:"
	@echo "  build    - Build the ftp-aio binary"
	@echo "  clean    - Clean build artifacts"
	@echo "  run      - Run with example configuration"
	@echo "  dev      - Run in development mode with FTP enabled"
	@echo "  test     - Run tests"
	@echo "  deps     - Install and tidy dependencies"
	@echo "  install  - Install binary to GOPATH/bin"
	@echo "  help     - Show this help message"
