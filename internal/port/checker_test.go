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

func TestFindFreePortWithExclusions_SkipsFrozenPorts(t *testing.T) {
	// Freeze port 50700
	frozen := map[int]bool{50700: true}

	port, err := FindFreePortWithExclusions(50700, 50710, 0, frozen)
	if err != nil {
		t.Fatalf("FindFreePortWithExclusions() error = %v", err)
	}

	if port == 50700 {
		t.Error("should have skipped frozen port 50700")
	}

	if port < 50701 || port > 50710 {
		t.Errorf("port %d not in expected range 50701-50710", port)
	}
}

func TestFindFreePortWithExclusions_SkipsMultipleFrozenPorts(t *testing.T) {
	// Freeze ports 50800, 50801, 50802
	frozen := map[int]bool{
		50800: true,
		50801: true,
		50802: true,
	}

	port, err := FindFreePortWithExclusions(50800, 50810, 0, frozen)
	if err != nil {
		t.Fatalf("FindFreePortWithExclusions() error = %v", err)
	}

	if frozen[port] {
		t.Errorf("returned frozen port %d", port)
	}

	if port < 50803 || port > 50810 {
		t.Errorf("port %d not in expected range 50803-50810", port)
	}
}

func TestFindFreePortWithExclusions_WrapAroundWithFrozen(t *testing.T) {
	// Freeze ports after lastUsed, forcing wrap-around
	frozen := map[int]bool{
		50906: true,
		50907: true,
		50908: true,
		50909: true,
		50910: true,
	}

	port, err := FindFreePortWithExclusions(50900, 50910, 50905, frozen)
	if err != nil {
		t.Fatalf("FindFreePortWithExclusions() error = %v", err)
	}

	// Should wrap around to 50900-50905
	if port < 50900 || port > 50905 {
		t.Errorf("port %d not in expected range 50900-50905", port)
	}
}

func TestFindFreePortWithExclusions_AllFrozen(t *testing.T) {
	// Freeze all ports in range
	frozen := map[int]bool{
		51000: true,
		51001: true,
		51002: true,
	}

	_, err := FindFreePortWithExclusions(51000, 51002, 0, frozen)
	if !errors.Is(err, ErrAllPortsBusy) {
		t.Errorf("expected ErrAllPortsBusy, got %v", err)
	}
}

func TestFindFreePortWithExclusions_NilFrozenMap(t *testing.T) {
	// Nil frozen map should work (backward compatible)
	port, err := FindFreePortWithExclusions(51100, 51110, 0, nil)
	if err != nil {
		t.Fatalf("FindFreePortWithExclusions() error = %v", err)
	}

	if port < 51100 || port > 51110 {
		t.Errorf("port %d not in range 51100-51110", port)
	}
}

func TestFindFreePortWithExclusions_EmptyFrozenMap(t *testing.T) {
	// Empty frozen map should work
	frozen := map[int]bool{}

	port, err := FindFreePortWithExclusions(51200, 51210, 0, frozen)
	if err != nil {
		t.Fatalf("FindFreePortWithExclusions() error = %v", err)
	}

	if port < 51200 || port > 51210 {
		t.Errorf("port %d not in range 51200-51210", port)
	}
}

func TestFindFreePortWithExclusions_BusyAndFrozen(t *testing.T) {
	// Occupy port 51300
	ln, err := net.Listen("tcp", ":51300")
	if err != nil {
		t.Skipf("cannot occupy port 51300, skipping test")
	}
	defer ln.Close()

	// Freeze port 51301
	frozen := map[int]bool{51301: true}

	port, err := FindFreePortWithExclusions(51300, 51310, 0, frozen)
	if err != nil {
		t.Fatalf("FindFreePortWithExclusions() error = %v", err)
	}

	if port == 51300 {
		t.Error("should have skipped busy port 51300")
	}
	if port == 51301 {
		t.Error("should have skipped frozen port 51301")
	}

	if port < 51302 || port > 51310 {
		t.Errorf("port %d not in expected range 51302-51310", port)
	}
}
