package port

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcessInfo contains information about a process using a port.
type ProcessInfo struct {
	PID     int
	Name    string
	Cwd     string // working directory
	Cmdline string // command line (truncated)
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

	if p.Cwd != "" {
		parts = append(parts, fmt.Sprintf("cwd=%s", p.Cwd))
	}

	return strings.Join(parts, ", ")
}

// GetPortProcess returns information about the process using the given port.
// Returns nil if the process cannot be determined (e.g., permission denied).
func GetPortProcess(port int) *ProcessInfo {
	// Try both IPv4 and IPv6
	if info := getPortProcessFromProc(port, "/proc/net/tcp"); info != nil {
		return info
	}
	return getPortProcessFromProc(port, "/proc/net/tcp6")
}

// getPortProcessFromProc parses /proc/net/tcp or /proc/net/tcp6 to find the inode,
// then searches /proc/*/fd/ to find which process owns that socket.
func getPortProcessFromProc(port int, procNetFile string) *ProcessInfo {
	inode := findSocketInode(port, procNetFile)
	if inode == 0 {
		return nil
	}

	pid := findProcessByInode(inode)
	if pid == 0 {
		return nil
	}

	return getProcessInfo(pid)
}

// findSocketInode searches /proc/net/tcp(6) for a listening socket on the given port.
// Returns the inode number or 0 if not found.
func findSocketInode(port int, procNetFile string) uint64 {
	file, err := os.Open(procNetFile)
	if err != nil {
		return 0
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

		// Field 9 is inode
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}

		return inode
	}

	return 0
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
