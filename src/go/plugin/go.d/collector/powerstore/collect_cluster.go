// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

func (c *Collector) collectClusterSpace() {
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
	mx := c.mx.cluster
	if last.PhysicalTotal != nil {
		mx.spacePhysicalTotal.Observe(float64(*last.PhysicalTotal))
	}
	if last.PhysicalUsed != nil {
		mx.spacePhysicalUsed.Observe(float64(*last.PhysicalUsed))
	}
	if last.LogicalProvisioned != nil {
		mx.spaceLogicalProvisioned.Observe(float64(*last.LogicalProvisioned))
	}
	if last.LogicalUsed != nil {
		mx.spaceLogicalUsed.Observe(float64(*last.LogicalUsed))
	}
	if last.DataPhysicalUsed != nil {
		mx.spaceDataPhysicalUsed.Observe(float64(*last.DataPhysicalUsed))
	}
	if last.SharedLogicalUsed != nil {
		mx.spaceSharedLogicalUsed.Observe(float64(*last.SharedLogicalUsed))
	}
	mx.spaceEfficiencyRatio.Observe(float64(last.EfficiencyRatio))
	mx.spaceDataReduction.Observe(float64(last.DataReduction))
	mx.spaceSnapshotSavings.Observe(float64(last.SnapshotSavings))
	mx.spaceThinSavings.Observe(float64(last.ThinSavings))
}
