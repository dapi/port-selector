package allocations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	store := NewStore()
	if store == nil {
		t.Fatal("expected non-nil store")
	}
	if store.Allocations == nil {
		t.Error("expected non-nil allocations map")
	}
	if len(store.Allocations) != 0 {
		t.Errorf("expected empty allocations, got %d", len(store.Allocations))
	}
	if store.LastIssuedPort != 0 {
		t.Errorf("expected LastIssuedPort 0, got %d", store.LastIssuedPort)
	}
}

func TestLoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	store, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.Allocations) != 0 {
		t.Errorf("expected empty store, got %d allocations", len(store.Allocations))
	}
}

func TestLoadCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, allocationsFileName)

	// Write corrupted YAML
	if err := os.WriteFile(path, []byte("not: valid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(tmpDir)
	if err == nil {
		t.Fatal("expected error for corrupted file")
	}
	if store != nil {
		t.Error("expected nil store for corrupted file")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Errorf("expected 'corrupted' in error message, got: %v", err)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	original := NewStore()
	original.LastIssuedPort = 3005
	original.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: time.Now().UTC(),
	}
	original.Allocations[3001] = &AllocationInfo{
		Directory:  "/home/user/project-b",
		AssignedAt: time.Now().UTC(),
	}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if len(loaded.Allocations) != len(original.Allocations) {
		t.Errorf("expected %d allocations, got %d", len(original.Allocations), len(loaded.Allocations))
	}

	if loaded.LastIssuedPort != original.LastIssuedPort {
		t.Errorf("expected LastIssuedPort %d, got %d", original.LastIssuedPort, loaded.LastIssuedPort)
	}

	// Check individual allocations
	for port, origInfo := range original.Allocations {
		loadedInfo := loaded.Allocations[port]
		if loadedInfo == nil {
			t.Errorf("missing allocation for port %d", port)
			continue
		}
		if loadedInfo.Directory != origInfo.Directory {
			t.Errorf("port %d: expected dir %s, got %s", port, origInfo.Directory, loadedInfo.Directory)
		}
	}
}

func TestFindByDirectory(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a"}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b"}

	tests := []struct {
		dir      string
		expected *int
	}{
		{"/home/user/project-a", intPtr(3000)},
		{"/home/user/project-b", intPtr(3001)},
		{"/home/user/project-c", nil},
	}

	for _, tc := range tests {
		result := store.FindByDirectory(tc.dir)
		if tc.expected == nil {
			if result != nil {
				t.Errorf("FindByDirectory(%s): expected nil, got port %d", tc.dir, result.Port)
			}
		} else {
			if result == nil {
				t.Errorf("FindByDirectory(%s): expected port %d, got nil", tc.dir, *tc.expected)
			} else if result.Port != *tc.expected {
				t.Errorf("FindByDirectory(%s): expected port %d, got %d", tc.dir, *tc.expected, result.Port)
			}
		}
	}
}

func TestFindByPort(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a"}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b"}

	tests := []struct {
		port     int
		expected *string
	}{
		{3000, strPtr("/home/user/project-a")},
		{3001, strPtr("/home/user/project-b")},
		{3002, nil},
	}

	for _, tc := range tests {
		result := store.FindByPort(tc.port)
		if tc.expected == nil {
			if result != nil {
				t.Errorf("FindByPort(%d): expected nil, got dir %s", tc.port, result.Directory)
			}
		} else {
			if result == nil {
				t.Errorf("FindByPort(%d): expected dir %s, got nil", tc.port, *tc.expected)
			} else if result.Directory != *tc.expected {
				t.Errorf("FindByPort(%d): expected dir %s, got %s", tc.port, *tc.expected, result.Directory)
			}
		}
	}
}

func TestSetAllocationNew(t *testing.T) {
	store := NewStore()

	store.SetAllocation("/home/user/project-a", 3000)

	if len(store.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(store.Allocations))
	}

	info := store.Allocations[3000]
	if info == nil {
		t.Fatal("expected allocation for port 3000")
	}
	if info.Directory != "/home/user/project-a" {
		t.Errorf("expected dir /home/user/project-a, got %s", info.Directory)
	}
}

func TestSetAllocationUpdate(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: time.Now().Add(-1 * time.Hour),
	}

	// Update port for same directory - should remove old and add new
	store.SetAllocation("/home/user/project-a", 3005)

	if len(store.Allocations) != 1 {
		t.Fatalf("expected 1 allocation after update, got %d", len(store.Allocations))
	}

	// Old port should be removed
	if store.Allocations[3000] != nil {
		t.Error("old port 3000 should be removed")
	}

	// New port should exist
	info := store.Allocations[3005]
	if info == nil {
		t.Fatal("expected allocation for new port 3005")
	}
	if info.Directory != "/home/user/project-a" {
		t.Errorf("expected dir /home/user/project-a, got %s", info.Directory)
	}
}

func TestSetAllocation_PortAsKey_NoDuplicates(t *testing.T) {
	store := NewStore()

	// Add first allocation
	store.SetAllocation("/home/user/project-a", 3000)

	// Try to add second allocation with same port but different directory
	store.SetAllocation("/home/user/project-b", 3000)

	// Should only have one allocation
	if len(store.Allocations) != 1 {
		t.Errorf("expected 1 allocation (port as key guarantees uniqueness), got %d", len(store.Allocations))
	}

	// Port should now belong to project-b
	info := store.Allocations[3000]
	if info == nil {
		t.Fatal("expected allocation for port 3000")
	}
	if info.Directory != "/home/user/project-b" {
		t.Errorf("expected dir /home/user/project-b, got %s", info.Directory)
	}
}

func TestAddAllocationForScan_MultiplePortsSameDirectory(t *testing.T) {
	store := NewStore()

	// Scan finds first port for directory
	store.AddAllocationForScan("/home/user/valera", 3011, "docker-proxy", "container123")

	// Scan finds second port for same directory
	store.AddAllocationForScan("/home/user/valera", 3014, "docker-proxy", "container456")

	// Both ports should exist (not replaced)
	if len(store.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(store.Allocations))
	}

	// Port 3011 should exist
	info3011 := store.Allocations[3011]
	if info3011 == nil {
		t.Fatal("expected allocation for port 3011")
	}
	if info3011.Directory != "/home/user/valera" {
		t.Errorf("expected dir /home/user/valera for port 3011, got %s", info3011.Directory)
	}

	// Port 3014 should also exist
	info3014 := store.Allocations[3014]
	if info3014 == nil {
		t.Fatal("expected allocation for port 3014")
	}
	if info3014.Directory != "/home/user/valera" {
		t.Errorf("expected dir /home/user/valera for port 3014, got %s", info3014.Directory)
	}
}

func TestAddAllocationForScan_UpdatesExistingPort(t *testing.T) {
	store := NewStore()

	// First scan
	store.AddAllocationForScan("/home/user/project-a", 3000, "node", "")

	// Same port found again with different directory (port moved)
	store.AddAllocationForScan("/home/user/project-b", 3000, "python", "")

	// Should have only one allocation (port updated, not duplicated)
	if len(store.Allocations) != 1 {
		t.Errorf("expected 1 allocation, got %d", len(store.Allocations))
	}

	// Port should now point to project-b
	info := store.Allocations[3000]
	if info == nil {
		t.Fatal("expected allocation for port 3000")
	}
	if info.Directory != "/home/user/project-b" {
		t.Errorf("expected dir /home/user/project-b, got %s", info.Directory)
	}
	if info.ProcessName != "python" {
		t.Errorf("expected process python, got %s", info.ProcessName)
	}
}

func TestAddAllocationForScan_DoesNotAffectSetAllocation(t *testing.T) {
	store := NewStore()

	// Regular allocation (should replace)
	store.SetAllocation("/home/user/project", 3000)
	store.SetAllocation("/home/user/project", 3001) // Should remove 3000

	if len(store.Allocations) != 1 {
		t.Errorf("SetAllocation should replace, expected 1 allocation, got %d", len(store.Allocations))
	}
	if store.Allocations[3000] != nil {
		t.Error("SetAllocation should have removed port 3000")
	}
	if store.Allocations[3001] == nil {
		t.Error("SetAllocation should have port 3001")
	}
}

func TestFindByDirectory_PathNormalization(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project"}

	tests := []struct {
		name     string
		input    string
		wantPort int
	}{
		{"exact match", "/home/user/project", 3000},
		{"trailing slash", "/home/user/project/", 3000},
		{"double slash", "/home/user//project", 3000},
		{"dot segments", "/home/user/./project", 3000},
		{"parent segments", "/home/user/other/../project", 3000},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := store.FindByDirectory(tc.input)
			if result == nil {
				t.Fatalf("expected allocation, got nil for %q", tc.input)
			}
			if result.Port != tc.wantPort {
				t.Errorf("expected port %d, got %d", tc.wantPort, result.Port)
			}
		})
	}
}

func TestLoad_NormalizesPathsFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, allocationsFileName)

	// Write YAML with non-normalized paths
	yamlContent := `last_issued_port: 3001
allocations:
  3000:
    directory: /home/user/project/
    assigned_at: 2025-01-02T10:30:00Z
  3001:
    directory: /home/user//other
    assigned_at: 2025-01-02T11:00:00Z
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Verify paths are normalized after load
	if store.Allocations[3000].Directory != "/home/user/project" {
		t.Errorf("expected normalized path /home/user/project, got %s", store.Allocations[3000].Directory)
	}
	if store.Allocations[3001].Directory != "/home/user/other" {
		t.Errorf("expected normalized path /home/user/other, got %s", store.Allocations[3001].Directory)
	}

	// Verify FindByDirectory works with normalized search
	result := store.FindByDirectory("/home/user/project")
	if result == nil {
		t.Fatal("FindByDirectory failed for normalized path")
	}
	if result.Port != 3000 {
		t.Errorf("expected port 3000, got %d", result.Port)
	}
}

func TestSortedByPort(t *testing.T) {
	store := NewStore()
	store.Allocations[3005] = &AllocationInfo{Directory: "/home/user/project-c"}
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a"}
	store.Allocations[3002] = &AllocationInfo{Directory: "/home/user/project-b"}

	sorted := store.SortedByPort()

	expectedPorts := []int{3000, 3002, 3005}
	if len(sorted) != len(expectedPorts) {
		t.Fatalf("expected %d sorted allocations, got %d", len(expectedPorts), len(sorted))
	}
	for i, alloc := range sorted {
		if alloc.Port != expectedPorts[i] {
			t.Errorf("sorted[%d]: expected port %d, got %d", i, expectedPorts[i], alloc.Port)
		}
	}
}

func TestRemoveByDirectory(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a"}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b"}
	store.Allocations[3002] = &AllocationInfo{Directory: "/home/user/project-c"}

	// Remove existing directory
	removed, found := store.RemoveByDirectory("/home/user/project-b")
	if !found {
		t.Fatal("expected to find allocation")
	}
	if removed.Port != 3001 {
		t.Errorf("expected removed port 3001, got %d", removed.Port)
	}
	if len(store.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(store.Allocations))
	}

	// Verify port was removed from map
	if store.Allocations[3001] != nil {
		t.Error("port 3001 should be removed from map")
	}

	// Try to remove non-existent directory
	_, found = store.RemoveByDirectory("/home/user/project-x")
	if found {
		t.Error("should not find non-existent allocation")
	}
}

func TestRemoveByDirectory_PathNormalization(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project"}

	// Remove with trailing slash
	removed, found := store.RemoveByDirectory("/home/user/project/")
	if !found {
		t.Fatal("expected to find allocation with trailing slash")
	}
	if removed.Port != 3000 {
		t.Errorf("expected port 3000, got %d", removed.Port)
	}
	if len(store.Allocations) != 0 {
		t.Error("allocation should be removed")
	}
}

func TestRemoveByPort(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a"}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b"}

	// Remove existing port
	found := store.RemoveByPort(3000)
	if !found {
		t.Error("expected to find allocation")
	}
	if len(store.Allocations) != 1 {
		t.Errorf("expected 1 allocation, got %d", len(store.Allocations))
	}
	if store.Allocations[3000] != nil {
		t.Error("port 3000 should be removed")
	}

	// Try to remove non-existent port
	found = store.RemoveByPort(9999)
	if found {
		t.Error("should not find non-existent port")
	}
}

func TestRemoveAll(t *testing.T) {
	store := NewStore()
	store.LastIssuedPort = 3005
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a"}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b"}
	store.Allocations[3002] = &AllocationInfo{Directory: "/home/user/project-c"}

	count := store.RemoveAll()
	if count != 3 {
		t.Errorf("expected 3 removed, got %d", count)
	}
	if len(store.Allocations) != 0 {
		t.Errorf("expected empty allocations, got %d", len(store.Allocations))
	}
	if store.LastIssuedPort != 0 {
		t.Errorf("expected LastIssuedPort to be reset to 0, got %d", store.LastIssuedPort)
	}

	// Remove from empty store
	count = store.RemoveAll()
	if count != 0 {
		t.Errorf("expected 0 removed from empty store, got %d", count)
	}
}

func TestRemoveExpired(t *testing.T) {
	now := time.Now()
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: now.Add(-48 * time.Hour), // 2 days old
		LastUsedAt: now.Add(-48 * time.Hour),
	}
	store.Allocations[3001] = &AllocationInfo{
		Directory:  "/home/user/project-b",
		AssignedAt: now.Add(-12 * time.Hour), // 12 hours old
		LastUsedAt: now.Add(-12 * time.Hour),
	}
	store.Allocations[3002] = &AllocationInfo{
		Directory:  "/home/user/project-c",
		AssignedAt: now.Add(-1 * time.Hour), // 1 hour old
		LastUsedAt: now.Add(-1 * time.Hour),
	}

	// TTL of 24 hours - should remove first allocation
	removed := store.RemoveExpired(24 * time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if len(store.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(store.Allocations))
	}

	// Verify first allocation is removed
	if store.Allocations[3000] != nil {
		t.Error("port 3000 should be removed (expired)")
	}
}

func TestRemoveExpired_UsesLastUsedAt(t *testing.T) {
	now := time.Now()
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: now.Add(-48 * time.Hour), // Assigned 2 days ago
		LastUsedAt: now.Add(-1 * time.Hour),  // But used 1 hour ago
	}

	// TTL of 24 hours - should NOT remove because LastUsedAt is recent
	removed := store.RemoveExpired(24 * time.Hour)
	if removed != 0 {
		t.Errorf("expected 0 removed (LastUsedAt is recent), got %d", removed)
	}
}

func TestRemoveExpired_ZeroTTL(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: time.Now().Add(-365 * 24 * time.Hour),
	}

	// Zero TTL should not remove anything
	removed := store.RemoveExpired(0)
	if removed != 0 {
		t.Errorf("expected 0 removed with zero TTL, got %d", removed)
	}
	if len(store.Allocations) != 1 {
		t.Error("allocation should remain")
	}
}

func TestRemoveExpired_NegativeTTL(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: time.Now(),
	}

	// Negative TTL should not remove anything
	removed := store.RemoveExpired(-1 * time.Hour)
	if removed != 0 {
		t.Errorf("expected 0 removed with negative TTL, got %d", removed)
	}
}

func TestUpdateLastUsed(t *testing.T) {
	oldTime := time.Now().Add(-24 * time.Hour)
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: oldTime,
		LastUsedAt: oldTime,
	}
	store.Allocations[3001] = &AllocationInfo{
		Directory:  "/home/user/project-b",
		AssignedAt: oldTime,
		LastUsedAt: oldTime,
	}

	// Update existing
	found := store.UpdateLastUsed("/home/user/project-a")
	if !found {
		t.Error("expected to find allocation")
	}

	// Verify timestamp was updated
	if store.Allocations[3000].LastUsedAt.Before(time.Now().Add(-1 * time.Second)) {
		t.Error("LastUsedAt should be updated to now")
	}
	// Verify other allocation unchanged
	if !store.Allocations[3001].LastUsedAt.Equal(oldTime) {
		t.Error("other allocation should not be modified")
	}

	// Update non-existent
	found = store.UpdateLastUsed("/home/user/project-x")
	if found {
		t.Error("should not find non-existent allocation")
	}
}

func TestUpdateLastUsed_PathNormalization(t *testing.T) {
	oldTime := time.Now().Add(-24 * time.Hour)
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project",
		AssignedAt: oldTime,
		LastUsedAt: oldTime,
	}

	// Update with trailing slash
	found := store.UpdateLastUsed("/home/user/project/")
	if !found {
		t.Error("expected to find allocation with trailing slash")
	}
	if store.Allocations[3000].LastUsedAt.Equal(oldTime) {
		t.Error("LastUsedAt should be updated")
	}
}

func TestSetLocked(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", Locked: false}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", Locked: false}

	// Lock existing allocation
	found := store.SetLocked("/home/user/project-a", true)
	if !found {
		t.Error("expected to find allocation")
	}
	if !store.Allocations[3000].Locked {
		t.Error("allocation should be locked")
	}

	// Unlock it
	found = store.SetLocked("/home/user/project-a", false)
	if !found {
		t.Error("expected to find allocation")
	}
	if store.Allocations[3000].Locked {
		t.Error("allocation should be unlocked")
	}

	// Try to lock non-existent
	found = store.SetLocked("/home/user/project-x", true)
	if found {
		t.Error("should not find non-existent allocation")
	}
}

func TestSetLocked_PathNormalization(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project", Locked: false}

	// Lock with trailing slash
	found := store.SetLocked("/home/user/project/", true)
	if !found {
		t.Error("expected to find allocation with trailing slash")
	}
	if !store.Allocations[3000].Locked {
		t.Error("allocation should be locked")
	}
}

func TestSetLockedByPort(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", Locked: false}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", Locked: false}

	// Lock by port
	found := store.SetLockedByPort(3000, true)
	if !found {
		t.Error("expected to find allocation")
	}
	if !store.Allocations[3000].Locked {
		t.Error("allocation should be locked")
	}
	if store.Allocations[3001].Locked {
		t.Error("other allocation should not be locked")
	}

	// Unlock by port
	found = store.SetLockedByPort(3000, false)
	if !found {
		t.Error("expected to find allocation")
	}
	if store.Allocations[3000].Locked {
		t.Error("allocation should be unlocked")
	}

	// Try non-existent port
	found = store.SetLockedByPort(9999, true)
	if found {
		t.Error("should not find non-existent port")
	}
}

func TestIsPortLocked(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", Locked: true}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", Locked: false}

	tests := []struct {
		name       string
		port       int
		currentDir string
		expected   bool
	}{
		// Locked port from different directory - should be locked
		{"locked port from other dir", 3000, "/home/user/project-b", true},
		// Locked port from same directory - should not be locked (can use own port)
		{"locked port from same dir", 3000, "/home/user/project-a", false},
		// Unlocked port - should not be locked
		{"unlocked port", 3001, "/home/user/project-a", false},
		// Non-existent port - should not be locked
		{"non-existent port", 9999, "/home/user/project-a", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := store.IsPortLocked(tc.port, tc.currentDir)
			if result != tc.expected {
				t.Errorf("IsPortLocked(%d, %s): expected %v, got %v", tc.port, tc.currentDir, tc.expected, result)
			}
		})
	}
}

func TestIsPortLocked_PathNormalization(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project", Locked: true}

	// Same directory with trailing slash - should not be locked
	result := store.IsPortLocked(3000, "/home/user/project/")
	if result {
		t.Error("port should not be locked for same directory (with trailing slash)")
	}
}

func TestSaveAndLoadWithLocked(t *testing.T) {
	tmpDir := t.TempDir()

	original := NewStore()
	original.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", Locked: true}
	original.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", Locked: false}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if len(loaded.Allocations) != 2 {
		t.Fatalf("expected 2 allocations, got %d", len(loaded.Allocations))
	}

	if !loaded.Allocations[3000].Locked {
		t.Error("port 3000 should be locked")
	}
	if loaded.Allocations[3001].Locked {
		t.Error("port 3001 should not be locked")
	}
}

func TestGetLockedPortsForExclusion(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", Locked: true}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", Locked: true}
	store.Allocations[3002] = &AllocationInfo{Directory: "/home/user/project-c", Locked: false}
	store.Allocations[3003] = &AllocationInfo{Directory: "/home/user/project-d", Locked: true}

	tests := []struct {
		name          string
		currentDir    string
		expectedPorts []int
	}{
		{
			name:          "from project-a - excludes other locked ports",
			currentDir:    "/home/user/project-a",
			expectedPorts: []int{3001, 3003}, // project-b and project-d are locked
		},
		{
			name:          "from project-b - excludes other locked ports",
			currentDir:    "/home/user/project-b",
			expectedPorts: []int{3000, 3003}, // project-a and project-d are locked
		},
		{
			name:          "from project-c - excludes all locked ports",
			currentDir:    "/home/user/project-c",
			expectedPorts: []int{3000, 3001, 3003}, // all locked except own
		},
		{
			name:          "from unknown directory - excludes all locked ports",
			currentDir:    "/home/user/project-x",
			expectedPorts: []int{3000, 3001, 3003},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := store.GetLockedPortsForExclusion(tc.currentDir)

			// Check count
			if len(result) != len(tc.expectedPorts) {
				t.Errorf("expected %d locked ports, got %d", len(tc.expectedPorts), len(result))
			}

			// Check each expected port is present
			for _, port := range tc.expectedPorts {
				if !result[port] {
					t.Errorf("expected port %d to be in exclusion set", port)
				}
			}
		})
	}
}

func TestGetLockedPortsForExclusion_PathNormalization(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project", Locked: true}

	// Query with trailing slash - should recognize as same directory
	result := store.GetLockedPortsForExclusion("/home/user/project/")
	if len(result) != 0 {
		t.Error("own directory (with trailing slash) should not be in exclusion set")
	}

	// Query from different directory
	result = store.GetLockedPortsForExclusion("/home/user/other")
	if len(result) != 1 || !result[3000] {
		t.Error("locked port from other directory should be in exclusion set")
	}
}

func TestGetLockedPortsForExclusion_EmptyStore(t *testing.T) {
	store := NewStore()

	result := store.GetLockedPortsForExclusion("/home/user/project")
	if len(result) != 0 {
		t.Error("empty store should return empty exclusion set")
	}
}

func TestGetLockedPortsForExclusion_NoLockedPorts(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", Locked: false}
	store.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", Locked: false}

	result := store.GetLockedPortsForExclusion("/home/user/project-c")
	if len(result) != 0 {
		t.Error("no locked ports should return empty exclusion set")
	}
}

func TestSetUnknownPortAllocation(t *testing.T) {
	store := NewStore()

	// Add first unknown port
	store.SetUnknownPortAllocation(3007, "")
	if len(store.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(store.Allocations))
	}
	info := store.Allocations[3007]
	if info == nil {
		t.Fatal("expected allocation for port 3007")
	}
	if info.Directory != "(unknown:3007)" {
		t.Errorf("expected directory (unknown:3007), got %s", info.Directory)
	}

	// Add second unknown port
	store.SetUnknownPortAllocation(3010, "")
	if len(store.Allocations) != 2 {
		t.Fatalf("expected 2 allocations, got %d", len(store.Allocations))
	}
	info = store.Allocations[3010]
	if info == nil {
		t.Fatal("expected allocation for port 3010")
	}
	if info.Directory != "(unknown:3010)" {
		t.Errorf("expected directory (unknown:3010), got %s", info.Directory)
	}

	// Verify first allocation is still intact
	if store.Allocations[3007] == nil {
		t.Error("first allocation was removed")
	}
}

func TestSetUnknownPortAllocation_FindByPort(t *testing.T) {
	store := NewStore()

	store.SetUnknownPortAllocation(3007, "")

	// Should be findable by port
	alloc := store.FindByPort(3007)
	if alloc == nil {
		t.Fatal("expected to find allocation by port")
	}
	if alloc.Directory != "(unknown:3007)" {
		t.Errorf("expected directory (unknown:3007), got %s", alloc.Directory)
	}
}

func TestSetUnknownPortAllocation_AssignedAtIsSet(t *testing.T) {
	store := NewStore()

	before := time.Now().Add(-1 * time.Second)
	store.SetUnknownPortAllocation(3007, "")
	after := time.Now().Add(1 * time.Second)

	info := store.Allocations[3007]
	if info.AssignedAt.IsZero() {
		t.Error("AssignedAt should be set")
	}
	if info.AssignedAt.Before(before) || info.AssignedAt.After(after) {
		t.Error("AssignedAt should be approximately now")
	}
}

func TestSetUnknownPortAllocation_RemoveByDirectory(t *testing.T) {
	store := NewStore()

	store.SetUnknownPortAllocation(3007, "")

	// Should be removable by directory
	removed, found := store.RemoveByDirectory("(unknown:3007)")
	if !found {
		t.Fatal("expected to find allocation by directory")
	}
	if removed.Port != 3007 {
		t.Errorf("expected port 3007, got %d", removed.Port)
	}
	if len(store.Allocations) != 0 {
		t.Error("allocation should be removed")
	}
}

func TestSetAllocationWithProcess_New(t *testing.T) {
	store := NewStore()

	store.SetAllocationWithProcess("/home/user/project-a", 3000, "ruby")

	if len(store.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(store.Allocations))
	}

	info := store.Allocations[3000]
	if info == nil {
		t.Fatal("expected allocation for port 3000")
	}
	if info.Directory != "/home/user/project-a" {
		t.Errorf("expected dir /home/user/project-a, got %s", info.Directory)
	}
	if info.ProcessName != "ruby" {
		t.Errorf("expected process_name 'ruby', got %q", info.ProcessName)
	}
}

func TestSetAllocationWithProcess_UpdatesExisting(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:   "/home/user/project-a",
		ProcessName: "old-process",
	}

	// Update with new port and process name - old port should be removed
	store.SetAllocationWithProcess("/home/user/project-a", 3005, "new-process")

	if len(store.Allocations) != 1 {
		t.Fatalf("expected 1 allocation after update, got %d", len(store.Allocations))
	}

	// Old port should be removed
	if store.Allocations[3000] != nil {
		t.Error("old port 3000 should be removed")
	}

	// New port should exist with new process name
	info := store.Allocations[3005]
	if info == nil {
		t.Fatal("expected allocation for new port 3005")
	}
	if info.ProcessName != "new-process" {
		t.Errorf("expected process_name 'new-process', got %q", info.ProcessName)
	}
}

func TestSetAllocationWithProcess_EmptyProcessNameDoesNotOverwrite(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:   "/home/user/project-a",
		ProcessName: "ruby",
	}

	// Update same port with empty process name should NOT overwrite existing
	store.SetAllocationWithProcess("/home/user/project-a", 3000, "")

	if store.Allocations[3000].ProcessName != "ruby" {
		t.Errorf("expected process_name to remain 'ruby', got %q", store.Allocations[3000].ProcessName)
	}
}

func TestSetUnknownPortAllocation_WithProcessName(t *testing.T) {
	store := NewStore()

	store.SetUnknownPortAllocation(3007, "docker-proxy")

	info := store.Allocations[3007]
	if info == nil {
		t.Fatal("expected allocation for port 3007")
	}
	if info.Directory != "(unknown:3007)" {
		t.Errorf("expected directory (unknown:3007), got %s", info.Directory)
	}
	if info.ProcessName != "docker-proxy" {
		t.Errorf("expected process_name 'docker-proxy', got %q", info.ProcessName)
	}
}

func TestSaveAndLoadWithProcessName(t *testing.T) {
	tmpDir := t.TempDir()

	original := NewStore()
	original.Allocations[3000] = &AllocationInfo{Directory: "/home/user/project-a", ProcessName: "ruby"}
	original.Allocations[3001] = &AllocationInfo{Directory: "/home/user/project-b", ProcessName: ""}
	original.Allocations[3002] = &AllocationInfo{Directory: "(unknown:3002)", ProcessName: "docker-proxy"}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if len(loaded.Allocations) != 3 {
		t.Fatalf("expected 3 allocations, got %d", len(loaded.Allocations))
	}

	if loaded.Allocations[3000].ProcessName != "ruby" {
		t.Errorf("expected process_name 'ruby', got %q", loaded.Allocations[3000].ProcessName)
	}
	if loaded.Allocations[3001].ProcessName != "" {
		t.Errorf("expected empty process_name, got %q", loaded.Allocations[3001].ProcessName)
	}
	if loaded.Allocations[3002].ProcessName != "docker-proxy" {
		t.Errorf("expected process_name 'docker-proxy', got %q", loaded.Allocations[3002].ProcessName)
	}
}

func TestGetLastIssuedPort(t *testing.T) {
	store := NewStore()
	if store.GetLastIssuedPort() != 0 {
		t.Errorf("expected 0, got %d", store.GetLastIssuedPort())
	}

	store.SetLastIssuedPort(3005)
	if store.GetLastIssuedPort() != 3005 {
		t.Errorf("expected 3005, got %d", store.GetLastIssuedPort())
	}
}

func TestGetFrozenPorts(t *testing.T) {
	now := time.Now()
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: now.Add(-30 * time.Minute),
		LastUsedAt: now.Add(-30 * time.Minute),
	}
	store.Allocations[3001] = &AllocationInfo{
		Directory:  "/home/user/project-b",
		AssignedAt: now.Add(-90 * time.Minute),
		LastUsedAt: now.Add(-90 * time.Minute),
	}
	store.Allocations[3002] = &AllocationInfo{
		Directory:  "/home/user/project-c",
		AssignedAt: now.Add(-5 * time.Minute),
		LastUsedAt: now.Add(-5 * time.Minute),
	}

	// Freeze period of 60 minutes
	frozen := store.GetFrozenPorts(60)

	// Should include ports used within last 60 minutes
	if len(frozen) != 2 {
		t.Errorf("expected 2 frozen ports, got %d", len(frozen))
	}
	if !frozen[3000] {
		t.Error("port 3000 should be frozen")
	}
	if !frozen[3002] {
		t.Error("port 3002 should be frozen")
	}
	if frozen[3001] {
		t.Error("port 3001 should NOT be frozen (90 min > 60 min)")
	}
}

func TestGetFrozenPorts_ZeroFreezePeriod(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:  "/home/user/project-a",
		AssignedAt: time.Now(),
	}

	frozen := store.GetFrozenPorts(0)
	if len(frozen) != 0 {
		t.Error("zero freeze period should return empty map")
	}
}

func TestCount(t *testing.T) {
	store := NewStore()
	if store.Count() != 0 {
		t.Errorf("expected 0, got %d", store.Count())
	}

	store.Allocations[3000] = &AllocationInfo{Directory: "/a"}
	store.Allocations[3001] = &AllocationInfo{Directory: "/b"}

	if store.Count() != 2 {
		t.Errorf("expected 2, got %d", store.Count())
	}
}

func TestWithStore(t *testing.T) {
	tmpDir := t.TempDir()

	// First call - create new store and add allocation
	err := WithStore(tmpDir, func(store *Store) error {
		store.SetAllocation("/home/user/project-a", 3000)
		store.SetLastIssuedPort(3000)
		return nil
	})
	if err != nil {
		t.Fatalf("WithStore failed: %v", err)
	}

	// Second call - verify data persisted and add another
	err = WithStore(tmpDir, func(store *Store) error {
		if store.Count() != 1 {
			t.Errorf("expected 1 allocation, got %d", store.Count())
		}
		if store.GetLastIssuedPort() != 3000 {
			t.Errorf("expected last issued port 3000, got %d", store.GetLastIssuedPort())
		}
		store.SetAllocation("/home/user/project-b", 3001)
		return nil
	})
	if err != nil {
		t.Fatalf("WithStore failed: %v", err)
	}

	// Third call - verify both allocations exist
	err = WithStore(tmpDir, func(store *Store) error {
		if store.Count() != 2 {
			t.Errorf("expected 2 allocations, got %d", store.Count())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithStore failed: %v", err)
	}
}

func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func TestWithStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	var wg sync.WaitGroup
	var successCount atomic.Int32
	const goroutines = 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		port := 3000 + i
		go func(p int) {
			defer wg.Done()
			err := WithStore(tmpDir, func(store *Store) error {
				time.Sleep(10 * time.Millisecond) // Simulate work
				store.SetAllocation(fmt.Sprintf("/project-%d", p), p)
				return nil
			})
			if err == nil {
				successCount.Add(1)
			}
		}(port)
	}
	wg.Wait()

	if int(successCount.Load()) != goroutines {
		t.Errorf("expected %d successful operations, got %d", goroutines, successCount.Load())
	}

	store, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if store.Count() != goroutines {
		t.Errorf("expected %d allocations, got %d", goroutines, store.Count())
	}
}

func TestWithStore_ErrorDoesNotSave(t *testing.T) {
	tmpDir := t.TempDir()

	// First call - create initial state
	err := WithStore(tmpDir, func(store *Store) error {
		store.SetAllocation("/project-a", 3000)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second call with error - changes should NOT be saved
	expectedErr := errors.New("intentional error")
	err = WithStore(tmpDir, func(store *Store) error {
		store.SetAllocation("/project-b", 3001) // This should NOT be saved
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Verify project-b was NOT saved
	loaded, loadErr := Load(tmpDir)
	if loadErr != nil {
		t.Fatalf("failed to load: %v", loadErr)
	}
	if loaded.Count() != 1 {
		t.Errorf("expected 1 allocation, got %d", loaded.Count())
	}
	if loaded.FindByPort(3001) != nil {
		t.Error("project-b should NOT be saved after error")
	}
}

func TestWithStore_CorruptedFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, allocationsFileName)

	// Write corrupted YAML
	if err := os.WriteFile(path, []byte("not: valid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}

	// WithStore should return error for corrupted file
	err := WithStore(tmpDir, func(store *Store) error {
		t.Error("callback should not be called for corrupted file")
		return nil
	})
	if err == nil {
		t.Error("expected error for corrupted file")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Errorf("expected 'corrupted' in error message, got: %v", err)
	}
}

func TestSaveAndLoadWithContainerID(t *testing.T) {
	tmpDir := t.TempDir()

	original := NewStore()
	original.Allocations[3000] = &AllocationInfo{
		Directory:   "/home/user/project-a",
		ContainerID: "abc123def456",
		ProcessName: "docker-proxy",
	}
	original.Allocations[3001] = &AllocationInfo{
		Directory:   "/home/user/project-b",
		ContainerID: "", // Empty container ID
		ProcessName: "node",
	}
	original.Allocations[3002] = &AllocationInfo{
		Directory:   "/home/user/project-c",
		ContainerID: "xyz789",
		ProcessName: "docker-proxy",
	}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}
	if len(loaded.Allocations) != 3 {
		t.Fatalf("expected 3 allocations, got %d", len(loaded.Allocations))
	}

	// Verify ContainerID persisted correctly
	if loaded.Allocations[3000].ContainerID != "abc123def456" {
		t.Errorf("expected container_id 'abc123def456', got %q", loaded.Allocations[3000].ContainerID)
	}
	if loaded.Allocations[3001].ContainerID != "" {
		t.Errorf("expected empty container_id, got %q", loaded.Allocations[3001].ContainerID)
	}
	if loaded.Allocations[3002].ContainerID != "xyz789" {
		t.Errorf("expected container_id 'xyz789', got %q", loaded.Allocations[3002].ContainerID)
	}
}

func TestFindByDirectory_IncludesContainerID(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:   "/home/user/project-a",
		ContainerID: "container123",
		ProcessName: "docker-proxy",
	}
	store.Allocations[3001] = &AllocationInfo{
		Directory:   "/home/user/project-b",
		ContainerID: "",
		ProcessName: "node",
	}

	// Test with ContainerID set
	result := store.FindByDirectory("/home/user/project-a")
	if result == nil {
		t.Fatal("expected to find allocation")
	}
	if result.ContainerID != "container123" {
		t.Errorf("expected ContainerID 'container123', got %q", result.ContainerID)
	}

	// Test with empty ContainerID
	result = store.FindByDirectory("/home/user/project-b")
	if result == nil {
		t.Fatal("expected to find allocation")
	}
	if result.ContainerID != "" {
		t.Errorf("expected empty ContainerID, got %q", result.ContainerID)
	}
}

func TestFindByPort_IncludesContainerID(t *testing.T) {
	store := NewStore()
	store.Allocations[3000] = &AllocationInfo{
		Directory:   "/home/user/project-a",
		ContainerID: "container456",
		ProcessName: "docker-proxy",
	}
	store.Allocations[3001] = &AllocationInfo{
		Directory:   "/home/user/project-b",
		ContainerID: "",
		ProcessName: "node",
	}

	// Test with ContainerID set
	result := store.FindByPort(3000)
	if result == nil {
		t.Fatal("expected to find allocation")
	}
	if result.ContainerID != "container456" {
		t.Errorf("expected ContainerID 'container456', got %q", result.ContainerID)
	}

	// Test with empty ContainerID
	result = store.FindByPort(3001)
	if result == nil {
		t.Fatal("expected to find allocation")
	}
	if result.ContainerID != "" {
		t.Errorf("expected empty ContainerID, got %q", result.ContainerID)
	}
}

func TestAddAllocationForScan_ContainerIDUpdate(t *testing.T) {
	t.Run("sets ContainerID on new allocation", func(t *testing.T) {
		store := NewStore()
		store.AddAllocationForScan("/home/user/project", 3000, "docker-proxy", "container123")

		info := store.Allocations[3000]
		if info == nil {
			t.Fatal("expected allocation for port 3000")
		}
		if info.ContainerID != "container123" {
			t.Errorf("expected ContainerID 'container123', got %q", info.ContainerID)
		}
	})

	t.Run("updates ContainerID on existing port", func(t *testing.T) {
		store := NewStore()
		store.Allocations[3000] = &AllocationInfo{
			Directory:   "/home/user/project-a",
			ContainerID: "old-container",
			ProcessName: "docker-proxy",
		}

		// Update with new ContainerID
		store.AddAllocationForScan("/home/user/project-b", 3000, "docker-proxy", "new-container")

		info := store.Allocations[3000]
		if info.ContainerID != "new-container" {
			t.Errorf("expected ContainerID 'new-container', got %q", info.ContainerID)
		}
	})

	t.Run("empty ContainerID does not overwrite existing", func(t *testing.T) {
		store := NewStore()
		store.Allocations[3000] = &AllocationInfo{
			Directory:   "/home/user/project-a",
			ContainerID: "existing-container",
			ProcessName: "docker-proxy",
		}

		// Update with empty ContainerID - should NOT overwrite
		store.AddAllocationForScan("/home/user/project-b", 3000, "node", "")

		info := store.Allocations[3000]
		if info.ContainerID != "existing-container" {
			t.Errorf("expected ContainerID to remain 'existing-container', got %q", info.ContainerID)
		}
	})

	t.Run("empty ContainerID on new allocation remains empty", func(t *testing.T) {
		store := NewStore()
		store.AddAllocationForScan("/home/user/project", 3000, "node", "")

		info := store.Allocations[3000]
		if info == nil {
			t.Fatal("expected allocation for port 3000")
		}
		if info.ContainerID != "" {
			t.Errorf("expected empty ContainerID, got %q", info.ContainerID)
		}
	})
}
