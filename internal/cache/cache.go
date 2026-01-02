// Package cache handles last-used port caching.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const lastUsedFileName = "last-used"

// GetLastUsed reads the last used port from cache.
// Returns 0 if cache doesn't exist or is invalid.
func GetLastUsed(configDir string) int {
	path := filepath.Join(configDir, lastUsedFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	port, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}

	if port <= 0 || port > 65535 {
		return 0
	}

	return port
}

// SetLastUsed saves the last used port to cache atomically.
func SetLastUsed(configDir string, port int) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, lastUsedFileName)
	tmpPath := path + ".tmp"

	// Write to temp file first
	data := []byte(strconv.Itoa(port))
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // cleanup on failure
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
