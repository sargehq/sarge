// Package logging provides structured logging using slog.
// Logs are written to .sarge/debug.log in append mode.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	// LogFileName is the name of the debug log file.
	LogFileName = "debug.log"
)

var (
	// defaultLogger is the package-level logger.
	defaultLogger *slog.Logger
	// logFile is the file handle for the log file.
	logFile *os.File
	// mu protects concurrent access to the logger.
	mu sync.RWMutex
)

// Init initializes the logger with the project root path and config directory name.
// Logs are written to <projectRoot>/<configDir>/debug.log in append mode.
// If projectRoot is empty, logging is disabled (writes to io.Discard).
func Init(projectRoot, configDir string) error {
	mu.Lock()
	defer mu.Unlock()

	// Close any existing log file.
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}

	var w io.Writer
	if projectRoot == "" {
		// No project root - disable logging.
		w = io.Discard
	} else {
		logPath := filepath.Join(projectRoot, configDir, LogFileName)

		// Ensure the config directory exists.
		dir := filepath.Join(projectRoot, configDir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			// Fall back to discard if we can't create the directory.
			w = io.Discard
		} else {
			// Open the log file in append mode.
			f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				// Fall back to discard if we can't open the file.
				w = io.Discard
			} else {
				logFile = f
				w = f
			}
		}
	}

	// Create a JSON handler for structured logging.
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	defaultLogger = slog.New(handler)

	return nil
}

// Close closes the log file.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		err := logFile.Close()
		logFile = nil
		return err
	}
	return nil
}

// Logger returns the default logger.
// If not initialized, returns a no-op logger.
func Logger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()

	if defaultLogger == nil {
		return slog.New(slog.NewJSONHandler(io.Discard, nil))
	}
	return defaultLogger
}

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	Logger().Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	Logger().Info(msg, args...)
}

// Warn logs at warning level.
func Warn(msg string, args ...any) {
	Logger().Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	Logger().Error(msg, args...)
}

// DebugContext logs at debug level with context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	Logger().DebugContext(ctx, msg, args...)
}

// InfoContext logs at info level with context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	Logger().InfoContext(ctx, msg, args...)
}

// WarnContext logs at warning level with context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	Logger().WarnContext(ctx, msg, args...)
}

// ErrorContext logs at error level with context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	Logger().ErrorContext(ctx, msg, args...)
}

// With returns a logger with the given attributes.
func With(args ...any) *slog.Logger {
	return Logger().With(args...)
}

// WithGroup returns a logger with the given group.
func WithGroup(name string) *slog.Logger {
	return Logger().WithGroup(name)
}
