package explorer

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// hierarchicalIndex stores swamp metadata in a Sanctuary → Realm → Swamp hierarchy.
type hierarchicalIndex struct {
	mu          sync.RWMutex
	sanctuaries map[string]*sanctuaryNode
}

type sanctuaryNode struct {
	name   string
	realms map[string]*realmNode
}

type realmNode struct {
	sanctuary string
	name      string
	swamps    map[string]*SwampDetail
}

func newIndex() *hierarchicalIndex {
	return &hierarchicalIndex{
		sanctuaries: make(map[string]*sanctuaryNode),
	}
}

// add inserts a swamp detail into the index. Thread-safe.
func (idx *hierarchicalIndex) add(detail *SwampDetail) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	sn, ok := idx.sanctuaries[detail.Sanctuary]
	if !ok {
		sn = &sanctuaryNode{
			name:   detail.Sanctuary,
			realms: make(map[string]*realmNode),
		}
		idx.sanctuaries[detail.Sanctuary] = sn
	}

	rn, ok := sn.realms[detail.Realm]
	if !ok {
		rn = &realmNode{
			sanctuary: detail.Sanctuary,
			name:      detail.Realm,
			swamps:    make(map[string]*SwampDetail),
		}
		sn.realms[detail.Realm] = rn
	}

	rn.swamps[detail.Swamp] = detail
}

// clear removes all entries from the index.
func (idx *hierarchicalIndex) clear() {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.sanctuaries = make(map[string]*sanctuaryNode)
}

// listSanctuaries returns all sanctuaries with aggregated stats, sorted by name.
func (idx *hierarchicalIndex) listSanctuaries() []*SanctuaryInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make([]*SanctuaryInfo, 0, len(idx.sanctuaries))
	for _, sn := range idx.sanctuaries {
		info := &SanctuaryInfo{
			Name:       sn.name,
			RealmCount: int64(len(sn.realms)),
		}
		for _, rn := range sn.realms {
			info.SwampCount += int64(len(rn.swamps))
			for _, sd := range rn.swamps {
				info.TotalSize += sd.FileSize
			}
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// listRealms returns all realms for a sanctuary, sorted by name.
func (idx *hierarchicalIndex) listRealms(sanctuary string) []*RealmInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	sn, ok := idx.sanctuaries[sanctuary]
	if !ok {
		return nil
	}

	result := make([]*RealmInfo, 0, len(sn.realms))
	for _, rn := range sn.realms {
		info := &RealmInfo{
			Sanctuary:  sanctuary,
			Name:       rn.name,
			SwampCount: int64(len(rn.swamps)),
		}
		for _, sd := range rn.swamps {
			info.TotalSize += sd.FileSize
		}
		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// listSwamps returns swamps matching the filter with pagination.
func (idx *hierarchicalIndex) listSwamps(filter *SwampFilter) *SwampListResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Collect matching swamps
	var all []*SwampDetail

	sanctuaries := idx.sanctuaries
	if filter.Sanctuary != "" {
		sn, ok := sanctuaries[filter.Sanctuary]
		if !ok {
			return &SwampListResult{Limit: filter.Limit}
		}
		sanctuaries = map[string]*sanctuaryNode{filter.Sanctuary: sn}
	}

	for _, sn := range sanctuaries {
		realms := sn.realms
		if filter.Realm != "" {
			rn, ok := realms[filter.Realm]
			if !ok {
				continue
			}
			realms = map[string]*realmNode{filter.Realm: rn}
		}

		for _, rn := range realms {
			for _, sd := range rn.swamps {
				if filter.SwampPrefix != "" && !strings.HasPrefix(sd.Swamp, filter.SwampPrefix) {
					continue
				}
				all = append(all, sd)
			}
		}
	}

	// Sort by full name
	sort.Slice(all, func(i, j int) bool {
		fi := all[i].Sanctuary + "/" + all[i].Realm + "/" + all[i].Swamp
		fj := all[j].Sanctuary + "/" + all[j].Realm + "/" + all[j].Swamp
		return fi < fj
	})

	total := int64(len(all))

	// Apply pagination
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if end > total {
		end = total
	}

	return &SwampListResult{
		Swamps: all[start:end],
		Total:  total,
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}
}

// getSwampDetail returns details for a specific swamp.
func (idx *hierarchicalIndex) getSwampDetail(sanctuary, realm, swamp string) (*SwampDetail, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	sn, ok := idx.sanctuaries[sanctuary]
	if !ok {
		return nil, fmt.Errorf("sanctuary %q not found", sanctuary)
	}
	rn, ok := sn.realms[realm]
	if !ok {
		return nil, fmt.Errorf("realm %q not found in sanctuary %q", realm, sanctuary)
	}
	sd, ok := rn.swamps[swamp]
	if !ok {
		return nil, fmt.Errorf("swamp %q not found in %s/%s", swamp, sanctuary, realm)
	}
	return sd, nil
}

// getSize returns aggregated size info at the specified level.
func (idx *hierarchicalIndex) getSize(sanctuary, realm, swamp string) (*SizeInfo, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	info := &SizeInfo{
		Sanctuary: sanctuary,
		Realm:     realm,
		Swamp:     swamp,
	}

	if sanctuary == "" {
		return nil, fmt.Errorf("sanctuary is required")
	}

	sn, ok := idx.sanctuaries[sanctuary]
	if !ok {
		return nil, fmt.Errorf("sanctuary %q not found", sanctuary)
	}

	if swamp != "" {
		// Specific swamp
		rn, ok := sn.realms[realm]
		if !ok {
			return nil, fmt.Errorf("realm %q not found", realm)
		}
		sd, ok := rn.swamps[swamp]
		if !ok {
			return nil, fmt.Errorf("swamp %q not found", swamp)
		}
		info.TotalSize = sd.FileSize
		info.FileCount = 1
		return info, nil
	}

	if realm != "" {
		// Specific realm
		rn, ok := sn.realms[realm]
		if !ok {
			return nil, fmt.Errorf("realm %q not found", realm)
		}
		for _, sd := range rn.swamps {
			info.TotalSize += sd.FileSize
			info.FileCount++
		}
		return info, nil
	}

	// Entire sanctuary
	for _, rn := range sn.realms {
		for _, sd := range rn.swamps {
			info.TotalSize += sd.FileSize
			info.FileCount++
		}
	}
	return info, nil
}
