package port

import (
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestIsPortFree_FreePort(t *testing.T) {
	// Find a free port first
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	// Now check if it's free
	if !IsPortFree(port) {
		t.Errorf("expected port %d to be free", port)
	}
}

func TestIsPortFree_BusyPort(t *testing.T) {
	// Occupy a port
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if IsPortFree(port) {
		t.Errorf("expected port %d to be busy", port)
	}
}

func TestFindFreePort_Basic(t *testing.T) {
	// Use high ports to avoid conflicts
	port, err := FindFreePort(50000, 50010, 0)
	if err != nil {
		t.Fatalf("FindFreePort() error = %v", err)
	}

	if port < 50000 || port > 50010 {
		t.Errorf("port %d not in range 50000-50010", port)
	}
}

func TestFindFreePort_SkipsBusyPort(t *testing.T) {
	// Occupy port 50100
	ln, err := net.Listen("tcp", ":50100")
	if err != nil {
		t.Fatalf("failed to listen on 50100: %v", err)
	}
	defer ln.Close()

	port, err := FindFreePort(50100, 50105, 0)
	if err != nil {
		t.Fatalf("FindFreePort() error = %v", err)
	}

	if port == 50100 {
		t.Error("should have skipped busy port 50100")
	}

	if port < 50101 || port > 50105 {
		t.Errorf("port %d not in expected range 50101-50105", port)
	}
}

func TestFindFreePort_StartsFromLastUsed(t *testing.T) {
	port, err := FindFreePort(50200, 50210, 50205)
	if err != nil {
		t.Fatalf("FindFreePort() error = %v", err)
	}

	// Should start from 50206 (lastUsed + 1)
	if port < 50206 {
		t.Errorf("expected port >= 50206, got %d", port)
	}
}

func TestFindFreePort_WrapAround(t *testing.T) {
	// Occupy ports 50306-50310
	listeners := make([]net.Listener, 0)
	for p := 50306; p <= 50310; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			// Port might already be busy, skip
			continue
		}
		listeners = append(listeners, ln)
	}
	defer func() {
		for _, ln := range listeners {
			ln.Close()
		}
	}()

	// Start from 50305, should wrap around to 50300-50305
	port, err := FindFreePort(50300, 50310, 50305)
	if err != nil {
		t.Fatalf("FindFreePort() error = %v", err)
	}

	// Should find a port in the range 50300-50305 (wrap-around)
	if port < 50300 || port > 50310 {
		t.Errorf("port %d not in range 50300-50310", port)
	}
}

func TestFindFreePort_AllBusy(t *testing.T) {
	// Occupy a small range
	listeners := make([]net.Listener, 0)
	for p := 50400; p <= 50402; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err != nil {
			t.Skipf("cannot occupy port %d, skipping test", p)
		}
		listeners = append(listeners, ln)
	}
	defer func() {
		for _, ln := range listeners {
			ln.Close()
		}
	}()

	_, err := FindFreePort(50400, 50402, 0)
	if !errors.Is(err, ErrAllPortsBusy) {
		t.Errorf("expected ErrAllPortsBusy, got %v", err)
	}
}

func TestFindFreePort_LastUsedOutOfRange(t *testing.T) {
	// lastUsed is outside the range, should start from start
	port, err := FindFreePort(50500, 50510, 49000)
	if err != nil {
		t.Fatalf("FindFreePort() error = %v", err)
	}

	if port < 50500 || port > 50510 {
		t.Errorf("port %d not in range 50500-50510", port)
	}
}

func TestFindFreePort_LastUsedIsEnd(t *testing.T) {
	// lastUsed is the end of range, should wrap to start
	port, err := FindFreePort(50600, 50605, 50605)
	if err != nil {
		t.Fatalf("FindFreePort() error = %v", err)
	}

	// Should wrap around to 50600
	if port < 50600 || port > 50605 {
		t.Errorf("port %d not in range 50600-50605", port)
	}
}
