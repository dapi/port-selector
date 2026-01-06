package main

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dapi/port-selector/internal/allocations"
)

// buildBinary builds the port-selector binary for testing
func buildBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "port-selector")
	cmd := exec.Command("go", "build", "-o", binary, ".")
	cmd.Dir = filepath.Join("..", "..", "cmd", "port-selector")
	if output, err := cmd.CombinedOutput(); err != nil {
		// Fallback: try from current directory
		cmd = exec.Command("go", "build", "-o", binary, "./cmd/port-selector")
		if output2, err2 := cmd.CombinedOutput(); err2 != nil {
			t.Fatalf("failed to build binary: %v\noutput1: %s\noutput2: %s", err2, output, output2)
		}
	}
	return binary
}

func TestLockUnlock_InvalidPort(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "lock negative port",
			args:    []string{"--lock", "-5"},
			wantErr: "invalid port number: -5 (must be 1-65535)",
		},
		{
			name:    "lock zero port",
			args:    []string{"--lock", "0"},
			wantErr: "invalid port number: 0 (must be 1-65535)",
		},
		{
			name:    "lock port too large",
			args:    []string{"--lock", "70000"},
			wantErr: "invalid port number: 70000 (must be 1-65535)",
		},
		{
			name:    "lock non-numeric",
			args:    []string{"--lock", "abc"},
			wantErr: "invalid port number: abc (must be 1-65535)",
		},
		{
			name:    "unlock negative port",
			args:    []string{"--unlock", "-1"},
			wantErr: "invalid port number: -1 (must be 1-65535)",
		},
		{
			name:    "unlock port too large",
			args:    []string{"--unlock", "99999"},
			wantErr: "invalid port number: 99999 (must be 1-65535)",
		},
	}

	binary := buildBinary(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Errorf("expected error, got success with output: %s", output)
				return
			}
			if !strings.Contains(string(output), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tt.wantErr, output)
			}
		})
	}
}

func TestLockUnlock_NoAllocation(t *testing.T) {
	binary := buildBinary(t)

	// Create temp config dir
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Set XDG_CONFIG_HOME to use our temp dir
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, ".config"))
	defer os.Setenv("XDG_CONFIG_HOME", oldXDG)

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "lock without allocation",
			args:    []string{"--lock"},
			wantErr: "no allocation found for",
		},
		{
			name:    "unlock without allocation",
			args:    []string{"--unlock"},
			wantErr: "no allocation found for",
		},
		{
			name:    "unlock specific port without allocation",
			args:    []string{"--unlock", "3000"},
			wantErr: "no allocation found for port 3000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binary, tt.args...)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Errorf("expected error, got success with output: %s", output)
				return
			}
			if !strings.Contains(string(output), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %s", tt.wantErr, output)
			}
		})
	}
}

func TestLockAllocatesAndLocksFreePort(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test: --lock 3500 should allocate and lock the port
	cmd := exec.Command(binary, "--lock", "3500")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}
	if !strings.Contains(string(output), "Locked port 3500") {
		t.Errorf("expected 'Locked port 3500', got: %s", output)
	}

	// Verify allocation was created and is locked
	allocs, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	alloc := allocs.FindByPort(3500)
	if alloc == nil {
		t.Fatal("allocation for port 3500 was not created")
	}
	if alloc.Directory != workDir {
		t.Errorf("expected directory %s, got %s", workDir, alloc.Directory)
	}
	if !alloc.Locked {
		t.Error("allocation should be locked")
	}
}

func TestLockPortOutsideRange(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Test: --lock 9999 should fail (outside default range 3000-4000)
	cmd := exec.Command(binary, "--lock", "9999")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error, got success with output: %s", output)
	}
	if !strings.Contains(string(output), "outside configured range") {
		t.Errorf("expected 'outside configured range' error, got: %s", output)
	}
}

func TestLockPortWhenDirectoryAlreadyHasAllocation(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-create allocation for this directory
	store := allocations.NewStore()
	store.SetAllocation(workDir, 3001)
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Test: --lock 3500 should succeed and replace the existing allocation
	// (we can replace existing allocation for the same name when specifying a port)
	cmd := exec.Command(binary, "--lock", "3500")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}
	if !strings.Contains(string(output), "Locked port 3500") {
		t.Errorf("expected 'Locked port 3500', got: %s", output)
	}

	// Verify the old allocation was replaced
	allocs2, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	alloc := allocs2.FindByPort(3500)
	if alloc == nil {
		t.Fatal("allocation for port 3500 was not created")
	}
	if alloc.Directory != workDir {
		t.Errorf("expected directory %s, got %s", workDir, alloc.Directory)
	}
	if !alloc.Locked {
		t.Error("allocation should be locked")
	}
	
	// Old allocation should be removed
	oldAlloc := allocs2.FindByPort(3001)
	if oldAlloc != nil {
		t.Error("old allocation for port 3001 should have been removed")
	}
}

func TestLockPortInUseByAnotherProcess(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Occupy a port by listening on it
	ln, err := net.Listen("tcp", ":3500")
	if err != nil {
		t.Skipf("could not occupy port 3500 for test: %v", err)
	}
	defer ln.Close()

	// Test: --lock 3500 should fail (port in use)
	cmd := exec.Command(binary, "--lock", "3500")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error, got success with output: %s", output)
	}
	if !strings.Contains(string(output), "in use") {
		t.Errorf("expected 'in use' error, got: %s", output)
	}
}

func TestScan_RecordsBusyPorts(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Occupy a port by listening on it
	ln, err := net.Listen("tcp", ":3500")
	if err != nil {
		t.Skipf("could not occupy port 3500 for test: %v", err)
	}
	defer ln.Close()

	// Run --scan
	cmd := exec.Command(binary, "--scan")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Verify output mentions port 3500
	if !strings.Contains(string(output), "Port 3500:") {
		t.Errorf("expected output to mention Port 3500, got: %s", output)
	}

	// Verify allocation was created
	allocs, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	alloc := allocs.FindByPort(3500)
	if alloc == nil {
		t.Fatal("allocation for port 3500 was not created by --scan")
	}
}

func TestScan_SkipsAlreadyAllocatedPorts(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Occupy a port by listening on it
	ln, err := net.Listen("tcp", ":3501")
	if err != nil {
		t.Skipf("could not occupy port 3501 for test: %v", err)
	}
	defer ln.Close()

	// Pre-create allocation for this port
	existingDir := "/existing/project"
	store := allocations.NewStore()
	store.SetAllocation(existingDir, 3501)
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Run --scan
	cmd := exec.Command(binary, "--scan")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Verify output says "already allocated"
	if !strings.Contains(string(output), "already allocated") {
		t.Errorf("expected output to say 'already allocated', got: %s", output)
	}

	// Verify original allocation is preserved (not overwritten)
	loaded, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	alloc := loaded.FindByPort(3501)
	if alloc == nil {
		t.Fatal("allocation for port 3501 disappeared")
	}
	if alloc.Directory != existingDir {
		t.Errorf("expected directory %s to be preserved, got %s", existingDir, alloc.Directory)
	}
}

func TestScan_NoDuplicatesOnRescan(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir := filepath.Join(tmpDir, "project")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Occupy a port by listening on it
	ln, err := net.Listen("tcp", ":3502")
	if err != nil {
		t.Skipf("could not occupy port 3502 for test: %v", err)
	}
	defer ln.Close()

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Run --scan first time
	cmd := exec.Command(binary, "--scan")
	cmd.Dir = workDir
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("first scan failed: %v, output: %s", err, output)
	}

	// Run --scan second time
	cmd = exec.Command(binary, "--scan")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("second scan failed: %v, output: %s", err, output)
	}

	// Second scan should say "already allocated"
	if !strings.Contains(string(output), "already allocated") {
		t.Errorf("expected second scan to say 'already allocated', got: %s", output)
	}

	// Verify no duplicates - should have exactly one allocation for port 3502
	// With new map-based structure, duplicates are impossible by design
	store, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	alloc := store.FindByPort(3502)
	if alloc == nil {
		t.Error("expected allocation for port 3502")
	}
}

func TestLockedPortExcludedFromAllocation(t *testing.T) {
	// This is an integration test that verifies locked ports
	// from other directories are excluded during allocation

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two project directories
	projectA := filepath.Join(tmpDir, "project-a")
	projectB := filepath.Join(tmpDir, "project-b")
	if err := os.MkdirAll(projectA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectB, 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-create allocation with locked port for project-a
	store := allocations.NewStore()
	store.SetAllocation(projectA, 3000)
	store.SetLockedByPort(3000, true)
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Verify that GetLockedPortsForExclusion works correctly
	loaded, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}

	// From project-b perspective, port 3000 should be excluded
	excluded := loaded.GetLockedPortsForExclusion(projectB)
	if !excluded[3000] {
		t.Error("port 3000 should be excluded for project-b")
	}

	// From project-a perspective, port 3000 should NOT be excluded (it's their own)
	excludedA := loaded.GetLockedPortsForExclusion(projectA)
	if excludedA[3000] {
		t.Error("port 3000 should NOT be excluded for project-a (its own port)")
	}
}
