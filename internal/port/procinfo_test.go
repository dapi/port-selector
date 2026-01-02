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
