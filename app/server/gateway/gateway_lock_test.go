package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/hydraide/hydraide/app/core/filesystem"
	"github.com/hydraide/hydraide/app/core/settings"
	"github.com/hydraide/hydraide/app/core/zeus"
	hydrapb "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newGatewayLockTestGateway(t *testing.T) Gateway {
	t.Helper()
	t.Setenv("HYDRAIDE_ROOT_PATH", t.TempDir())
	settingsInterface := settings.New(3, 2000)
	zeusInterface := zeus.New(settingsInterface, filesystem.New())
	zeusInterface.StartHydra()
	return Gateway{SettingsInterface: settingsInterface, ZeusInterface: zeusInterface}
}

// A client that gives up waiting must not take the lock later. Before the fix
// the gateway detached the client context, so the abandoned request stayed in
// the queue, acquired the lock when the holder unlocked, and kept it for its
// full TTL with nobody able to unlock it.
func TestGatewayLock_AbandonedWaiterDoesNotAcquire(t *testing.T) {
	gw := newGatewayLockTestGateway(t)
	const key = "gateway-lock-abandoned"
	const ttlMs = int64(10 * 60 * 1000)

	first, err := gw.Lock(context.Background(), &hydrapb.LockRequest{Key: key, TTL: ttlMs})
	require.NoError(t, err)

	waitCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = gw.Lock(waitCtx, &hydrapb.LockRequest{Key: key, TTL: ttlMs})
	require.Error(t, err)
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))

	_, err = gw.Unlock(context.Background(), &hydrapb.UnlockRequest{Key: key, LockID: first.GetLockID()})
	require.NoError(t, err)

	// The key must be free now, not held by the abandoned second request.
	thirdCtx, cancel3 := context.WithTimeout(context.Background(), time.Second)
	defer cancel3()
	third, err := gw.Lock(thirdCtx, &hydrapb.LockRequest{Key: key, TTL: ttlMs})
	require.NoError(t, err, "abandoned waiter took the lock")
	_, err = gw.Unlock(context.Background(), &hydrapb.UnlockRequest{Key: key, LockID: third.GetLockID()})
	require.NoError(t, err)
}

// The handler's context is cancelled once the RPC returns. A granted lock must
// survive that: it is released only by Unlock or by its TTL. This is the
// behaviour the removed context.WithoutCancel used to protect.
func TestGatewayLock_GrantedLockSurvivesContextCancel(t *testing.T) {
	gw := newGatewayLockTestGateway(t)
	const key = "gateway-lock-survives"
	const ttlMs = int64(60 * 1000)

	rpcCtx, endRPC := context.WithCancel(context.Background())
	held, err := gw.Lock(rpcCtx, &hydrapb.LockRequest{Key: key, TTL: ttlMs})
	require.NoError(t, err)
	endRPC()

	probeCtx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = gw.Lock(probeCtx, &hydrapb.LockRequest{Key: key, TTL: ttlMs})
	require.Error(t, err, "granted lock was released when the RPC context ended")

	_, err = gw.Unlock(context.Background(), &hydrapb.UnlockRequest{Key: key, LockID: held.GetLockID()})
	require.NoError(t, err)
}

// A request whose context is already gone is refused and leaves nothing
// behind, even on a free key.
func TestGatewayLock_CancelledContextOnFreeKeyLeavesKeyFree(t *testing.T) {
	gw := newGatewayLockTestGateway(t)
	const key = "gateway-lock-precancelled"

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := gw.Lock(ctx, &hydrapb.LockRequest{Key: key, TTL: 60 * 1000})
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))

	liveCtx, cancelLive := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelLive()
	resp, err := gw.Lock(liveCtx, &hydrapb.LockRequest{Key: key, TTL: 60 * 1000})
	require.NoError(t, err)
	_, err = gw.Unlock(context.Background(), &hydrapb.UnlockRequest{Key: key, LockID: resp.GetLockID()})
	require.NoError(t, err)
}
