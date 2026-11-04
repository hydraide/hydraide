package lock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// queueLen returns the number of callers queued (holder included) for key.
func queueLen(l Lock, key string) int {
	v, ok := l.(*lock).queues.Load(key)
	if !ok {
		return 0
	}
	q := v.(*queue)
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.callers)
}

// waitQueueLen polls until the queue for key reaches want callers.
func waitQueueLen(t *testing.T, l Lock, key string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if queueLen(l, key) == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("queue length for %q: got %d, want %d", key, queueLen(l, key), want)
}

// A waiter whose context ends while another caller holds the key must leave
// the queue and must not take the lock when the holder unlocks.
func TestLockCancelledWaiterNeverAcquires(t *testing.T) {
	l := New()
	const key = "k"

	holderID, err := l.Lock(context.Background(), key, time.Minute)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	abandonedID, err := l.Lock(ctx, key, time.Minute)
	require.Error(t, err)
	require.Empty(t, abandonedID)

	// Only the holder is left in the queue.
	require.Equal(t, 1, queueLen(l, key))

	require.NoError(t, l.Unlock(key, holderID))
	require.Equal(t, 0, queueLen(l, key))

	// The key is free right away, not after the abandoned waiter's TTL.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel2()
	nextID, err := l.Lock(ctx2, key, time.Minute)
	require.NoError(t, err)
	require.NoError(t, l.Unlock(key, nextID))
}

// With holder, cancelled waiter, and two live waiters queued in that order,
// Unlock hands the key to the first live waiter (FIFO is kept) without delay.
func TestLockNextLiveWaiterGetsLockAfterCancelledOne(t *testing.T) {
	l := New()
	const key = "k"

	holderID, err := l.Lock(context.Background(), key, time.Minute)
	require.NoError(t, err)

	type result struct {
		name string
		id   string
		at   time.Time
	}
	results := make(chan result, 2)

	cancelCtx, cancel := context.WithCancel(context.Background())
	abandonedDone := make(chan error, 1)
	go func() {
		_, err := l.Lock(cancelCtx, key, time.Minute)
		abandonedDone <- err
	}()
	waitQueueLen(t, l, key, 2)

	startWaiter := func(name string, wantLen int) {
		go func() {
			id, err := l.Lock(context.Background(), key, time.Minute)
			if err != nil {
				t.Errorf("%s: %v", name, err)
				return
			}
			results <- result{name: name, id: id, at: time.Now()}
		}()
		waitQueueLen(t, l, key, wantLen)
	}
	startWaiter("first", 3)
	startWaiter("second", 4)

	cancel()
	require.Error(t, <-abandonedDone)
	waitQueueLen(t, l, key, 3)

	unlockedAt := time.Now()
	require.NoError(t, l.Unlock(key, holderID))

	select {
	case r := <-results:
		require.Equal(t, "first", r.name)
		require.Less(t, r.at.Sub(unlockedAt), 100*time.Millisecond)
		require.NoError(t, l.Unlock(key, r.id))
	case <-time.After(time.Second):
		t.Fatal("first live waiter did not get the lock after Unlock")
	}

	select {
	case r := <-results:
		require.Equal(t, "second", r.name)
		require.NoError(t, l.Unlock(key, r.id))
	case <-time.After(time.Second):
		t.Fatal("second live waiter did not get the lock")
	}
	require.Equal(t, 0, queueLen(l, key))
}

// A context that is already done must never be granted the lock, even on a
// free key where the ready channel is closed before select runs and select
// would otherwise pick between ready and ctx.Done() at random.
func TestLockAlreadyCancelledContextNeverAcquires(t *testing.T) {
	l := New()
	const key = "k"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for i := 0; i < 1000; i++ {
		id, err := l.Lock(ctx, key, time.Minute)
		require.Error(t, err, "iteration %d", i)
		require.Empty(t, id)
		require.Equal(t, 0, queueLen(l, key))
	}
}

// The waiter's cancel races with the holder's Unlock. Whatever the outcome,
// the lock must not leak: either the waiter got the lock (and unlocks it),
// or it got an error and the key is free immediately afterwards.
func TestLockCancelGrantRaceDoesNotLeak(t *testing.T) {
	l := New()
	const key = "k"
	iterations := 2000
	if testing.Short() {
		iterations = 200
	}

	granted, refused := 0, 0
	for i := 0; i < iterations; i++ {
		holderID, err := l.Lock(context.Background(), key, time.Minute)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		type res struct {
			id  string
			err error
		}
		waiterDone := make(chan res, 1)
		go func() {
			id, err := l.Lock(ctx, key, time.Minute)
			waiterDone <- res{id, err}
		}()
		waitQueueLen(t, l, key, 2)

		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			cancel()
		}()
		go func() {
			defer wg.Done()
			<-start
			if err := l.Unlock(key, holderID); err != nil {
				t.Errorf("iteration %d: holder unlock: %v", i, err)
			}
		}()
		close(start)
		wg.Wait()

		r := <-waiterDone
		if r.err == nil {
			granted++
			require.NoError(t, l.Unlock(key, r.id), "iteration %d", i)
		} else {
			refused++
		}

		// No one may be left holding the key.
		require.Equal(t, 0, queueLen(l, key), "iteration %d: lock leaked", i)
	}
	t.Logf("granted=%d refused=%d", granted, refused)
}

// A granted lock is bound to its TTL, not to the caller's context: cancelling
// the context after Lock returned must not release it.
func TestLockCancelAfterAcquireKeepsLock(t *testing.T) {
	l := New()
	const key = "k"

	ctx, cancel := context.WithCancel(context.Background())
	id, err := l.Lock(ctx, key, time.Minute)
	require.NoError(t, err)
	cancel()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	_, err = l.Lock(ctx2, key, time.Minute)
	require.Error(t, err, "lock was released by the caller's context cancel")

	require.NoError(t, l.Unlock(key, id))
}
