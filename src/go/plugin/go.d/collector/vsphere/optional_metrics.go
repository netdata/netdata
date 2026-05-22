// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func optionalMetricNames() []string {
	var names []string
	names = append(names, datastoreClusterOptionalMetricNames()...)
	names = append(names, powerMetricsOptionalMetricNames()...)
	names = append(names, vsanOptionalMetricNames()...)
	return names
}

func (c *Collector) writeOptionalMetrics(meter metrix.SnapshotMeter) {
	c.writeDatastoreClusterMetrics(meter)
	c.writePowerMetrics(meter)
	c.writeVSANMetrics(meter)
}

func sortedVMs(vms rs.VMs) []*rs.VM {
	out := make([]*rs.VM, 0, len(vms))
	for _, vm := range vms {
		out = append(out, vm)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}
