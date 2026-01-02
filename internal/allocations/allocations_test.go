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

func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}
