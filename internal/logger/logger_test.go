package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInit_EmptyPath(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	err := Init("")
	if err != nil {
		t.Errorf("Init with empty path should not return error, got: %v", err)
	}
	if globalLogger != nil {
		t.Error("Init with empty path should leave globalLogger nil")
	}
}

func TestInit_NonExistentDirectory(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	err := Init("/nonexistent/directory/log.txt")
	if err == nil {
		t.Error("Init with non-existent directory should return error")
	}
	if !strings.Contains(err.Error(), "log directory does not exist") {
		t.Errorf("Error should mention directory, got: %v", err)
	}
}

func TestInit_ValidPath(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	err := Init(logPath)
	if err != nil {
		t.Errorf("Init with valid path should not return error, got: %v", err)
	}
	if globalLogger == nil {
		t.Error("Init with valid path should set globalLogger")
	}
	if globalLogger.path != logPath {
		t.Errorf("Logger path should be %s, got: %s", logPath, globalLogger.path)
	}
}

func TestInit_HomeExpansion(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home directory")
	}

	// Use a path that should exist
	testPath := "~/test-port-selector-log.txt"
	expectedPath := filepath.Join(home, "test-port-selector-log.txt")

	err = Init(testPath)
	if err != nil {
		t.Errorf("Init with ~ path should not return error, got: %v", err)
	}
	if globalLogger == nil {
		t.Fatal("globalLogger should not be nil")
	}
	if globalLogger.path != expectedPath {
		t.Errorf("Path should be expanded to %s, got: %s", expectedPath, globalLogger.path)
	}

	// Cleanup
	globalLogger = nil
	os.Remove(expectedPath)
}

func TestLog_WhenLoggerNil(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	// Should not panic
	Log(AllocAdd, Field("port", 3000))
}

func TestLog_WritesToFile(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	Log(AllocAdd, Field("port", 3000), Field("dir", "/test/dir"))

	// Read the log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logLine := string(content)

	// Check format
	if !strings.Contains(logLine, "ALLOC_ADD") {
		t.Errorf("Log should contain event type, got: %s", logLine)
	}
	if !strings.Contains(logLine, "port=3000") {
		t.Errorf("Log should contain port field, got: %s", logLine)
	}
	if !strings.Contains(logLine, "dir=/test/dir") {
		t.Errorf("Log should contain dir field, got: %s", logLine)
	}

	// Check timestamp format (RFC3339)
	if !strings.Contains(logLine, "Z ") && !strings.Contains(logLine, "+") {
		t.Errorf("Log should contain RFC3339 timestamp, got: %s", logLine)
	}
}

func TestLog_AppendsToFile(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	Log(AllocAdd, Field("port", 3000))
	Log(AllocDelete, Field("port", 3001))

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Errorf("Expected 2 log lines, got %d: %v", len(lines), lines)
	}
}

func TestField(t *testing.T) {
	tests := []struct {
		key      string
		value    interface{}
		expected string
	}{
		{"port", 3000, "port=3000"},
		{"dir", "/test/path", "dir=/test/path"},
		{"locked", true, "locked=true"},
		{"count", 5, "count=5"},
		{"dir", "/path with spaces", `dir="/path with spaces"`},
		{"msg", "hello\tworld", `msg="hello\tworld"`},
		{"text", "line1\nline2", `text="line1\nline2"`},
	}

	for _, tt := range tests {
		result := Field(tt.key, tt.value)
		if result != tt.expected {
			t.Errorf("Field(%q, %v) = %q, want %q", tt.key, tt.value, result, tt.expected)
		}
	}
}

func TestEventConstants(t *testing.T) {
	events := []string{
		AllocAdd,
		AllocUpdate,
		AllocLock,
		AllocDelete,
		AllocDeleteAll,
		AllocExpire,
	}

	for _, event := range events {
		if event == "" {
			t.Error("Event constant should not be empty")
		}
	}
}

func TestLog_TimestampFormat(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	Log(AllocAdd, Field("port", 3000))

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logLine := string(content)

	// Extract timestamp (first part before space)
	parts := strings.SplitN(logLine, " ", 2)
	if len(parts) < 2 {
		t.Fatalf("Invalid log line format: %s", logLine)
	}

	timestamp := parts[0]
	_, err = time.Parse(time.RFC3339, timestamp)
	if err != nil {
		t.Errorf("Timestamp should be RFC3339 format, got: %s, error: %v", timestamp, err)
	}
}

func TestLog_ConcurrentAccess(t *testing.T) {
	// Reset global logger
	globalLogger = nil

	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	err := Init(logPath)
	if err != nil {
		t.Fatalf("Failed to init logger: %v", err)
	}

	const goroutines = 10
	const logsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < logsPerGoroutine; j++ {
				Log(AllocAdd, Field("goroutine", id), Field("iteration", j))
			}
		}(i)
	}

	wg.Wait()

	// Verify all entries were written
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	expectedLines := goroutines * logsPerGoroutine
	if len(lines) != expectedLines {
		t.Errorf("Expected %d log lines, got %d", expectedLines, len(lines))
	}
}
