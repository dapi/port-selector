package allocations

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	list := Load(tmpDir)
	if len(list.Allocations) != 0 {
		t.Errorf("expected empty list, got %d allocations", len(list.Allocations))
	}
}

func TestLoadCorrupted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, allocationsFileName)

	// Write corrupted YAML
	if err := os.WriteFile(path, []byte("not: valid: yaml: ["), 0644); err != nil {
		t.Fatal(err)
	}

	list := Load(tmpDir)
	if len(list.Allocations) != 0 {
		t.Errorf("expected empty list for corrupted file, got %d allocations", len(list.Allocations))
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()

	original := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", AssignedAt: time.Now().UTC()},
			{Port: 3001, Directory: "/home/user/project-b", AssignedAt: time.Now().UTC()},
		},
	}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded := Load(tmpDir)
	if len(loaded.Allocations) != len(original.Allocations) {
		t.Errorf("expected %d allocations, got %d", len(original.Allocations), len(loaded.Allocations))
	}

	for i, alloc := range loaded.Allocations {
		if alloc.Port != original.Allocations[i].Port {
			t.Errorf("allocation %d: expected port %d, got %d", i, original.Allocations[i].Port, alloc.Port)
		}
		if alloc.Directory != original.Allocations[i].Directory {
			t.Errorf("allocation %d: expected dir %s, got %s", i, original.Allocations[i].Directory, alloc.Directory)
		}
	}
}

func TestFindByDirectory(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a"},
			{Port: 3001, Directory: "/home/user/project-b"},
		},
	}

	tests := []struct {
		dir      string
		expected *int
	}{
		{"/home/user/project-a", intPtr(3000)},
		{"/home/user/project-b", intPtr(3001)},
		{"/home/user/project-c", nil},
	}

	for _, tc := range tests {
		result := list.FindByDirectory(tc.dir)
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
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a"},
			{Port: 3001, Directory: "/home/user/project-b"},
		},
	}

	tests := []struct {
		port     int
		expected *string
	}{
		{3000, strPtr("/home/user/project-a")},
		{3001, strPtr("/home/user/project-b")},
		{3002, nil},
	}

	for _, tc := range tests {
		result := list.FindByPort(tc.port)
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
	list := &AllocationList{}

	list.SetAllocation("/home/user/project-a", 3000)

	if len(list.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(list.Allocations))
	}

	if list.Allocations[0].Port != 3000 {
		t.Errorf("expected port 3000, got %d", list.Allocations[0].Port)
	}
	if list.Allocations[0].Directory != "/home/user/project-a" {
		t.Errorf("expected dir /home/user/project-a, got %s", list.Allocations[0].Directory)
	}
}

func TestSetAllocationUpdate(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", AssignedAt: time.Now().Add(-1 * time.Hour)},
		},
	}

	list.SetAllocation("/home/user/project-a", 3005)

	if len(list.Allocations) != 1 {
		t.Fatalf("expected 1 allocation after update, got %d", len(list.Allocations))
	}

	if list.Allocations[0].Port != 3005 {
		t.Errorf("expected port 3005 after update, got %d", list.Allocations[0].Port)
	}
}

func TestFindByDirectory_PathNormalization(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project"},
		},
	}

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
			result := list.FindByDirectory(tc.input)
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
	yamlContent := `allocations:
  - port: 3000
    directory: /home/user/project/
    assigned_at: 2025-01-02T10:30:00Z
  - port: 3001
    directory: /home/user//other
    assigned_at: 2025-01-02T11:00:00Z
`
	if err := os.WriteFile(path, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	list := Load(tmpDir)

	// Verify paths are normalized after load
	if list.Allocations[0].Directory != "/home/user/project" {
		t.Errorf("expected normalized path /home/user/project, got %s", list.Allocations[0].Directory)
	}
	if list.Allocations[1].Directory != "/home/user/other" {
		t.Errorf("expected normalized path /home/user/other, got %s", list.Allocations[1].Directory)
	}

	// Verify FindByDirectory works with normalized search
	result := list.FindByDirectory("/home/user/project")
	if result == nil {
		t.Fatal("FindByDirectory failed for normalized path")
	}
	if result.Port != 3000 {
		t.Errorf("expected port 3000, got %d", result.Port)
	}
}

func TestSortedByPort(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3005, Directory: "/home/user/project-c"},
			{Port: 3000, Directory: "/home/user/project-a"},
			{Port: 3002, Directory: "/home/user/project-b"},
		},
	}

	sorted := list.SortedByPort()

	expectedPorts := []int{3000, 3002, 3005}
	for i, alloc := range sorted {
		if alloc.Port != expectedPorts[i] {
			t.Errorf("sorted[%d]: expected port %d, got %d", i, expectedPorts[i], alloc.Port)
		}
	}

	// Verify original list is unchanged
	if list.Allocations[0].Port != 3005 {
		t.Error("original list was modified")
	}
}

func TestRemoveByDirectory(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a"},
			{Port: 3001, Directory: "/home/user/project-b"},
			{Port: 3002, Directory: "/home/user/project-c"},
		},
	}

	// Remove existing directory
	removed, found := list.RemoveByDirectory("/home/user/project-b")
	if !found {
		t.Fatal("expected to find allocation")
	}
	if removed.Port != 3001 {
		t.Errorf("expected removed port 3001, got %d", removed.Port)
	}
	if len(list.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(list.Allocations))
	}

	// Verify remaining allocations
	if list.Allocations[0].Port != 3000 || list.Allocations[1].Port != 3002 {
		t.Error("wrong allocations remaining")
	}

	// Try to remove non-existent directory
	_, found = list.RemoveByDirectory("/home/user/project-x")
	if found {
		t.Error("should not find non-existent allocation")
	}
}

func TestRemoveByDirectory_PathNormalization(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project"},
		},
	}

	// Remove with trailing slash
	removed, found := list.RemoveByDirectory("/home/user/project/")
	if !found {
		t.Fatal("expected to find allocation with trailing slash")
	}
	if removed.Port != 3000 {
		t.Errorf("expected port 3000, got %d", removed.Port)
	}
	if len(list.Allocations) != 0 {
		t.Error("allocation should be removed")
	}
}

func TestRemoveAll(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a"},
			{Port: 3001, Directory: "/home/user/project-b"},
			{Port: 3002, Directory: "/home/user/project-c"},
		},
	}

	count := list.RemoveAll()
	if count != 3 {
		t.Errorf("expected 3 removed, got %d", count)
	}
	if len(list.Allocations) != 0 {
		t.Errorf("expected empty list, got %d", len(list.Allocations))
	}

	// Remove from empty list
	count = list.RemoveAll()
	if count != 0 {
		t.Errorf("expected 0 removed from empty list, got %d", count)
	}
}

func TestRemoveExpired(t *testing.T) {
	now := time.Now()
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", AssignedAt: now.Add(-48 * time.Hour)}, // 2 days old
			{Port: 3001, Directory: "/home/user/project-b", AssignedAt: now.Add(-12 * time.Hour)}, // 12 hours old
			{Port: 3002, Directory: "/home/user/project-c", AssignedAt: now.Add(-1 * time.Hour)},  // 1 hour old
		},
	}

	// TTL of 24 hours - should remove first allocation
	removed := list.RemoveExpired(24 * time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if len(list.Allocations) != 2 {
		t.Errorf("expected 2 allocations, got %d", len(list.Allocations))
	}

	// Verify remaining allocations
	if list.Allocations[0].Port != 3001 || list.Allocations[1].Port != 3002 {
		t.Error("wrong allocations remaining")
	}
}

func TestRemoveExpired_ZeroTTL(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", AssignedAt: time.Now().Add(-365 * 24 * time.Hour)},
		},
	}

	// Zero TTL should not remove anything
	removed := list.RemoveExpired(0)
	if removed != 0 {
		t.Errorf("expected 0 removed with zero TTL, got %d", removed)
	}
	if len(list.Allocations) != 1 {
		t.Error("allocation should remain")
	}
}

func TestRemoveExpired_NegativeTTL(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", AssignedAt: time.Now()},
		},
	}

	// Negative TTL should not remove anything
	removed := list.RemoveExpired(-1 * time.Hour)
	if removed != 0 {
		t.Errorf("expected 0 removed with negative TTL, got %d", removed)
	}
}

func TestUpdateLastUsed(t *testing.T) {
	oldTime := time.Now().Add(-24 * time.Hour)
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", AssignedAt: oldTime},
			{Port: 3001, Directory: "/home/user/project-b", AssignedAt: oldTime},
		},
	}

	// Update existing
	found := list.UpdateLastUsed("/home/user/project-a")
	if !found {
		t.Error("expected to find allocation")
	}

	// Verify timestamp was updated
	if list.Allocations[0].AssignedAt.Before(time.Now().Add(-1 * time.Second)) {
		t.Error("AssignedAt should be updated to now")
	}
	// Verify other allocation unchanged
	if !list.Allocations[1].AssignedAt.Equal(oldTime) {
		t.Error("other allocation should not be modified")
	}

	// Update non-existent
	found = list.UpdateLastUsed("/home/user/project-x")
	if found {
		t.Error("should not find non-existent allocation")
	}
}

func TestUpdateLastUsed_PathNormalization(t *testing.T) {
	oldTime := time.Now().Add(-24 * time.Hour)
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project", AssignedAt: oldTime},
		},
	}

	// Update with trailing slash
	found := list.UpdateLastUsed("/home/user/project/")
	if !found {
		t.Error("expected to find allocation with trailing slash")
	}
	if list.Allocations[0].AssignedAt.Equal(oldTime) {
		t.Error("AssignedAt should be updated")
	}
}

func TestSetLocked(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", Locked: false},
			{Port: 3001, Directory: "/home/user/project-b", Locked: false},
		},
	}

	// Lock existing allocation
	found := list.SetLocked("/home/user/project-a", true)
	if !found {
		t.Error("expected to find allocation")
	}
	if !list.Allocations[0].Locked {
		t.Error("allocation should be locked")
	}

	// Unlock it
	found = list.SetLocked("/home/user/project-a", false)
	if !found {
		t.Error("expected to find allocation")
	}
	if list.Allocations[0].Locked {
		t.Error("allocation should be unlocked")
	}

	// Try to lock non-existent
	found = list.SetLocked("/home/user/project-x", true)
	if found {
		t.Error("should not find non-existent allocation")
	}
}

func TestSetLocked_PathNormalization(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project", Locked: false},
		},
	}

	// Lock with trailing slash
	found := list.SetLocked("/home/user/project/", true)
	if !found {
		t.Error("expected to find allocation with trailing slash")
	}
	if !list.Allocations[0].Locked {
		t.Error("allocation should be locked")
	}
}

func TestSetLocked_Idempotent(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project", Locked: false},
		},
	}

	// Lock twice should succeed both times (idempotent)
	found1 := list.SetLocked("/home/user/project", true)
	if !found1 {
		t.Error("first lock should succeed")
	}
	if !list.Allocations[0].Locked {
		t.Error("allocation should be locked after first call")
	}

	found2 := list.SetLocked("/home/user/project", true)
	if !found2 {
		t.Error("second lock should also succeed (idempotent)")
	}
	if !list.Allocations[0].Locked {
		t.Error("allocation should still be locked after second call")
	}

	// Unlock twice should also be idempotent
	found3 := list.SetLocked("/home/user/project", false)
	if !found3 {
		t.Error("first unlock should succeed")
	}
	if list.Allocations[0].Locked {
		t.Error("allocation should be unlocked after first call")
	}

	found4 := list.SetLocked("/home/user/project", false)
	if !found4 {
		t.Error("second unlock should also succeed (idempotent)")
	}
	if list.Allocations[0].Locked {
		t.Error("allocation should still be unlocked after second call")
	}
}

func TestSetLockedByPort(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", Locked: false},
			{Port: 3001, Directory: "/home/user/project-b", Locked: false},
		},
	}

	// Lock by port
	found := list.SetLockedByPort(3000, true)
	if !found {
		t.Error("expected to find allocation")
	}
	if !list.Allocations[0].Locked {
		t.Error("allocation should be locked")
	}
	if list.Allocations[1].Locked {
		t.Error("other allocation should not be locked")
	}

	// Unlock by port
	found = list.SetLockedByPort(3000, false)
	if !found {
		t.Error("expected to find allocation")
	}
	if list.Allocations[0].Locked {
		t.Error("allocation should be unlocked")
	}

	// Try non-existent port
	found = list.SetLockedByPort(9999, true)
	if found {
		t.Error("should not find non-existent port")
	}
}

func TestIsPortLocked(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", Locked: true},
			{Port: 3001, Directory: "/home/user/project-b", Locked: false},
		},
	}

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
			result := list.IsPortLocked(tc.port, tc.currentDir)
			if result != tc.expected {
				t.Errorf("IsPortLocked(%d, %s): expected %v, got %v", tc.port, tc.currentDir, tc.expected, result)
			}
		})
	}
}

func TestIsPortLocked_PathNormalization(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project", Locked: true},
		},
	}

	// Same directory with trailing slash - should not be locked
	result := list.IsPortLocked(3000, "/home/user/project/")
	if result {
		t.Error("port should not be locked for same directory (with trailing slash)")
	}
}

func TestSaveAndLoadWithLocked(t *testing.T) {
	tmpDir := t.TempDir()

	original := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", Locked: true},
			{Port: 3001, Directory: "/home/user/project-b", Locked: false},
		},
	}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded := Load(tmpDir)
	if len(loaded.Allocations) != 2 {
		t.Fatalf("expected 2 allocations, got %d", len(loaded.Allocations))
	}

	if !loaded.Allocations[0].Locked {
		t.Error("first allocation should be locked")
	}
	if loaded.Allocations[1].Locked {
		t.Error("second allocation should not be locked")
	}
}

func TestGetLockedPortsForExclusion(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", Locked: true},
			{Port: 3001, Directory: "/home/user/project-b", Locked: true},
			{Port: 3002, Directory: "/home/user/project-c", Locked: false},
			{Port: 3003, Directory: "/home/user/project-d", Locked: true},
		},
	}

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
			result := list.GetLockedPortsForExclusion(tc.currentDir)

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
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project", Locked: true},
		},
	}

	// Query with trailing slash - should recognize as same directory
	result := list.GetLockedPortsForExclusion("/home/user/project/")
	if len(result) != 0 {
		t.Error("own directory (with trailing slash) should not be in exclusion set")
	}

	// Query from different directory
	result = list.GetLockedPortsForExclusion("/home/user/other")
	if len(result) != 1 || !result[3000] {
		t.Error("locked port from other directory should be in exclusion set")
	}
}

func TestGetLockedPortsForExclusion_EmptyList(t *testing.T) {
	list := &AllocationList{}

	result := list.GetLockedPortsForExclusion("/home/user/project")
	if len(result) != 0 {
		t.Error("empty list should return empty exclusion set")
	}
}

func TestGetLockedPortsForExclusion_NoLockedPorts(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", Locked: false},
			{Port: 3001, Directory: "/home/user/project-b", Locked: false},
		},
	}

	result := list.GetLockedPortsForExclusion("/home/user/project-c")
	if len(result) != 0 {
		t.Error("no locked ports should return empty exclusion set")
	}
}

func TestSetUnknownPortAllocation(t *testing.T) {
	list := &AllocationList{}

	// Add first unknown port
	list.SetUnknownPortAllocation(3007, "")
	if len(list.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(list.Allocations))
	}
	if list.Allocations[0].Port != 3007 {
		t.Errorf("expected port 3007, got %d", list.Allocations[0].Port)
	}
	if list.Allocations[0].Directory != "(unknown:3007)" {
		t.Errorf("expected directory (unknown:3007), got %s", list.Allocations[0].Directory)
	}

	// Add second unknown port - should NOT overwrite the first
	list.SetUnknownPortAllocation(3010, "")
	if len(list.Allocations) != 2 {
		t.Fatalf("expected 2 allocations, got %d", len(list.Allocations))
	}
	if list.Allocations[1].Port != 3010 {
		t.Errorf("expected port 3010, got %d", list.Allocations[1].Port)
	}
	if list.Allocations[1].Directory != "(unknown:3010)" {
		t.Errorf("expected directory (unknown:3010), got %s", list.Allocations[1].Directory)
	}

	// Verify first allocation is still intact
	if list.Allocations[0].Port != 3007 {
		t.Error("first allocation was overwritten")
	}
}

func TestSetUnknownPortAllocation_FindByPort(t *testing.T) {
	list := &AllocationList{}

	list.SetUnknownPortAllocation(3007, "")

	// Should be findable by port
	alloc := list.FindByPort(3007)
	if alloc == nil {
		t.Fatal("expected to find allocation by port")
	}
	if alloc.Directory != "(unknown:3007)" {
		t.Errorf("expected directory (unknown:3007), got %s", alloc.Directory)
	}
}

func TestSetUnknownPortAllocation_AssignedAtIsSet(t *testing.T) {
	list := &AllocationList{}

	before := time.Now().Add(-1 * time.Second)
	list.SetUnknownPortAllocation(3007, "")
	after := time.Now().Add(1 * time.Second)

	if list.Allocations[0].AssignedAt.IsZero() {
		t.Error("AssignedAt should be set")
	}
	if list.Allocations[0].AssignedAt.Before(before) || list.Allocations[0].AssignedAt.After(after) {
		t.Error("AssignedAt should be approximately now")
	}
}

func TestSetUnknownPortAllocation_RemoveByDirectory(t *testing.T) {
	list := &AllocationList{}

	list.SetUnknownPortAllocation(3007, "")

	// Should be removable by directory
	removed, found := list.RemoveByDirectory("(unknown:3007)")
	if !found {
		t.Fatal("expected to find allocation by directory")
	}
	if removed.Port != 3007 {
		t.Errorf("expected port 3007, got %d", removed.Port)
	}
	if len(list.Allocations) != 0 {
		t.Error("allocation should be removed")
	}
}

func TestSetAllocationWithProcess_New(t *testing.T) {
	list := &AllocationList{}

	list.SetAllocationWithProcess("/home/user/project-a", 3000, "ruby")

	if len(list.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(list.Allocations))
	}

	alloc := list.Allocations[0]
	if alloc.Port != 3000 {
		t.Errorf("expected port 3000, got %d", alloc.Port)
	}
	if alloc.Directory != "/home/user/project-a" {
		t.Errorf("expected dir /home/user/project-a, got %s", alloc.Directory)
	}
	if alloc.ProcessName != "ruby" {
		t.Errorf("expected process_name 'ruby', got %q", alloc.ProcessName)
	}
}

func TestSetAllocationWithProcess_UpdatesExisting(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", ProcessName: "old-process"},
		},
	}

	// Update with new process name
	list.SetAllocationWithProcess("/home/user/project-a", 3005, "new-process")

	if len(list.Allocations) != 1 {
		t.Fatalf("expected 1 allocation after update, got %d", len(list.Allocations))
	}

	alloc := list.Allocations[0]
	if alloc.Port != 3005 {
		t.Errorf("expected port 3005, got %d", alloc.Port)
	}
	if alloc.ProcessName != "new-process" {
		t.Errorf("expected process_name 'new-process', got %q", alloc.ProcessName)
	}
}

func TestSetAllocationWithProcess_EmptyProcessNameDoesNotOverwrite(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", ProcessName: "ruby"},
		},
	}

	// Update with empty process name should NOT overwrite existing
	list.SetAllocationWithProcess("/home/user/project-a", 3005, "")

	if list.Allocations[0].ProcessName != "ruby" {
		t.Errorf("expected process_name to remain 'ruby', got %q", list.Allocations[0].ProcessName)
	}
}

func TestSetUnknownPortAllocation_WithProcessName(t *testing.T) {
	list := &AllocationList{}

	list.SetUnknownPortAllocation(3007, "docker-proxy")

	if len(list.Allocations) != 1 {
		t.Fatalf("expected 1 allocation, got %d", len(list.Allocations))
	}

	alloc := list.Allocations[0]
	if alloc.Port != 3007 {
		t.Errorf("expected port 3007, got %d", alloc.Port)
	}
	if alloc.Directory != "(unknown:3007)" {
		t.Errorf("expected directory (unknown:3007), got %s", alloc.Directory)
	}
	if alloc.ProcessName != "docker-proxy" {
		t.Errorf("expected process_name 'docker-proxy', got %q", alloc.ProcessName)
	}
}

func TestSaveAndLoadWithProcessName(t *testing.T) {
	tmpDir := t.TempDir()

	original := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", ProcessName: "ruby"},
			{Port: 3001, Directory: "/home/user/project-b", ProcessName: ""},
			{Port: 3002, Directory: "(unknown:3002)", ProcessName: "docker-proxy"},
		},
	}

	if err := Save(tmpDir, original); err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	loaded := Load(tmpDir)
	if len(loaded.Allocations) != 3 {
		t.Fatalf("expected 3 allocations, got %d", len(loaded.Allocations))
	}

	if loaded.Allocations[0].ProcessName != "ruby" {
		t.Errorf("expected process_name 'ruby', got %q", loaded.Allocations[0].ProcessName)
	}
	if loaded.Allocations[1].ProcessName != "" {
		t.Errorf("expected empty process_name, got %q", loaded.Allocations[1].ProcessName)
	}
	if loaded.Allocations[2].ProcessName != "docker-proxy" {
		t.Errorf("expected process_name 'docker-proxy', got %q", loaded.Allocations[2].ProcessName)
	}
}

func TestSetAllocationWithProcess_DirectoryPortChangeRemovesConflict(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", ProcessName: "ruby"},
			{Port: 3001, Directory: "/home/user/project-b", ProcessName: "node"},
		},
	}

	// Change project-a from port 3000 to port 3001 (which belongs to project-b)
	list.SetAllocationWithProcess("/home/user/project-a", 3001, "ruby")

	// Should have only 1 allocation now (project-b's allocation should be removed)
	if len(list.Allocations) != 1 {
		t.Errorf("expected 1 allocation, got %d", len(list.Allocations))
	}

	// The allocation should be for project-a with port 3001
	alloc := list.FindByPort(3001)
	if alloc == nil {
		t.Fatal("expected to find allocation for port 3001")
	}
	if alloc.Directory != "/home/user/project-a" {
		t.Errorf("expected directory /home/user/project-a, got %s", alloc.Directory)
	}

	// Port 3000 should be free
	if list.FindByPort(3000) != nil {
		t.Error("port 3000 should have no allocation")
	}
}

func TestSetAllocationWithProcess_DuplicatePortDifferentDirectory(t *testing.T) {
	list := &AllocationList{
		Allocations: []Allocation{
			{Port: 3000, Directory: "/home/user/project-a", ProcessName: "ruby"},
		},
	}

	// Try to allocate the same port to a different directory
	// This simulates what happens during --scan when the same port is used
	// by different processes/directories
	list.SetAllocationWithProcess("/home/user/project-b", 3000, "node")

	// Port 3000 should only appear once - the new directory should replace the old one
	count := 0
	for _, alloc := range list.Allocations {
		if alloc.Port == 3000 {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected port 3000 to appear once, got %d times", count)
	}

	// The new directory should be assigned to this port
	alloc := list.FindByPort(3000)
	if alloc == nil {
		t.Fatal("expected to find allocation for port 3000")
	}
	if alloc.Directory != "/home/user/project-b" {
		t.Errorf("expected directory /home/user/project-b, got %s", alloc.Directory)
	}
	if alloc.ProcessName != "node" {
		t.Errorf("expected process_name 'node', got %q", alloc.ProcessName)
	}
}

func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}
