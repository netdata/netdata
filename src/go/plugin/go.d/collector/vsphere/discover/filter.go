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
	numC, numH, numV, numD := len(res.Clusters), len(res.Hosts), len(res.VMs), len(res.Datastores)
	removed += d.removeUnmatchedClusters(res.Clusters)
	d.removeOrphanedResourcePools(res.ResourcePools, res.Clusters)
	removed += d.removeUnmatchedHosts(res.Hosts)
	removed += d.removeUnmatchedVMs(res.VMs)
	removed += d.removeUnmatchedDatastores(res.Datastores)
	d.Infof("discovering : filtering : filtered %d/%d clusters, %d/%d hosts, %d/%d vms, %d/%d datastores, %d resource pools remaining, process took %s",
		numC-len(res.Clusters),
		numC,
		numH-len(res.Hosts),
		numH,
		numV-len(res.VMs),
		numV,
		numD-len(res.Datastores),
		numD,
		len(res.ResourcePools),
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

func (d Discoverer) matchDatastore(ds *rs.Datastore) bool {
	if d.DatastoreMatcher == nil {
		return true
	}
	return d.DatastoreMatcher.Match(ds)
}

func (d Discoverer) removeUnmatchedDatastores(datastores rs.Datastores) (removed int) {
	for _, ds := range datastores {
		if !d.matchDatastore(ds) {
			removed++
			datastores.Remove(ds.ID)
		}
	}
	d.Debugf("discovering : filtering : removed %d unmatched datastores", removed)
	return removed
}

func (d Discoverer) matchCluster(cluster *rs.Cluster) bool {
	// dummy clusters (standalone host placeholders) are always excluded
	if isDummyCluster(cluster.ID) {
		return false
	}
	if d.ClusterMatcher == nil {
		return true
	}
	return d.ClusterMatcher.Match(cluster)
}

func (d Discoverer) removeUnmatchedClusters(clusters rs.Clusters) (removed int) {
	for _, c := range clusters {
		if !d.matchCluster(c) {
			removed++
			clusters.Remove(c.ID)
		}
	}
	d.Debugf("discovering : filtering : removed %d unmatched/dummy clusters", removed)
	return removed
}

// removeOrphanedResourcePools removes pools whose owner cluster was filtered out.
func (d Discoverer) removeOrphanedResourcePools(pools rs.ResourcePools, clusters rs.Clusters) {
	var removed int
	for _, rp := range pools {
		if clusters.Get(rp.ParentID) == nil {
			removed++
			pools.Remove(rp.ID)
		}
	}
	if removed > 0 {
		d.Debugf("discovering : filtering : removed %d orphaned resource pools", removed)
	}
}
