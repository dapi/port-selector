package port

import (
	"net"
	"os"
	"os/user"
	"runtime"
	"strconv"
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
		{
			name: "docker container info",
			info: &ProcessInfo{
				PID:         12345,
				Name:        "docker-proxy",
				ContainerID: "abc123def",
				Cwd:         "/home/user/project",
			},
			contains: []string{"pid=12345", "docker-proxy", "container=abc123def", "cwd=/home/user/project"},
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

	// Should have a username (socket owner)
	if info.User == "" {
		t.Error("User is empty")
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

func TestResolveUID_CurrentUser(t *testing.T) {
	currentUser, err := user.Current()
	if err != nil {
		t.Skip("cannot get current user")
	}

	uid, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		t.Skip("cannot parse current user UID")
	}

	result := resolveUID(uid)
	if result != currentUser.Username {
		t.Errorf("resolveUID(%d) = %q, want %q", uid, result, currentUser.Username)
	}
}

func TestResolveUID_NegativeUID(t *testing.T) {
	result := resolveUID(-1)
	if result != "" {
		t.Errorf("resolveUID(-1) = %q, want empty string", result)
	}
}

func TestResolveUID_NonExistentUID(t *testing.T) {
	// Use a very high UID that's unlikely to exist
	highUID := 999999
	result := resolveUID(highUID)

	// Should return the UID as string since user doesn't exist
	expected := strconv.Itoa(highUID)
	if result != expected {
		t.Errorf("resolveUID(%d) = %q, want %q", highUID, result, expected)
	}
}

func TestResolveUID_RootUser(t *testing.T) {
	// UID 0 is always root
	result := resolveUID(0)
	if result != "root" {
		t.Errorf("resolveUID(0) = %q, want \"root\"", result)
	}
}
