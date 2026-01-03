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
	// Allocation events
	AllocAdd       = "ALLOC_ADD"
	AllocUpdate    = "ALLOC_UPDATE"
	AllocLock      = "ALLOC_LOCK"
	AllocDelete    = "ALLOC_DELETE"
	AllocDeleteAll = "ALLOC_DELETE_ALL"
	AllocExpire    = "ALLOC_EXPIRE"

	// History events
	HistoryAdd     = "HISTORY_ADD"
	HistoryCleanup = "HISTORY_CLEANUP"

	// Cache events
	CacheUpdate = "CACHE_UPDATE"
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
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("log directory does not exist: %s", dir)
	}

	globalLogger = &Logger{path: path}
	return nil
}

// Log writes an event to the log file.
// If logger is not initialized or path is empty, this is a no-op.
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
func Field(key string, value interface{}) string {
	return fmt.Sprintf("%s=%v", key, value)
}
