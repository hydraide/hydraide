package portfinder

import (
	"fmt"
	"net"
	"testing"
)

func TestIsPortFree_Free(t *testing.T) {
	port := pickEphemeralPort(t)
	if !IsPortFree(port) {
		t.Fatalf("expected port %d to be free", port)
	}
}

func TestIsPortFree_Taken(t *testing.T) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port
	if IsPortFree(port) {
		t.Fatalf("expected port %d to be busy", port)
	}
}

func TestFindFreePair_DefaultsWhenFree(t *testing.T) {
	start := pickEphemeralPort(t)
	grpc, health, err := FindFreePair(nil, start, 10, 5)
	if err != nil {
		t.Fatalf("FindFreePair: %v", err)
	}
	if grpc != start || health != start+1 {
		t.Fatalf("got %d/%d, want %d/%d", grpc, health, start, start+1)
	}
}

func TestFindFreePair_SkipsReserved(t *testing.T) {
	start := pickEphemeralPort(t)
	reserved := map[int]bool{start: true}

	grpc, health, err := FindFreePair(reserved, start, 10, 5)
	if err != nil {
		t.Fatalf("FindFreePair: %v", err)
	}
	if grpc == start {
		t.Fatalf("expected to skip reserved %d, got grpc=%d", start, grpc)
	}
	if health != grpc+1 {
		t.Fatalf("health %d != grpc+1 %d", health, grpc+1)
	}
	if grpc != start+10 {
		t.Fatalf("expected next bumped slot %d, got %d", start+10, grpc)
	}
}

func TestFindFreePair_SkipsBoundPort(t *testing.T) {
	start := pickEphemeralPort(t)

	// Hold the start port so the finder must bump.
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", start))
	if err != nil {
		t.Skipf("could not bind start port %d: %v", start, err)
	}
	defer l.Close()

	grpc, _, err := FindFreePair(nil, start, 10, 5)
	if err != nil {
		t.Fatalf("FindFreePair: %v", err)
	}
	if grpc == start {
		t.Fatalf("expected to skip bound port %d, got %d", start, grpc)
	}
}

func TestFindFreePair_ExhaustsAttempts(t *testing.T) {
	// Reserve every candidate slot to force exhaustion.
	start := pickEphemeralPort(t)
	reserved := map[int]bool{}
	for i := 0; i < 5; i++ {
		reserved[start+i*10] = true
		reserved[start+i*10+1] = true
	}
	if _, _, err := FindFreePair(reserved, start, 10, 5); err == nil {
		t.Fatal("expected error when all candidate pairs are reserved")
	}
}

// pickEphemeralPort asks the OS for a free port and immediately releases it.
// The window between release and reuse is small enough to be reliable in
// practice for these unit tests.
func pickEphemeralPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}
