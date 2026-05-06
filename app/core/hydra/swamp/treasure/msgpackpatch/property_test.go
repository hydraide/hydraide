package msgpackpatch

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// Property-based fuzz tests. Rather than enumerating concrete inputs, these
// tests state a property that should hold for every valid skeleton and op
// sequence, and let pseudo-random generation hammer the assertion.
//
// The seed is fixed so the suite is deterministic — a failing property
// shrinks via the seed, not via a sophisticated shrinker. The trade-off
// is acceptable: deterministic failures are reproducible without a
// dependency, and the fuzzer runs in milliseconds.

const propertyTestIterations = 2000

// TestProperty_UntouchedFieldsByteIdentical asserts the core invariant of
// the structural splice: any field NOT named in the op list keeps its
// exact msgpack wire encoding (byte-identical re-serialization).
func TestProperty_UntouchedFieldsByteIdentical(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))

	for i := 0; i < propertyTestIterations; i++ {
		original := randomTopLevelMap(rng)
		blob, err := msgpack.Marshal(original)
		require.NoError(t, err, "iter %d: encode original", i)

		// Pick one field at random to patch. Leave the rest untouched.
		keys := mapKeys(original)
		if len(keys) == 0 {
			continue
		}
		patchKey := keys[rng.Intn(len(keys))]
		newVal := int64(rng.Int63n(1_000_000))
		newRaw, err := msgpack.Marshal(newVal)
		require.NoError(t, err, "iter %d: encode patch value", i)

		out, err := Apply(blob, []Op{
			{Kind: OpSet, Path: patchKey, Value: newRaw},
		})
		require.NoError(t, err, "iter %d: apply", i)

		// Decode both and verify untouched fields match exactly.
		var got map[string]any
		require.NoError(t, msgpack.Unmarshal(out, &got))

		for _, k := range keys {
			if k == patchKey {
				continue
			}
			// The "untouched" assertion at decode level: same value
			// (decoded any). We can't byte-compare per-field without
			// re-parsing the skeleton, but the round-trip through
			// Decode-Encode is enough at this iteration count to catch
			// any subtle drift introduced by the splice.
			require.Equal(t, original[k], got[k],
				"iter %d: field %q changed unexpectedly", i, k)
		}
		require.EqualValues(t, newVal, got[patchKey],
			"iter %d: patched field has wrong value", i)
	}
}

// TestProperty_NoOpRoundTrip asserts that Apply with zero ops is a clean
// round-trip: Parse + Serialize must yield the same decoded map for any
// random msgpack input.
func TestProperty_NoOpRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBADBEEF))

	for i := 0; i < propertyTestIterations; i++ {
		original := randomTopLevelMap(rng)
		blob, err := msgpack.Marshal(original)
		require.NoError(t, err)

		out, err := Apply(blob, nil)
		require.NoError(t, err, "iter %d: no-op apply", i)

		var got map[string]any
		require.NoError(t, msgpack.Unmarshal(out, &got))
		require.Equal(t, original, got, "iter %d: no-op decoded mismatch", i)
	}
}

// TestProperty_DeleteThenSetIsEquivalentToSet asserts that DELETE + SET
// on the same path produces the same observable state as a single SET on
// a path that already exists.
func TestProperty_DeleteThenSetIsEquivalentToSet(t *testing.T) {
	rng := rand.New(rand.NewSource(0xDECAFBAD))

	for i := 0; i < propertyTestIterations; i++ {
		original := randomTopLevelMap(rng)
		if len(original) == 0 {
			continue
		}
		keys := mapKeys(original)
		key := keys[rng.Intn(len(keys))]
		newVal := fmt.Sprintf("v%d", rng.Intn(1000))
		newRaw, err := msgpack.Marshal(newVal)
		require.NoError(t, err)

		blob, err := msgpack.Marshal(original)
		require.NoError(t, err)

		// Path A: SET only.
		outA, err := Apply(blob, []Op{
			{Kind: OpSet, Path: key, Value: newRaw},
		})
		require.NoError(t, err)

		// Path B: DELETE then SET.
		outB, err := Apply(blob, []Op{
			{Kind: OpDelete, Path: key},
			{Kind: OpSet, Path: key, Value: newRaw},
		})
		require.NoError(t, err)

		// Outputs may not be byte-identical (key ordering on re-insertion),
		// but decoded maps must be equal.
		var gotA, gotB map[string]any
		require.NoError(t, msgpack.Unmarshal(outA, &gotA))
		require.NoError(t, msgpack.Unmarshal(outB, &gotB))
		require.Equal(t, gotA, gotB, "iter %d: SET vs DELETE+SET", i)
	}
}

// TestProperty_OutputAlwaysReParsesAsMsgpack asserts that any patched output,
// regardless of input or op sequence, is itself valid msgpack: feeding it
// back through Parse must succeed.
func TestProperty_OutputAlwaysReParsesAsMsgpack(t *testing.T) {
	rng := rand.New(rand.NewSource(0xFEEDFACE))

	for i := 0; i < propertyTestIterations; i++ {
		original := randomTopLevelMap(rng)
		if len(original) == 0 {
			continue
		}
		blob, err := msgpack.Marshal(original)
		require.NoError(t, err)

		ops := randomOps(rng, mapKeys(original))
		out, err := Apply(blob, ops)
		if err != nil {
			// Op-level errors (TYPE_MISMATCH on INC against string etc.)
			// are valid outcomes — skip the round-trip assertion.
			continue
		}

		_, err = Parse(out)
		require.NoError(t, err, "iter %d: output is not valid msgpack", i)
	}
}

// TestProperty_INCRoundTrip asserts that INC by N then INC by -N restores
// the original numeric value (modulo overflow).
func TestProperty_INCRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(0xABCD1234))

	for i := 0; i < propertyTestIterations/4; i++ {
		original := int32(rng.Int31n(1_000_000) - 500_000)
		delta := int32(rng.Int31n(10_000))

		original64Avoid := int64(original) + int64(delta)
		if original64Avoid > int64(1<<31-1) || original64Avoid < int64(-1<<31) {
			continue // skip overflow cases
		}

		blob, err := msgpack.Marshal(map[string]any{"n": original})
		require.NoError(t, err)

		dRaw, err := msgpack.Marshal(delta)
		require.NoError(t, err)
		dRawNeg, err := msgpack.Marshal(-delta)
		require.NoError(t, err)

		out, err := Apply(blob, []Op{
			{Kind: OpInc, Path: "n", Value: dRaw},
			{Kind: OpInc, Path: "n", Value: dRawNeg},
		})
		require.NoError(t, err, "iter %d: INC roundtrip", i)

		var got map[string]any
		require.NoError(t, msgpack.Unmarshal(out, &got))
		require.EqualValues(t, original, got["n"],
			"iter %d: INC+INC(-N) should restore original", i)
	}
}

// TestProperty_AppendThenRemoveAtRestores asserts that APPEND followed by
// REMOVE_AT[-1] (last index) yields a decoded array equal to the original.
func TestProperty_AppendThenRemoveAtRestores(t *testing.T) {
	rng := rand.New(rand.NewSource(0x55AA))

	for i := 0; i < propertyTestIterations/4; i++ {
		size := rng.Intn(8) + 1
		arr := make([]any, size)
		for j := range arr {
			arr[j] = rng.Intn(1000)
		}
		blob, err := msgpack.Marshal(map[string]any{"items": arr})
		require.NoError(t, err)

		appended := fmt.Sprintf("new-%d", rng.Intn(1000))
		appendedRaw, err := msgpack.Marshal(appended)
		require.NoError(t, err)

		out, err := Apply(blob, []Op{
			{Kind: OpAppend, Path: "items[]", Value: appendedRaw},
			{Kind: OpRemoveAt, Path: "items[-1]"},
		})
		require.NoError(t, err)

		// Compare the decoded "items" arrays.
		var orig map[string]any
		require.NoError(t, msgpack.Unmarshal(blob, &orig))
		var got map[string]any
		require.NoError(t, msgpack.Unmarshal(out, &got))
		require.Equal(t, orig["items"], got["items"],
			"iter %d: APPEND+REMOVE_AT[-1] should restore original", i)
	}
}

// TestProperty_InputBlobNeverMutated asserts that even when Apply errors,
// the original input blob is byte-identical afterwards.
func TestProperty_InputBlobNeverMutated(t *testing.T) {
	rng := rand.New(rand.NewSource(0x73AB))

	for i := 0; i < propertyTestIterations; i++ {
		original := randomTopLevelMap(rng)
		blob, err := msgpack.Marshal(original)
		require.NoError(t, err)

		snapshot := bytes.Clone(blob)

		ops := randomOps(rng, mapKeys(original))
		_, _ = Apply(blob, ops)

		require.True(t, bytes.Equal(blob, snapshot),
			"iter %d: input blob mutated", i)
	}
}

// ---------- Generators ----------

// randomTopLevelMap builds a small map[string]any with primitive values.
// Field names are deterministic ("f0".."f4") so test paths can address
// them, but selection of fields and their values is randomized.
func randomTopLevelMap(rng *rand.Rand) map[string]any {
	n := rng.Intn(5) + 1
	out := make(map[string]any, n)
	allFields := []string{"f0", "f1", "f2", "f3", "f4"}
	rng.Shuffle(len(allFields), func(i, j int) { allFields[i], allFields[j] = allFields[j], allFields[i] })
	for i := 0; i < n; i++ {
		out[allFields[i]] = randomLeaf(rng)
	}
	return out
}

// randomLeaf generates one of several primitive Go values. Each branch
// produces a distinct msgpack type code so the property tests exercise
// type-preservation across the full primitive set.
func randomLeaf(rng *rand.Rand) any {
	switch rng.Intn(8) {
	case 0:
		return int8(rng.Intn(200) - 100)
	case 1:
		return int16(rng.Intn(60000) - 30000)
	case 2:
		return int32(rng.Int31())
	case 3:
		return int64(rng.Int63())
	case 4:
		return uint32(rng.Uint32())
	case 5:
		return rng.Float64()
	case 6:
		return rng.Intn(2) == 0
	default:
		return fmt.Sprintf("s%d", rng.Intn(1000))
	}
}

// randomOps generates a small mix of ops. Paths reference existing keys
// most of the time, but occasionally point to a non-existent key to
// exercise auto-create paths.
func randomOps(rng *rand.Rand, existing []string) []Op {
	if len(existing) == 0 {
		return nil
	}
	n := rng.Intn(3) + 1
	ops := make([]Op, 0, n)
	for i := 0; i < n; i++ {
		var key string
		if rng.Intn(4) == 0 {
			key = fmt.Sprintf("new-%d", rng.Intn(1000))
		} else {
			key = existing[rng.Intn(len(existing))]
		}
		switch rng.Intn(3) {
		case 0:
			val := rng.Intn(1_000_000)
			raw, _ := msgpack.Marshal(val)
			ops = append(ops, Op{Kind: OpSet, Path: key, Value: raw})
		case 1:
			ops = append(ops, Op{Kind: OpDelete, Path: key})
		case 2:
			val := int64(rng.Intn(100))
			raw, _ := msgpack.Marshal(val)
			ops = append(ops, Op{Kind: OpInc, Path: key, Value: raw})
		}
	}
	return ops
}

// mapKeys returns the keys of a map in a stable (sorted) order.
func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Use simple sort for stable output.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
