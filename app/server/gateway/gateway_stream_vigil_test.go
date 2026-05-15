package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/core/zeus"
	"github.com/hydraide/hydraide/app/name"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// streamVigilRig spins up a real Zeus + Hydra wired Gateway so the two
// streaming-read RPCs can be exercised end-to-end without the gRPC layer.
type streamVigilRig struct {
	gw        Gateway
	swampName string
	islandID  uint64
}

func newStreamVigilRig(t *testing.T, sanctuary, realm, swampN string) *streamVigilRig {
	t.Helper()
	const (
		maxDepth        = 3
		maxFolderPerLvl = 2000
	)
	settingsInterface := settings.New(maxDepth, maxFolderPerLvl)
	settingsInterface.RegisterPattern(
		name.New().Sanctuary(sanctuary).Realm("*").Swamp("*"),
		false, 1,
		&settings.FileSystemSettings{WriteIntervalSec: 1, MaxFileSizeByte: 8192},
	)
	fsInterface := filesystem.New()
	zeusInterface := zeus.New(settingsInterface, fsInterface)
	zeusInterface.StartHydra()

	gw := Gateway{
		SettingsInterface:     settingsInterface,
		ZeusInterface:         zeusInterface,
		DefaultCloseAfterIdle: 1,
		DefaultWriteInterval:  1,
		DefaultFileSize:       8192,
	}

	return &streamVigilRig{
		gw:        gw,
		swampName: name.New().Sanctuary(sanctuary).Realm(realm).Swamp(swampN).Get(),
		islandID:  1,
	}
}

// summonAndSeed loads the swamp into Hydra memory and writes one treasure so
// the beacon walk / key lookup in the RPC reaches stream.Send().
func (r *streamVigilRig) summonAndSeed(t *testing.T, key string) {
	t.Helper()
	hydraInterface := r.gw.ZeusInterface.GetHydra()
	swampObj, err := hydraInterface.SummonSwamp(context.Background(), r.islandID, name.Load(r.swampName))
	require.NoError(t, err)

	swampObj.BeginVigil()
	tr := swampObj.CreateTreasure(key)
	gid := tr.StartTreasureGuard(true)
	tr.SetContentString(gid, "seed-"+key)
	tr.Save(gid)
	tr.ReleaseTreasureGuard(gid)
	swampObj.CeaseVigil()

	// Precondition: the seeding vigil pair above is balanced.
	require.False(t, swampObj.HasActiveVigils(), "precondition: no active vigils after seeding")
}

// --- panic-injecting streams ------------------------------------------------
//
// Every method is a stub except Send(), which panics. Send() is invoked from
// inside the per-query body *after* BeginVigil(). The function-level
// defer handlePanic() recovers the panic and does NOT re-panic, so the RPC
// returns normally. On the buggy (defer-less) code path the manual
// CeaseVigil() is skipped during the stack unwind and the vigil leaks forever.

type panicIndexStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (p *panicIndexStream) Context() context.Context { return p.ctx }
func (p *panicIndexStream) Send(*hydrapb.GetByIndexStreamFromManyResponse) error {
	panic("injected panic inside GetByIndexStreamFromMany per-query loop")
}

type panicGetStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (p *panicGetStream) Context() context.Context { return p.ctx }
func (p *panicGetStream) Send(*hydrapb.GetStreamResponse) error {
	panic("injected panic inside GetStream per-query loop")
}

// assertNoVigilLeak summons the swamp again and verifies the vigil counter is
// back to zero and Destroy() does not block on WaitForActiveVigilsClosed().
func assertNoVigilLeak(t *testing.T, r *streamVigilRig) {
	t.Helper()
	hydraInterface := r.gw.ZeusInterface.GetHydra()
	swampObj, err := hydraInterface.SummonSwamp(context.Background(), r.islandID, name.Load(r.swampName))
	require.NoError(t, err)

	require.False(t, swampObj.HasActiveVigils(),
		"vigil leaked: CeaseVigil() was skipped when the per-query body panicked")

	done := make(chan struct{})
	go func() {
		swampObj.Destroy()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Destroy() blocked on WaitForActiveVigilsClosed() — vigil leak")
	}
}

func TestGetByIndexStreamFromMany_VigilNotLeakedOnPanic(t *testing.T) {
	rig := newStreamVigilRig(t, "gw-vigil-1", "stream", "index")
	rig.summonAndSeed(t, "k1")

	req := &hydrapb.GetByIndexStreamFromManyRequest{
		Queries: []*hydrapb.SwampQuery{{
			IslandID:  rig.islandID,
			SwampName: rig.swampName,
			IndexType: hydrapb.IndexType_CREATION_TIME,
			// no filters -> beacon-walk path -> seeded treasure ->
			// loop reaches stream.Send() -> panicIndexStream panics
		}},
	}

	// handlePanic() recovers the injected panic; the RPC returns without
	// re-panicking. The recover happens inside the test goroutine, so guard it.
	func() {
		defer func() { _ = recover() }()
		_ = rig.gw.GetByIndexStreamFromMany(req, &panicIndexStream{ctx: context.Background()})
	}()

	assertNoVigilLeak(t, rig)
}

func TestGetStream_VigilNotLeakedOnPanic(t *testing.T) {
	rig := newStreamVigilRig(t, "gw-vigil-2", "stream", "profile")
	rig.summonAndSeed(t, "k1")

	req := &hydrapb.GetStreamRequest{
		Queries: []*hydrapb.ProfileSwampQuery{{
			IslandID:  rig.islandID,
			SwampName: rig.swampName,
			Keys:      []string{"k1"},
			// nil filters -> evaluateNativeProfileFilterGroup passes ->
			// loop reaches stream.Send() -> panicGetStream panics
		}},
	}

	func() {
		defer func() { _ = recover() }()
		_ = rig.gw.GetStream(req, &panicGetStream{ctx: context.Background()})
	}()

	assertNoVigilLeak(t, rig)
}
