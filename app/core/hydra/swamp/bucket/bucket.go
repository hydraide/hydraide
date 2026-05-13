// Package bucket implements auto-built field-bucket indexes for swamps.
//
// A Bucket is an in-memory equality index keyed by the canonical value of
// a single dotted body field path. Buckets are built lazily: the first
// filter that can use a bucket triggers the build, the cost is one body
// pass over the swamp, and the bucket lives for the swamp's summoned
// lifetime. Closing the swamp drops every bucket.
//
// This package contains only the bucket data structure and its lifecycle.
// The decision of whether to build a bucket, and the binding to the swamp
// SaveFunction hooks, live in app/core/hydra/swamp. The decision of which
// filters route through a bucket lives in app/server/gateway.
package bucket

import (
	"sync"
	"sync/atomic"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/bucket/valuecanon"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/vmihailenco/msgpack/v5"
)

// RangeOp is the comparison operator for a range lookup. v1 stubs only;
// the range view is wired in v1.1.
type RangeOp uint8

const (
	RangeOpGT RangeOp = iota + 1
	RangeOpGE
	RangeOpLT
	RangeOpLE
)

// PendingOpKind tags a buffered mutation that arrived while a build was
// in flight. The build replays these in FIFO order at the end.
type PendingOpKind uint8

const (
	PendingInsert PendingOpKind = iota + 1
	PendingUpdate
	PendingDelete
)

// PendingOp is a mutation buffered during a build.
type PendingOp struct {
	Kind PendingOpKind
	T    treasure.Treasure // Insert / Update
	Key  string            // Delete
}

// Bucket is the public surface of a field-bucket index.
type Bucket interface {
	FieldPath() string

	// Equality view (v1).
	EqualityInitialized() bool
	LookupEqual(value any) []treasure.Treasure
	LookupIn(values []any) []treasure.Treasure
	BuildEquality(snapshot map[string]treasure.Treasure) error
	CountForValue(value any) int

	// Range view (v1.1 — v1 stubs return zero/false).
	RangeInitialized() bool
	LookupRange(op RangeOp, value any) []treasure.Treasure
	LookupBetween(lo, hi any, incLo, incHi bool) []treasure.Treasure
	BuildRange(snapshot map[string]treasure.Treasure) error

	// Mutation hooks. The caller (swamp) decides per-bucket whether to
	// invoke these; the bucket itself decides whether to apply directly
	// or buffer into the pending list (when buildInFlight == 1).
	OnInsert(t treasure.Treasure) error
	OnUpdate(t treasure.Treasure) error
	OnDelete(key string)

	// Build-in-flight pending buffer.
	EnqueuePending(op PendingOp)
	DrainPending() error

	BuildInFlight() bool
	SetBuildInFlight(v bool)

	Count() int
	Reset()
}

// New constructs an empty Bucket for the given dotted field path.
func New(fieldPath string) Bucket {
	return &bucket{
		fieldPath: fieldPath,
		byValue:   map[valuecanon.Key]map[string]treasure.Treasure{},
		byKey:     map[string]valuecanon.Key{},
	}
}

type bucket struct {
	fieldPath string

	mu sync.RWMutex

	// Equality view.
	equalityInit atomic.Int32
	byValue      map[valuecanon.Key]map[string]treasure.Treasure
	byKey        map[string]valuecanon.Key

	// Range view (v1: untouched, RangeInitialized always false).
	rangeInit atomic.Int32

	// Pending mutations during build. Separate mutex so the Save path can
	// enqueue without contending on the main bucket mutex while the build
	// holds it.
	pendingMu     sync.Mutex
	pending       []PendingOp
	buildInFlight atomic.Int32

	// Serializes BuildEquality so that two concurrent GetOrBuildBucket
	// callers cooperate: the winner runs the build, the loser blocks here
	// and returns to an already-initialized bucket.
	buildOnce sync.Once
}

func (b *bucket) FieldPath() string { return b.fieldPath }

func (b *bucket) EqualityInitialized() bool { return b.equalityInit.Load() == 1 }
func (b *bucket) RangeInitialized() bool    { return b.rangeInit.Load() == 1 }
func (b *bucket) BuildInFlight() bool       { return b.buildInFlight.Load() == 1 }

func (b *bucket) SetBuildInFlight(v bool) {
	if v {
		b.buildInFlight.Store(1)
	} else {
		b.buildInFlight.Store(0)
	}
}

// BuildEquality populates the equality view from a swamp snapshot. The
// snapshot is a treasure-key → Treasure map cloned by the caller before
// this runs, so the build does not hold any swamp-side lock.
//
// Concurrency contract: the caller must SetBuildInFlight(true) before
// publishing this bucket into the swamp's bucket map, and call
// DrainPending() + SetBuildInFlight(false) after BuildEquality returns.
// The buildOnce guarantees a single goroutine builds even if multiple
// callers race on the same bucket.
func (b *bucket) BuildEquality(snapshot map[string]treasure.Treasure) error {
	var buildErr error
	b.buildOnce.Do(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for key, t := range snapshot {
			k, ok := extractKey(t, b.fieldPath)
			if !ok {
				k = valuecanon.NullKey
			}
			insertLocked(b, key, k, t)
		}
		b.equalityInit.Store(1)
	})
	return buildErr
}

// LookupEqual returns every Treasure currently mapped to the given value.
// The returned slice is a copy of the bucket's internal slice, safe to
// inspect after the bucket releases its read lock.
func (b *bucket) LookupEqual(value any) []treasure.Treasure {
	if !b.EqualityInitialized() {
		return nil
	}
	want := valuecanon.Canonicalize(value)
	b.mu.RLock()
	defer b.mu.RUnlock()
	return collectMatchingLocked(b, want)
}

// LookupIn returns the union of LookupEqual over the given values, with
// duplicate Treasures removed by key.
func (b *bucket) LookupIn(values []any) []treasure.Treasure {
	if !b.EqualityInitialized() || len(values) == 0 {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	seen := make(map[string]struct{})
	out := make([]treasure.Treasure, 0)
	for _, v := range values {
		want := valuecanon.Canonicalize(v)
		for _, t := range collectMatchingLocked(b, want) {
			k := t.GetKey()
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// CountForValue returns the number of Treasures mapped to value. Used by
// the planner to pick the most selective indexable leg.
func (b *bucket) CountForValue(value any) int {
	if !b.EqualityInitialized() {
		return 0
	}
	want := valuecanon.Canonicalize(value)
	b.mu.RLock()
	defer b.mu.RUnlock()
	return countMatchingLocked(b, want)
}

// OnInsert handles a new treasure. The decision of whether to buffer
// (during build) or apply directly is made atomically under pendingMu
// to close the race window between the build finishing its last drain
// and clearing the buildInFlight flag.
func (b *bucket) OnInsert(t treasure.Treasure) error {
	if b.tryEnqueue(PendingOp{Kind: PendingInsert, T: t}) {
		return nil
	}
	if !b.EqualityInitialized() {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	k, ok := extractKey(t, b.fieldPath)
	if !ok {
		k = valuecanon.NullKey
	}
	insertOrUpdateLocked(b, t.GetKey(), k, t)
	return nil
}

func (b *bucket) OnUpdate(t treasure.Treasure) error {
	if b.tryEnqueue(PendingOp{Kind: PendingUpdate, T: t}) {
		return nil
	}
	if !b.EqualityInitialized() {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	k, ok := extractKey(t, b.fieldPath)
	if !ok {
		k = valuecanon.NullKey
	}
	insertOrUpdateLocked(b, t.GetKey(), k, t)
	return nil
}

func (b *bucket) OnDelete(key string) {
	if b.tryEnqueue(PendingOp{Kind: PendingDelete, Key: key}) {
		return
	}
	if !b.EqualityInitialized() {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	deleteLocked(b, key)
}

// tryEnqueue appends op to the pending buffer iff a build is still in
// flight, atomically under pendingMu. Returns true if enqueued. The
// caller falls through to the direct-apply path on false. This closes
// the TOCTOU race between BuildInFlight() check and the drain loop's
// atomic clearing of the flag.
func (b *bucket) tryEnqueue(op PendingOp) bool {
	b.pendingMu.Lock()
	defer b.pendingMu.Unlock()
	if b.buildInFlight.Load() != 1 {
		return false
	}
	b.pending = append(b.pending, op)
	return true
}

// EnqueuePending unconditionally appends an op. Reserved for tests; the
// production Save path goes through OnInsert/OnUpdate/OnDelete which
// route via tryEnqueue.
func (b *bucket) EnqueuePending(op PendingOp) {
	b.pendingMu.Lock()
	b.pending = append(b.pending, op)
	b.pendingMu.Unlock()
}

// DrainPending replays buffered ops in FIFO order against the equality
// maps, then atomically clears the buildInFlight flag. The loop keeps
// running until it observes an empty pending buffer with pendingMu
// held; new tryEnqueue callers either land in this drain (still
// buildInFlight=1) or fall through to the direct path (after the flip).
//
// Concurrency contract: the caller publishes the bucket into the
// swamp map with buildInFlight already set to 1, runs BuildEquality
// (which is sync.Once-protected), then calls DrainPending. After
// DrainPending returns, BuildInFlight() is false.
func (b *bucket) DrainPending() error {
	for {
		b.pendingMu.Lock()
		if len(b.pending) == 0 {
			b.buildInFlight.Store(0)
			b.pendingMu.Unlock()
			return nil
		}
		ops := b.pending
		b.pending = nil
		b.pendingMu.Unlock()

		b.mu.Lock()
		for _, op := range ops {
			switch op.Kind {
			case PendingInsert, PendingUpdate:
				k, ok := extractKey(op.T, b.fieldPath)
				if !ok {
					k = valuecanon.NullKey
				}
				insertOrUpdateLocked(b, op.T.GetKey(), k, op.T)
			case PendingDelete:
				deleteLocked(b, op.Key)
			}
		}
		b.mu.Unlock()
	}
}

func (b *bucket) Count() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.byKey)
}

// Reset drops every Treasure reference and returns the bucket to its
// pre-build state. Tests use this; the swamp drops the entire bucket
// map on close instead of calling Reset.
func (b *bucket) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.byValue = map[valuecanon.Key]map[string]treasure.Treasure{}
	b.byKey = map[string]valuecanon.Key{}
	b.equalityInit.Store(0)
	b.rangeInit.Store(0)
	b.pendingMu.Lock()
	b.pending = nil
	b.pendingMu.Unlock()
	b.buildInFlight.Store(0)
	b.buildOnce = sync.Once{}
}

// ---- Range view stubs (v1) ----

func (b *bucket) LookupRange(op RangeOp, value any) []treasure.Treasure {
	return nil
}

func (b *bucket) LookupBetween(lo, hi any, incLo, incHi bool) []treasure.Treasure {
	return nil
}

func (b *bucket) BuildRange(snapshot map[string]treasure.Treasure) error {
	return nil
}

// ---- locked helpers (b.mu held) ----

func insertLocked(b *bucket, treasureKey string, valueKey valuecanon.Key, t treasure.Treasure) {
	slot, ok := b.byValue[valueKey]
	if !ok {
		slot = map[string]treasure.Treasure{}
		b.byValue[valueKey] = slot
	}
	slot[treasureKey] = t
	b.byKey[treasureKey] = valueKey
}

func insertOrUpdateLocked(b *bucket, treasureKey string, newKey valuecanon.Key, t treasure.Treasure) {
	if oldKey, exists := b.byKey[treasureKey]; exists {
		if oldKey == newKey {
			// Same value: still refresh the Treasure pointer in case it
			// is a fresh instance, but leave the value bucket untouched.
			b.byValue[oldKey][treasureKey] = t
			return
		}
		removeFromSlotLocked(b, oldKey, treasureKey)
	}
	insertLocked(b, treasureKey, newKey, t)
}

func deleteLocked(b *bucket, treasureKey string) {
	oldKey, exists := b.byKey[treasureKey]
	if !exists {
		return
	}
	removeFromSlotLocked(b, oldKey, treasureKey)
	delete(b.byKey, treasureKey)
}

func removeFromSlotLocked(b *bucket, valueKey valuecanon.Key, treasureKey string) {
	slot, ok := b.byValue[valueKey]
	if !ok {
		return
	}
	delete(slot, treasureKey)
	if len(slot) == 0 {
		delete(b.byValue, valueKey)
	}
}

// collectMatchingLocked returns every Treasure whose canonical key equals
// want. It scans byValue slots to honor cross-kind equality (e.g. int64(5)
// matches uint64(5) and float64(5.0)). Same-kind hits go through a direct
// map lookup as a fast path.
func collectMatchingLocked(b *bucket, want valuecanon.Key) []treasure.Treasure {
	out := make([]treasure.Treasure, 0)
	if slot, ok := b.byValue[want]; ok {
		for _, t := range slot {
			out = append(out, t)
		}
	}
	for k, slot := range b.byValue {
		if k == want {
			continue // already handled by fast path
		}
		if !valuecanon.Equal(k, want) {
			continue
		}
		for _, t := range slot {
			out = append(out, t)
		}
	}
	return out
}

func countMatchingLocked(b *bucket, want valuecanon.Key) int {
	n := 0
	if slot, ok := b.byValue[want]; ok {
		n += len(slot)
	}
	for k, slot := range b.byValue {
		if k == want {
			continue
		}
		if valuecanon.Equal(k, want) {
			n += len(slot)
		}
	}
	return n
}

// extractKey decodes the treasure body to a map and returns the canonical
// key for the configured field path. Returns (NullKey, true) when the body
// is decodable but the field is absent or nil. Returns (NullKey, false)
// only when the body itself is non-decodable; the caller can treat that
// as null too.
func extractKey(t treasure.Treasure, fieldPath string) (valuecanon.Key, bool) {
	body, err := t.GetContentByteArray()
	if err != nil || len(body) == 0 {
		return valuecanon.NullKey, true
	}
	var m map[string]any
	if err := msgpack.Unmarshal(body, &m); err != nil {
		return valuecanon.NullKey, false
	}
	v := extractFieldByPath(m, fieldPath)
	return valuecanon.Canonicalize(v), true
}

// extractFieldByPath navigates a dotted path. Wildcard / pseudo-field
// syntax is intentionally not supported here: a bucket only indexes
// simple body fields per the design. Filters that use [*] or #len are
// not bucket-eligible and route through full-scan.
func extractFieldByPath(m map[string]any, path string) any {
	if path == "" {
		return m
	}
	var current any = m
	start := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '.' {
			continue
		}
		part := path[start:i]
		start = i + 1
		cm, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = cm[part]
		if !ok {
			return nil
		}
	}
	return current
}
