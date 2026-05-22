// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func (c *Collector) writeOptionalMetrics() {
	c.writeDatastoreClusterMetrics()
	c.writePowerMetrics()
	c.writeVSANMetrics()
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
