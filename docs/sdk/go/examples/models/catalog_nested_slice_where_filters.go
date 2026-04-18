package models

import (
	"log/slog"
	"time"

	"github.com/hydraide/hydraide/sdk/go/hydraidego"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/name"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/hydraidehelper"
	"github.com/hydraide/hydraide/sdk/go/hydraidego/utils/repo"
)

// --- Data Model ---

// AssistantDomainPayload represents a domain in the assistant's sending queue.
// A domain can participate in multiple campaigns — each participation is a CampaignEntry.
type AssistantDomainPayload struct {
	Key             string                 `hydraide:"key"`
	Payload         *AssistantDomainData   `hydraide:"value"`
	Meta            *hydraidego.SearchMeta `hydraide:"searchMeta"`
}

type AssistantDomainData struct {
	DomainName      string
	CampaignEntries []*DomainCampaignEntry
}

type DomainCampaignEntry struct {
	CampaignID      string
	EmailTemplateID string
	EmailType       int8      // Main(1) / FollowUp(2)
	Status          int8      // Active(1) / Excluded(2) / Finished(3)
	ExclusionReason int8      // 0=none, 1=partner, 2=prospect
	NextSendAt      time.Time
}

// --- FilterNestedSliceWhere Examples ---

// SearchReadyToSend finds domains with at least one CampaignEntry that is:
//   - Active (Status=1)
//   - In one of the given active campaigns
//   - Ready to send (NextSendAt <= now and NextSendAt is set)
//
// This is the worker's main query, executed every 7 minutes per assistant.
func (d *AssistantDomainPayload) SearchReadyToSend(r repo.Repo, assistantSwamp name.Name, activeCampaignIDs []string) ([]*AssistantDomainPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	now := time.Now()

	filters := hydraidego.FilterAND(
		hydraidego.FilterNestedSliceWhere("CampaignEntries",
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),                                  // Active
			hydraidego.FilterBytesFieldStringIn("CampaignID", activeCampaignIDs...),                         // In active campaigns
			hydraidego.FilterBytesFieldTime(hydraidego.LessThanOrEqual, "NextSendAt", now),                   // Ready to send
			hydraidego.FilterBytesFieldTime(hydraidego.GreaterThan, "NextSendAt", time.Time{}),               // NextSendAt is set (not zero)
		),
	)

	var results []*AssistantDomainPayload
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		results = append(results, model.(*AssistantDomainPayload))
		return nil
	})
	if err != nil {
		slog.Error("SearchReadyToSend", "error", err)
		return nil, err
	}
	return results, nil
}

// CountActiveByCampaign counts how many domains have an active entry for a specific campaign.
// Uses KeysOnly mode for maximum performance (no payload deserialization).
func (d *AssistantDomainPayload) CountActiveByCampaign(r repo.Repo, assistantSwamp name.Name, campaignID string) (int32, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
		KeysOnly:   true,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterNestedSliceWhere("CampaignEntries",
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "CampaignID", campaignID),
		),
	)

	var count int32
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		count++
		return nil
	})
	if err != nil {
		return 0, err
	}
	return count, nil
}

// --- FilterNestedSliceAll Example ---

// SearchAllFinished finds domains where EVERY campaign entry is finished.
// Empty CampaignEntries evaluates to true (vacuous truth).
func (d *AssistantDomainPayload) SearchAllFinished(r repo.Repo, assistantSwamp name.Name) ([]*AssistantDomainPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterNestedSliceAll("CampaignEntries",
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 3), // Finished
		),
	)

	var results []*AssistantDomainPayload
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		results = append(results, model.(*AssistantDomainPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- FilterNestedSliceNone Example ---

// SearchNoActiveEntries finds domains with NO active campaign entries.
// Useful for cleanup: these domains can be removed from the queue.
func (d *AssistantDomainPayload) SearchNoActiveEntries(r repo.Repo, assistantSwamp name.Name) ([]*AssistantDomainPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterNestedSliceNone("CampaignEntries",
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1), // Active
		),
	)

	var results []*AssistantDomainPayload
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		results = append(results, model.(*AssistantDomainPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- FilterNestedSliceCount Example ---

// SearchMultiCampaignDomains finds domains participating in 3 or more active campaigns.
func (d *AssistantDomainPayload) SearchMultiCampaignDomains(r repo.Repo, assistantSwamp name.Name) ([]*AssistantDomainPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterNestedSliceCount("CampaignEntries",
			hydraidego.GreaterThanOrEqual, 3,
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1), // Active
		),
	)

	var results []*AssistantDomainPayload
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		results = append(results, model.(*AssistantDomainPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- IN Filter Examples ---

// SearchByStatusSet finds domains where at least one entry has a Status in the given set.
// Demonstrates FilterBytesFieldInt32In combined with FilterNestedSliceWhere.
func (d *AssistantDomainPayload) SearchByStatusSet(r repo.Repo, assistantSwamp name.Name, campaignID string, statuses []int32) ([]*AssistantDomainPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterAND(
		hydraidego.FilterNestedSliceWhere("CampaignEntries",
			hydraidego.FilterBytesFieldString(hydraidego.Equal, "CampaignID", campaignID),
			hydraidego.FilterBytesFieldInt32In("Status", statuses...),
		),
	)

	var results []*AssistantDomainPayload
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		results = append(results, model.(*AssistantDomainPayload))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

// --- Label Tracking Example ---

// SearchWithLabels demonstrates label tracking on NestedSliceWhere filters.
// The SearchMeta field in the model will contain matched labels.
func (d *AssistantDomainPayload) SearchWithLabels(r repo.Repo, assistantSwamp name.Name) ([]*AssistantDomainPayload, error) {
	ctx, cancel := hydraidehelper.CreateHydraContext()
	defer cancel()

	h := r.GetHydraidego()
	index := &hydraidego.Index{
		IndexType:  hydraidego.IndexCreationTime,
		IndexOrder: hydraidego.IndexOrderDesc,
	}

	filters := hydraidego.FilterOR(
		hydraidego.FilterNestedSliceWhere("CampaignEntries",
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 1),
		).WithLabel("has-active"),
		hydraidego.FilterNestedSliceWhere("CampaignEntries",
			hydraidego.FilterBytesFieldInt8(hydraidego.Equal, "Status", 3),
		).WithLabel("has-finished"),
	)

	var results []*AssistantDomainPayload
	err := h.CatalogReadManyStream(ctx, assistantSwamp, index, filters, AssistantDomainPayload{}, func(model any) error {
		m := model.(*AssistantDomainPayload)
		if m.Meta != nil {
			slog.Info("matched", "domain", m.Key, "labels", m.Meta.MatchedLabels)
		}
		results = append(results, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
