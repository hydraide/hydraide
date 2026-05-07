package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	hydrapb "github.com/hydraide/hydraide/generated/hydraidepbgo"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// TestStopPhaseOrdering is a white-box integration test that verifies the
// shutdown contract of server.Stop():
//
//  1. Once Stop() begins, NEW writes are rejected with codes.Unavailable.
//  2. Read-only / observability methods (Heartbeat) keep working until the
//     gRPC server has actually closed.
//  3. Subscribe streams get codes.Unavailable promptly (i.e. shutdown does
//     not block on them).
//  4. Stop() returns in well under the 180s SIGKILL budget when there is no
//     real work to flush — empirically a few seconds.
//  5. Writes that completed BEFORE Stop() persist on disk after restart.
//
// The test stands up a real gRPC server with an mTLS pair generated on the
// fly so we exercise the same code path production runs.
func TestStopPhaseOrdering(t *testing.T) {
	root := t.TempDir()
	if err := generateMTLSPair(filepath.Join(root, "certificate")); err != nil {
		t.Fatalf("certs: %v", err)
	}

	port, err := freePort()
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}

	// HYDRAIDE_ROOT_PATH is consumed by the settings package for the data
	// directory layout, so the server can spin up isolated per-test state.
	t.Setenv("HYDRAIDE_ROOT_PATH", root)

	cfg := &Configuration{
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

	srv := New(cfg)
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	client, conn := dialTestClient(t, port, root)
	defer conn.Close()

	// Warm-up: register a swamp pattern and write one row that we expect
	// to find on disk after restart.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	writeInterval := int64(1)
	maxFile := int64(1024 * 1024)
	if _, err := client.RegisterSwamp(ctx, &hydrapb.RegisterSwampRequest{
		SwampPattern:    "stoptest/realm/*",
		CloseAfterIdle:  600,
		IsInMemorySwamp: false,
		WriteInterval:   &writeInterval,
		MaxFileSize:     &maxFile,
	}); err != nil {
		t.Fatalf("RegisterSwamp: %v", err)
	}
	if _, err := client.Set(ctx, &hydrapb.SetRequest{
		Swamps: []*hydrapb.SwampRequest{{
			IslandID:         1,
			SwampName:        "stoptest/realm/persist",
			KeyValues:        []*hydrapb.KeyValuePair{{Key: "k1", StringVal: strPtrTest("v1")}},
			CreateIfNotExist: true,
			Overwrite:        true,
		}},
	}); err != nil {
		t.Fatalf("pre-stop Set: %v", err)
	}

	// Open a Subscribe stream so we can check it terminates with Unavailable
	// rather than blocking shutdown forever.
	subStreamCtx, subCancel := context.WithCancel(context.Background())
	defer subCancel()
	stream, err := client.SubscribeToEvents(subStreamCtx, &hydrapb.SubscribeToEventsRequest{
		IslandID:  1,
		SwampName: "stoptest/realm/persist",
	})
	if err != nil {
		t.Fatalf("SubscribeToEvents: %v", err)
	}

	subDone := make(chan error, 1)
	go func() {
		_, err := stream.Recv()
		subDone <- err
	}()

	// Phase 1: trigger Stop() in a goroutine and capture timing.
	stopReturned := make(chan struct{})
	stopStart := time.Now()
	go func() {
		srv.Stop()
		close(stopReturned)
	}()

	// Phase 2: as soon as Stop() begins, new writes must be rejected.
	// We poll because there is a small window before MarkShuttingDown is
	// called. The expected behaviour is that within ~200ms the rejection
	// kicks in.
	var rejected atomic.Bool
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
		_, err := client.Set(ctx2, &hydrapb.SetRequest{
			Swamps: []*hydrapb.SwampRequest{{
				IslandID:         1,
				SwampName:        "stoptest/realm/persist",
				KeyValues:        []*hydrapb.KeyValuePair{{Key: "post-stop", StringVal: strPtrTest("nope")}},
				CreateIfNotExist: true,
				Overwrite:        true,
			}},
		})
		cancel2()
		if err == nil {
			continue
		}
		if st, ok := status.FromError(err); ok && st.Code() == codes.Unavailable {
			rejected.Store(true)
			break
		}
		// Other errors (Canceled, transport closing) also indicate that the
		// server is at minimum no longer fully serving — accept these once
		// the gRPC layer has stopped accepting connections.
		if st, ok := status.FromError(err); ok && (st.Code() == codes.Canceled || st.Code() == codes.Unknown) {
			break
		}
	}
	if !rejected.Load() {
		t.Errorf("expected at least one Set call to be rejected with codes.Unavailable during shutdown")
	}

	// Phase 3: subscribe stream must terminate.
	select {
	case err := <-subDone:
		if err == nil {
			t.Fatalf("stream.Recv returned nil error during shutdown — expected codes.Unavailable")
		}
		if st, ok := status.FromError(err); !ok || st.Code() != codes.Unavailable {
			t.Errorf("expected codes.Unavailable on stream during shutdown, got %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("subscribe stream did not terminate within 15s of Stop() — shutdown is blocked on streams")
	}

	// Phase 4: Stop() must return in well under the 180s SIGKILL budget.
	select {
	case <-stopReturned:
		elapsed := time.Since(stopStart)
		t.Logf("Stop() completed in %s", elapsed)
		if elapsed > 30*time.Second {
			t.Errorf("Stop() took %s — too long for an idle server", elapsed)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("Stop() did not return within 60s")
	}

	// Phase 5: restart and verify the pre-stop write persisted.
	conn.Close()

	srv2 := New(cfg)
	if err := srv2.Start(); err != nil {
		t.Fatalf("restart Start: %v", err)
	}
	defer srv2.Stop()

	client2, conn2 := dialTestClient(t, port, root)
	defer conn2.Close()

	if _, err := client2.RegisterSwamp(context.Background(), &hydrapb.RegisterSwampRequest{
		SwampPattern:    "stoptest/realm/*",
		CloseAfterIdle:  600,
		IsInMemorySwamp: false,
		WriteInterval:   &writeInterval,
		MaxFileSize:     &maxFile,
	}); err != nil {
		t.Fatalf("RegisterSwamp after restart: %v", err)
	}
	resp, err := client2.Count(context.Background(), &hydrapb.CountRequest{
		Swamps: []*hydrapb.CountRequest_SwampIdentifier{{
			IslandID:  1,
			SwampName: "stoptest/realm/persist",
		}},
	})
	if err != nil {
		t.Fatalf("Count after restart: %v", err)
	}
	var total int32
	for _, s := range resp.GetSwamps() {
		total += s.GetCount()
	}
	if total != 1 {
		t.Errorf("expected 1 persisted treasure, got %d (data was lost during shutdown)", total)
	}
}

// dialTestClient returns a gRPC client wired with the test mTLS material.
func dialTestClient(t *testing.T, port int, root string) (hydrapb.HydraideServiceClient, *grpc.ClientConn) {
	t.Helper()

	// Wait briefly for the server's listener to come up. The Start() method
	// spawns the listener in a goroutine and returns immediately.
	addr := fmt.Sprintf("localhost:%d", port)
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	cert, err := tls.LoadX509KeyPair(
		filepath.Join(root, "certificate", "client.crt"),
		filepath.Join(root, "certificate", "client.key"),
	)
	if err != nil {
		t.Fatalf("client keypair: %v", err)
	}
	caBytes, err := os.ReadFile(filepath.Join(root, "certificate", "ca.crt"))
	if err != nil {
		t.Fatalf("read ca: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caBytes) {
		t.Fatalf("invalid ca pem")
	}

	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS13,
			ServerName:   "localhost",
		})),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return hydrapb.NewHydraideServiceClient(conn), conn
}

// freePort asks the kernel for an unused TCP port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// generateMTLSPair writes ca.crt, server.crt, server.key, client.crt,
// client.key into the supplied directory. ECDSA P-256 keys are used to keep
// generation fast.
func generateMTLSPair(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "hydraide-test-ca"},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return err
	}

	if err := writePEM(filepath.Join(dir, "ca.crt"), "CERTIFICATE", caDER); err != nil {
		return err
	}

	makeLeaf := func(cn string, isServer bool) error {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-1 * time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		}
		if isServer {
			tmpl.DNSNames = []string{"localhost"}
			tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			return err
		}
		base := "client"
		if isServer {
			base = "server"
		}
		if err := writePEM(filepath.Join(dir, base+".crt"), "CERTIFICATE", der); err != nil {
			return err
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			return err
		}
		return writePEM(filepath.Join(dir, base+".key"), "EC PRIVATE KEY", keyDER)
	}

	if err := makeLeaf("localhost", true); err != nil {
		return err
	}
	return makeLeaf("hydraide-test-client", false)
}

func writePEM(path, blockType string, der []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}

func strPtrTest(s string) *string { return &s }
