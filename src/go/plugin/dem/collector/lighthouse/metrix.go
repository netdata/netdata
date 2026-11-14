// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
)

type metrics struct {
	performance metrix.SnapshotGauge
	fcp         metrix.SnapshotGauge
	lcp         metrix.SnapshotGauge
	tbt         metrix.SnapshotGauge
	si          metrix.SnapshotGauge
	cls         metrix.SnapshotGauge
}

func newMetrics(m metrix.SnapshotMeter) syntheticjob.Metrics {
	return metrics{
		performance: m.Gauge("performance", metrix.WithFloat(true)),
		fcp:         m.Gauge("fcp", metrix.WithFloat(true)),
		lcp:         m.Gauge("lcp", metrix.WithFloat(true)),
		tbt:         m.Gauge("tbt", metrix.WithFloat(true)),
		si:          m.Gauge("si", metrix.WithFloat(true)),
		cls:         m.Gauge("cls", metrix.WithFloat(true)),
	}
}
func (m metrics) Write(run monitor.Observation) {
	if v := run.Metrics; run.HasMetrics {
		observe(m.performance, v.Performance)
		observe(m.fcp, v.FCPMS)
		observe(m.lcp, v.LCPMS)
		observe(m.tbt, v.TBTMS)
		observe(m.si, v.SIMS)
		observe(m.cls, v.CLS)
	}
}

// observe publishes a present measurement; the runner accepts only finite, non-negative lab values.
func observe(gauge metrix.SnapshotGauge, value monitor.Scalar) {
	if value.Present {
		gauge.Observe(value.Value)
	}
}
