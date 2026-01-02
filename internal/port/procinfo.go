package port

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ProcessInfo contains information about a process using a port.
type ProcessInfo struct {
	PID     int
	Name    string
	Cwd     string // working directory
	Cmdline string // command line (truncated)
	UID     int    // user ID owning the socket
	User    string // username (resolved from UID)
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
// Returns nil if the process cannot be determined.
// Always returns at least UID/User if the socket is found in /proc/net/tcp.
// Falls back to parsing `ss -tlnp` output to get process name when PID is not accessible.
func GetPortProcess(port int) *ProcessInfo {
	// Try both IPv4 and IPv6 via /proc
	info := getPortProcessFromProc(port, "/proc/net/tcp")
	if info == nil {
		info = getPortProcessFromProc(port, "/proc/net/tcp6")
	}
	if info == nil {
		return nil
	}

	// If we have full info (PID found), return it
	if info.PID != 0 {
		return info
	}

	// We have UID but no PID - try ss fallback to get process name
	if ssInfo := getPortProcessFromSS(port); ssInfo != nil {
		info.PID = ssInfo.PID
		info.Name = ssInfo.Name
		if ssInfo.Cwd != "" {
			info.Cwd = ssInfo.Cwd
		}
	}

	return info
}

// getPortProcessFromProc parses /proc/net/tcp or /proc/net/tcp6 to find the inode,
// then searches /proc/*/fd/ to find which process owns that socket.
// Returns ProcessInfo with at least UID/User if socket found, full info if process accessible.
func getPortProcessFromProc(port int, procNetFile string) *ProcessInfo {
	sockInfo := findSocketInfo(port, procNetFile)
	if sockInfo == nil {
		return nil
	}

	// We have UID from /proc/net/tcp - resolve username
	info := &ProcessInfo{
		UID: sockInfo.UID,
	}
	if u, err := user.LookupId(strconv.Itoa(sockInfo.UID)); err == nil {
		info.User = u.Username
	}

	// Try to find the process by inode
	pid := findProcessByInode(sockInfo.Inode)
	if pid == 0 {
		// Can't find PID (permission denied for /proc/*/fd/), but we have UID
		return info
	}

	// We found the PID, get full process info
	fullInfo := getProcessInfo(pid)
	fullInfo.UID = info.UID
	fullInfo.User = info.User
	return fullInfo
}

// socketInfo contains information extracted from /proc/net/tcp.
type socketInfo struct {
	Inode uint64
	UID   int
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
		uid, _ := strconv.Atoi(fields[7])

		// Field 9 is inode
		inode, err := strconv.ParseUint(fields[9], 10, 64)
		if err != nil {
			continue
		}

		return &socketInfo{Inode: inode, UID: uid}
	}

	return nil
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

// ssProcessRegex matches the process info in ss output: users:(("name",pid=12345,fd=7))
var ssProcessRegex = regexp.MustCompile(`users:\(\("([^"]+)",pid=(\d+),fd=\d+\)\)`)

// getPortProcessFromSS uses the `ss` command to get process info for a port.
// This is a fallback for when /proc/*/fd/ is not accessible (e.g., root-owned processes).
func getPortProcessFromSS(port int) *ProcessInfo {
	// Run ss -tlnp to get listening TCP sockets with process info
	cmd := exec.Command("ss", "-tlnp")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	portStr := fmt.Sprintf(":%d", port)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()

		// Check if this line contains our port
		// Format: "LISTEN 0 4096 127.0.0.1:3000 0.0.0.0:* users:(("ruby",pid=876344,fd=6))"
		if !strings.Contains(line, portStr) {
			continue
		}

		// Verify it's the local address port, not peer port
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		localAddr := fields[3]
		// Check if local address ends with our port
		if !strings.HasSuffix(localAddr, portStr) {
			continue
		}

		// Try to extract process info using regex
		matches := ssProcessRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		name := matches[1]
		pid, err := strconv.Atoi(matches[2])
		if err != nil {
			continue
		}

		// We have PID and name from ss, now try to get cwd
		info := &ProcessInfo{
			PID:  pid,
			Name: name,
		}

		// Try to read cwd (may fail for root processes, that's ok)
		procDir := fmt.Sprintf("/proc/%d", pid)
		if cwd, err := os.Readlink(filepath.Join(procDir, "cwd")); err == nil {
			info.Cwd = cwd
		}

		return info
	}

	return nil
}
