// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"cmp"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// AcquisitionCacheInput belongs to one committed cache generation. Its maps
// and source objects are immutable, allowing a poll to retain references only.
type AcquisitionCacheInput struct {
	Kind      string
	Profile   string
	RootOID   string
	ConfigID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	Marker    bool
	OIDs      map[string]map[string]string
	TableTags map[string]map[string]string
	Tags      map[string]string
	Metadata  map[string]ddsnmp.MetaTag
	Sources   []*ddsnmp.SourceOperation
}

// CachedInputs snapshots cache owners, not their historical raw payloads.
// Like Collect, only the synchronous collector owner calls this method.
func (c *Collector) CachedInputs() []AcquisitionCacheInput {
	var result []AcquisitionCacheInput
	for _, profile := range c.sortedProfileStates() {
		if input := profile.cache.tagEvidence; input != nil {
			item := *input
			item.Profile = profile.profile.SourceFile
			result = append(result, item)
		}
		if input := profile.cache.metadataEvidence; input != nil {
			item := *input
			item.Profile = profile.profile.SourceFile
			result = append(result, item)
		}
	}
	c.tableCache.mu.RLock()
	for root, configs := range c.tableCache.tables {
		for configID, entry := range configs {
			if entry.evidence == nil {
				continue
			}
			input := *entry.evidence
			input.RootOID, input.ConfigID = root, configID
			input.ExpiresAt = c.tableCache.timestamps[root].Add(c.tableCache.tableTTLs[root])
			result = append(result, input)
		}
	}
	c.tableCache.mu.RUnlock()
	slices.SortFunc(result, func(a, b AcquisitionCacheInput) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.Profile, b.Profile), cmp.Compare(a.RootOID, b.RootOID), cmp.Compare(a.ConfigID, b.ConfigID))
	})
	return result
}

func (s *tableCollectionSession) cacheSources(req *tableCollectionRequest) []*ddsnmp.SourceOperation {
	recorder := sourceRecorder(s.collector.snmpClient)
	if recorder == nil {
		return nil
	}
	var sources []*ddsnmp.SourceOperation
	seen := make(map[*ddsnmp.SourceOperation]bool)
	if operation := recorder.Operation(req.route.sourceOperation); operation != nil {
		sources = append(sources, operation)
		seen[operation] = true
	}
	for _, dependency := range req.dependencies {
		if operation := recorder.Operation(dependency.sourceOperation); operation != nil && !seen[operation] {
			sources = append(sources, operation)
			seen[operation] = true
		}
	}
	return sources
}

func (c *acquisitionProfileCollection) retainInputRoutes(recorder *SourceRecorder) []AcquisitionRouteReport {
	if c == nil {
		return nil
	}
	var result []AcquisitionRouteReport
	for _, route := range c.routes {
		if route.Kind != AcquisitionRouteKindProfileTagScalar && route.Kind != AcquisitionRouteKindMetadataScalar {
			continue
		}
		route.Sources = slices.Clone(route.Sources)
		if recorder != nil {
			for i := range route.Sources {
				route.Sources[i].ContextID = recorder.ContextID
			}
		}
		result = append(result, route)
	}
	return result
}

func (c *acquisitionProfileCollection) restoreInputRoutes(routes []AcquisitionRouteReport) {
	if c == nil {
		return
	}
	for _, route := range routes {
		route.Source = AcquisitionRouteSourceCache
		route.Reused = true
		c.routes[route.Ordinal] = route
	}
}
