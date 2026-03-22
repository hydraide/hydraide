package models

import (
	"log/slog"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- Models ---

// DizzletPayload represents a domain catalog entry with MsgPack-encoded BytesVal.
type DizzletPayload struct {
	Domain  string `hydraide:"key"`
	Payload []byte `hydraide:"value"` // MsgPack-encoded DizzletData
}

// DizzletData is the decoded structure stored inside Payload.
// type DizzletData struct {
//     LLMSiteFunctions      []int8    // e.g. [1, 7, 2] — e-commerce, booking, services
//     LLMIndustrySectors    []int8    // e.g. [1, 6] — hospitality, health_wellness
//     PaymentProviders      []string  // e.g. ["Barion", "PayPal", "Stripe"]
//     LLMDetailedActivities []string  // e.g. ["custom tattoo design", "piercing"]
//     LLMGenericActivities  []string  // e.g. ["tattoo", "piercing", "body-art"]
// }

// --- SliceContains Examples ---

// SearchBookingSites finds domains where LLMSiteFunctions contains 7 (Booking).
func (d *DizzletPayload) SearchBookingSites(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 50,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		slog.Error("SearchBookingSites", "error", err)
		return nil, err
	}
	return results, nil
}

// SearchByIndustry finds domains in Hospitality (1) OR Health (6) sectors using OR logic.
func (d *DizzletPayload) SearchByIndustry(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterOR(
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(1)),
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMIndustrySectors", int8(6)),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// SearchByPaymentProvider finds domains that accept Barion payments.
func (d *DizzletPayload) SearchByPaymentProvider(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsString("PaymentProviders", "Barion"),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- SliceNotContains Examples ---

// ExcludeEcommerce finds domains that do NOT have e-commerce (1) in SiteFunctions.
func (d *DizzletPayload) ExcludeEcommerce(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceNotContainsInt8("LLMSiteFunctions", int8(1)),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Substring Examples ---

// SearchByActivity finds domains where any detailed activity contains "tattoo" (case-insensitive).
func (d *DizzletPayload) SearchByActivity(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsSubstring("LLMDetailedActivities", "tattoo"),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// SearchTattooExcludePermanent finds tattoo parlors but excludes permanent makeup studios.
func (d *DizzletPayload) SearchTattooExcludePermanent(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterBytesFieldSliceContainsSubstring("LLMDetailedActivities", "tattoo"),
		hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "permanent makeup"),
		hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "cosmetic tattoo"),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Compound Examples ---

// ComplexSearch demonstrates combining multiple filter types in one query:
// Booking sites in Budapest area with Barion payment and at least 1 contact.
func (d *DizzletPayload) ComplexSearch(r repo.Repo) ([]*DizzletPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	swamp := name.New().Sanctuary("catalog").Realm("dizzlets").Swamp("com")
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		MaxResults: 10,
	}

	filters := hydraidego.FilterAND(
		// Booking site
		hydraidego.FilterBytesFieldSliceContainsInt8("LLMSiteFunctions", int8(7)),
		// Accepts Barion
		hydraidego.FilterBytesFieldSliceContainsString("PaymentProviders", "Barion"),
		// No permanent makeup
		hydraidego.FilterBytesFieldSliceNotContainsSubstring("LLMDetailedActivities", "permanent makeup"),
		// Has at least 1 contact
		hydraidego.FilterBytesFieldSliceLen(hydraidego.GreaterThan, "LLMContacts", 0),
		// Within 50km of Budapest
		hydraidego.GeoDistance("Lat", "Lng", 47.497, 19.040, 50.0, hydraidego.GeoInside),
	)

	var results []*DizzletPayload
	err := h.CatalogReadManyStream(ctx, swamp, index, filters, DizzletPayload{}, func(model any) error {
		results = append(results, model.(*DizzletPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
