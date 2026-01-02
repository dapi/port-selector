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

// Allocation represents a single directory-to-port mapping.
type Allocation struct {
	Port       int       `yaml:"port"`
	Directory  string    `yaml:"directory"`
	AssignedAt time.Time `yaml:"assigned_at"`
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

// SortedByPort returns allocations sorted by port number (ascending).
func (l *AllocationList) SortedByPort() []Allocation {
	sorted := make([]Allocation, len(l.Allocations))
	copy(sorted, l.Allocations)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Port < sorted[j].Port
	})

	return sorted
}
