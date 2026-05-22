// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func (c *Collector) resourceEnrichmentLabels(resourceID string) []metrix.Label {
	if c.resources == nil {
		return nil
	}

	labels := make(map[string]string)
	for key, value := range c.userMetadataLabels(resourceID) {
		if key != "" && value != "" {
			labels[key] = value
		}
	}

	if len(labels) == 0 {
		return nil
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]metrix.Label, 0, len(keys))
	for _, key := range keys {
		out = append(out, metrix.Label{Key: key, Value: labels[key]})
	}
	return out
}

func (c *Collector) userMetadataLabels(resourceID string) map[string]string {
	if c.resources == nil {
		return nil
	}
	if vm := c.resources.VMs.Get(resourceID); vm != nil {
		return vm.Labels
	}
	if host := c.resources.Hosts.Get(resourceID); host != nil {
		return host.Labels
	}
	if ds := c.resources.Datastores.Get(resourceID); ds != nil {
		return ds.Labels
	}
	if cluster := c.resources.Clusters.Get(resourceID); cluster != nil {
		return cluster.Labels
	}
	if rp := c.resources.ResourcePools.Get(resourceID); rp != nil {
		return rp.Labels
	}
	if sp := c.resources.StoragePods.Get(resourceID); sp != nil {
		return sp.Labels
	}
	return nil
}
