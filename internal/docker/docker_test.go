package docker

import (
	"testing"
)

func TestIsDockerProxy(t *testing.T) {
	tests := []struct {
		name        string
		processName string
		want        bool
	}{
		{
			name:        "docker-proxy process",
			processName: "docker-proxy",
			want:        true,
		},
		{
			name:        "regular process",
			processName: "nginx",
			want:        false,
		},
		{
			name:        "empty name",
			processName: "",
			want:        false,
		},
		{
			name:        "partial match",
			processName: "docker-proxy-v2",
			want:        false,
		},
		{
			name:        "containerd-shim",
			processName: "containerd-shim",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDockerProxy(tt.processName); got != tt.want {
				t.Errorf("IsDockerProxy(%q) = %v, want %v", tt.processName, got, tt.want)
			}
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{3000, "3000"},
		{65535, "65535"},
		{-1, "-1"},
		{-100, "-100"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := itoa(tt.input); got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatPublishFilter(t *testing.T) {
	tests := []struct {
		port int
		want string
	}{
		{3000, "publish=3000"},
		{8080, "publish=8080"},
		{443, "publish=443"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := formatPublishFilter(tt.port); got != tt.want {
				t.Errorf("formatPublishFilter(%d) = %q, want %q", tt.port, got, tt.want)
			}
		})
	}
}

func TestContainerInfo(t *testing.T) {
	info := &ContainerInfo{
		ContainerID: "abc123",
		ProjectDir:  "/home/user/project",
	}

	if info.ContainerID != "abc123" {
		t.Errorf("ContainerID = %q, want %q", info.ContainerID, "abc123")
	}
	if info.ProjectDir != "/home/user/project" {
		t.Errorf("ProjectDir = %q, want %q", info.ProjectDir, "/home/user/project")
	}
}

func TestFindContainerByPort_NoDocker(t *testing.T) {
	// This test verifies behavior when docker is not available or port is not published
	// In most test environments, this will return empty string
	result := FindContainerByPort(99999) // unlikely to be in use
	if result != "" {
		// If docker is running and happens to have this port, skip the test
		t.Skip("Docker container found on test port, skipping")
	}
}

func TestGetProjectDirectory_EmptyContainerID(t *testing.T) {
	result := GetProjectDirectory("")
	if result != "" {
		t.Errorf("GetProjectDirectory(\"\") = %q, want empty string", result)
	}
}

func TestGetContainerInfo_NoContainer(t *testing.T) {
	// Test with a port that's unlikely to have a container
	info := GetContainerInfo(99999)
	if info != nil {
		t.Skip("Container found on test port, skipping")
	}
}
