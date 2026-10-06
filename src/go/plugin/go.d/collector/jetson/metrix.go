// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	gpuUtilization metrix.SnapshotGauge
	gpuFrequency   metrix.SnapshotGauge
	gpcFrequency   metrix.SnapshotGaugeVec
	emcUtilization metrix.SnapshotGauge
	emcFrequency   metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	m := store.Write().SnapshotMeter("")
	return &collectorMetrics{
		gpuUtilization: m.Gauge("gpu_utilization", metrix.WithFloat(true)),
		gpuFrequency:   m.Gauge("gpu_frequency", metrix.WithFloat(true)),
		gpcFrequency:   m.Vec("gpc").Gauge("gpu_gpc_frequency", metrix.WithFloat(true)),
		emcUtilization: m.Gauge("emc_utilization", metrix.WithFloat(true)),
		emcFrequency:   m.Gauge("emc_frequency", metrix.WithFloat(true)),
	}
}
