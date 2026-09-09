// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

var enumPoints = map[string]metrix.StateSetPoint{
	"active":   {States: map[string]bool{"active": true}},
	"disabled": {States: map[string]bool{"disabled": true}},
	"down":     {States: map[string]bool{"down": true}},
	"err":      {States: map[string]bool{"err": true}},
	"failed":   {States: map[string]bool{"failed": true}},
	"in":       {States: map[string]bool{"in": true}},
	"inactive": {States: map[string]bool{"inactive": true}},
	"ok":       {States: map[string]bool{"ok": true}},
	"out":      {States: map[string]bool{"out": true}},
	"success":  {States: map[string]bool{"success": true}},
	"up":       {States: map[string]bool{"up": true}},
	"warn":     {States: map[string]bool{"warn": true}},
}

type collectorMetrics struct {
	meter metrix.SnapshotMeter

	componentCollectionStatus    metrix.StateSetInstrument
	clusterHealthStatus          metrix.StateSetInstrument
	clusterHosts                 metrix.SnapshotGauge
	clusterMonitors              metrix.SnapshotGauge
	clusterOSDs                  metrix.SnapshotGauge
	clusterOSDsByStatus          metrix.SnapshotGauge
	clusterManagers              metrix.SnapshotGauge
	clusterObjectGateways        metrix.SnapshotGauge
	clusterISCSIGateways         metrix.SnapshotGauge
	clusterISCSIGatewaysByStatus metrix.SnapshotGauge
	clusterCapacityUtilization   metrix.SnapshotGauge
	clusterCapacityBytes         metrix.SnapshotGauge
	clusterObjects               metrix.SnapshotGauge
	clusterObjectCopiesHealth    metrix.SnapshotGauge
	clusterObjectsUnfound        metrix.SnapshotGauge
	clusterPools                 metrix.SnapshotGauge
	clusterPGs                   metrix.SnapshotGauge
	clusterPGsByStatus           metrix.SnapshotGauge
	clusterPGsPerOSD             metrix.SnapshotGauge
	clusterClientIOBytes         metrix.SnapshotGauge
	clusterClientIOPS            metrix.SnapshotGauge
	clusterRecoveryBytes         metrix.SnapshotGauge
	clusterScrubStatus           metrix.StateSetInstrument

	osdOperationalStatus metrix.StateSetInstrument
	osdMembershipStatus  metrix.StateSetInstrument
	osdSpaceBytes        metrix.SnapshotGauge
	osdIOBytes           metrix.SnapshotGauge
	osdIOPS              metrix.SnapshotGauge
	osdLatency           metrix.SnapshotGauge

	poolSpaceUtilization metrix.SnapshotGauge
	poolSpaceBytes       metrix.SnapshotGauge
	poolObjects          metrix.SnapshotGauge
	poolIOBytes          metrix.SnapshotCounter
	poolOperations       metrix.SnapshotCounter

	componentLabels map[string]metrix.LabelSet
	statusLabels    map[string]metrix.LabelSet
	stateLabels     map[string]metrix.LabelSet
	directionLabels map[string]metrix.LabelSet
	stageLabels     map[string]metrix.LabelSet
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	float := metrix.WithFloat(true)

	return &collectorMetrics{
		meter: meter,

		componentCollectionStatus: meter.StateSet("component_collection_status",
			metrix.WithStateSetStates("success", "failed"), metrix.WithStateSetMode(metrix.ModeEnum)),
		clusterHealthStatus: meter.StateSet("cluster_health_status",
			metrix.WithStateSetStates("ok", "warn", "err"), metrix.WithStateSetMode(metrix.ModeEnum)),
		clusterHosts:                 meter.Gauge("cluster_hosts"),
		clusterMonitors:              meter.Gauge("cluster_monitors"),
		clusterOSDs:                  meter.Gauge("cluster_osds"),
		clusterOSDsByStatus:          meter.Gauge("cluster_osds_by_status"),
		clusterManagers:              meter.Gauge("cluster_managers"),
		clusterObjectGateways:        meter.Gauge("cluster_object_gateways"),
		clusterISCSIGateways:         meter.Gauge("cluster_iscsi_gateways"),
		clusterISCSIGatewaysByStatus: meter.Gauge("cluster_iscsi_gateways_by_status"),
		clusterCapacityUtilization:   meter.Gauge("cluster_physical_capacity_utilization_percent", float),
		clusterCapacityBytes:         meter.Gauge("cluster_physical_capacity_bytes"),
		clusterObjects:               meter.Gauge("cluster_objects"),
		clusterObjectCopiesHealth:    meter.Gauge("cluster_object_copies_health_percent", float),
		clusterObjectsUnfound:        meter.Gauge("cluster_objects_unfound_percent", float),
		clusterPools:                 meter.Gauge("cluster_pools"),
		clusterPGs:                   meter.Gauge("cluster_pgs"),
		clusterPGsByStatus:           meter.Gauge("cluster_pgs_by_status"),
		clusterPGsPerOSD:             meter.Gauge("cluster_pgs_per_osd", float),
		clusterClientIOBytes:         meter.Gauge("cluster_client_io_bytes_per_sec", float),
		clusterClientIOPS:            meter.Gauge("cluster_client_iops", float),
		clusterRecoveryBytes:         meter.Gauge("cluster_recovery_bytes_per_sec", float),
		clusterScrubStatus: meter.StateSet("cluster_scrub_status",
			metrix.WithStateSetStates("disabled", "active", "inactive"), metrix.WithStateSetMode(metrix.ModeEnum)),

		osdOperationalStatus: meter.StateSet("osd_operational_status",
			metrix.WithStateSetStates("up", "down"), metrix.WithStateSetMode(metrix.ModeEnum)),
		osdMembershipStatus: meter.StateSet("osd_membership_status",
			metrix.WithStateSetStates("in", "out"), metrix.WithStateSetMode(metrix.ModeEnum)),
		osdSpaceBytes: meter.Gauge("osd_space_bytes"),
		osdIOBytes:    meter.Gauge("osd_io_bytes_per_sec", float),
		osdIOPS:       meter.Gauge("osd_iops", float),
		osdLatency:    meter.Gauge("osd_latency_ms", float),

		poolSpaceUtilization: meter.Gauge("pool_space_utilization_percent", float),
		poolSpaceBytes:       meter.Gauge("pool_space_bytes"),
		poolObjects:          meter.Gauge("pool_objects"),
		poolIOBytes:          meter.Counter("pool_io_bytes_total"),
		poolOperations:       meter.Counter("pool_operations_total"),

		componentLabels: labelSets(meter, "component", "health", "osds", "pools"),
		statusLabels: labelSets(meter, "status",
			"up", "down", "in", "out", "active", "standby", "clean", "working", "warning", "unknown"),
		stateLabels:     labelSets(meter, "state", "avail", "used", "degraded", "misplaced"),
		directionLabels: labelSets(meter, "direction", "read", "write", "written"),
		stageLabels:     labelSets(meter, "stage", "commit", "apply"),
	}
}

func labelSets(meter metrix.SnapshotMeter, key string, values ...string) map[string]metrix.LabelSet {
	sets := make(map[string]metrix.LabelSet, len(values))
	for _, value := range values {
		sets[value] = meter.LabelSet(metrix.Label{Key: key, Value: value})
	}
	return sets
}

func (m *collectorMetrics) clusterLabels(fsid string) metrix.LabelSet {
	return m.meter.LabelSet(metrix.Label{Key: "fsid", Value: fsid})
}

func (m *collectorMetrics) osdLabels(fsid string, sample osdMetricSample) metrix.LabelSet {
	return m.meter.LabelSet(
		metrix.Label{Key: "fsid", Value: fsid},
		metrix.Label{Key: "osd_uuid", Value: sample.uuid},
		metrix.Label{Key: "osd_name", Value: sample.name},
		metrix.Label{Key: "device_class", Value: sample.deviceClass},
	)
}

func (m *collectorMetrics) poolLabels(fsid string, sample poolMetricSample) metrix.LabelSet {
	return m.meter.LabelSet(
		metrix.Label{Key: "fsid", Value: fsid},
		metrix.Label{Key: "pool_name", Value: sample.name},
	)
}

func observeEnum(instrument metrix.StateSetInstrument, state string, labels metrix.LabelSet) {
	instrument.ObserveStateSet(enumPoints[state], labels)
}

func (m *collectorMetrics) writeComponentStatuses(labels metrix.LabelSet, healthErr, osdErr, poolErr error) {
	m.componentCollectionStatus.ObserveStateSet(
		enumPoints[collectionStatus(healthErr)], labels, m.componentLabels["health"])
	m.componentCollectionStatus.ObserveStateSet(
		enumPoints[collectionStatus(osdErr)], labels, m.componentLabels["osds"])
	m.componentCollectionStatus.ObserveStateSet(
		enumPoints[collectionStatus(poolErr)], labels, m.componentLabels["pools"])
}

func (m *collectorMetrics) writeOSD(fsid string, sample osdMetricSample) {
	labels := m.osdLabels(fsid, sample)
	operational := "down"
	if sample.up {
		operational = "up"
	}
	membership := "out"
	if sample.in {
		membership = "in"
	}
	observeEnum(m.osdOperationalStatus, operational, labels)
	observeEnum(m.osdMembershipStatus, membership, labels)
	m.osdSpaceBytes.Observe(float64(sample.total-sample.available), labels, m.stateLabels["used"])
	m.osdSpaceBytes.Observe(float64(sample.available), labels, m.stateLabels["avail"])
	m.osdIOBytes.Observe(sample.readBytes, labels, m.directionLabels["read"])
	m.osdIOBytes.Observe(sample.writeBytes, labels, m.directionLabels["written"])
	m.osdIOPS.Observe(sample.readOps, labels, m.directionLabels["read"])
	m.osdIOPS.Observe(sample.writeOps, labels, m.directionLabels["write"])
	m.osdLatency.Observe(sample.commitMS, labels, m.stageLabels["commit"])
	m.osdLatency.Observe(sample.applyMS, labels, m.stageLabels["apply"])
}

func (m *collectorMetrics) writePool(fsid string, sample poolMetricSample) {
	labels := m.poolLabels(fsid, sample)
	m.poolSpaceUtilization.Observe(sample.utilizationRatio*100, labels)
	m.poolSpaceBytes.Observe(float64(sample.used), labels, m.stateLabels["used"])
	m.poolSpaceBytes.Observe(float64(sample.available), labels, m.stateLabels["avail"])
	m.poolObjects.Observe(float64(sample.objects), labels)
	m.poolIOBytes.ObserveTotal(float64(sample.readBytes), labels, m.directionLabels["read"])
	m.poolIOBytes.ObserveTotal(float64(sample.writtenBytes), labels, m.directionLabels["written"])
	m.poolOperations.ObserveTotal(float64(sample.readOps), labels, m.directionLabels["read"])
	m.poolOperations.ObserveTotal(float64(sample.writeOps), labels, m.directionLabels["write"])
}

func collectionStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return "success"
}
