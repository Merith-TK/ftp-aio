package utils

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// GracefulShutdown handles graceful shutdown of the application
func GracefulShutdown(ctx context.Context, cancel context.CancelFunc, logger *Logger, shutdownFn func() error) {
	// Create a channel to receive OS signals
	sigChan := make(chan os.Signal, 1)

	// Register the channel to receive specific signals
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Wait for signal
	sig := <-sigChan
	logger.Info("Received signal %s, initiating graceful shutdown...", sig)

	// Cancel the context to signal all goroutines to stop
	cancel()

	// Create a timeout context for shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Channel to signal shutdown completion
	done := make(chan error, 1)

	// Run shutdown function in a goroutine
	go func() {
		if shutdownFn != nil {
			done <- shutdownFn()
		} else {
			done <- nil
		}
	}()

	// Wait for shutdown to complete or timeout
	select {
	case err := <-done:
		if err != nil {
			logger.Error("Error during shutdown: %v", err)
		} else {
			logger.Info("Graceful shutdown completed")
		}
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout exceeded, forcing exit")
	}
}
