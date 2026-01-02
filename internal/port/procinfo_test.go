package port

import (
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestProcessInfo_String(t *testing.T) {
	tests := []struct {
		name     string
		info     *ProcessInfo
		contains []string
	}{
		{
			name: "full info",
			info: &ProcessInfo{
				PID:  12345,
				Name: "myapp",
				Cwd:  "/home/user/project",
			},
			contains: []string{"pid=12345", "myapp", "cwd=/home/user/project"},
		},
		{
			name: "pid only",
			info: &ProcessInfo{
				PID: 12345,
			},
			contains: []string{"pid=12345"},
		},
		{
			name:     "nil info",
			info:     nil,
			contains: []string{"unknown process"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.info.String()
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("String() = %q, want to contain %q", result, s)
				}
			}
		})
	}
}

func TestGetPortProcess_OwnProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GetPortProcess only works on Linux")
	}

	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Get the port
	addr := ln.Addr().(*net.TCPAddr)
	port := addr.Port

	// Get process info for our own process
	info := GetPortProcess(port)
	if info == nil {
		t.Fatal("GetPortProcess returned nil for our own listening port")
	}

	// Should be our own PID
	if info.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}

	// Should have a process name
	if info.Name == "" {
		t.Error("Name is empty")
	}

	// Should have a cwd (our working directory)
	if info.Cwd == "" {
		t.Error("Cwd is empty")
	}
}

func TestGetPortProcess_NoListener(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GetPortProcess only works on Linux")
	}

	// Use a port that's unlikely to be in use
	info := GetPortProcess(59999)

	// Should return nil for a port with no listener
	if info != nil {
		t.Errorf("GetPortProcess returned non-nil for unused port: %+v", info)
	}
}

func TestSSProcessRegex(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantPID     string
		shouldMatch bool
	}{
		{
			name:        "standard format",
			input:       `users:(("ruby",pid=876344,fd=6))`,
			wantName:    "ruby",
			wantPID:     "876344",
			shouldMatch: true,
		},
		{
			name:        "docker-proxy",
			input:       `users:(("docker-proxy",pid=585980,fd=7))`,
			wantName:    "docker-proxy",
			wantPID:     "585980",
			shouldMatch: true,
		},
		{
			name:        "python process",
			input:       `users:(("python",pid=466018,fd=4))`,
			wantName:    "python",
			wantPID:     "466018",
			shouldMatch: true,
		},
		{
			name:        "process with hyphen",
			input:       `users:(("node-server",pid=12345,fd=3))`,
			wantName:    "node-server",
			wantPID:     "12345",
			shouldMatch: true,
		},
		{
			name:        "no process info",
			input:       `LISTEN 0 4096 127.0.0.1:9099 0.0.0.0:*`,
			shouldMatch: false,
		},
		{
			name:        "empty string",
			input:       ``,
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := ssProcessRegex.FindStringSubmatch(tt.input)
			if tt.shouldMatch {
				if matches == nil {
					t.Errorf("regex did not match input: %q", tt.input)
					return
				}
				if len(matches) < 3 {
					t.Errorf("not enough matches: got %d, want at least 3", len(matches))
					return
				}
				if matches[1] != tt.wantName {
					t.Errorf("name = %q, want %q", matches[1], tt.wantName)
				}
				if matches[2] != tt.wantPID {
					t.Errorf("PID = %q, want %q", matches[2], tt.wantPID)
				}
			} else {
				if matches != nil {
					t.Errorf("regex matched when it should not: %v", matches)
				}
			}
		})
	}
}

func TestGetPortProcessFromSS_Integration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("ss command only available on Linux")
	}

	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Get the port
	addr := ln.Addr().(*net.TCPAddr)
	port := addr.Port

	// Get process info using ss fallback
	info := getPortProcessFromSS(port)
	if info == nil {
		t.Skip("ss did not return process info (may require permissions)")
	}

	// Should be our own PID
	if info.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}

	// Should have a process name
	if info.Name == "" {
		t.Error("Name is empty")
	}
}

func TestGetPortProcess_ReturnsUID(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GetPortProcess only works on Linux")
	}

	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Get the port
	addr := ln.Addr().(*net.TCPAddr)
	port := addr.Port

	// Get process info
	info := GetPortProcess(port)
	if info == nil {
		t.Fatal("GetPortProcess returned nil for our own listening port")
	}

	// Should have our UID
	if info.UID != os.Getuid() {
		t.Errorf("UID = %d, want %d", info.UID, os.Getuid())
	}

	// Should have resolved username
	if info.User == "" {
		t.Error("User is empty")
	}
}

func TestFindSocketInfo(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("findSocketInfo only works on Linux")
	}

	// Start a listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	// Get the port
	addr := ln.Addr().(*net.TCPAddr)
	port := addr.Port

	// Find socket info
	sockInfo := findSocketInfo(port, "/proc/net/tcp")
	if sockInfo == nil {
		t.Fatal("findSocketInfo returned nil for our own listening port")
	}

	// Should have an inode
	if sockInfo.Inode == 0 {
		t.Error("Inode is 0")
	}

	// Should have our UID
	if sockInfo.UID != os.Getuid() {
		t.Errorf("UID = %d, want %d", sockInfo.UID, os.Getuid())
	}
}
