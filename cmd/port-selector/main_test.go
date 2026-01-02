package main

import (
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
			name:    "lock specific port without allocation",
			args:    []string{"--lock", "3000"},
			wantErr: "no allocation found for port 3000",
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
	allocs := &allocations.AllocationList{
		Allocations: []allocations.Allocation{
			{
				Port:      3000,
				Directory: projectA,
				Locked:    true,
			},
		},
	}
	if err := allocations.Save(configDir, allocs); err != nil {
		t.Fatal(err)
	}

	// Verify that GetLockedPortsForExclusion works correctly
	loaded := allocations.Load(configDir)

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
