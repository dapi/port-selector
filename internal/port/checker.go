// Package port handles port availability checking.
package port

import (
	"errors"
	"fmt"
	"net"

	"github.com/dapi/port-selector/internal/debug"
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
	return FindFreePortWithExclusions(start, end, lastUsed, nil)
}

// FindFreePortWithExclusions finds the first available port excluding frozen ports.
// frozenPorts is a set of ports that should be skipped even if they're technically free.
func FindFreePortWithExclusions(start, end, lastUsed int, frozenPorts map[int]bool) (int, error) {
	// Determine starting point
	startFrom := start
	if lastUsed >= start && lastUsed < end {
		startFrom = lastUsed + 1
	}

	// If lastUsed was the last port in range, wrap to start
	if startFrom > end {
		startFrom = start
	}

	debug.Printf("port", "searching from %d to %d (wrap at %d)", startFrom, end, start)

	checked := 0

	// First pass: from startFrom to end
	for port := startFrom; port <= end; port++ {
		if frozenPorts != nil && frozenPorts[port] {
			debug.Printf("port", "port %d is frozen, skipping", port)
			continue // Skip frozen port
		}
		checked++
		if IsPortFree(port) {
			debug.Printf("port", "port %d is free (checked %d ports)", port, checked)
			return port, nil
		}
		debug.Printf("port", "port %d is busy", port)
	}

	// Second pass: from start to startFrom-1 (wrap-around)
	if startFrom > start {
		debug.Printf("port", "wrapping around to check ports %d to %d", start, startFrom-1)
		for port := start; port < startFrom; port++ {
			if frozenPorts != nil && frozenPorts[port] {
				debug.Printf("port", "port %d is frozen, skipping", port)
				continue // Skip frozen port
			}
			checked++
			if IsPortFree(port) {
				debug.Printf("port", "port %d is free (checked %d ports)", port, checked)
				return port, nil
			}
			debug.Printf("port", "port %d is busy", port)
		}
	}

	debug.Printf("port", "no free ports found after checking %d ports", checked)
	return 0, ErrAllPortsBusy
}
