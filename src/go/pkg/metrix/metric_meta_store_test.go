// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricMetaScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"reader returns declared metric metadata": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				cc := cycleController(t, s)
				g := s.Write().SnapshotMeter("apache").Gauge(
					"workers_busy",
					WithDescription("Busy workers"),
					WithChartFamily("Workers"),
					WithUnit("workers"),
				)

				cc.BeginCycle()
				g.Observe(5)
				cc.CommitCycleSuccess()

				meta, ok := s.Read().MetricMeta("apache.workers_busy")
				require.True(t, ok)
				assert.Equal(t, "Busy workers", meta.Description)
				assert.Equal(t, "Workers", meta.ChartFamily)
				assert.Equal(t, "workers", meta.Unit)
			},
		},
		"unknown metric metadata is unavailable": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				meta, ok := s.Read().MetricMeta("missing.metric")
				assert.False(t, ok)
				assert.Equal(t, MetricMeta{}, meta)
			},
		},
		"metadata redeclaration conflict panics": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				w := s.Write().SnapshotMeter("apache")
				_ = w.Gauge("workers_busy", WithDescription("Busy workers"))
				expectPanic(t, func() {
					_ = w.Gauge("workers_busy", WithDescription("Workers currently busy"))
				})
			},
		},
		"redeclare without metadata options keeps first metadata": {
			run: func(t *testing.T) {
				s := NewCollectorStore()
				w := s.Write().SnapshotMeter("apache")
				g := w.Gauge(
					"workers_busy",
					WithDescription("Busy workers"),
					WithChartFamily("Workers"),
					WithUnit("workers"),
				)
				_ = w.Gauge("workers_busy")

				cc := cycleController(t, s)
				cc.BeginCycle()
				g.Observe(1)
				cc.CommitCycleSuccess()

				meta, ok := s.Read().MetricMeta("apache.workers_busy")
				require.True(t, ok)
				assert.Equal(t, "Busy workers", meta.Description)
				assert.Equal(t, "Workers", meta.ChartFamily)
				assert.Equal(t, "workers", meta.Unit)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
