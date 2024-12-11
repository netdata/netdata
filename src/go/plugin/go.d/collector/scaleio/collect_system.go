// SPDX-License-Identifier: GPL-3.0-or-later

package scaleio

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/scaleio/client"

func (c *Collector) collectSystem(ss client.SystemStatistics) systemMetrics {
	var sm systemMetrics
	collectSystemCapacity(&sm, ss)
	collectSystemWorkload(&sm, ss)
	collectSystemRebalance(&sm, ss)
	collectSystemRebuild(&sm, ss)
	collectSystemComponents(&sm, ss)
	return sm
}

func collectSystemCapacity(sm *systemMetrics, ss client.SystemStatistics) {
	collectCapacity(&sm.Capacity, ss.CapacityStatistics)
}

func collectCapacity(m *capacity, ss client.CapacityStatistics) {
	// Health
	m.Protected = ss.ProtectedCapacityInKb
	m.InMaintenance = ss.InMaintenanceCapacityInKb
	m.Degraded = sum(ss.DegradedFailedCapacityInKb, ss.DegradedHealthyCapacityInKb)
	m.Failed = ss.FailedCapacityInKb
	m.UnreachableUnused = ss.UnreachableUnusedCapacityInKb

	// Capacity
	m.MaxCapacity = ss.MaxCapacityInKb
	m.ThickInUse = ss.ThickCapacityInUseInKb
	m.ThinInUse = ss.ThinCapacityInUseInKb
	m.Snapshot = ss.SnapCapacityInUseOccupiedInKb
	m.Spare = ss.SpareCapacityInKb
	m.Decreased = sum(ss.MaxCapacityInKb, -ss.CapacityLimitInKb) // TODO: probably wrong
	// Note: can't use 'UnusedCapacityInKb' directly, dashboard shows calculated value
	used := sum(
		ss.ProtectedCapacityInKb,
		ss.InMaintenanceCapacityInKb,
		m.Decreased,
		m.Degraded,
		ss.FailedCapacityInKb,
		ss.SpareCapacityInKb,
		ss.UnreachableUnusedCapacityInKb,
		ss.SnapCapacityInUseOccupiedInKb,
	)
	m.Unused = sum(ss.MaxCapacityInKb, -used)

	// Other
	m.InUse = ss.CapacityInUseInKb
	m.AvailableForVolumeAllocation = ss.CapacityAvailableForVolumeAllocationInKb
}

func collectSystemComponents(sm *systemMetrics, ss client.SystemStatistics) {
	m := &sm.Components

	m.Devices = ss.NumOfDevices
	m.FaultSets = ss.NumOfFaultSets
	m.MappedToAllVolumes = ss.NumOfMappedToAllVolumes
	m.ProtectionDomains = ss.NumOfProtectionDomains
	m.RfcacheDevices = ss.NumOfRfcacheDevices
	m.Sdc = ss.NumOfSdc
	m.Sds = ss.NumOfSds
	m.Snapshots = ss.NumOfSnapshots
	m.StoragePools = ss.NumOfStoragePools
	m.VTrees = ss.NumOfVtrees
	m.Volumes = ss.NumOfVolumes
	m.ThickBaseVolumes = ss.NumOfThickBaseVolumes
	m.ThinBaseVolumes = ss.NumOfThinBaseVolumes
	m.UnmappedVolumes = ss.NumOfUnmappedVolumes
	m.MappedVolumes = sum(ss.NumOfVolumes, -ss.NumOfUnmappedVolumes)
}

func collectSystemWorkload(sm *systemMetrics, ss client.SystemStatistics) {
	m := &sm.Workload

	m.Total.BW.set(
		calcBW(ss.TotalReadBwc),
		calcBW(ss.TotalWriteBwc),
	)
	m.Frontend.BW.set(
		calcBW(ss.UserDataReadBwc),
		calcBW(ss.UserDataWriteBwc),
	)
	m.Backend.Primary.BW.set(
		calcBW(ss.PrimaryReadBwc),
		calcBW(ss.PrimaryWriteBwc),
	)
	m.Backend.Secondary.BW.set(
		calcBW(ss.SecondaryReadBwc),
		calcBW(ss.SecondaryWriteBwc),
	)
	m.Backend.Total.BW.set(
		sumFloat(m.Backend.Primary.BW.Read, m.Backend.Secondary.BW.Read),
		sumFloat(m.Backend.Primary.BW.Write, m.Backend.Secondary.BW.Write),
	)

	m.Total.IOPS.set(
		calcIOPS(ss.TotalReadBwc),
		calcIOPS(ss.TotalWriteBwc),
	)
	m.Frontend.IOPS.set(
		calcIOPS(ss.UserDataReadBwc),
		calcIOPS(ss.UserDataWriteBwc),
	)
	m.Backend.Primary.IOPS.set(
		calcIOPS(ss.PrimaryReadBwc),
		calcIOPS(ss.PrimaryWriteBwc),
	)
	m.Backend.Secondary.IOPS.set(
		calcIOPS(ss.SecondaryReadBwc),
		calcIOPS(ss.SecondaryWriteBwc),
	)
	m.Backend.Total.IOPS.set(
		sumFloat(m.Backend.Primary.IOPS.Read, m.Backend.Secondary.IOPS.Read),
		sumFloat(m.Backend.Primary.IOPS.Write, m.Backend.Secondary.IOPS.Write),
	)

	m.Total.IOSize.set(
		calcIOSize(ss.TotalReadBwc),
		calcIOSize(ss.TotalWriteBwc),
	)
	m.Frontend.IOSize.set(
		calcIOSize(ss.UserDataReadBwc),
		calcIOSize(ss.UserDataWriteBwc),
	)
	m.Backend.Primary.IOSize.set(
		calcIOSize(ss.PrimaryReadBwc),
		calcIOSize(ss.PrimaryWriteBwc),
	)
	m.Backend.Secondary.IOSize.set(
		calcIOSize(ss.SecondaryReadBwc),
		calcIOSize(ss.SecondaryWriteBwc),
	)
	m.Backend.Total.IOSize.set(
		sumFloat(m.Backend.Primary.IOSize.Read, m.Backend.Secondary.IOSize.Read),
		sumFloat(m.Backend.Primary.IOSize.Write, m.Backend.Secondary.IOSize.Write),
	)
}

func collectSystemRebuild(sm *systemMetrics, ss client.SystemStatistics) {
	m := &sm.Rebuild

	m.Forward.BW.set(
		calcBW(ss.FwdRebuildReadBwc),
		calcBW(ss.FwdRebuildWriteBwc),
	)
	m.Backward.BW.set(
		calcBW(ss.BckRebuildReadBwc),
		calcBW(ss.BckRebuildWriteBwc),
	)
	m.Normal.BW.set(
		calcBW(ss.NormRebuildReadBwc),
		calcBW(ss.NormRebuildWriteBwc),
	)
	m.Total.BW.set(
		sumFloat(m.Forward.BW.Read, m.Backward.BW.Read, m.Normal.BW.Read),
		sumFloat(m.Forward.BW.Write, m.Backward.BW.Write, m.Normal.BW.Write),
	)

	m.Forward.IOPS.set(
		calcIOPS(ss.FwdRebuildReadBwc),
		calcIOPS(ss.FwdRebuildWriteBwc),
	)
	m.Backward.IOPS.set(
		calcIOPS(ss.BckRebuildReadBwc),
		calcIOPS(ss.BckRebuildWriteBwc),
	)
	m.Normal.IOPS.set(
		calcIOPS(ss.NormRebuildReadBwc),
		calcIOPS(ss.NormRebuildWriteBwc),
	)
	m.Total.IOPS.set(
		sumFloat(m.Forward.IOPS.Read, m.Backward.IOPS.Read, m.Normal.IOPS.Read),
		sumFloat(m.Forward.IOPS.Write, m.Backward.IOPS.Write, m.Normal.IOPS.Write),
	)

	m.Forward.IOSize.set(
		calcIOSize(ss.FwdRebuildReadBwc),
		calcIOSize(ss.FwdRebuildWriteBwc),
	)
	m.Backward.IOSize.set(
		calcIOSize(ss.BckRebuildReadBwc),
		calcIOSize(ss.BckRebuildWriteBwc),
	)
	m.Normal.IOSize.set(
		calcIOSize(ss.NormRebuildReadBwc),
		calcIOSize(ss.NormRebuildWriteBwc),
	)
	m.Total.IOSize.set(
		sumFloat(m.Forward.IOSize.Read, m.Backward.IOSize.Read, m.Normal.IOSize.Read),
		sumFloat(m.Forward.IOSize.Write, m.Backward.IOSize.Write, m.Normal.IOSize.Write),
	)

	m.Forward.Pending = ss.PendingFwdRebuildCapacityInKb
	m.Backward.Pending = ss.PendingBckRebuildCapacityInKb
	m.Normal.Pending = ss.PendingNormRebuildCapacityInKb
	m.Total.Pending = sum(m.Forward.Pending, m.Backward.Pending, m.Normal.Pending)
}

func collectSystemRebalance(sm *systemMetrics, ss client.SystemStatistics) {
	m := &sm.Rebalance

	m.BW.set(
		calcBW(ss.RebalanceReadBwc),
		calcBW(ss.RebalanceWriteBwc),
	)

	m.IOPS.set(
		calcIOPS(ss.RebalanceReadBwc),
		calcIOPS(ss.RebalanceWriteBwc),
	)

	m.IOSize.set(
		calcIOSize(ss.RebalanceReadBwc),
		calcIOSize(ss.RebalanceWriteBwc),
	)

	m.Pending = ss.PendingRebalanceCapacityInKb
	m.TimeUntilFinish = divFloat(float64(m.Pending), m.BW.ReadWrite)
}

func calcBW(bwc client.Bwc) float64     { return div(bwc.TotalWeightInKb, bwc.NumSeconds) }
func calcIOPS(bwc client.Bwc) float64   { return div(bwc.NumOccured, bwc.NumSeconds) }
func calcIOSize(bwc client.Bwc) float64 { return div(bwc.TotalWeightInKb, bwc.NumOccured) }

func sum(a, b int64, others ...int64) (res int64) {
	for _, v := range others {
		res += v
	}
	return res + a + b
}

func sumFloat(a, b float64, others ...float64) (res float64) {
	for _, v := range others {
		res += v
	}
	return res + a + b
}

func div(a, b int64) float64 {
	return divFloat(float64(a), float64(b))
}

func divFloat(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
