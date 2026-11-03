// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse

import (
	"math"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

type metrics struct {
	performance metrix.SnapshotGauge
	fcp         metrix.SnapshotGauge
	lcp         metrix.SnapshotGauge
	tbt         metrix.SnapshotGauge
	si          metrix.SnapshotGauge
	cls         metrix.SnapshotGauge
}

func newMetrics(m metrix.SnapshotMeter) metrics {
	return metrics{
		performance: m.Gauge("performance", metrix.WithFloat(true)),
		fcp:         m.Gauge("fcp", metrix.WithFloat(true)),
		lcp:         m.Gauge("lcp", metrix.WithFloat(true)),
		tbt:         m.Gauge("tbt", metrix.WithFloat(true)),
		si:          m.Gauge("si", metrix.WithFloat(true)),
		cls:         m.Gauge("cls", metrix.WithFloat(true)),
	}
}
func (m metrics) write(run model.Run) {
	if v := run.Metrics; v != nil {
		observe(m.performance, v.Performance)
		observe(m.fcp, v.FCPMS)
		observe(m.lcp, v.LCPMS)
		observe(m.tbt, v.TBTMS)
		observe(m.si, v.SIMS)
		observe(m.cls, v.CLS)
	}
}
func observe(gauge metrix.SnapshotGauge, value *float64) {
	if value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0) {
		gauge.Observe(*value)
	}
}
