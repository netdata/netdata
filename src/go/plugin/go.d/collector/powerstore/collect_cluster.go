// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectClusterSpace(mx *metrics) {
	if len(c.discovered.clusters) == 0 {
		return
	}

	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	cl := c.discovered.clusters[0]
	sm, err := c.client.SpaceMetricsByCluster(cl.ID)
	if err != nil {
		c.Warningf("error collecting cluster space metrics: %v", err)
		return
	}
	if len(sm) == 0 {
		return
	}

	last := sm[len(sm)-1]
	if last.PhysicalTotal != nil {
		mx.Cluster.Space.PhysicalTotal = *last.PhysicalTotal
	}
	if last.PhysicalUsed != nil {
		mx.Cluster.Space.PhysicalUsed = *last.PhysicalUsed
	}
	if last.LogicalProvisioned != nil {
		mx.Cluster.Space.LogicalProvisioned = *last.LogicalProvisioned
	}
	if last.LogicalUsed != nil {
		mx.Cluster.Space.LogicalUsed = *last.LogicalUsed
	}
	if last.DataPhysicalUsed != nil {
		mx.Cluster.Space.DataPhysicalUsed = *last.DataPhysicalUsed
	}
	if last.SharedLogicalUsed != nil {
		mx.Cluster.Space.SharedLogicalUsed = *last.SharedLogicalUsed
	}
	mx.Cluster.Space.EfficiencyRatio = last.EfficiencyRatio
	mx.Cluster.Space.DataReduction = last.DataReduction
	mx.Cluster.Space.SnapshotSavings = last.SnapshotSavings
	mx.Cluster.Space.ThinSavings = last.ThinSavings
}
