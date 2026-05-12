package hydraidego

import (
	"context"
	"fmt"

	hydraidepbgo "github.com/hydraide/hydraide/sdk/go/hydraidego/v3/hydraidepbgo"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/v3/name"
)

// WithCap attaches a server-enforced quota check to this builder. When
// Cap is set the server counts records matching cap.Filter under the
// same swamp guard as the patch, simulates the per-key post-op state,
// and skips per-key mutations that would push the matching count above
// cap.MaxMatching. See the Cap proto docs for the (pre, post) four-cell
// rule.
//
// Cap on explicit-key Patch surfaces is restricted to BytesField filters
// (i.e. filters operating on a path inside the msgpack body). Metadata
// filters (CreatedAt, UpdatedAt, ExpiredAt, value-typed filters) are
// rejected with ErrCodeInvalidModel because the engine has no way to
// simulate post-mutation metadata for arbitrary patch op-sets.
//
// cap == nil clears any previously set Cap.
func (b *PatchBuilder) WithCap(cap *Cap) *PatchBuilder {
	if b.encodeError != nil {
		return b
	}
	b.cap = cap
	return b
}

// ExecWithResult is the Cap-aware variant of Exec: in addition to the
// per-key status, it returns the request-level *PatchResult containing
// CapReached. Use ExecWithResult when the builder carries WithCap and
// the caller needs to distinguish "cap budget exhausted, back off" from
// other per-key outcomes.
//
// Identical semantics to Exec otherwise. Without WithCap the returned
// PatchResult.CapReached is always false.
func (b *PatchBuilder) ExecWithResult() (*PatchResult, error) {
	if b.encodeError != nil {
		return nil, NewError(ErrCodeInvalidModel, b.encodeError.Error())
	}
	if b.h == nil {
		return nil, NewError(ErrCodeInvalidModel, "PatchBuilder is not bound to a client; use CatalogPatch(ctx, swamp, key) for single-key dispatch")
	}
	st, capReached, err := b.h.runPatchWithCap(b.ctx, b.swampName, b.key, b.ops, b.cond, b.create, b.meta, b.cap)
	if err != nil {
		return nil, err
	}
	return &PatchResult{Status: st, CapReached: capReached}, nil
}

// PatchResult is the request-level outcome of a Cap-aware single-key
// patch. CapReached is true when the request carried a Cap and the
// per-key check rejected the mutation with PatchStatusCapExceeded.
type PatchResult struct {
	Status     PatchStatus
	CapReached bool
}

// runPatchWithCap is the Cap-aware variant of runPatch. It accepts an
// optional *Cap and forwards it on the wire; the response's CapReached
// signal is returned alongside the per-key status.
func (h *hydraidego) runPatchWithCap(
	ctx context.Context,
	swampName name.Name,
	key string,
	ops []*hydraidepbgo.PatchOp,
	cond *hydraidepbgo.PatchCondition,
	createIfNotExist bool,
	meta *hydraidepbgo.PatchMeta,
	cap *Cap,
) (PatchStatus, bool, error) {
	if len(ops) == 0 && meta == nil {
		return PatchStatusInternalError, false, NewError(ErrCodeInvalidModel, "ops list is empty and meta is nil")
	}
	wireCap, capErr := buildWireCap(cap)
	if capErr != nil {
		return PatchStatusInternalError, false, capErr
	}
	resp, err := h.client.GetServiceClient(swampName).PatchTreasures(ctx, &hydraidepbgo.PatchTreasuresRequest{
		IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:        swampName.Get(),
		CreateIfNotExist: createIfNotExist,
		Meta:             meta,
		Cap:              wireCap,
		Patches: []*hydraidepbgo.TreasurePatch{
			{Key: key, Ops: ops, Condition: cond},
		},
	})
	if err != nil {
		return PatchStatusInternalError, false, translatePatchGRPCError(err)
	}
	if len(resp.GetResults()) == 0 {
		return PatchStatusInternalError, false, NewError(ErrCodeUnknown, "empty PatchTreasures response")
	}
	r := resp.GetResults()[0]
	st := PatchStatus(r.GetStatus())
	if st == PatchStatusInternalError && r.GetError() != "" {
		return st, resp.GetCapReached(), NewError(ErrCodeInternalDatabaseError, r.GetError())
	}
	return st, resp.GetCapReached(), nil
}

// PatchFieldsManyResult is the request-level outcome of a Cap-aware
// PatchFieldsMany call. CapReached is true when at least one per-key
// patch in the batch was rejected with PatchStatusCapExceeded.
type PatchFieldsManyResult struct {
	CapReached bool
}

// CatalogPatchFieldsManyWithCap is the Cap-aware variant of
// CatalogPatchFieldsMany. Cap applies to the whole batch (single swamp),
// not per-builder; per-builder WithCap on a batch member is ignored.
//
// The iterator fires per-key with the per-key PatchStatus —
// PatchStatusCapExceeded signals the per-key check rejected the
// mutation. The returned *PatchFieldsManyResult carries the
// request-level CapReached signal.
func (h *hydraidego) CatalogPatchFieldsManyWithCap(
	ctx context.Context,
	swampName name.Name,
	requests []*PatchManyRequest,
	cap *Cap,
	iterator PatchManyIteratorFunc,
) (*PatchFieldsManyResult, error) {
	if len(requests) == 0 {
		return &PatchFieldsManyResult{}, nil
	}
	wireCap, capErr := buildWireCap(cap)
	if capErr != nil {
		return nil, capErr
	}
	patches := make([]*hydraidepbgo.TreasurePatch, 0, len(requests))
	var createIfNotExist bool
	for i, req := range requests {
		if req == nil || req.Builder == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: nil request or nil Builder", i))
		}
		b := req.Builder
		if b.encodeError != nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%q): %v", i, b.key, b.encodeError))
		}
		if b.key == "" {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d: builder has empty key", i))
		}
		if len(b.ops) == 0 && b.meta == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%q): builder has no ops and no meta", i, b.key))
		}
		if i == 0 {
			createIfNotExist = b.create
		} else if b.create != createIfNotExist {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%q): NoCreate flag differs from request 0; split the batch", i, b.key))
		}
		patches = append(patches, &hydraidepbgo.TreasurePatch{
			Key:       b.key,
			Ops:       b.ops,
			Condition: b.cond,
			Meta:      b.meta,
		})
	}

	resp, err := h.client.GetServiceClient(swampName).PatchTreasures(ctx, &hydraidepbgo.PatchTreasuresRequest{
		IslandID:         swampName.GetIslandID(h.client.GetAllIslands()),
		SwampName:        swampName.Get(),
		Patches:          patches,
		CreateIfNotExist: createIfNotExist,
		Cap:              wireCap,
	})
	if err != nil {
		return nil, translatePatchGRPCError(err)
	}

	if iterator != nil {
		for _, r := range resp.GetResults() {
			if err := iterator(r.GetKey(), PatchStatus(r.GetStatus()), r.GetError()); err != nil {
				return &PatchFieldsManyResult{CapReached: resp.GetCapReached()}, err
			}
		}
	}
	return &PatchFieldsManyResult{CapReached: resp.GetCapReached()}, nil
}

// PatchManyToManyResult is one entry in the per-swamp result slice
// returned by CatalogPatchManyToManyWithResults. CapReached mirrors the
// per-swamp Cap signal; SwampErr is non-nil for swamp-level failures.
type PatchManyToManyResult struct {
	SwampName  name.Name
	CapReached bool
	SwampErr   error
}

// CatalogPatchManyToManyWithResults is the Cap-aware variant of
// CatalogPatchManyToMany. It returns one PatchManyToManyResult per
// input request (in input order) alongside the per-key iterator stream.
// Per-swamp Cap is sourced from CatalogPatchManyToManyRequest.Cap.
func (h *hydraidego) CatalogPatchManyToManyWithResults(
	ctx context.Context,
	requests []*CatalogPatchManyToManyRequest,
	iterator CatalogPatchManyToManyIteratorFunc,
) ([]*PatchManyToManyResult, error) {
	if len(requests) == 0 {
		return nil, nil
	}

	results := make([]*PatchManyToManyResult, len(requests))
	wireRequests := make([]*hydraidepbgo.PatchTreasuresRequest, len(requests))
	for i, req := range requests {
		results[i] = &PatchManyToManyResult{}
		if req == nil {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d is nil", i))
		}
		results[i].SwampName = req.SwampName
		if len(req.Patches) == 0 {
			return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d (%s): Patches is empty", i, req.SwampName.Get()))
		}
		wireCap, capErr := buildWireCap(req.Cap)
		if capErr != nil {
			return nil, capErr
		}
		patches := make([]*hydraidepbgo.TreasurePatch, 0, len(req.Patches))
		var createIfNotExist bool
		for j, p := range req.Patches {
			if p == nil || p.Builder == nil {
				return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d patch %d: nil patch or Builder", i, j))
			}
			b := p.Builder
			if b.encodeError != nil {
				return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d patch %d (%q): %v", i, j, b.key, b.encodeError))
			}
			if b.key == "" {
				return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d patch %d: empty key", i, j))
			}
			if len(b.ops) == 0 && b.meta == nil {
				return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d patch %d (%q): no ops and no meta", i, j, b.key))
			}
			if j == 0 {
				createIfNotExist = b.create
			} else if b.create != createIfNotExist {
				return nil, NewError(ErrCodeInvalidModel, fmt.Sprintf("request %d patch %d (%q): NoCreate flag differs from patch 0; split the batch", i, j, b.key))
			}
			patches = append(patches, &hydraidepbgo.TreasurePatch{
				Key:       b.key,
				Ops:       b.ops,
				Condition: b.cond,
				Meta:      b.meta,
			})
		}
		wireRequests[i] = &hydraidepbgo.PatchTreasuresRequest{
			IslandID:         req.SwampName.GetIslandID(h.client.GetAllIslands()),
			SwampName:        req.SwampName.Get(),
			Patches:          patches,
			CreateIfNotExist: createIfNotExist,
			Cap:              wireCap,
		}
	}

	// Group by destination server (consistent hashing on SwampName) and
	// send one PatchTreasuresMany RPC per server.
	type serverGroup struct {
		client  hydraidepbgo.HydraideServiceClient
		indices []int
	}
	groups := make(map[string]*serverGroup)
	for i, req := range requests {
		clientAndHost := h.client.GetServiceClientAndHost(req.SwampName)
		g, ok := groups[clientAndHost.Host]
		if !ok {
			g = &serverGroup{client: clientAndHost.GrpcClient}
			groups[clientAndHost.Host] = g
		}
		g.indices = append(g.indices, i)
	}

	for _, g := range groups {
		batch := make([]*hydraidepbgo.PatchTreasuresRequest, 0, len(g.indices))
		for _, idx := range g.indices {
			batch = append(batch, wireRequests[idx])
		}
		resp, err := g.client.PatchTreasuresMany(ctx, &hydraidepbgo.PatchTreasuresManyRequest{
			Requests: batch,
		})
		if err != nil {
			return results, translatePatchGRPCError(err)
		}
		entries := resp.GetResponses()
		if len(entries) != len(g.indices) {
			return results, NewError(ErrCodeInternalDatabaseError, fmt.Sprintf("PatchTreasuresMany: server returned %d entries for %d requests", len(entries), len(g.indices)))
		}
		for k, idx := range g.indices {
			entry := entries[k]
			results[idx].CapReached = entry.GetCapReached()
			if errMsg := entry.GetError(); errMsg != "" {
				results[idx].SwampErr = NewError(ErrCodeInternalDatabaseError, errMsg)
				if iterator != nil {
					if iterErr := iterator(requests[idx].SwampName, "", PatchStatusInternalError, errMsg, results[idx].SwampErr); iterErr != nil {
						return results, iterErr
					}
				}
				continue
			}
			if iterator == nil {
				continue
			}
			for _, r := range entry.GetResults() {
				if iterErr := iterator(requests[idx].SwampName, r.GetKey(), PatchStatus(r.GetStatus()), r.GetError(), nil); iterErr != nil {
					return results, iterErr
				}
			}
		}
	}
	return results, nil
}
