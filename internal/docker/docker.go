// Package docker provides Docker-specific functionality for port discovery.
package docker

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dapi/port-selector/internal/debug"
)

// ContainerInfo contains information about a Docker container using a port.
type ContainerInfo struct {
	ContainerID string
	ProjectDir  string // from compose label or bind mount
}

// IsDockerProxy checks if the given process name indicates a docker-proxy process.
func IsDockerProxy(processName string) bool {
	return processName == "docker-proxy"
}

// IsDockerAvailable checks if the docker CLI is available.
func IsDockerAvailable() bool {
	_, err := exec.LookPath("docker")
	available := err == nil
	debug.Printf("docker", "CLI available: %v", available)
	return available
}

// FindContainerByPort finds a container that publishes the given port.
// Returns empty string if no container is found.
func FindContainerByPort(port int) string {
	debug.Printf("docker", "looking for container on port %d", port)

	if !IsDockerAvailable() {
		return ""
	}

	filter := formatPublishFilter(port)
	debug.Printf("docker", "running: docker ps --filter %s --format {{.ID}}", filter)

	cmd := exec.Command("docker", "ps", "--filter", filter, "--format", "{{.ID}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		debug.Printf("docker", "docker ps failed: %v", err)
		return ""
	}

	containerID := strings.TrimSpace(out.String())
	if containerID == "" {
		debug.Printf("docker", "no container found on port %d", port)
		return ""
	}

	// If multiple containers, take the first one
	if idx := strings.Index(containerID, "\n"); idx != -1 {
		containerID = containerID[:idx]
	}

	debug.Printf("docker", "found container: %s", containerID)
	return containerID
}

// GetProjectDirectory returns the project directory for a container.
// It first tries the docker-compose label, then falls back to bind mounts.
func GetProjectDirectory(containerID string) string {
	if containerID == "" {
		return ""
	}

	debug.Printf("docker", "getting project directory for container %s", containerID)

	// Try docker-compose label first
	if dir := getComposeWorkingDir(containerID); dir != "" {
		debug.Printf("docker", "found compose working dir: %s", dir)
		return dir
	}

	// Fallback to bind mount
	dir := getBindMountSource(containerID)
	if dir != "" {
		debug.Printf("docker", "found bind mount source: %s", dir)
	} else {
		debug.Printf("docker", "no project directory found for container %s", containerID)
	}
	return dir
}

// GetContainerInfo returns full container information for a port.
// This is a convenience function that combines FindContainerByPort and GetProjectDirectory.
func GetContainerInfo(port int) *ContainerInfo {
	containerID := FindContainerByPort(port)
	if containerID == "" {
		return nil
	}

	projectDir := GetProjectDirectory(containerID)

	return &ContainerInfo{
		ContainerID: containerID,
		ProjectDir:  projectDir,
	}
}

// formatPublishFilter creates the filter string for docker ps.
func formatPublishFilter(port int) string {
	return "publish=" + strconv.Itoa(port)
}

// getComposeWorkingDir gets the working directory from docker-compose label.
func getComposeWorkingDir(containerID string) string {
	debug.Printf("docker", "checking compose label for container %s", containerID)

	cmd := exec.Command("docker", "inspect", containerID,
		"--format", "{{index .Config.Labels \"com.docker.compose.project.working_dir\"}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		debug.Printf("docker", "docker inspect failed: %v", err)
		return ""
	}

	dir := strings.TrimSpace(out.String())
	// docker inspect returns "<no value>" if label doesn't exist
	if dir == "" || dir == "<no value>" {
		debug.Printf("docker", "no compose label found")
		return ""
	}

	return dir
}

// getBindMountSource gets the first bind mount source directory.
func getBindMountSource(containerID string) string {
	debug.Printf("docker", "checking bind mounts for container %s", containerID)

	// Use Go template to iterate over mounts and find bind mounts
	cmd := exec.Command("docker", "inspect", containerID,
		"--format", "{{range .Mounts}}{{if eq .Type \"bind\"}}{{.Source}}\n{{end}}{{end}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		debug.Printf("docker", "docker inspect failed: %v", err)
		return ""
	}

	result := strings.TrimSpace(out.String())
	if result == "" {
		debug.Printf("docker", "no bind mounts found")
		return ""
	}

	// Return the first bind mount
	if idx := strings.Index(result, "\n"); idx != -1 {
		return result[:idx]
	}

	return result
}
