// Package pathutil provides utilities for path manipulation.
package pathutil

import (
	"os"
	"strings"
)

// ShortenHomePath replaces the user's home directory with ~ in the given path.
// If the path doesn't start with the home directory, it's returned unchanged.
func ShortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}

	// Exact match
	if path == home {
		return "~"
	}

	// Path starts with home directory
	if strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}

	return path
}
