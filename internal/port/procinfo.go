package port

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dapi/port-selector/internal/docker"
)

// ProcessInfo contains information about a process using a port.
type ProcessInfo struct {
	PID         int
	Name        string
	Cwd         string // working directory
	Cmdline     string // command line (truncated)
	ContainerID string // Docker container ID (if applicable)
	User        string // socket owner username (from /proc/net/tcp UID)
}

// socketInfo contains socket inode and owner UID from /proc/net/tcp.
type socketInfo struct {
	Inode uint64
	UID   int
}

// String returns a human-readable description of the process.
func (p *ProcessInfo) String() string {
	if p == nil {
		return "unknown process"
	}

	parts := []string{fmt.Sprintf("pid=%d", p.PID)}

	if p.Name != "" {
		parts = append(parts, p.Name)
	}

	if p.ContainerID != "" {
		parts = append(parts, fmt.Sprintf("container=%s", p.ContainerID))
	}

	if p.Cwd != "" {
		parts = append(parts, fmt.Sprintf("cwd=%s", p.Cwd))
	}

	return strings.Join(parts, ", ")
}

// GetPortProcess returns information about the process using the given port.
// If the process is docker-proxy, it attempts to resolve the actual project
// directory from the Docker container.
// Returns nil if the port is not in use.
// If the process cannot be fully determined (e.g., permission denied),
// returns partial info with at least the User field populated.
func GetPortProcess(port int) *ProcessInfo {
	// Try both IPv4 and IPv6
	var info *ProcessInfo
	if info = getPortProcessFromProc(port, "/proc/net/tcp"); info == nil {
		info = getPortProcessFromProc(port, "/proc/net/tcp6")
	}

	if info == nil {
		return nil
	}

	// Check if this is a docker-proxy process
	if docker.IsDockerProxy(info.Name) {
		enrichWithDocker(info, port)
	} else if info.PID == 0 && info.User == "root" {
		// Without sudo we can't get process name, but if it's root-owned,
		// try Docker detection as a fallback (docker-proxy runs as root)
		enrichWithDocker(info, port)
	}

	return info
}

// enrichWithDocker enhances ProcessInfo with Docker container information.
// It replaces the useless "/" cwd with the actual project directory.
func enrichWithDocker(info *ProcessInfo, port int) {
	containerInfo := docker.GetContainerInfo(port)
	if containerInfo == nil {
		return
	}

	info.ContainerID = containerInfo.ContainerID

	// Replace useless "/" with actual project directory
	if containerInfo.ProjectDir != "" {
		info.Cwd = containerInfo.ProjectDir
	}
}

// getPortProcessFromProc parses /proc/net/tcp or /proc/net/tcp6 to find the inode and UID,
// then searches /proc/*/fd/ to find which process owns that socket.
// If the process cannot be determined but the socket exists, returns partial info with User.
func getPortProcessFromProc(port int, procNetFile string) *ProcessInfo {
	sockInfo := findSocketInfo(port, procNetFile)
	if sockInfo == nil {
		return nil
	}

	// Resolve UID to username
	username := resolveUID(sockInfo.UID)

	pid := findProcessByInode(sockInfo.Inode)
	if pid == 0 {
		// Could not find process (permission denied or race condition),
		// but we know the socket exists - return partial info with username
		return &ProcessInfo{
			User: username,
		}
	}

	info := getProcessInfo(pid)
	info.User = username
	return info
}

// findSocketInfo searches /proc/net/tcp(6) for a listening socket on the given port.
// Returns socket info (inode and UID) or nil if not found.
func findSocketInfo(port int, procNetFile string) *socketInfo {
	file, err := os.Open(procNetFile)
	if err != nil {
		return nil
	}
	defer file.Close()

	// Port in hex (network byte order for local port)
	portHex := fmt.Sprintf("%04X", port)

	scanner := bufio.NewScanner(file)
	scanner.Scan() // skip header line

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}

		// Field 1 is local_address (hex_ip:hex_port)
		localAddr := fields[1]
		parts := strings.Split(localAddr, ":")
		if len(parts) != 2 {
			continue
		}

		localPort := parts[1]
		if localPort != portHex {
			continue
		}

		// Field 3 is state: 0A = LISTEN
		if fields[3] != "0A" {
			continue
		}

		// Field 7 is UID
		uid, err := strconv.Atoi(fields[7])
		if err != nil {
			uid = -1
		}

		// Field 9 is inode
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}

		return &socketInfo{
			Inode: inode,
			UID:   uid,
		}
	}

	return nil
}

// resolveUID converts a numeric UID to a username.
// Returns the UID as string if username cannot be resolved.
func resolveUID(uid int) string {
	if uid < 0 {
		return ""
	}
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return strconv.Itoa(uid)
	}
	return u.Username
}

// findProcessByInode searches /proc/*/fd/ for a socket with the given inode.
// Returns the PID or 0 if not found.
func findProcessByInode(inode uint64) int {
	socketLink := fmt.Sprintf("socket:[%d]", inode)

	procDirs, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, entry := range procDirs {
		if !entry.IsDir() {
			continue
		}

		// Check if directory name is a PID (numeric)
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue // permission denied or process gone
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}

			if link == socketLink {
				return pid
			}
		}
	}

	return 0
}

// getProcessInfo reads process information from /proc/[pid]/.
func getProcessInfo(pid int) *ProcessInfo {
	info := &ProcessInfo{PID: pid}

	procDir := fmt.Sprintf("/proc/%d", pid)

	// Read process name from /proc/[pid]/comm
	if data, err := os.ReadFile(filepath.Join(procDir, "comm")); err == nil {
		info.Name = strings.TrimSpace(string(data))
	}

	// Read working directory from /proc/[pid]/cwd
	if cwd, err := os.Readlink(filepath.Join(procDir, "cwd")); err == nil {
		info.Cwd = cwd
	}

	return info
}
