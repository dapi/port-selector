// Package allocations handles directory-to-port mapping persistence.
package allocations

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

const allocationsFileName = "allocations.yaml"

// UnknownDirectory is used when a port is busy but we can't determine
// which directory/process owns it (e.g., root-owned processes like docker-proxy).
const UnknownDirectory = "(unknown)"

// Allocation represents a single directory-to-port mapping.
type Allocation struct {
	Port       int       `yaml:"port"`
	Directory  string    `yaml:"directory"`
	AssignedAt time.Time `yaml:"assigned_at"`
	Locked     bool      `yaml:"locked,omitempty"`
}

// AllocationList is the root structure for the allocations file.
type AllocationList struct {
	Allocations []Allocation `yaml:"allocations"`
}

// Load reads allocations from the config directory.
// Returns empty list if file doesn't exist.
// Logs warning and returns empty list if file is corrupted.
func Load(configDir string) *AllocationList {
	path := filepath.Join(configDir, allocationsFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "warning: cannot read allocations file: %v\n", err)
		}
		return &AllocationList{}
	}

	var list AllocationList
	if err := yaml.Unmarshal(data, &list); err != nil {
		fmt.Fprintf(os.Stderr, "warning: allocations file corrupted, starting fresh: %v\n", err)
		return &AllocationList{}
	}

	// Normalize all directory paths loaded from YAML
	for i := range list.Allocations {
		list.Allocations[i].Directory = filepath.Clean(list.Allocations[i].Directory)
	}

	return &list
}

// Save writes allocations to the config directory atomically.
func Save(configDir string, list *AllocationList) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, allocationsFileName)
	tmpPath := path + ".tmp"

	data, err := yaml.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal allocations: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// FindByDirectory returns the allocation for a given directory, or nil if not found.
// Directory path is normalized before comparison.
func (l *AllocationList) FindByDirectory(dir string) *Allocation {
	dir = filepath.Clean(dir)
	for i := range l.Allocations {
		if l.Allocations[i].Directory == dir {
			return &l.Allocations[i]
		}
	}
	return nil
}

// FindByPort returns the allocation for a given port, or nil if not found.
func (l *AllocationList) FindByPort(port int) *Allocation {
	for i := range l.Allocations {
		if l.Allocations[i].Port == port {
			return &l.Allocations[i]
		}
	}
	return nil
}

// SetAllocation adds or updates an allocation for a directory.
// Directory path is normalized before storing.
func (l *AllocationList) SetAllocation(dir string, port int) {
	dir = filepath.Clean(dir)
	now := time.Now().UTC()

	// Check if directory already has an allocation
	for i := range l.Allocations {
		if l.Allocations[i].Directory == dir {
			l.Allocations[i].Port = port
			l.Allocations[i].AssignedAt = now
			return
		}
	}

	// Add new allocation
	l.Allocations = append(l.Allocations, Allocation{
		Port:       port,
		Directory:  dir,
		AssignedAt: now,
	})
}

// SetUnknownPortAllocation adds an allocation for a busy port with unknown ownership.
// Each unknown port gets a unique directory marker like "(unknown:3007)".
// This prevents multiple unknown ports from overwriting each other.
// Note: Caller should check FindByPort() first to avoid duplicates.
func (l *AllocationList) SetUnknownPortAllocation(port int) {
	now := time.Now().UTC()
	dir := fmt.Sprintf("(unknown:%d)", port)

	l.Allocations = append(l.Allocations, Allocation{
		Port:       port,
		Directory:  dir,
		AssignedAt: now,
	})
}

// SortedByPort returns allocations sorted by port number (ascending).
func (l *AllocationList) SortedByPort() []Allocation {
	sorted := make([]Allocation, len(l.Allocations))
	copy(sorted, l.Allocations)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Port < sorted[j].Port
	})

	return sorted
}

// RemoveByDirectory removes the allocation for a given directory.
// Returns the removed allocation and true if found, nil and false otherwise.
// Directory path is normalized before comparison.
func (l *AllocationList) RemoveByDirectory(dir string) (*Allocation, bool) {
	dir = filepath.Clean(dir)
	for i := range l.Allocations {
		if l.Allocations[i].Directory == dir {
			removed := l.Allocations[i]
			l.Allocations = append(l.Allocations[:i], l.Allocations[i+1:]...)
			return &removed, true
		}
	}
	return nil, false
}

// RemoveAll clears all allocations and returns the count of removed items.
func (l *AllocationList) RemoveAll() int {
	count := len(l.Allocations)
	l.Allocations = nil
	return count
}

// RemoveExpired removes allocations older than the given TTL.
// Returns the count of removed items.
func (l *AllocationList) RemoveExpired(ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}

	cutoff := time.Now().Add(-ttl)
	count := 0
	kept := make([]Allocation, 0, len(l.Allocations))

	for _, alloc := range l.Allocations {
		if alloc.AssignedAt.After(cutoff) {
			kept = append(kept, alloc)
		} else {
			count++
		}
	}

	l.Allocations = kept
	return count
}

// UpdateLastUsed updates the AssignedAt timestamp for a given directory to now.
// Returns true if allocation was found and updated.
func (l *AllocationList) UpdateLastUsed(dir string) bool {
	dir = filepath.Clean(dir)
	for i := range l.Allocations {
		if l.Allocations[i].Directory == dir {
			l.Allocations[i].AssignedAt = time.Now().UTC()
			return true
		}
	}
	return false
}

// SetLocked sets the locked status for an allocation identified by directory.
// Returns true if allocation was found and updated.
func (l *AllocationList) SetLocked(dir string, locked bool) bool {
	dir = filepath.Clean(dir)
	for i := range l.Allocations {
		if l.Allocations[i].Directory == dir {
			l.Allocations[i].Locked = locked
			return true
		}
	}
	return false
}

// SetLockedByPort sets the locked status for an allocation identified by port.
// Returns true if allocation was found and updated.
func (l *AllocationList) SetLockedByPort(port int, locked bool) bool {
	for i := range l.Allocations {
		if l.Allocations[i].Port == port {
			l.Allocations[i].Locked = locked
			return true
		}
	}
	return false
}

// IsPortLocked checks if a port is locked by another directory.
// Returns true if the port is allocated to a different directory and is locked.
// Directory path is normalized before comparison.
func (l *AllocationList) IsPortLocked(port int, currentDir string) bool {
	currentDir = filepath.Clean(currentDir)
	for i := range l.Allocations {
		if l.Allocations[i].Port == port {
			// Port belongs to current directory - not considered locked for this directory
			if l.Allocations[i].Directory == currentDir {
				return false
			}
			// Port belongs to another directory - check if it's locked
			return l.Allocations[i].Locked
		}
	}
	return false
}

// GetLockedPortsForExclusion returns a map of ports that are locked by directories
// other than the current one. These ports should be excluded during port allocation.
// Directory path is normalized before comparison.
func (l *AllocationList) GetLockedPortsForExclusion(currentDir string) map[int]bool {
	currentDir = filepath.Clean(currentDir)
	locked := make(map[int]bool)
	for _, alloc := range l.Allocations {
		if alloc.Locked && alloc.Directory != currentDir {
			locked[alloc.Port] = true
		}
	}
	return locked
}
