// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func (d Discoverer) setHierarchy(res *rs.Resources) error {
	d.Debug("discovering : hierarchy : start setting resources hierarchy process")
	t := time.Now()

	c := d.setClustersHierarchy(res)
	h := d.setHostsHierarchy(res)
	v := d.setVMsHierarchy(res)

	// notSet := len(res.Clusters) + len(res.Hosts) + len(res.VMs) - (c + h + v)
	d.Infof("discovering : hierarchy : set %d/%d clusters, %d/%d hosts, %d/%d vms, process took %s",
		c, len(res.Clusters),
		h, len(res.Hosts),
		v, len(res.VMs),
		time.Since(t),
	)

	return nil
}

func (d Discoverer) setClustersHierarchy(res *rs.Resources) (set int) {
	for _, cluster := range res.Clusters {
		if setClusterHierarchy(cluster, res) {
			set++
		}
	}
	return set
}

func (d Discoverer) setHostsHierarchy(res *rs.Resources) (set int) {
	for _, host := range res.Hosts {
		if setHostHierarchy(host, res) {
			set++
		}
	}
	return set
}

func (d Discoverer) setVMsHierarchy(res *rs.Resources) (set int) {
	for _, vm := range res.VMs {
		if setVMHierarchy(vm, res) {
			set++
		}
	}
	return set
}

func setClusterHierarchy(cluster *rs.Cluster, res *rs.Resources) bool {
	dc := res.DataCenters.Get(cluster.ParentID)
	if dc == nil {
		return false
	}
	cluster.Hier.DC.Set(dc.ID, dc.Name)
	return cluster.Hier.IsSet()
}

func setHostHierarchy(host *rs.Host, res *rs.Resources) bool {
	cr := res.Clusters.Get(host.ParentID)
	if cr == nil {
		return false
	}
	host.Hier.Cluster.Set(cr.ID, cr.Name)

	dc := res.DataCenters.Get(cr.ParentID)
	if dc == nil {
		return false
	}
	host.Hier.DC.Set(dc.ID, dc.Name)
	return host.Hier.IsSet()
}

func setVMHierarchy(vm *rs.VM, res *rs.Resources) bool {
	h := res.Hosts.Get(vm.ParentID)
	if h == nil {
		return false
	}
	vm.Hier.Host.Set(h.ID, h.Name)

	cr := res.Clusters.Get(h.ParentID)
	if cr == nil {
		return false
	}
	vm.Hier.Cluster.Set(cr.ID, cr.Name)

	dc := res.DataCenters.Get(cr.ParentID)
	if dc == nil {
		return false
	}
	vm.Hier.DC.Set(dc.ID, dc.Name)
	return vm.Hier.IsSet()
}
