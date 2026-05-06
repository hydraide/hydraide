// Package setup is the single source of truth for connecting to the
// HydrAIDE instance the example tree runs against. Every recipe and every
// reference app reaches HydrAIDE through here so that:
//
//   - There is exactly one place that knows about HYDRA_HOST and HYDRA_CERT.
//   - The defaults match the docker-compose layout in this directory.
//   - Tests get the same connection logic as runnable examples plus
//     automatic per-test Swamp cleanup via t.Cleanup.
//
// A .env.local file at the example tree root can override the defaults to
// point at a unit HydrAIDE instance (never live — see docs/sdk/go/testing.md).
package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/client"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

const (
	// AllIslands matches the docker-compose default and is fixed for the
	// lifetime of an instance — changing it would break Swamp routing.
	AllIslands = 1000

	// MaxMessageSize is the gRPC frame ceiling. 10 MiB is generous for the
	// example payloads and matches the compose configuration.
	MaxMessageSize = 10 * 1024 * 1024

	defaultHost   = "localhost:5980"
	defaultCertOK = "../../certificate"
)

// Config carries the resolved connection parameters. Read it from the
// environment via Load.
type Config struct {
	Host               string
	CertDir            string
	ConnectionAnalysis bool
}

// Load resolves the connection config from the environment. The cert
// directory falls back to a discovery walk that searches upward for a
// `certificate/` folder, so recipes work regardless of how deeply they
// are nested in the example tree.
func Load() Config {
	certDir := os.Getenv("HYDRA_CERT")
	if certDir == "" {
		certDir = discoverCertDir()
	}
	cfg := Config{
		Host:    envOr("HYDRA_HOST", defaultHost),
		CertDir: certDir,
	}
	cfg.ConnectionAnalysis = os.Getenv("HYDRA_CONNECTION_ANALYSIS") == "true"
	return cfg
}

// discoverCertDir walks upward from the current working directory looking
// for a `certificate/ca.crt`. This makes the in-tree examples work from
// any depth without needing to set HYDRA_CERT explicitly.
func discoverCertDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return defaultCertOK
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "certificate")
		if _, err := os.Stat(filepath.Join(candidate, "ca.crt")); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return defaultCertOK
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// NewClient connects to HydrAIDE using the resolved config. The returned
// cleanup function is a no-op for now (the underlying client manages its
// own connections) but stays in the signature so callers can defer it
// without thinking about lifecycle changes later.
func NewClient(_ context.Context) (repo.Repo, func(), error) {
	cfg := Load()

	certDir, err := filepath.Abs(cfg.CertDir)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve cert dir: %w", err)
	}

	r := repo.New([]*client.Server{
		{
			Host:          cfg.Host,
			FromIsland:    1,
			ToIsland:      AllIslands,
			CACrtPath:     filepath.Join(certDir, "ca.crt"),
			ClientCrtPath: filepath.Join(certDir, "client.crt"),
			ClientKeyPath: filepath.Join(certDir, "client.key"),
		},
	}, AllIslands, MaxMessageSize, cfg.ConnectionAnalysis)

	return r, func() {}, nil
}

// MustClient is the panic-on-error variant for short-lived recipe main
// functions where surfacing an error is unnecessary noise.
func MustClient(ctx context.Context) (repo.Repo, func()) {
	r, cleanup, err := NewClient(ctx)
	if err != nil {
		panic(err)
	}
	return r, cleanup
}

// NewTestClient is the test-suite entry point. It returns a connected repo
// and registers a cleanup that destroys every Swamp the test recorded via
// TrackSwamp. Tests can therefore declare their Swamps and trust that the
// instance is left clean afterwards.
func NewTestClient(t *testing.T) (repo.Repo, *Tracker) {
	t.Helper()
	r, cleanup, err := NewClient(context.Background())
	if err != nil {
		t.Fatalf("connect to HydrAIDE: %v", err)
	}
	tracker := &Tracker{repo: r}
	t.Cleanup(func() {
		tracker.destroyAll(t)
		cleanup()
	})
	return r, tracker
}

// Tracker remembers Swamps a test created so they can be destroyed on
// teardown. Per-test isolation is the load-bearing convention for running
// these tests in parallel against a shared instance.
type Tracker struct {
	mu    sync.Mutex
	repo  repo.Repo
	names []name.Name
}

// Track records a Swamp so it will be destroyed at the end of the test.
func (tr *Tracker) Track(n name.Name) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.names = append(tr.names, n)
}

func (tr *Tracker) destroyAll(t *testing.T) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if len(tr.names) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h := tr.repo.GetHydraidego()
	for _, n := range tr.names {
		if err := h.Destroy(ctx, n); err != nil {
			t.Logf("cleanup: destroy %s: %v", n.Get(), err)
		}
	}
}

// EventStatusName returns a short human label for log output. The SDK
// does not ship a Stringer for EventStatus.
func EventStatusName(s hydraidego.EventStatus) string {
	switch s {
	case hydraidego.StatusUnknown:
		return "UNKNOWN"
	case hydraidego.StatusSwampNotFound:
		return "SWAMP_NOT_FOUND"
	case hydraidego.StatusTreasureNotFound:
		return "TREASURE_NOT_FOUND"
	case hydraidego.StatusNew:
		return "NEW"
	case hydraidego.StatusModified:
		return "MODIFIED"
	case hydraidego.StatusNothingChanged:
		return "NOTHING_CHANGED"
	case hydraidego.StatusDeleted:
		return "DELETED"
	}
	return fmt.Sprintf("EventStatus(%d)", s)
}

// Pattern registers a swamp pattern via RegisterSwamp using sensible
// defaults for the example tree. Recipes that need different lifetimes
// should call RegisterSwamp directly.
//
// The encoding is MessagePack — which is required for PatchTreasures and
// for cross-language SDKs to read the data. The example tree never uses
// GOB-encoded swamps.
func Pattern(ctx context.Context, r repo.Repo, pattern name.Name) error {
	errs := r.GetHydraidego().RegisterSwamp(ctx, &hydraidego.RegisterSwampRequest{
		SwampPattern:    pattern,
		CloseAfterIdle:  10 * time.Minute,
		IsInMemorySwamp: false,
		FilesystemSettings: &hydraidego.SwampFilesystemSettings{
			WriteInterval:  time.Second,
			MaxFileSize:    8192,
			EncodingFormat: hydraidego.EncodingMsgPack,
		},
	})
	if len(errs) == 0 {
		return nil
	}
	combined := errs[0]
	for _, e := range errs[1:] {
		combined = fmt.Errorf("%v; %v", combined, e)
	}
	return combined
}
