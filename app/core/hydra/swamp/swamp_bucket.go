package swamp

import (
	"github.com/hydraide/hydraide/app/core/hydra/swamp/bucket"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
)

// GetOrBuildBucket returns the bucket for fieldPath, building its
// equality view inline if this is the first call. Build cost is
// proportional to the swamp size (one body pass); subsequent calls
// are O(1). Concurrent first-callers cooperate via sync.Once inside
// the bucket so only one snapshot is taken.
//
// Lock contract:
//
//  1. Fast path: bucketsMu.RLock to lookup, RUnlock, return if already
//     initialised.
//  2. Slow path: bucketsMu.Lock, register the bucket with
//     buildInFlight=1, Unlock. From this moment on, every Save sees
//     the bucket and routes its mutation through tryEnqueue, which
//     buffers until DrainPending atomically flips the flag.
//  3. Snapshot beaconKey AFTER releasing bucketsMu so the build does
//     not block writers. The snapshot-vs-pending correctness proof in
//     the design (section 5.6) requires that every Save which already
//     ran beaconKey.Add by snapshot time is either in the snapshot or
//     in pending; the SaveFunction ordering (beaconKey.Add then
//     notifyBucketsInsert) preserves this.
//  4. BuildEquality (sync.Once) + DrainPending. The drain loop ends
//     by clearing buildInFlight=1 atomically under pendingMu.
func (s *swamp) GetOrBuildBucket(fieldPath string) bucket.Bucket {
	s.bucketsMu.RLock()
	if s.buckets != nil {
		if b, ok := s.buckets[fieldPath]; ok && b.EqualityInitialized() {
			s.bucketsMu.RUnlock()
			return b
		}
	}
	s.bucketsMu.RUnlock()

	s.bucketsMu.Lock()
	if s.buckets == nil {
		s.buckets = make(map[string]bucket.Bucket)
	}
	b, exists := s.buckets[fieldPath]
	if !exists {
		b = bucket.New(fieldPath)
		b.SetBuildInFlight(true)
		s.buckets[fieldPath] = b
	}
	s.bucketsMu.Unlock()

	if !b.EqualityInitialized() {
		snapshot := s.beaconKey.CloneUnorderedTreasures(false)
		_ = b.BuildEquality(snapshot)
		_ = b.DrainPending()
	}
	return b
}

// LookupByBucketEqual wraps GetOrBuildBucket + LookupEqual.
func (s *swamp) LookupByBucketEqual(fieldPath string, value any) []treasure.Treasure {
	return s.GetOrBuildBucket(fieldPath).LookupEqual(value)
}

// LookupByBucketIn wraps GetOrBuildBucket + LookupIn.
func (s *swamp) LookupByBucketIn(fieldPath string, values []any) []treasure.Treasure {
	if len(values) == 0 {
		return nil
	}
	return s.GetOrBuildBucket(fieldPath).LookupIn(values)
}

// BucketCount returns the number of buckets currently held by the
// swamp. Useful for telemetry and soft-cap warnings; not part of the
// per-call hot path.
func (s *swamp) BucketCount() int {
	s.bucketsMu.RLock()
	defer s.bucketsMu.RUnlock()
	return len(s.buckets)
}

// dropAllBuckets releases every bucket. Called from Close / Destroy
// to drop derived state — buckets are not persisted.
func (s *swamp) dropAllBuckets() {
	s.bucketsMu.Lock()
	s.buckets = nil
	s.bucketsMu.Unlock()
}

// notifyBucketsInsert forwards a new treasure to every initialised
// bucket. Must run AFTER beaconKey.Add(t) so that the snapshot-vs-
// pending invariant holds for a build that registers concurrently.
func (s *swamp) notifyBucketsInsert(t treasure.Treasure) {
	s.bucketsMu.RLock()
	if len(s.buckets) == 0 {
		s.bucketsMu.RUnlock()
		return
	}
	bs := make([]bucket.Bucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		bs = append(bs, b)
	}
	s.bucketsMu.RUnlock()
	for _, b := range bs {
		_ = b.OnInsert(t)
	}
}

// notifyBucketsUpdate forwards a modified treasure to every bucket.
func (s *swamp) notifyBucketsUpdate(t treasure.Treasure) {
	s.bucketsMu.RLock()
	if len(s.buckets) == 0 {
		s.bucketsMu.RUnlock()
		return
	}
	bs := make([]bucket.Bucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		bs = append(bs, b)
	}
	s.bucketsMu.RUnlock()
	for _, b := range bs {
		_ = b.OnUpdate(t)
	}
}

// notifyBucketsDelete forwards a delete to every bucket.
func (s *swamp) notifyBucketsDelete(key string) {
	s.bucketsMu.RLock()
	if len(s.buckets) == 0 {
		s.bucketsMu.RUnlock()
		return
	}
	bs := make([]bucket.Bucket, 0, len(s.buckets))
	for _, b := range s.buckets {
		bs = append(bs, b)
	}
	s.bucketsMu.RUnlock()
	for _, b := range bs {
		b.OnDelete(key)
	}
}
