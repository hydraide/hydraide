package swamp

import (
	"fmt"
	"testing"

	"github.com/hydraide/hydraide/app/core/hydra/swamp/treasure/msgpackpatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// bodyMatchesStatus is the canonical body-only Cap predicate the Patch
// gateway builds: decode the msgpack body and check Status == target.
func bodyMatchesStatus(target string) func([]byte) bool {
	return func(raw []byte) bool {
		if len(raw) == 0 {
			return false
		}
		var decoded map[string]interface{}
		if err := msgpack.Unmarshal(raw, &decoded); err != nil {
			return false
		}
		v, ok := decoded["status"].(string)
		return ok && v == target
	}
}

// ---------- PatchFields Cap — pre/post 4-cell matrix ----------

func TestSwamp_PatchFields_Cap_NoToYes_AcceptedWhenBudget(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-noyes-ok")
	seedMsgpack(t, s, "k", map[string]any{"status": "pending"})

	budget := int32(1)
	opts := PatchFieldsOptions{
		CapPredicate:  bodyMatchesStatus("claimed"),
		CapBudgetLeft: &budget,
	}
	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status)
	assert.Equal(t, int32(0), budget, "budget consumed by (no,yes) accept")
}

func TestSwamp_PatchFields_Cap_NoToYes_RejectedWhenNoBudget(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-noyes-reject")
	seedMsgpack(t, s, "k", map[string]any{"status": "pending"})

	budget := int32(0)
	opts := PatchFieldsOptions{
		CapPredicate:  bodyMatchesStatus("claimed"),
		CapBudgetLeft: &budget,
	}
	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusCapExceeded, res.Status)
	// Body unchanged.
	body := readPatchedMap(t, s, "k")
	assert.Equal(t, "pending", body["status"])
}

func TestSwamp_PatchFields_Cap_YesToYes_Idempotent(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-yesyes")
	seedMsgpack(t, s, "k", map[string]any{"status": "claimed"})

	// Budget=0; (yes,yes) idempotent re-patch must still proceed because
	// the matching count does not grow.
	budget := int32(0)
	opts := PatchFieldsOptions{
		CapPredicate:  bodyMatchesStatus("claimed"),
		CapBudgetLeft: &budget,
	}
	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status)
	assert.Equal(t, int32(0), budget, "(yes,yes) must not consume budget")
}

func TestSwamp_PatchFields_Cap_YesToNo_Shrinking(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-yesno")
	seedMsgpack(t, s, "k", map[string]any{"status": "claimed"})

	budget := int32(0)
	opts := PatchFieldsOptions{
		CapPredicate:  bodyMatchesStatus("claimed"),
		CapBudgetLeft: &budget,
	}
	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "done")}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status, "(yes,no) must always proceed")
	body := readPatchedMap(t, s, "k")
	assert.Equal(t, "done", body["status"])
}

func TestSwamp_PatchFields_Cap_NoToNo_Untouched(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-nono")
	seedMsgpack(t, s, "k", map[string]any{"status": "pending", "other": int8(0)})

	budget := int32(0)
	opts := PatchFieldsOptions{
		CapPredicate:  bodyMatchesStatus("claimed"),
		CapBudgetLeft: &budget,
	}
	// SET on a different field — neither pre nor post matches Cap.Filter.
	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "other", Value: encMsgpack(t, int8(1))}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status, "(no,no) must always proceed")
}

// ---------- PatchFields Cap — Create branch ----------

func TestSwamp_PatchFields_Cap_CreateAsNoToYes(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-create-noyes")

	// Treasure does not exist → pre is treated as "no" (no body to read).
	budget := int32(1)
	opts := PatchFieldsOptions{
		CreateIfNotExist: true,
		CapPredicate:     bodyMatchesStatus("claimed"),
		CapBudgetLeft:    &budget,
	}
	res, err := s.PatchFields("new-key",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusCreated, res.Status)
	assert.Equal(t, int32(0), budget, "create→matching is (no,yes) and consumes budget")
}

func TestSwamp_PatchFields_Cap_CreateRejectedWhenNoBudget(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-create-reject")

	budget := int32(0)
	opts := PatchFieldsOptions{
		CreateIfNotExist: true,
		CapPredicate:     bodyMatchesStatus("claimed"),
		CapBudgetLeft:    &budget,
	}
	res, err := s.PatchFields("new-key",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
		nil, opts)
	require.NoError(t, err)
	assert.Equal(t, PatchStatusCapExceeded, res.Status)
}

// ---------- PatchFields Cap — regression twin ----------

func TestSwamp_PatchFields_NoCap_RegressionTwin(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-nocap-twin")
	seedMsgpack(t, s, "k", map[string]any{"status": "pending"})

	// Without Cap, behaviour is byte-identical to pre-Cap PatchFields.
	res, err := s.PatchFields("k",
		[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
		nil, PatchFieldsOptions{})
	require.NoError(t, err)
	assert.Equal(t, PatchStatusPatched, res.Status)
	body := readPatchedMap(t, s, "k")
	assert.Equal(t, "claimed", body["status"])
}

// ---------- PatchFields Cap — batch budget exhaustion ----------

func TestSwamp_PatchFields_Cap_BatchExhausts(t *testing.T) {
	s := patchTestSwamp(t, "patch", "cap-batch")

	// 5 "pending" records.
	for i := 0; i < 5; i++ {
		seedMsgpack(t, s, fmt.Sprintf("k-%d", i), map[string]any{"status": "pending"})
	}

	// Cap=2 → first 2 (no,yes) transitions accepted, next 3 rejected.
	budget := int32(2)
	opts := PatchFieldsOptions{
		CapPredicate:  bodyMatchesStatus("claimed"),
		CapBudgetLeft: &budget,
	}
	accepted := 0
	rejected := 0
	for i := 0; i < 5; i++ {
		res, err := s.PatchFields(fmt.Sprintf("k-%d", i),
			[]msgpackpatch.Op{{Kind: msgpackpatch.OpSet, Path: "status", Value: encMsgpack(t, "claimed")}},
			nil, opts)
		require.NoError(t, err)
		switch res.Status {
		case PatchStatusPatched:
			accepted++
		case PatchStatusCapExceeded:
			rejected++
		default:
			t.Fatalf("unexpected status %v for k-%d", res.Status, i)
		}
	}
	assert.Equal(t, 2, accepted)
	assert.Equal(t, 3, rejected)
	assert.Equal(t, int32(0), budget)
}
