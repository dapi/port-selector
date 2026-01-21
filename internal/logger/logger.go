// Package logger provides structured logging for port-selector operations.
package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Event types for logging.
const (
	AllocAdd       = "ALLOC_ADD"
	AllocUpdate    = "ALLOC_UPDATE"
	AllocLock      = "ALLOC_LOCK"
	AllocDelete    = "ALLOC_DELETE"
	AllocDeleteAll = "ALLOC_DELETE_ALL"
	AllocExpire    = "ALLOC_EXPIRE"
	AllocExternal  = "ALLOC_EXTERNAL" // For registering external ports
	AllocRefresh   = "ALLOC_REFRESH"  // For refresh operations
)

// Logger handles writing events to a log file.
type Logger struct {
	path string
	mu   sync.Mutex
}

var (
	globalLogger *Logger
	globalMu     sync.Mutex
)

// Init initializes the global logger with the given path.
// If path is empty, logging is disabled.
func Init(path string) error {
	globalMu.Lock()
	defer globalMu.Unlock()

	if path == "" {
		globalLogger = nil
		return nil
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to expand home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Check if directory exists
	dir := filepath.Dir(path)
	if stat, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("log directory does not exist: %s", dir)
		}
		return fmt.Errorf("failed to stat log directory: %w", err)
	} else if !stat.IsDir() {
		return fmt.Errorf("log path parent is not a directory: %s", dir)
	}

	// Verify we can write to the log file
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot write to log file: %w", err)
	}
	f.Close()

	globalLogger = &Logger{path: path}
	return nil
}

// Log writes an event to the log file.
// If logger is not initialized, this is a no-op.
func Log(event string, fields ...string) {
	globalMu.Lock()
	logger := globalLogger
	globalMu.Unlock()

	if logger == nil {
		return
	}

	logger.log(event, fields...)
}

func (l *Logger) log(event string, fields ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestamp := time.Now().UTC().Format(time.RFC3339)
	line := fmt.Sprintf("%s %s", timestamp, event)

	if len(fields) > 0 {
		line += " " + strings.Join(fields, " ")
	}
	line += "\n"

	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to open log file: %v\n", err)
		return
	}
	defer f.Close()

	if _, err := f.WriteString(line); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write to log file: %v\n", err)
	}
}

// Field creates a key=value pair for logging.
// Values containing spaces, tabs, or newlines are automatically quoted.
func Field(key string, value interface{}) string {
	str := fmt.Sprintf("%v", value)
	if strings.ContainsAny(str, " \t\n") {
		return fmt.Sprintf("%s=%q", key, str)
	}
	return fmt.Sprintf("%s=%s", key, str)
}
