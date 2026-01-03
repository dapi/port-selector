package debug

import (
	"bytes"
	"os"
	"sync"
	"testing"
)

func TestSetEnabledAndIsEnabled(t *testing.T) {
	// Reset to known state
	SetEnabled(false)

	if IsEnabled() {
		t.Error("expected IsEnabled() to return false initially")
	}

	SetEnabled(true)
	if !IsEnabled() {
		t.Error("expected IsEnabled() to return true after SetEnabled(true)")
	}

	SetEnabled(false)
	if IsEnabled() {
		t.Error("expected IsEnabled() to return false after SetEnabled(false)")
	}
}

func TestPrintfWhenEnabled(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	SetEnabled(true)
	Printf("test", "hello %s", "world")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	expected := "[DEBUG] test: hello world\n"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestPrintfWhenDisabled(t *testing.T) {
	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	SetEnabled(false)
	Printf("test", "should not appear")

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("expected empty output when disabled, got %q", output)
	}
}

func TestThreadSafety(t *testing.T) {
	// Test concurrent access to SetEnabled and IsEnabled
	var wg sync.WaitGroup
	iterations := 1000

	// Multiple goroutines toggling enabled state
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				SetEnabled(j%2 == 0)
				_ = IsEnabled()
			}
		}(i)
	}

	// Multiple goroutines calling Printf
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				Printf("goroutine", "message %d from %d", j, id)
			}
		}(i)
	}

	wg.Wait()
	// If we get here without a data race, the test passes
	// Run with: go test -race ./internal/debug/...
}

func TestPrintfFormatting(t *testing.T) {
	tests := []struct {
		name     string
		module   string
		format   string
		args     []interface{}
		expected string
	}{
		{
			name:     "simple message",
			module:   "main",
			format:   "starting",
			args:     nil,
			expected: "[DEBUG] main: starting\n",
		},
		{
			name:     "with string arg",
			module:   "config",
			format:   "loading from %s",
			args:     []interface{}{"/path/to/file"},
			expected: "[DEBUG] config: loading from /path/to/file\n",
		},
		{
			name:     "with int arg",
			module:   "allocations",
			format:   "loaded %d allocations",
			args:     []interface{}{5},
			expected: "[DEBUG] allocations: loaded 5 allocations\n",
		},
		{
			name:     "with multiple args",
			module:   "port",
			format:   "checking port %d, pid=%d",
			args:     []interface{}{3000, 12345},
			expected: "[DEBUG] port: checking port 3000, pid=12345\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			SetEnabled(true)
			Printf(tt.module, tt.format, tt.args...)

			w.Close()
			os.Stderr = oldStderr

			var buf bytes.Buffer
			buf.ReadFrom(r)
			output := buf.String()

			if output != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, output)
			}
		})
	}
}
