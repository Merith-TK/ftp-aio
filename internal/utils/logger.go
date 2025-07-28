package utils

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel represents different log levels
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// Logger provides simple logging functionality
type Logger struct {
	level  LogLevel
	format string
	logger *log.Logger
}

// NewLogger creates a new logger with the specified level and format
func NewLogger(level, format string) *Logger {
	logLevel := parseLogLevel(level)

	logger := &Logger{
		level:  logLevel,
		format: format,
		logger: log.New(os.Stdout, "", 0),
	}

	return logger
}

// parseLogLevel converts a string log level to LogLevel enum
func parseLogLevel(level string) LogLevel {
	switch strings.ToLower(level) {
	case "debug":
		return DEBUG
	case "info":
		return INFO
	case "warn", "warning":
		return WARN
	case "error":
		return ERROR
	default:
		return INFO
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level <= DEBUG {
		l.log("DEBUG", format, args...)
	}
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	if l.level <= INFO {
		l.log("INFO", format, args...)
	}
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level <= WARN {
		l.log("WARN", format, args...)
	}
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	if l.level <= ERROR {
		l.log("ERROR", format, args...)
	}
}

// log formats and prints a log message
func (l *Logger) log(level, format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var output string
	if l.format == "json" {
		// Simple JSON-like format
		output = fmt.Sprintf(`{"time":"%s","level":"%s","message":"%s"}`, timestamp, level, message)
	} else {
		// Simple text format
		output = fmt.Sprintf("[%s] %s: %s", timestamp, level, message)
	}

	l.logger.Println(output)
}
