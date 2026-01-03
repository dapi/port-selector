//go:build unix

package allocations

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/dapi/port-selector/internal/debug"
)

// openAndLock opens the allocations file and acquires an exclusive lock.
func openAndLock(configDir string) (*file, error) {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, allocationsFileName)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open allocations file: %w", err)
	}

	// Acquire exclusive lock (blocking)
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	debug.Printf("allocations", "acquired lock on %s", path)
	return &file{path: path, f: f}, nil
}

// unlock releases the lock and closes the file.
func (fl *file) unlock() {
	if fl.f != nil {
		if err := syscall.Flock(int(fl.f.Fd()), syscall.LOCK_UN); err != nil {
			debug.Printf("allocations", "warning: failed to release lock on %s: %v", fl.path, err)
		}
		if err := fl.f.Close(); err != nil {
			debug.Printf("allocations", "warning: failed to close %s: %v", fl.path, err)
		}
		debug.Printf("allocations", "released lock on %s", fl.path)
	}
}
