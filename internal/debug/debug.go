// Package debug provides debug logging functionality.
package debug

import (
	"fmt"
	"os"
)

// Enabled controls whether debug output is printed.
var Enabled bool

// Printf prints a debug message to stderr if debug mode is enabled.
// Format: [DEBUG] module: message
func Printf(module, format string, args ...interface{}) {
	if Enabled {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[DEBUG] %s: %s\n", module, msg)
	}
}
