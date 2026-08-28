// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var stageStates = []string{string(stateOK), string(stateWaiting), string(stateFailed), string(stateSkipped)}

func (c *Collector) writeMetrics(results stageResults) {
	meter := c.store.Write().SnapshotMeter("")
	duration := meter.Gauge("stage_duration_ms")
	latencyExceeded := meter.Gauge("stage_latency_exceeded")
	attempts := meter.Counter("stage_attempts_total")
	retries := meter.Counter("stage_retries_total")
	operations := meter.Counter("stage_operations_total")
	failures := meter.Counter("stage_failures_total")

	for _, stage := range stageOrder {
		result := results[stage]
		c.stats[stage].add(result)
		counters := c.stats[stage]
		labels := meter.LabelSet(
			metrix.Label{Key: "stage", Value: string(stage)},
			metrix.Label{Key: "reason", Value: result.reason},
		)

		meter.WithLabels(
			metrix.Label{Key: "stage", Value: string(stage)},
			metrix.Label{Key: "reason", Value: result.reason},
		).StateSet(
			"stage_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(stageStates...),
		).Enable(string(result.state))
		duration.Observe(metrix.SampleValue(float64(result.duration.Nanoseconds())/1e6), labels)

		exceeded := int64(0)
		if result.state == stateOK && c.LatencyThresholdMS > 0 &&
			result.duration >= time.Duration(c.LatencyThresholdMS)*time.Millisecond {
			exceeded = 1
		}
		latencyExceeded.Observe(metrix.SampleValue(exceeded), labels)
		// Cumulative request counters must not use the mutable current-result reason
		// as series identity; otherwise each reason starts a new zero-based counter.
		stageOnly := meter.LabelSet(metrix.Label{Key: "stage", Value: string(stage)})
		operations.ObserveTotal(metrix.SampleValue(counters.operations), stageOnly)
		attempts.ObserveTotal(metrix.SampleValue(counters.attempts), stageOnly)
		retries.ObserveTotal(metrix.SampleValue(counters.retries), stageOnly)
		failures.ObserveTotal(metrix.SampleValue(counters.failures), stageOnly)
	}
}
