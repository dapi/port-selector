// Package port handles port availability checking.
package port

import (
	"errors"
	"fmt"
	"net"
)

// ErrAllPortsBusy is returned when all ports in the range are busy.
var ErrAllPortsBusy = errors.New("all ports in range are busy")

// IsPortFree checks if a port is available for binding.
func IsPortFree(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// FindFreePort finds the first available port in the given range.
// It starts searching from lastUsed+1 and wraps around to start if needed.
// Returns ErrAllPortsBusy if no ports are available.
func FindFreePort(start, end, lastUsed int) (int, error) {
	// Determine starting point
	startFrom := start
	if lastUsed >= start && lastUsed < end {
		startFrom = lastUsed + 1
	}

	// If lastUsed was the last port in range, wrap to start
	if startFrom > end {
		startFrom = start
	}

	// First pass: from startFrom to end
	for port := startFrom; port <= end; port++ {
		if IsPortFree(port) {
			return port, nil
		}
	}

	// Second pass: from start to startFrom-1 (wrap-around)
	if startFrom > start {
		for port := start; port < startFrom; port++ {
			if IsPortFree(port) {
				return port, nil
			}
		}
	}

	return 0, ErrAllPortsBusy
}
