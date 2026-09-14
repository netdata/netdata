// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

// Called after successful consumer commits. Partial source failures retain only
// the sources actually still owned by the BGP cache; licensing replaces a set.
func (c *Collector) commitNormalConsumers(pms []*ddsnmp.ProfileMetrics) {
	n := c.normal
	if n.current == nil {
		return
	}
	if n.bgpSources == nil {
		n.bgpSources = make(map[string][]*ddsnmp.SourceOperation)
	}
	current := make(map[string]diagnostics.NormalProfile, len(n.current.document.Profiles))
	for _, profile := range n.current.document.Profiles {
		current[profile.Source] = profile
	}
	n.licenseSources = nil
	for _, pm := range pms {
		if pm == nil {
			continue
		}
		profile := current[pm.Source]
		if pm.BGPCollectError == nil {
			n.bgpSources[pm.Source] = n.consumerSources(profile.Acquisition, false)
		}
		n.licenseSources = append(n.licenseSources, n.consumerSources(profile.Acquisition, true)...)
	}
}

func (n *normalDiagnostics) consumerSources(report ddsnmpcollector.AcquisitionProfileReport, licensing bool) []*ddsnmp.SourceOperation {
	seen := make(map[*ddsnmp.SourceOperation]bool)
	var result []*ddsnmp.SourceOperation
	for _, route := range report.Routes {
		isLicense := route.Kind == ddsnmpcollector.AcquisitionRouteKindLicenseScalar || route.Kind == ddsnmpcollector.AcquisitionRouteKindLicenseTable
		isBGP := route.Kind == ddsnmpcollector.AcquisitionRouteKindBGPScalar || route.Kind == ddsnmpcollector.AcquisitionRouteKindBGPTable
		if (!licensing && !isBGP) || (licensing && !isLicense) {
			continue
		}
		for _, binding := range route.Sources {
			operation := n.recorder.Operation(binding.Operation)
			if operation != nil && !seen[operation] {
				seen[operation] = true
				result = append(result, operation)
			}
		}
	}
	return result
}

func (c *Collector) captureNormalBGP() (diagnostics.NormalBGP, map[string][]*ddsnmp.SourceOperation) {
	result := diagnostics.NormalBGP{Enabled: c.bgp != nil}
	if c.bgp == nil || c.bgp.peerCache == nil {
		return result, nil
	}
	cache := c.bgp.peerCache
	cache.mu.RLock()
	result.LastUpdate, result.LastFailure, result.StaleAfterNanos = cache.lastUpdate, cache.lastFailure, int64(cache.staleAfter)
	owners := make(map[string]bool)
	for _, entry := range cache.entries {
		result.Entries = append(result.Entries, normalBGPPeer(entry))
		owners[entry.source] = true
	}
	cache.mu.RUnlock()
	slices.SortFunc(result.Entries, func(a, b diagnostics.NormalBGPPeer) int { return strings.Compare(a.Key, b.Key) })
	for source := range c.normal.bgpSources {
		if !owners[source] {
			delete(c.normal.bgpSources, source)
		}
	}
	return result, maps.Clone(c.normal.bgpSources)
}

func (c *Collector) captureNormalLicensing() (diagnostics.NormalLicensing, []*ddsnmp.SourceOperation) {
	result := diagnostics.NormalLicensing{Enabled: c.licensing != nil}
	if c.licensing == nil || c.licensing.cache == nil {
		return result, nil
	}
	cache := c.licensing.cache
	cache.mu.RLock()
	result.NormalizedAt = cache.lastUpdate
	result.Rows = make([]diagnostics.NormalLicense, len(cache.rows))
	for i, row := range cache.rows {
		result.Rows[i] = normalLicense(row)
	}
	cache.mu.RUnlock()
	return result, c.normal.licenseSources
}
