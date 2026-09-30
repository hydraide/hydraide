package server

import (
	"net"
	"path/filepath"
	"strconv"
	"testing"
)

// startupTestConfig builds a Configuration with a fresh mTLS pair and an
// isolated HYDRAIDE_ROOT_PATH, mirroring TestStopPhaseOrdering.
func startupTestConfig(t *testing.T, port int) *Configuration {
	t.Helper()
	root := t.TempDir()
	if err := generateMTLSPair(filepath.Join(root, "certificate")); err != nil {
		t.Fatalf("certs: %v", err)
	}
	t.Setenv("HYDRAIDE_ROOT_PATH", root)
	return &Configuration{
		CertificateCrtFile:    filepath.Join(root, "certificate", "server.crt"),
		CertificateKeyFile:    filepath.Join(root, "certificate", "server.key"),
		ClientCAFile:          filepath.Join(root, "certificate", "ca.crt"),
		HydraServerPort:       port,
		HydraMaxMessageSize:   16 * 1024 * 1024,
		DefaultCloseAfterIdle: 600,
		DefaultWriteInterval:  2,
		DefaultFileSize:       8 * 1024 * 1024,
		UseV2Engine:           true,
	}
}

// TestStart_PortInUse_ReturnsError: a listener failure must come back from
// Start() instead of being swallowed by the serving goroutine's panic handler
// (which left a process running with no gRPC port).
func TestStart_PortInUse_ReturnsError(t *testing.T) {
	occupied, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port

	srv := New(startupTestConfig(t, port))
	defer srv.Stop()
	if err := srv.Start(); err == nil {
		t.Fatalf("Start on an occupied port %d returned nil, want an error", port)
	}
}

// TestStart_MissingCertificate_ReturnsError: same for a TLS setup failure.
func TestStart_MissingCertificate_ReturnsError(t *testing.T) {
	port, err := freePort()
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	cfg := startupTestConfig(t, port)
	cfg.CertificateCrtFile = filepath.Join(t.TempDir(), "missing.crt")

	srv := New(cfg)
	defer srv.Stop()
	if err := srv.Start(); err == nil {
		t.Fatal("Start with a missing certificate returned nil, want an error")
	}
	// the listener opened before the TLS failure must be released
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		t.Fatalf("port %d still bound after the failed Start: %v", port, err)
	}
	_ = lis.Close()
}

// TestStop_RightAfterStart_ReleasesPort: Stop() called immediately after
// Start() must shut the gRPC server down. The server used to be created inside
// the serving goroutine, so a quick Stop saw a nil s.grpcServer, skipped the
// gRPC shutdown, and the port stayed bound.
func TestStop_RightAfterStart_ReleasesPort(t *testing.T) {
	for i := 0; i < 3; i++ {
		port, err := freePort()
		if err != nil {
			t.Fatalf("freePort: %v", err)
		}
		srv := New(startupTestConfig(t, port))
		if err := srv.Start(); err != nil {
			t.Fatalf("Start: %v", err)
		}
		srv.Stop()

		lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
		if err != nil {
			t.Fatalf("iteration %d: port %d still bound after Stop(): %v", i, port, err)
		}
		_ = lis.Close()
	}
}
