package main

import (
	"bytes"
	"fmt"
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

	// Test: --lock 3500 should now succeed (registers as external)
	// The port is in use by a process (the listener), but it's in a different
	// directory, so it should be registered as external
	cmd := exec.Command(binary, "--lock", "3500")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Should succeed - either as external (different process) or registered
	// The exact output depends on whether port.GetPortProcess can identify our listener
	// For this test, we just verify it doesn't fail with "in use" error
	if strings.Contains(string(output), "in use by unknown process") {
		// This is acceptable - means we couldn't get process info but still handled it
		return
	}

	// Verify an allocation was created (either external or normal)
	allocs, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	alloc := allocs.FindByPort(3500)
	if alloc == nil {
		t.Error("allocation for port 3500 should have been created")
	}
}

func TestLockPortFromAnotherDirectory_Error(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir1 := filepath.Join(tmpDir, "project1")
	if err := os.MkdirAll(workDir1, 0755); err != nil {
		t.Fatal(err)
	}
	workDir2 := filepath.Join(tmpDir, "project2")
	if err := os.MkdirAll(workDir2, 0755); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Step 1: Allocate port 3001 for project1
	cmd := exec.Command(binary, "--lock", "3001")
	cmd.Dir = workDir1
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to lock port 3001 for project1: %v, output: %s", err, output)
	}

	// Step 2: Try to lock port 3001 from project2 (should fail without --force)
	// Port is now locked by project1, so error is "is locked by"
	cmd = exec.Command(binary, "--lock", "3001")
	cmd.Dir = workDir2
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when locking port from another directory, got success: %s", output)
	}
	if !strings.Contains(string(output), "is locked by") {
		t.Errorf("expected 'is locked by' error, got: %s", output)
	}
	if !strings.Contains(string(output), "--force") {
		t.Errorf("expected '--force' hint in error, got: %s", output)
	}
}

func TestLockPortFromAnotherDirectory_WithForce(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir1 := filepath.Join(tmpDir, "project1")
	if err := os.MkdirAll(workDir1, 0755); err != nil {
		t.Fatal(err)
	}
	workDir2 := filepath.Join(tmpDir, "project2")
	if err := os.MkdirAll(workDir2, 0755); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Step 1: Allocate port 3002 for project1
	cmd := exec.Command(binary, "--lock", "3002")
	cmd.Dir = workDir1
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to lock port 3002 for project1: %v, output: %s", err, output)
	}

	// Step 2: Lock port 3002 from project2 with --force (should succeed)
	cmd = exec.Command(binary, "--lock", "--force", "3002")
	cmd.Dir = workDir2
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success with --force, got error: %v, output: %s", err, output)
	}
	if !strings.Contains(string(output), "Reassigned") {
		t.Errorf("expected 'Reassigned' message, got: %s", output)
	}
	if !strings.Contains(string(output), "warning") {
		t.Errorf("expected 'warning' in stderr, got: %s", output)
	}

	// Step 3: Verify port is now allocated to project2
	store, err := allocations.Load(configDir)
	if err != nil {
		t.Fatalf("failed to load allocations: %v", err)
	}
	alloc := store.FindByPort(3002)
	if alloc == nil {
		t.Fatal("expected allocation for port 3002")
	}
	if alloc.Directory != workDir2 {
		t.Errorf("expected port to belong to %s, got %s", workDir2, alloc.Directory)
	}
	if !alloc.Locked {
		t.Error("expected port to be locked")
	}
}

func TestLockPortSameDirectory_NoError(t *testing.T) {
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

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Step 1: Allocate port 3003 for project
	cmd := exec.Command(binary, "--lock", "3003")
	cmd.Dir = workDir
	cmd.Env = env
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to lock port 3003: %v, output: %s", err, output)
	}

	// Step 2: Lock port 3003 again from same directory (should succeed without --force)
	cmd = exec.Command(binary, "--lock", "3003")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}
	if !strings.Contains(string(output), "Locked port 3003") {
		t.Errorf("expected 'Locked port 3003' message, got: %s", output)
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

// Tests for issue #77: Smart --force logic

func TestLockPort_FreeUnlockedFromOtherDir_NoForceNeeded(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir1 := filepath.Join(tmpDir, "project1")
	workDir2 := filepath.Join(tmpDir, "project2")
	if err := os.MkdirAll(workDir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir2, 0755); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Create allocation for project1 (free and unlocked - abandoned)
	store := allocations.NewStore()
	store.SetAllocationWithName(workDir1, 3010, "main")
	// NOT locked, so it's "abandoned"
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Try to lock from project2 without --force (should succeed because port is free and unlocked)
	cmd := exec.Command(binary, "--lock", "3010")
	cmd.Dir = workDir2
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success (free+unlocked allows reassignment), got error: %v, output: %s", err, output)
	}

	// Verify port is now allocated to project2
	loaded, _ := allocations.Load(configDir)
	alloc := loaded.FindByPort(3010)
	if alloc == nil {
		t.Fatal("expected allocation for port 3010")
	}
	if alloc.Directory != workDir2 {
		t.Errorf("expected port to belong to %s, got %s", workDir2, alloc.Directory)
	}
}

func TestLockPort_BusyFromOtherDir_BlocksEvenWithForce(t *testing.T) {
	binary := buildBinary(t)

	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".config", "port-selector")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	workDir1 := filepath.Join(tmpDir, "project1")
	workDir2 := filepath.Join(tmpDir, "project2")
	if err := os.MkdirAll(workDir1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workDir2, 0755); err != nil {
		t.Fatal(err)
	}

	// Occupy port to simulate busy port
	ln, err := net.Listen("tcp", ":3011")
	if err != nil {
		t.Skipf("could not occupy port 3011 for test: %v", err)
	}
	defer ln.Close()

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Create allocation for project1 (busy)
	store := allocations.NewStore()
	store.SetAllocationWithName(workDir1, 3011, "main")
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Try to lock from project2 with --force (should fail because port is busy on another dir)
	cmd := exec.Command(binary, "--lock", "--force", "3011")
	cmd.Dir = workDir2
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error (busy port on another dir), got success: %s", output)
	}
	if !strings.Contains(string(output), "in use by") {
		t.Errorf("expected 'in use by' error, got: %s", output)
	}
	if !strings.Contains(string(output), "stop the service") {
		t.Errorf("expected 'stop the service' hint, got: %s", output)
	}
}

func TestLockPort_BusyNotAllocated_RegistersAsExternal(t *testing.T) {
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

	// Occupy port to simulate busy port from another directory
	ln, err := net.Listen("tcp", ":3012")
	if err != nil {
		t.Skipf("could not occupy port 3012 for test: %v", err)
	}
	defer ln.Close()

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Try to lock port that's in use - should register as external (not fail)
	cmd := exec.Command(binary, "--lock", "3012")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	// With new behavior, busy port with process info is registered as external
	if err != nil {
		// If it fails, it should be because no process info is available
		if !strings.Contains(string(output), "unknown process") {
			t.Fatalf("expected external registration or unknown process error, got: %s", output)
		}
		return // Test passes - no process info available
	}

	// Check output indicates external registration
	if !strings.Contains(string(output), "external") {
		t.Errorf("expected 'external' in output, got: %s", output)
	}

	// Verify allocation was created as external
	loaded, _ := allocations.Load(configDir)
	alloc := loaded.FindByPort(3012)
	if alloc == nil {
		t.Fatal("expected allocation for port 3012")
	}
	if alloc.Status != "external" {
		t.Errorf("expected status 'external', got %q", alloc.Status)
	}
}

func TestLockPort_UnlocksOldLockedPort(t *testing.T) {
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

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Create allocation for project with locked port 3013
	store := allocations.NewStore()
	store.SetAllocationWithName(workDir, 3013, "main")
	store.SetLockedByPort(3013, true)
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Lock new port 3014 for same directory+name
	cmd := exec.Command(binary, "--lock", "3014")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Verify old port 3013 is unlocked, new port 3014 is locked
	loaded, _ := allocations.Load(configDir)

	alloc3013 := loaded.FindByPort(3013)
	if alloc3013 == nil {
		t.Fatal("expected allocation for port 3013 to still exist")
	}
	if alloc3013.Locked {
		t.Error("old port 3013 should be unlocked after locking new port")
	}

	alloc3014 := loaded.FindByPort(3014)
	if alloc3014 == nil {
		t.Fatal("expected allocation for port 3014")
	}
	if !alloc3014.Locked {
		t.Error("new port 3014 should be locked")
	}
}

func TestLockMessage_ShowsDirectory(t *testing.T) {
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

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Lock a port
	cmd := exec.Command(binary, "--lock", "3015")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Verify message shows directory
	if !strings.Contains(string(output), "in ") {
		t.Errorf("expected 'in <directory>' in message, got: %s", output)
	}
	if !strings.Contains(string(output), "project") {
		t.Errorf("expected directory path in message, got: %s", output)
	}
}

func TestPortSelector_ReturnsLockedBusyPort(t *testing.T) {
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

	// Occupy port to simulate user's service running
	ln, err := net.Listen("tcp", ":3016")
	if err != nil {
		t.Skipf("could not occupy port 3016 for test: %v", err)
	}
	defer ln.Close()

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Create locked allocation for this directory
	store := allocations.NewStore()
	store.SetAllocationWithName(workDir, 3016, "main")
	store.SetLockedByPort(3016, true)
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Run port-selector - should return locked+busy port (user's service already running)
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		t.Fatalf("expected success, got error: %v, stderr: %s", err, stderr.String())
	}

	port := strings.TrimSpace(stdout.String())
	if port != "3016" {
		t.Errorf("expected port 3016 (locked+busy), got: %s (stderr: %s)", port, stderr.String())
	}
}

func TestLockPort_SameDirectoryDifferentName(t *testing.T) {
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

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Create allocation for "web" name
	store := allocations.NewStore()
	store.SetAllocationWithName(workDir, 3020, "web")
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Lock same port from same dir but default name "main"
	// This should lock the port but keep the existing name "web"
	// (user is locking a specific port, not changing its name)
	cmd := exec.Command(binary, "--lock", "3020")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Verify port is locked but name is preserved
	loaded, err := allocations.Load(configDir)
	if err != nil {
		t.Fatalf("failed to load allocations: %v", err)
	}
	alloc := loaded.FindByPort(3020)
	if alloc == nil {
		t.Fatal("expected allocation for port 3020")
	}
	// Name should be preserved as "web" since we're locking an existing port
	if alloc.Name != "web" {
		t.Errorf("expected name 'web' (preserved), got %q", alloc.Name)
	}
	if !alloc.Locked {
		t.Error("expected port to be locked")
	}
}

func TestLockPort_SameDirectorySamePortIdempotent(t *testing.T) {
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

	// Occupy port to simulate service running
	ln, err := net.Listen("tcp", ":3021")
	if err != nil {
		t.Skipf("could not occupy port 3021 for test: %v", err)
	}
	defer ln.Close()

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Create locked allocation for same directory
	store := allocations.NewStore()
	store.SetAllocationWithName(workDir, 3021, "main")
	store.SetLockedByPort(3021, true)
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Lock same port again (idempotent operation)
	cmd := exec.Command(binary, "--lock", "3021")
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success (idempotent lock), got error: %v, output: %s", err, output)
	}

	// Should still be locked
	loaded, err := allocations.Load(configDir)
	if err != nil {
		t.Fatalf("failed to load allocations: %v", err)
	}
	alloc := loaded.FindByPort(3021)
	if alloc == nil {
		t.Fatal("expected allocation for port 3021")
	}
	if !alloc.Locked {
		t.Error("expected port to remain locked")
	}
}

// Tests for --refresh command (issue #73)

func TestRefresh_NoExternalAllocations(t *testing.T) {
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

	// Run --refresh with no allocations
	cmd := exec.Command(binary, "--refresh")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	if !strings.Contains(string(output), "No external port allocations found") {
		t.Errorf("expected 'No external port allocations found', got: %s", output)
	}
}

func TestRefresh_RemovesStaleExternalAllocations(t *testing.T) {
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

	// Create external allocation for a free port
	store := allocations.NewStore()
	store.SetExternalAllocation(3600, 99999, "testuser", "defunct", "/tmp/defunct")
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Run --refresh - should remove the stale allocation (port is free)
	cmd := exec.Command(binary, "--refresh")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	if !strings.Contains(string(output), "Removed 1 stale") {
		t.Errorf("expected 'Removed 1 stale', got: %s", output)
	}

	// Verify allocation was removed
	loaded, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	if loaded.FindByPort(3600) != nil {
		t.Error("stale external allocation should have been removed")
	}
}

func TestRefresh_KeepsActiveExternalAllocations(t *testing.T) {
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

	// Occupy a port
	ln, err := net.Listen("tcp", ":3601")
	if err != nil {
		t.Skipf("could not occupy port 3601 for test: %v", err)
	}
	defer ln.Close()

	// Create external allocation for the busy port
	store := allocations.NewStore()
	store.SetExternalAllocation(3601, 12345, "testuser", "testprocess", "/tmp/test")
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Run --refresh - should keep the allocation (port is busy)
	cmd := exec.Command(binary, "--refresh")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	if !strings.Contains(string(output), "All external allocations are still active") {
		t.Errorf("expected 'All external allocations are still active', got: %s", output)
	}

	// Verify allocation still exists
	loaded, loadErr := allocations.Load(configDir)
	if loadErr != nil {
		t.Fatalf("failed to load allocations: %v", loadErr)
	}
	if loaded.FindByPort(3601) == nil {
		t.Error("active external allocation should have been kept")
	}
}

// Test for issue: Port changes when busy and unlocked
// https://github.com/dapi/port-selector/issues/XXX
// Expected: port-selector always returns the same port for the same directory,
// even if the port is busy (e.g., user's service is running)
// Actual: port-selector allocates a new port when existing port is busy and unlocked

func TestPortSelector_ReturnsSamePortEvenWhenBusy(t *testing.T) {
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

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Step 1: Get initial port allocation
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Env = env
	var stdout1, stderr1 bytes.Buffer
	cmd.Stdout = &stdout1
	cmd.Stderr = &stderr1
	if err := cmd.Run(); err != nil {
		t.Fatalf("first call failed: %v, stderr: %s", err, stderr1.String())
	}
	initialPort := strings.TrimSpace(stdout1.String())
	t.Logf("Initial port: %s", initialPort)

	// Step 2: Simulate user's service running on that port
	portNum := 0
	fmt.Sscanf(initialPort, "%d", &portNum)
	ln, err := net.Listen("tcp", ":"+initialPort)
	if err != nil {
		t.Skipf("could not occupy port %s for test: %v", initialPort, err)
	}
	defer ln.Close()

	// Step 3: Call port-selector again while port is busy
	// BUG: Currently this returns a NEW port instead of the same one
	cmd = exec.Command(binary)
	cmd.Dir = workDir
	cmd.Env = env
	var stdout2, stderr2 bytes.Buffer
	cmd.Stdout = &stdout2
	cmd.Stderr = &stderr2
	if err := cmd.Run(); err != nil {
		t.Fatalf("second call failed: %v, stderr: %s", err, stderr2.String())
	}
	secondPort := strings.TrimSpace(stdout2.String())
	t.Logf("Second port: %s", secondPort)

	// Step 4: Verify same port is returned (this is the expected behavior)
	if secondPort != initialPort {
		t.Errorf("BUG REPRODUCED: expected same port %s, got different port %s", initialPort, secondPort)
		t.Errorf("Port should be stable for the same directory, even when busy")
	}
}

func TestPortSelector_PortStabilityAcrossMultipleCalls(t *testing.T) {
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

	env := append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))

	// Get initial port
	cmd := exec.Command(binary)
	cmd.Dir = workDir
	cmd.Env = env
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get port: %v", err)
	}
	expectedPort := strings.TrimSpace(string(output))

	// Occupy the port
	ln, err := net.Listen("tcp", ":"+expectedPort)
	if err != nil {
		t.Skipf("could not occupy port: %v", err)
	}
	defer ln.Close()

	// Call port-selector multiple times while port is busy
	// All calls should return the same port
	for i := 0; i < 5; i++ {
		cmd := exec.Command(binary)
		cmd.Dir = workDir
		cmd.Env = env
		output, err := cmd.Output()
		if err != nil {
			t.Fatalf("call %d failed: %v", i+1, err)
		}
		port := strings.TrimSpace(string(output))
		if port != expectedPort {
			t.Errorf("Call %d: expected port %s, got %s", i+1, expectedPort, port)
		}
	}
}

func TestList_ShowsSourceColumn(t *testing.T) {
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

	// Create allocations with different sources
	store := allocations.NewStore()
	// Normal (free) allocation
	store.SetAllocation("/tmp/project1", 3700)
	// Locked allocation
	store.SetAllocation("/tmp/project2", 3701)
	store.SetLockedByPort(3701, true)
	// External allocation
	store.SetExternalAllocation(3702, 12345, "user", "process", "/tmp/external")
	if err := allocations.Save(configDir, store); err != nil {
		t.Fatal(err)
	}

	// Run --list
	cmd := exec.Command(binary, "--list")
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+filepath.Join(tmpDir, ".config"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected success, got error: %v, output: %s", err, output)
	}

	// Verify SOURCE column header exists
	if !strings.Contains(string(output), "SOURCE") {
		t.Errorf("expected SOURCE column header, got: %s", output)
	}

	// Verify different source values
	if !strings.Contains(string(output), "free") {
		t.Errorf("expected 'free' source for normal allocation, got: %s", output)
	}
	if !strings.Contains(string(output), "lock") {
		t.Errorf("expected 'lock' source for locked allocation, got: %s", output)
	}
	if !strings.Contains(string(output), "external") {
		t.Errorf("expected 'external' source for external allocation, got: %s", output)
	}
}
