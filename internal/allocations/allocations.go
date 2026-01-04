// Package allocations handles port allocation persistence with file locking.
// It manages directory-to-port mappings, freeze periods, and last-used port tracking
// in a single file with flock-based locking to prevent race conditions.
package allocations

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dapi/port-selector/internal/debug"
	"github.com/dapi/port-selector/internal/logger"
	"gopkg.in/yaml.v3"
)

const allocationsFileName = "allocations.yaml"

// AllocationInfo represents a single port allocation entry.
type AllocationInfo struct {
	Directory   string    `yaml:"directory"`
	AssignedAt  time.Time `yaml:"assigned_at"`
	LastUsedAt  time.Time `yaml:"last_used_at,omitempty"`
	Locked      bool      `yaml:"locked,omitempty"`
	ProcessName string    `yaml:"process_name,omitempty"`
	ContainerID string    `yaml:"container_id,omitempty"`
}

// Store is the root structure for the allocations file.
// Allocations uses port number as key to guarantee uniqueness.
type Store struct {
	LastIssuedPort int                     `yaml:"last_issued_port,omitempty"`
	Allocations    map[int]*AllocationInfo `yaml:"allocations"`
}

// file holds the opened file handle for locking.
type file struct {
	path string
	f    *os.File
}

// Allocation represents a single port allocation (for external use).
type Allocation struct {
	Port        int
	Directory   string
	AssignedAt  time.Time
	LastUsedAt  time.Time
	Locked      bool
	ProcessName string
	ContainerID string
}

// NewStore creates an empty store.
func NewStore() *Store {
	return &Store{
		Allocations: make(map[int]*AllocationInfo),
	}
}

// read reads the store from the locked file.
func (fl *file) read() (*Store, error) {
	// Seek to beginning
	if _, err := fl.f.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("failed to seek: %w", err)
	}

	stat, err := fl.f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// Empty file - return new store
	if stat.Size() == 0 {
		debug.Printf("allocations", "file is empty, returning new store")
		return NewStore(), nil
	}

	data := make([]byte, stat.Size())
	n, err := fl.f.Read(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	data = data[:n]

	var store Store
	if err := yaml.Unmarshal(data, &store); err != nil {
		debug.Printf("allocations", "YAML parse error: %v", err)
		fmt.Fprintf(os.Stderr, "ERROR: allocations file corrupted: %v\n", err)
		fmt.Fprintf(os.Stderr, "       File: %s\n", fl.path)
		fmt.Fprintf(os.Stderr, "       Use --forget-all to reset, or fix the file manually.\n")
		return nil, fmt.Errorf("allocations file corrupted: %w", err)
	}

	if store.Allocations == nil {
		store.Allocations = make(map[int]*AllocationInfo)
	}

	// Normalize directory paths
	for port, info := range store.Allocations {
		if info != nil {
			info.Directory = filepath.Clean(info.Directory)
			store.Allocations[port] = info
		}
	}

	debug.Printf("allocations", "loaded %d allocations, last_issued_port=%d",
		len(store.Allocations), store.LastIssuedPort)
	return &store, nil
}

// write writes the store to the locked file.
func (fl *file) write(store *Store) error {
	data, err := yaml.Marshal(store)
	if err != nil {
		return fmt.Errorf("failed to marshal store: %w", err)
	}

	// Truncate and seek to beginning
	if err := fl.f.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate: %w", err)
	}
	if _, err := fl.f.Seek(0, 0); err != nil {
		return fmt.Errorf("failed to seek: %w", err)
	}

	if _, err := fl.f.Write(data); err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}

	if err := fl.f.Sync(); err != nil {
		return fmt.Errorf("failed to sync: %w", err)
	}

	debug.Printf("allocations", "saved %d allocations", len(store.Allocations))
	return nil
}

// WithStore executes a function with exclusive access to the allocations store.
// The store is automatically loaded before and saved after the function executes.
// Returns the result of the function.
func WithStore(configDir string, fn func(*Store) error) error {
	fl, err := openAndLock(configDir)
	if err != nil {
		return err
	}
	defer fl.unlock()

	store, err := fl.read()
	if err != nil {
		return err
	}

	if err := fn(store); err != nil {
		return err
	}

	return fl.write(store)
}

// Load reads allocations from the config directory (without locking).
// Returns empty store if file doesn't exist, error for other failures.
// Use WithStore for operations that need locking.
func Load(configDir string) (*Store, error) {
	path := filepath.Join(configDir, allocationsFileName)
	debug.Printf("allocations", "loading from %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			debug.Printf("allocations", "file does not exist, returning empty store")
			return NewStore(), nil
		}
		debug.Printf("allocations", "failed to read file: %v", err)
		return nil, fmt.Errorf("cannot read allocations file: %w", err)
	}

	var store Store
	if err := yaml.Unmarshal(data, &store); err != nil {
		debug.Printf("allocations", "YAML parse error: %v", err)
		return nil, fmt.Errorf("allocations file corrupted (use --forget-all to reset): %w", err)
	}

	if store.Allocations == nil {
		store.Allocations = make(map[int]*AllocationInfo)
	}

	// Normalize directory paths
	for port, info := range store.Allocations {
		if info != nil {
			info.Directory = filepath.Clean(info.Directory)
			store.Allocations[port] = info
		}
	}

	debug.Printf("allocations", "loaded %d allocations", len(store.Allocations))
	return &store, nil
}

// Save writes store to the config directory (without locking).
// Use WithStore for operations that need locking.
func Save(configDir string, store *Store) error {
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	path := filepath.Join(configDir, allocationsFileName)
	tmpPath := path + ".tmp"

	debug.Printf("allocations", "saving %d allocations to %s", len(store.Allocations), path)

	data, err := yaml.Marshal(store)
	if err != nil {
		return fmt.Errorf("failed to marshal store: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	debug.Printf("allocations", "saved successfully")
	return nil
}

// FindByDirectory returns the allocation for a given directory, or nil if not found.
// When multiple ports are allocated to the same directory, returns the most recently used one
// (by LastUsedAt, or AssignedAt if LastUsedAt is not set).
func (s *Store) FindByDirectory(dir string) *Allocation {
	dir = filepath.Clean(dir)
	var bestPort int
	var bestInfo *AllocationInfo
	var bestTime time.Time

	for port, info := range s.Allocations {
		if info == nil || info.Directory != dir {
			continue
		}

		// Determine the time to compare (prefer LastUsedAt, fallback to AssignedAt)
		checkTime := info.LastUsedAt
		if checkTime.IsZero() {
			checkTime = info.AssignedAt
		}

		// Select the port with the most recent time (use lower port number as tiebreaker for determinism)
		if bestInfo == nil || checkTime.After(bestTime) || (checkTime.Equal(bestTime) && port < bestPort) {
			bestPort = port
			bestInfo = info
			bestTime = checkTime
		}
	}

	if bestInfo == nil {
		return nil
	}

	return &Allocation{
		Port:        bestPort,
		Directory:   bestInfo.Directory,
		AssignedAt:  bestInfo.AssignedAt,
		LastUsedAt:  bestInfo.LastUsedAt,
		Locked:      bestInfo.Locked,
		ProcessName: bestInfo.ProcessName,
		ContainerID: bestInfo.ContainerID,
	}
}

// FindByPort returns the allocation for a given port, or nil if not found.
func (s *Store) FindByPort(port int) *Allocation {
	info := s.Allocations[port]
	if info == nil {
		return nil
	}
	return &Allocation{
		Port:        port,
		Directory:   info.Directory,
		AssignedAt:  info.AssignedAt,
		LastUsedAt:  info.LastUsedAt,
		Locked:      info.Locked,
		ProcessName: info.ProcessName,
		ContainerID: info.ContainerID,
	}
}

// PortChecker is a function that checks if a port is free.
type PortChecker func(port int) bool

// SetAllocation adds or updates an allocation for a directory.
// If the directory already has a different port, the old port is removed.
func (s *Store) SetAllocation(dir string, port int) {
	s.SetAllocationWithPortCheck(dir, port, "", nil)
}

// SetAllocationWithProcess adds or updates a port allocation for the given directory.
// If the directory already has a different port, the old port is removed.
func (s *Store) SetAllocationWithProcess(dir string, port int, processName string) {
	s.SetAllocationWithPortCheck(dir, port, processName, nil)
}

// SetAllocationWithPortCheck adds or updates a port allocation for the given directory.
// If the directory already has a different port, the old port(s) are cleaned up safely:
// - If isPortFree is provided, only deletes old ports that are actually free
// - If isPortFree is nil, deletes all old ports (legacy behavior)
// - All old ports for the directory are processed (no early break)
func (s *Store) SetAllocationWithPortCheck(dir string, newPort int, processName string, isPortFree PortChecker) {
	dir = filepath.Clean(dir)
	now := time.Now().UTC()

	// Collect old ports for this directory (different from new port)
	var oldPorts []int
	for p, info := range s.Allocations {
		if info != nil && info.Directory == dir && p != newPort {
			oldPorts = append(oldPorts, p)
		}
	}

	// Process all old ports (safe cleanup)
	for _, oldPort := range oldPorts {
		if isPortFree != nil {
			if isPortFree(oldPort) {
				// Port is free - safe to delete
				delete(s.Allocations, oldPort)
				debug.Printf("allocations", "removed old allocation port %d for directory %s (superseded)", oldPort, dir)
				logger.Log(logger.AllocDelete,
					logger.Field("port", oldPort),
					logger.Field("dir", dir),
					logger.Field("reason", "superseded_by_new"),
					logger.Field("new_port", newPort))
			} else {
				// Port is still in use - keep allocation, TTL will clean it up later
				debug.Printf("allocations", "keeping old allocation port %d for directory %s (still in use)", oldPort, dir)
				logger.Log(logger.AllocUpdate,
					logger.Field("port", oldPort),
					logger.Field("dir", dir),
					logger.Field("reason", "old_port_still_in_use"))
			}
		} else {
			// No port checker - delete unconditionally (legacy behavior)
			delete(s.Allocations, oldPort)
			debug.Printf("allocations", "removed old allocation port %d for directory %s", oldPort, dir)
			logger.Log(logger.AllocDelete,
				logger.Field("port", oldPort),
				logger.Field("dir", dir),
				logger.Field("reason", "superseded_by_new"),
				logger.Field("new_port", newPort))
		}
	}

	// Update or create allocation for the port
	existing := s.Allocations[newPort]
	if existing != nil {
		// Update existing
		existing.Directory = dir
		existing.AssignedAt = now
		existing.LastUsedAt = now
		if processName != "" {
			existing.ProcessName = processName
		}
		// Log update
		logger.Log(logger.AllocUpdate, logger.Field("port", newPort), logger.Field("dir", dir))
	} else {
		// Create new
		s.Allocations[newPort] = &AllocationInfo{
			Directory:   dir,
			AssignedAt:  now,
			LastUsedAt:  now,
			ProcessName: processName,
		}
		// Log new allocation
		if processName != "" {
			logger.Log(logger.AllocAdd, logger.Field("port", newPort), logger.Field("dir", dir), logger.Field("process", processName))
		} else {
			logger.Log(logger.AllocAdd, logger.Field("port", newPort), logger.Field("dir", dir))
		}
	}
}

// AddAllocationForScan adds a port allocation without removing existing allocations
// for the same directory. This is used by --scan to allow multiple ports per directory
// (e.g., Docker Compose projects with multiple services).
func (s *Store) AddAllocationForScan(dir string, port int, processName, containerID string) {
	dir = filepath.Clean(dir)
	now := time.Now().UTC()

	// Check if this exact port already has an allocation
	if existing := s.Allocations[port]; existing != nil {
		// Update existing allocation for this port
		existing.Directory = dir
		existing.LastUsedAt = now
		if processName != "" {
			existing.ProcessName = processName
		}
		if containerID != "" {
			existing.ContainerID = containerID
		}
		logger.Log(logger.AllocUpdate, logger.Field("port", port), logger.Field("dir", dir))
		return
	}

	// Create new allocation (don't remove other ports for this directory)
	s.Allocations[port] = &AllocationInfo{
		Directory:   dir,
		AssignedAt:  now,
		LastUsedAt:  now,
		ProcessName: processName,
		ContainerID: containerID,
	}
	if processName != "" {
		logger.Log(logger.AllocAdd, logger.Field("port", port), logger.Field("dir", dir), logger.Field("process", processName))
	} else {
		logger.Log(logger.AllocAdd, logger.Field("port", port), logger.Field("dir", dir))
	}
}

// SetUnknownPortAllocation adds an allocation for a busy port with unknown ownership.
func (s *Store) SetUnknownPortAllocation(port int, processName string) {
	now := time.Now().UTC()
	dir := fmt.Sprintf("(unknown:%d)", port)

	s.Allocations[port] = &AllocationInfo{
		Directory:   dir,
		AssignedAt:  now,
		LastUsedAt:  now,
		ProcessName: processName,
	}

	logger.Log(logger.AllocAdd, logger.Field("port", port), logger.Field("dir", dir), logger.Field("process", processName))
}

// GetLastIssuedPort returns the last issued port number.
func (s *Store) GetLastIssuedPort() int {
	return s.LastIssuedPort
}

// SetLastIssuedPort sets the last issued port number.
func (s *Store) SetLastIssuedPort(port int) {
	s.LastIssuedPort = port
}

// SortedByPort returns allocations sorted by port number (ascending).
func (s *Store) SortedByPort() []Allocation {
	var result []Allocation
	for port, info := range s.Allocations {
		if info != nil {
			result = append(result, Allocation{
				Port:        port,
				Directory:   info.Directory,
				AssignedAt:  info.AssignedAt,
				LastUsedAt:  info.LastUsedAt,
				Locked:      info.Locked,
				ProcessName: info.ProcessName,
				ContainerID: info.ContainerID,
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Port < result[j].Port
	})

	return result
}

// RemoveByDirectory removes the allocation for a given directory.
// Returns the removed allocation and true if found, nil and false otherwise.
func (s *Store) RemoveByDirectory(dir string) (*Allocation, bool) {
	dir = filepath.Clean(dir)
	for port, info := range s.Allocations {
		if info != nil && info.Directory == dir {
			removed := &Allocation{
				Port:        port,
				Directory:   info.Directory,
				AssignedAt:  info.AssignedAt,
				LastUsedAt:  info.LastUsedAt,
				Locked:      info.Locked,
				ProcessName: info.ProcessName,
				ContainerID: info.ContainerID,
			}
			delete(s.Allocations, port)
			logger.Log(logger.AllocDelete, logger.Field("port", port), logger.Field("dir", dir))
			return removed, true
		}
	}
	return nil, false
}

// RemoveByPort removes the allocation for a given port.
// Returns true if found and removed.
func (s *Store) RemoveByPort(port int) bool {
	if info, exists := s.Allocations[port]; exists {
		logger.Log(logger.AllocDelete, logger.Field("port", port), logger.Field("dir", info.Directory))
		delete(s.Allocations, port)
		return true
	}
	return false
}

// RemoveAll clears all allocations and returns the count of removed items.
func (s *Store) RemoveAll() int {
	count := len(s.Allocations)
	s.Allocations = make(map[int]*AllocationInfo)
	s.LastIssuedPort = 0
	if count > 0 {
		logger.Log(logger.AllocDeleteAll, logger.Field("count", count))
	}
	return count
}

// RemoveExpired removes allocations older than the given TTL.
// Returns the count of removed items.
func (s *Store) RemoveExpired(ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}

	cutoff := time.Now().Add(-ttl)
	count := 0

	for port, info := range s.Allocations {
		if info == nil {
			continue
		}
		// Use LastUsedAt if available, otherwise AssignedAt
		checkTime := info.LastUsedAt
		if checkTime.IsZero() {
			checkTime = info.AssignedAt
		}
		if checkTime.Before(cutoff) {
			logger.Log(logger.AllocExpire, logger.Field("port", port), logger.Field("dir", info.Directory), logger.Field("ttl", ttl.String()))
			delete(s.Allocations, port)
			count++
		}
	}

	return count
}

// UpdateLastUsed updates the LastUsedAt timestamp for a given directory to now.
// When multiple ports exist for the directory, updates the most recently used one.
// Returns true if allocation was found and updated.
func (s *Store) UpdateLastUsed(dir string) bool {
	// Find the best allocation (most recently used)
	alloc := s.FindByDirectory(dir)
	if alloc == nil {
		return false
	}
	return s.UpdateLastUsedByPort(alloc.Port)
}

// UpdateLastUsedByPort updates the LastUsedAt timestamp for a specific port to now.
// Returns true if allocation was found and updated.
func (s *Store) UpdateLastUsedByPort(port int) bool {
	info := s.Allocations[port]
	if info == nil {
		return false
	}
	info.LastUsedAt = time.Now().UTC()
	logger.Log(logger.AllocUpdate, logger.Field("port", port), logger.Field("dir", info.Directory))
	return true
}

// SetLocked sets the locked status for an allocation identified by directory.
// Returns true if allocation was found and updated.
func (s *Store) SetLocked(dir string, locked bool) bool {
	dir = filepath.Clean(dir)
	for port, info := range s.Allocations {
		if info != nil && info.Directory == dir {
			info.Locked = locked
			s.Allocations[port] = info
			logger.Log(logger.AllocLock, logger.Field("port", port), logger.Field("locked", locked))
			return true
		}
	}
	return false
}

// SetLockedByPort sets the locked status for an allocation identified by port.
// Returns true if allocation was found and updated.
func (s *Store) SetLockedByPort(port int, locked bool) bool {
	if info := s.Allocations[port]; info != nil {
		info.Locked = locked
		logger.Log(logger.AllocLock, logger.Field("port", port), logger.Field("locked", locked))
		return true
	}
	return false
}

// IsPortLocked checks if a port is locked by another directory.
// Returns true if the port is allocated to a different directory and is locked.
func (s *Store) IsPortLocked(port int, currentDir string) bool {
	currentDir = filepath.Clean(currentDir)
	info := s.Allocations[port]
	if info == nil {
		return false
	}
	// Port belongs to current directory - not considered locked for this directory
	if info.Directory == currentDir {
		return false
	}
	// Port belongs to another directory - check if it's locked
	return info.Locked
}

// GetLockedPortsForExclusion returns a map of ports that are locked by directories
// other than the current one. These ports should be excluded during port allocation.
func (s *Store) GetLockedPortsForExclusion(currentDir string) map[int]bool {
	currentDir = filepath.Clean(currentDir)
	locked := make(map[int]bool)
	for port, info := range s.Allocations {
		if info != nil && info.Locked && info.Directory != currentDir {
			locked[port] = true
		}
	}
	return locked
}

// GetFrozenPorts returns ports that were recently used (within freeze period).
// This replaces the history package functionality.
func (s *Store) GetFrozenPorts(freezePeriod time.Duration) map[int]bool {
	frozen := make(map[int]bool)
	if freezePeriod <= 0 {
		return frozen
	}

	cutoff := time.Now().Add(-freezePeriod)

	for port, info := range s.Allocations {
		if info == nil {
			continue
		}
		// Use LastUsedAt if available, otherwise AssignedAt
		checkTime := info.LastUsedAt
		if checkTime.IsZero() {
			checkTime = info.AssignedAt
		}
		if checkTime.After(cutoff) {
			frozen[port] = true
		}
	}

	return frozen
}

// Count returns the number of allocations.
func (s *Store) Count() int {
	return len(s.Allocations)
}
