// Package debug provides debug logging functionality.
package debug

import (
	"fmt"
	"os"
	"sync/atomic"
)

// enabled controls whether debug output is printed.
// Uses atomic.Bool for thread-safety when accessed from multiple goroutines.
var enabled atomic.Bool

// SetEnabled sets the debug mode state.
func SetEnabled(v bool) {
	enabled.Store(v)
}

// IsEnabled returns true if debug mode is enabled.
func IsEnabled() bool {
	return enabled.Load()
}

// Printf prints a debug message to stderr if debug mode is enabled.
// Format: [DEBUG] module: message
func Printf(module, format string, args ...interface{}) {
	if enabled.Load() {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[DEBUG] %s: %s\n", module, msg)
	}
}
