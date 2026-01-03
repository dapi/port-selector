//go:build windows

package allocations

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dapi/port-selector/internal/debug"
)

var (
	windowsWarningOnce sync.Once
)

// openAndLock opens the allocations file.
// Note: On Windows, file locking is not implemented. Concurrent access
// from multiple processes may cause data corruption.
func openAndLock(configDir string) (*file, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, allocationsFileName)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open allocations file: %w", err)
	}

	// Warn user once per process about missing file locking on Windows
	windowsWarningOnce.Do(func() {
		fmt.Fprintln(os.Stderr, "warning: file locking not available on Windows, concurrent access may cause data corruption")
	})

	debug.Printf("allocations", "opened %s (no locking on Windows)", path)
	return &file{path: path, f: f}, nil
}

// unlock closes the file.
func (fl *file) unlock() {
	if fl.f != nil {
		if err := fl.f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close %s: %v\n", fl.path, err)
		}
		debug.Printf("allocations", "closed %s", fl.path)
	}
}
