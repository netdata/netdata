// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func (d Discoverer) matchHost(host *rs.Host) bool {
	if d.HostMatcher == nil {
		return true
	}
	return d.HostMatcher.Match(host)
}

func (d Discoverer) matchVM(vm *rs.VM) bool {
	if d.VMMatcher == nil {
		return true
	}
	return d.VMMatcher.Match(vm)
}

func (d Discoverer) removeUnmatched(res *rs.Resources) (removed int) {
	d.Debug("discovering : filtering : starting filtering resources process")
	t := time.Now()
	numH, numV := len(res.Hosts), len(res.VMs)
	removed += d.removeUnmatchedHosts(res.Hosts)
	removed += d.removeUnmatchedVMs(res.VMs)
	d.Infof("discovering : filtering : filtered %d/%d hosts, %d/%d vms, process took %s",
		numH-len(res.Hosts),
		numH,
		numV-len(res.VMs),
		numV,
		time.Since(t))
	return
}

func (d Discoverer) removeUnmatchedHosts(hosts rs.Hosts) (removed int) {
	for _, v := range hosts {
		if !d.matchHost(v) {
			removed++
			hosts.Remove(v.ID)
		}
	}
	d.Debugf("discovering : filtering : removed %d unmatched hosts", removed)
	return removed
}

func (d Discoverer) removeUnmatchedVMs(vms rs.VMs) (removed int) {
	for _, v := range vms {
		if !d.matchVM(v) {
			removed++
			vms.Remove(v.ID)
		}
	}
	d.Debugf("discovering : filtering : removed %d unmatched vms", removed)
	return removed
}
