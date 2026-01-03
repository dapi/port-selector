// Package docker provides Docker-specific functionality for port discovery.
package docker

import (
	"bytes"
	"os/exec"
	"strings"
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
	return err == nil
}

// FindContainerByPort finds a container that publishes the given port.
// Returns empty string if no container is found.
func FindContainerByPort(port int) string {
	if !IsDockerAvailable() {
		return ""
	}

	cmd := exec.Command("docker", "ps", "--filter", formatPublishFilter(port), "--format", "{{.ID}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}

	containerID := strings.TrimSpace(out.String())
	if containerID == "" {
		return ""
	}

	// If multiple containers, take the first one
	if idx := strings.Index(containerID, "\n"); idx != -1 {
		containerID = containerID[:idx]
	}

	return containerID
}

// GetProjectDirectory returns the project directory for a container.
// It first tries the docker-compose label, then falls back to bind mounts.
func GetProjectDirectory(containerID string) string {
	if containerID == "" {
		return ""
	}

	// Try docker-compose label first
	if dir := getComposeWorkingDir(containerID); dir != "" {
		return dir
	}

	// Fallback to bind mount
	return getBindMountSource(containerID)
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
	return strings.Replace("publish=PORT", "PORT", itoa(port), 1)
}

// getComposeWorkingDir gets the working directory from docker-compose label.
func getComposeWorkingDir(containerID string) string {
	cmd := exec.Command("docker", "inspect", containerID,
		"--format", "{{index .Config.Labels \"com.docker.compose.project.working_dir\"}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}

	dir := strings.TrimSpace(out.String())
	// docker inspect returns "<no value>" if label doesn't exist
	if dir == "" || dir == "<no value>" {
		return ""
	}

	return dir
}

// getBindMountSource gets the first bind mount source directory.
func getBindMountSource(containerID string) string {
	// Use Go template to iterate over mounts and find bind mounts
	cmd := exec.Command("docker", "inspect", containerID,
		"--format", "{{range .Mounts}}{{if eq .Type \"bind\"}}{{.Source}}\n{{end}}{{end}}")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		return ""
	}

	result := strings.TrimSpace(out.String())
	if result == "" {
		return ""
	}

	// Return the first bind mount
	if idx := strings.Index(result, "\n"); idx != -1 {
		return result[:idx]
	}

	return result
}

// itoa converts int to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var neg bool
	if n < 0 {
		neg = true
		n = -n
	}

	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}

	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}
