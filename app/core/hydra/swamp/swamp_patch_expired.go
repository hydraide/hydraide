package swamp

import (
	"sync/atomic"
	"time"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure"
	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
)

// PatchExpired atomically selects up to howMany expired treasures from
// the swamp and applies the same op-set + meta to each one in place.
// See the swampInterface.PatchExpired godoc for the full contract.
//
// Implementation outline:
//  1. Build the expirationTimeBeaconASC index (idempotent if already
//     built — same warm-up as CloneAndDeleteExpiredTreasures).
//  2. SelectExpiredForPatch removes up to howMany expired treasures
//     from the ordered slice under beacon mu. Concurrent callers see
//     disjoint subsets after this step.
//  3. For each selected treasure: take the per-key guard, run
//     ApplyWithCondition, persist via Save(). The Save path's
//     IsExpirationTimeChanged branch handles re-indexing for treasures
//     whose ExpiredAt actually moved.
//  4. ReindexExpiration is called once at the end with the full
//     selected set. Idempotent re-insertion handles two cases that
//     Save() does not cover:
//     - Failed patches (CONDITION_NOT_MET / TYPE_MISMATCH / etc.) keep
//     their original ExpiredAt and need to be put back so the next
//     caller can retry them.
//     - Successful patches whose ExpiredAt was unchanged still need
//     to be visible in the expiration index (e.g. ops-only patch
//     where Meta did not touch ExpiredAt).
func (s *swamp) PatchExpired(howMany int32, ops []msgpackpatch.Op, condition *msgpackpatch.Condition, meta *PatchFieldsMeta) ([]PatchExpiredEntry, error) {

	if howMany == 0 || (len(ops) == 0 && meta == nil) {
		// Either nothing to select or nothing to apply. Treat as no-op.
		// (The gateway is expected to validate inputs more strictly,
		// but this guard keeps the engine layer safe to call directly.)
		return nil, nil
	}

	atomic.StoreInt64(&s.lastInteractionTime, time.Now().UnixNano())

	// Same warm-up as CloneAndDeleteExpiredTreasures — ensure the
	// expiration-time indexes are built before we try to select.
	s.buildBeacon(s.expirationTimeBeaconASC, s.expirationTimeBeaconDESC, BeaconTypeExpirationTime)

	selected := s.expirationTimeBeaconASC.SelectExpiredForPatch(int(howMany))
	if len(selected) == 0 {
		return nil, nil
	}

	// Mirror the same removal on the DESC beacon to keep both indexes
	// consistent. Use Delete-by-key (idempotent if not present).
	for _, t := range selected {
		s.deleteTreasureIfBeaconInitialized(s.expirationTimeBeaconDESC, t.GetKey())
	}

	results := make([]PatchExpiredEntry, 0, len(selected))

	for _, treasureObj := range selected {
		entry := s.applyPatchExpiredOne(treasureObj, ops, condition, meta)
		results = append(results, entry)
	}

	// Re-insert all selected treasures into the expiration index.
	// Idempotent: ReindexExpiration drops existing entries by key
	// before re-adding, so it is safe to call regardless of whether
	// SaveFunction's IsExpirationTimeChanged branch already re-added
	// any of them.
	s.expirationTimeBeaconASC.ReindexExpiration(selected)
	// Re-add to DESC by appending each + re-sort. addToExpirationTimeBeacon
	// handles both ASC and DESC, but we already did ASC via ReindexExpiration
	// so reuse the DESC half by manual Add + sort. Simpler: invoke
	// addToExpirationTimeBeacon for each treasure; it Add()s into both
	// (Add is no-op when the key is already present in ASC, idempotent for
	// DESC).
	for _, t := range selected {
		if t.GetExpirationTime() == 0 {
			continue
		}
		if s.expirationTimeBeaconDESC.IsInitialized() {
			s.expirationTimeBeaconDESC.Add(t)
		}
	}
	if s.expirationTimeBeaconDESC.IsInitialized() {
		// Sort DESC once at the end.
		_ = s.expirationTimeBeaconDESC.SortByExpirationTimeDesc()
	}

	return results, nil
}

// applyPatchExpiredOne runs one per-treasure patch under the treasure's
// guard and returns the outcome. It mirrors the per-key flow inside
// PatchFields, minus the create-if-not-exist branch (PatchExpired only
// touches existing treasures).
func (s *swamp) applyPatchExpiredOne(treasureObj treasure.Treasure, ops []msgpackpatch.Op, condition *msgpackpatch.Condition, meta *PatchFieldsMeta) PatchExpiredEntry {

	guardID := treasureObj.StartTreasureGuard(true)
	defer treasureObj.ReleaseTreasureGuard(guardID)

	entry := PatchExpiredEntry{Key: treasureObj.GetKey()}

	switch treasureObj.GetContentType() {
	case treasure.ContentTypeByteArray:
		// proceed below
	case treasure.ContentTypeVoid:
		// Race: the treasure was deleted between selection and guard
		// acquisition. Surface as KEY_NOT_FOUND for the caller; the
		// final ExpiredAt is whatever the treasure currently holds.
		entry.Status = PatchStatusKeyNotFound
		entry.ExpiredAt = expirationTimeAsTime(treasureObj.GetExpirationTime())
		return entry
	default:
		entry.Status = PatchStatusTypeMismatch
		entry.Error = "treasure is not a ByteArray"
		entry.ExpiredAt = expirationTimeAsTime(treasureObj.GetExpirationTime())
		return entry
	}

	raw, err := treasureObj.GetContentByteArray()
	if err != nil {
		entry.Status = PatchStatusInternalError
		entry.Error = err.Error()
		entry.ExpiredAt = expirationTimeAsTime(treasureObj.GetExpirationTime())
		return entry
	}
	if len(raw) < 2 || raw[0] != patchMsgpackMagic0 || raw[1] != patchMsgpackMagic1 {
		entry.Status = PatchStatusEncodingNotSupported
		entry.Error = "treasure ByteArray is not msgpack-encoded (missing magic prefix)"
		entry.ExpiredAt = expirationTimeAsTime(treasureObj.GetExpirationTime())
		return entry
	}
	inputBody := raw[2:]

	// When meta-only (empty ops), ApplyWithCondition with no ops is a
	// no-op pass-through; condition still gates whether we apply meta.
	out, applyErr := msgpackpatch.ApplyWithCondition(inputBody, ops, condition)
	if applyErr != nil {
		entry.Status = classifyPatchError(applyErr)
		entry.Error = applyErr.Error()
		entry.ExpiredAt = expirationTimeAsTime(treasureObj.GetExpirationTime())
		return entry
	}

	// On meta-only patches the body is unchanged, but we still call
	// SetContentByteArray to take the IsContentChanged path so Save
	// flushes the metadata change. msgpackpatch.Apply with zero ops
	// still re-encodes the body deterministically; that is fine.
	treasureObj.SetContentByteArray(guardID, wrapMsgpackBody(out))
	applyPatchMeta(treasureObj, guardID, meta, false)
	treasureObj.Save(guardID)

	entry.Status = PatchStatusPatched
	entry.NewMsgpack = out
	entry.ExpiredAt = expirationTimeAsTime(treasureObj.GetExpirationTime())
	return entry
}

// expirationTimeAsTime converts a UnixNano-style expiration time int64 to
// a time.Time. Returns a zero time when expirationTime == 0.
func expirationTimeAsTime(expirationTime int64) time.Time {
	if expirationTime == 0 {
		return time.Time{}
	}
	return time.Unix(0, expirationTime).UTC()
}
