// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
)

// SchedulerSnapshot is a typed scheduler state snapshot used by v2 collectors.
type SchedulerSnapshot struct {
	Scheduler string

	Running   int64
	Queued    int64
	Scheduled int64
	Started   uint64
	Finished  uint64
	Skipped   uint64
	Next      time.Duration

	Jobs []JobMetricsSnapshot
}

// JobMetricsSnapshot contains per-job runtime metrics and raw perfdata samples.
type JobMetricsSnapshot struct {
	JobID   string
	JobName string

	State      string
	Attempt    int
	MaxAttempt int

	Running     bool
	Retrying    bool
	PeriodSkip  bool
	CPUMissing  bool
	Duration    time.Duration
	CPUTime     time.Duration
	RSS         int64
	DiskRead    int64
	DiskWrite   int64
	PerfSamples []output.PerfDatum
}

func clonePerfDatumList(src []output.PerfDatum) []output.PerfDatum {
	if len(src) == 0 {
		return nil
	}
	out := make([]output.PerfDatum, 0, len(src))
	for _, datum := range src {
		item := datum
		item.Min = cloneFloatPtr(datum.Min)
		item.Max = cloneFloatPtr(datum.Max)
		item.Warn = cloneThresholdRange(datum.Warn)
		item.Crit = cloneThresholdRange(datum.Crit)
		out = append(out, item)
	}
	return out
}

func cloneFloatPtr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneThresholdRange(r *output.ThresholdRange) *output.ThresholdRange {
	if r == nil {
		return nil
	}
	return &output.ThresholdRange{
		Raw:       r.Raw,
		Inclusive: r.Inclusive,
		Low:       cloneFloatPtr(r.Low),
		High:      cloneFloatPtr(r.High),
	}
}
