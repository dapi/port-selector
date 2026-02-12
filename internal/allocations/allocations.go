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

// UnknownDirectoryFormat is the format string for unknown directory placeholders.
const UnknownDirectoryFormat = "(unknown:%d)"

// AllocationStatus represents the type of allocation.
type AllocationStatus string

// Status constants for allocations.
const (
	StatusNormal   AllocationStatus = ""         // Normal allocation (empty for backward compat)
	StatusExternal AllocationStatus = "external" // External process using this port
)

// AllocationInfo represents a single port allocation entry.
type AllocationInfo struct {
	Directory           string           `yaml:"directory"`
	AssignedAt          time.Time        `yaml:"assigned_at"`
	LastUsedAt          time.Time        `yaml:"last_used_at,omitempty"`
	Locked              bool             `yaml:"locked,omitempty"`
	ProcessName         string           `yaml:"process_name,omitempty"`
	ContainerID         string           `yaml:"container_id,omitempty"`
	Name                string           `yaml:"name,omitempty"`
	Status              AllocationStatus `yaml:"status,omitempty"`                // StatusNormal or StatusExternal
	LockedAt            time.Time        `yaml:"locked_at,omitempty"`             // Time when port was locked
	ExternalPID         int              `yaml:"external_pid,omitempty"`          // PID of external process (0 = unknown)
	ExternalUser        string           `yaml:"external_user,omitempty"`         // User of external process
	ExternalProcessName string           `yaml:"external_process_name,omitempty"` // Name of external process
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
	Port                int
	Directory           string
	AssignedAt          time.Time
	LastUsedAt          time.Time
	Locked              bool
	ProcessName         string
	ContainerID         string
	Name                string
	Status              AllocationStatus // StatusNormal or StatusExternal
	LockedAt            time.Time        // Time when port was locked
	ExternalPID         int              // PID of external process (0 = unknown)
	ExternalUser        string           // User of external process
	ExternalProcessName string           // Name of external process
}

// toAllocation converts AllocationInfo to Allocation with the given port number.
func (info *AllocationInfo) toAllocation(port int) *Allocation {
	return &Allocation{
		Port:                port,
		Directory:           info.Directory,
		AssignedAt:          info.AssignedAt,
		LastUsedAt:          info.LastUsedAt,
		Locked:              info.Locked,
		ProcessName:         info.ProcessName,
		ContainerID:         info.ContainerID,
		Name:                info.Name,
		Status:              info.Status,
		LockedAt:            info.LockedAt,
		ExternalPID:         info.ExternalPID,
		ExternalUser:        info.ExternalUser,
		ExternalProcessName: info.ExternalProcessName,
	}
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

	// Normalize directory paths and names
	for port, info := range store.Allocations {
		if info != nil {
			info.Directory = filepath.Clean(info.Directory)
			// Normalize empty name to "main" for legacy allocations
			if info.Name == "" {
				info.Name = "main"
			}
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

	// Normalize directory paths and names
	for port, info := range store.Allocations {
		if info != nil {
			info.Directory = filepath.Clean(info.Directory)
			// Normalize empty name to "main" for legacy allocations
			if info.Name == "" {
				info.Name = "main"
			}
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

	return bestInfo.toAllocation(bestPort)
}

// FindByPort returns the allocation for a given port, or nil if not found.
func (s *Store) FindByPort(port int) *Allocation {
	info := s.Allocations[port]
	if info == nil {
		return nil
	}
	return info.toAllocation(port)
}

// PortChecker is a function that checks if a port is free.
type PortChecker func(port int) bool

// SetAllocation adds or updates an allocation for a directory.
// If the directory already has a different port, the old port is removed.
func (s *Store) SetAllocation(dir string, port int) {
	s.SetAllocationWithPortCheck(dir, port, "", nil)
}

// SetAllocationWithName adds or updates a port allocation for the given directory and name.
func (s *Store) SetAllocationWithName(dir string, port int, name string) {
	s.SetAllocationWithPortCheckAndName(dir, port, "", name, nil)
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
	s.SetAllocationWithPortCheckAndName(dir, newPort, processName, "main", isPortFree)
}

// SetAllocationWithPortCheckAndName adds or updates a port allocation for the given directory and name.
// If the directory/name already has a different port, the old port(s) are cleaned up safely.
func (s *Store) SetAllocationWithPortCheckAndName(dir string, newPort int, processName string, name string, isPortFree PortChecker) {
	dir = filepath.Clean(dir)
	now := time.Now().UTC()
	name = normalizeName(name)

	// Collect old ports for this directory and name (different from new port)
	var oldPorts []int
	for p, info := range s.Allocations {
		if info != nil && info.Directory == dir && normalizeName(info.Name) == name && p != newPort {
			oldPorts = append(oldPorts, p)
		}
	}

	// Process all old ports (safe cleanup)
	for _, oldPort := range oldPorts {
		oldInfo := s.Allocations[oldPort]
		if oldInfo == nil {
			continue
		}

		// Never delete locked ports - they must be explicitly unlocked or forgotten
		if oldInfo.Locked {
			debug.Printf("allocations", "keeping old allocation port %d for directory %s name %s (locked)", oldPort, dir, name)
			logger.Log(logger.AllocUpdate,
				logger.Field("port", oldPort),
				logger.Field("dir", dir),
				logger.Field("name", name),
				logger.Field("reason", "locked_port_preserved"))
			continue
		}

		if isPortFree != nil {
			if isPortFree(oldPort) {
				// Port is free - safe to delete
				delete(s.Allocations, oldPort)
				debug.Printf("allocations", "removed old allocation port %d for directory %s name %s (superseded)", oldPort, dir, name)
				logger.Log(logger.AllocDelete,
					logger.Field("port", oldPort),
					logger.Field("dir", dir),
					logger.Field("name", name),
					logger.Field("reason", "superseded_by_new"),
					logger.Field("new_port", newPort))
			} else {
				// Port is still in use - keep allocation, TTL will clean it up later
				debug.Printf("allocations", "keeping old allocation port %d for directory %s name %s (still in use)", oldPort, dir, name)
				logger.Log(logger.AllocUpdate,
					logger.Field("port", oldPort),
					logger.Field("dir", dir),
					logger.Field("name", name),
					logger.Field("reason", "old_port_still_in_use"))
			}
		} else {
			// No port checker - delete unconditionally (legacy behavior)
			delete(s.Allocations, oldPort)
			debug.Printf("allocations", "removed old allocation port %d for directory %s name %s", oldPort, dir, name)
			logger.Log(logger.AllocDelete,
				logger.Field("port", oldPort),
				logger.Field("dir", dir),
				logger.Field("name", name),
				logger.Field("reason", "superseded_by_new"),
				logger.Field("new_port", newPort))
		}
	}

	// Update or create allocation for the port
	existing := s.Allocations[newPort]
	if existing != nil {
		// Update existing
		existing.Directory = dir
		existing.Name = name
		existing.AssignedAt = now
		existing.LastUsedAt = now
		if processName != "" {
			existing.ProcessName = processName
		}
		// Log update
		logger.Log(logger.AllocUpdate,
			logger.Field("port", newPort),
			logger.Field("dir", dir),
			logger.Field("name", name))
	} else {
		// Create new
		s.Allocations[newPort] = &AllocationInfo{
			Directory:   dir,
			Name:        name,
			AssignedAt:  now,
			LastUsedAt:  now,
			ProcessName: processName,
		}
		// Log new allocation
		if processName != "" {
			logger.Log(logger.AllocAdd,
				logger.Field("port", newPort),
				logger.Field("dir", dir),
				logger.Field("name", name),
				logger.Field("process", processName))
		} else {
			logger.Log(logger.AllocAdd,
				logger.Field("port", newPort),
				logger.Field("dir", dir),
				logger.Field("name", name))
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
		// Keep existing name if any, otherwise set to "main"
		if existing.Name == "" {
			existing.Name = "main"
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
		Name:        "main",
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
	dir := fmt.Sprintf(UnknownDirectoryFormat, port)

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
			result = append(result, *info.toAllocation(port))
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
			removed := info.toAllocation(port)
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
// Locked allocations are never removed by TTL - they must be explicitly unlocked or forgotten.
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
		// Never expire locked allocations
		if info.Locked {
			debug.Printf("allocations", "skipping TTL expiration for locked port %d", port)
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
			if locked {
				info.LockedAt = time.Now().UTC()
			}
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
		if locked {
			info.LockedAt = time.Now().UTC()
		}
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

// normalizeName returns the normalized name (empty -> "main").
func normalizeName(name string) string {
	if name == "" {
		return "main"
	}
	return name
}

// FindByDirectoryAndName returns the allocation for a given directory and name, or nil if not found.
// When multiple ports are allocated to the same directory/name, returns the most recently used one.
// Note: This method does not check port availability. Use FindByDirectoryAndNameWithPriority
// for smart selection that considers locked status and port availability.
func (s *Store) FindByDirectoryAndName(dir string, name string) *Allocation {
	dir = filepath.Clean(dir)
	name = normalizeName(name)
	var bestPort int
	var bestInfo *AllocationInfo
	var bestTime time.Time

	for port, info := range s.Allocations {
		if info == nil || info.Directory != dir || info.Name != name {
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

	return bestInfo.toAllocation(bestPort)
}

// FindByDirectoryAndNameWithPriority returns the best allocation for a given directory and name.
// Uses smart priority based on locked status and port availability:
// 1. Locked + free → return (reserved port is available)
// 2. Locked + busy → return (user's service is already running on their reserved port)
// 3. Unlocked + free → return (can reuse this port)
// 4. Unlocked + busy → skip (port in use, find another)
//
// Returns nil if no suitable allocation found (all are unlocked+busy or none exist).
func (s *Store) FindByDirectoryAndNameWithPriority(dir string, name string, isPortFree PortChecker) *Allocation {
	dir = filepath.Clean(dir)
	name = normalizeName(name)

	// Collect all matching allocations
	type candidate struct {
		port int
		info *AllocationInfo
		free bool
	}
	var candidates []candidate

	for port, info := range s.Allocations {
		if info == nil || info.Directory != dir || info.Name != name {
			continue
		}
		free := isPortFree != nil && isPortFree(port)
		candidates = append(candidates, candidate{port: port, info: info, free: free})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Priority selection:
	// 1. Locked + free
	// 2. Locked + busy
	// 3. Unlocked + free
	// 4. Unlocked + busy (skip)

	var best *candidate

	for i := range candidates {
		c := &candidates[i]

		// Skip unlocked + busy (priority 4)
		if !c.info.Locked && !c.free {
			continue
		}

		if best == nil {
			best = c
			continue
		}

		// Compare priorities
		bestPriority := getPriority(best.info.Locked, best.free)
		cPriority := getPriority(c.info.Locked, c.free)

		if cPriority < bestPriority {
			// Lower priority number = better
			best = c
		} else if cPriority == bestPriority {
			// Same priority - use most recent time, then lower port as tiebreaker
			bestTime := best.info.LastUsedAt
			if bestTime.IsZero() {
				bestTime = best.info.AssignedAt
			}
			cTime := c.info.LastUsedAt
			if cTime.IsZero() {
				cTime = c.info.AssignedAt
			}
			if cTime.After(bestTime) || (cTime.Equal(bestTime) && c.port < best.port) {
				best = c
			}
		}
	}

	if best == nil {
		return nil
	}

	return &Allocation{
		Port:        best.port,
		Directory:   best.info.Directory,
		AssignedAt:  best.info.AssignedAt,
		LastUsedAt:  best.info.LastUsedAt,
		Locked:      best.info.Locked,
		ProcessName: best.info.ProcessName,
		ContainerID: best.info.ContainerID,
		Name:        best.info.Name,
	}
}

// getPriority returns priority number for locked/free combination (lower = better).
func getPriority(locked, free bool) int {
	if locked && free {
		return 1
	}
	if locked && !free {
		return 2
	}
	if !locked && free {
		return 3
	}
	return 4 // unlocked + busy (should be skipped)
}

// RemoveByDirectoryAndName removes the allocation for a given directory and name.
// Returns the removed allocation and true if found, nil and false otherwise.
func (s *Store) RemoveByDirectoryAndName(dir string, name string) (*Allocation, bool) {
	dir = filepath.Clean(dir)
	name = normalizeName(name)
	for port, info := range s.Allocations {
		if info != nil && info.Directory == dir && info.Name == name {
			removed := info.toAllocation(port)
			delete(s.Allocations, port)
			logger.Log(logger.AllocDelete, logger.Field("port", port), logger.Field("dir", dir), logger.Field("name", name))
			return removed, true
		}
	}
	return nil, false
}

// GetAllocatedPortsForDirectory returns all ports allocated to a given directory.
func (s *Store) GetAllocatedPortsForDirectory(dir string) map[int]bool {
	dir = filepath.Clean(dir)
	ports := make(map[int]bool)
	for port, info := range s.Allocations {
		if info != nil && info.Directory == dir {
			ports[port] = true
		}
	}
	return ports
}

// UpdateLastUsedByDirectoryAndName updates the LastUsedAt timestamp for a given directory and name to now.
// Returns true if allocation was found and updated.
func (s *Store) UpdateLastUsedByDirectoryAndName(dir string, name string) bool {
	alloc := s.FindByDirectoryAndName(dir, name)
	if alloc == nil {
		return false
	}
	info := s.Allocations[alloc.Port]
	if info == nil {
		return false
	}
	info.LastUsedAt = time.Now().UTC()
	logger.Log(logger.AllocUpdate,
		logger.Field("port", alloc.Port),
		logger.Field("dir", dir),
		logger.Field("name", name))
	return true
}

// SetLockedByDirectoryAndName sets the locked status for an allocation identified by directory and name.
// Returns true if allocation was found and updated.
func (s *Store) SetLockedByDirectoryAndName(dir string, name string, locked bool) bool {
	dir = filepath.Clean(dir)
	name = normalizeName(name)
	for port, info := range s.Allocations {
		if info != nil && info.Directory == dir && info.Name == name {
			info.Locked = locked
			if locked {
				info.LockedAt = time.Now().UTC()
			}
			logger.Log(logger.AllocLock, logger.Field("port", port), logger.Field("locked", locked), logger.Field("name", name))
			return true
		}
	}
	return false
}

// SetLockedByPortAndName sets the locked status for an allocation identified by port and name.
// Returns true if allocation was found and updated with the same name.
func (s *Store) SetLockedByPortAndName(port int, name string, locked bool) bool {
	info := s.Allocations[port]
	if info == nil {
		return false
	}
	name = normalizeName(name)
	if info.Name != name {
		return false
	}
	info.Locked = locked
	if locked {
		info.LockedAt = time.Now().UTC()
	}
	logger.Log(logger.AllocLock, logger.Field("port", port), logger.Field("locked", locked), logger.Field("name", name))
	return true
}

// UnlockOtherLockedPorts unlocks all locked ports for the given directory and name,
// except the specified port. This ensures the invariant: at most one locked port
// per directory+name combination.
// Returns the count of ports that were unlocked.
func (s *Store) UnlockOtherLockedPorts(dir string, name string, exceptPort int) int {
	dir = filepath.Clean(dir)
	name = normalizeName(name)
	debug.Printf("allocations", "UnlockOtherLockedPorts: dir=%s name=%s exceptPort=%d", dir, name, exceptPort)
	count := 0
	for port, info := range s.Allocations {
		if info == nil || port == exceptPort {
			continue
		}
		if info.Directory == dir && info.Name == name && info.Locked {
			info.Locked = false
			logger.Log(logger.AllocLock,
				logger.Field("port", port),
				logger.Field("locked", false),
				logger.Field("name", name),
				logger.Field("reason", "new_lock_for_same_name"))
			count++
		}
	}
	return count
}

// SetExternalAllocation registers a port as used by an external process.
// This is used when a port is already in use by another directory/process.
// The allocation is marked with Status="external" and stores process information.
func (s *Store) SetExternalAllocation(port int, pid int, user, processName, cwd string) {
	now := time.Now().UTC()

	existing := s.Allocations[port]
	if existing != nil {
		// Update existing allocation to external status
		existing.Status = StatusExternal
		existing.LastUsedAt = now
		existing.ExternalPID = pid
		existing.ExternalUser = user
		existing.ExternalProcessName = processName
		// Keep existing directory if any, otherwise use process cwd
		if existing.Directory == "" || existing.Directory == fmt.Sprintf(UnknownDirectoryFormat, port) {
			if cwd != "" {
				existing.Directory = cwd
			}
		}
		logger.Log(logger.AllocExternal,
			logger.Field("port", port),
			logger.Field("dir", existing.Directory),
			logger.Field("pid", pid),
			logger.Field("user", user),
			logger.Field("process", processName),
			logger.Field("action", "update"))
		return
	}

	// Create new external allocation
	dir := cwd
	if dir == "" {
		dir = fmt.Sprintf(UnknownDirectoryFormat, port)
	}

	s.Allocations[port] = &AllocationInfo{
		Directory:           dir,
		AssignedAt:          now,
		LastUsedAt:          now,
		Status:              StatusExternal,
		ExternalPID:         pid,
		ExternalUser:        user,
		ExternalProcessName: processName,
		Name:                "main",
	}
	logger.Log(logger.AllocExternal,
		logger.Field("port", port),
		logger.Field("dir", dir),
		logger.Field("pid", pid),
		logger.Field("user", user),
		logger.Field("process", processName),
		logger.Field("action", "create"))
}

// RefreshExternalAllocations removes stale external allocations (ports that are now free).
// Updates LastUsedAt for allocations that are still active.
// Returns the count of removed allocations.
// Panics if isPortFree is nil (programming error).
func (s *Store) RefreshExternalAllocations(isPortFree PortChecker) int {
	if isPortFree == nil {
		panic("RefreshExternalAllocations: isPortFree function cannot be nil")
	}

	var removedPorts []int
	var updatedPorts []int

	for port, info := range s.Allocations {
		if info == nil || info.Status != StatusExternal {
			continue
		}

		if isPortFree(port) {
			// Port is now free - remove the external allocation
			removedPorts = append(removedPorts, port)
		} else {
			// Port is still busy - update LastUsedAt
			info.LastUsedAt = time.Now().UTC()
			updatedPorts = append(updatedPorts, port)
		}
	}

	// Remove stale allocations
	for _, port := range removedPorts {
		info := s.Allocations[port]
		logger.Log(logger.AllocDelete,
			logger.Field("port", port),
			logger.Field("dir", info.Directory),
			logger.Field("reason", "stale_external"))
		delete(s.Allocations, port)
	}

	// Log updated allocations
	for _, port := range updatedPorts {
		info := s.Allocations[port]
		logger.Log(logger.AllocUpdate,
			logger.Field("port", port),
			logger.Field("dir", info.Directory),
			logger.Field("reason", "external_still_active"))
	}

	if len(removedPorts) > 0 {
		logger.Log(logger.AllocRefresh,
			logger.Field("removed", len(removedPorts)),
			logger.Field("updated", len(updatedPorts)))
	}

	return len(removedPorts)
}
