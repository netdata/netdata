// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var ownershipStates = []string{string(stateOK), string(stateWaiting), string(stateFailed), string(stateSkipped)}

var multisitePhaseOrder = []multisitePhase{
	multisiteSetup,
	multisiteSourcePut,
	multisiteReplication,
	multisiteSourceDelete,
	multisiteDeleteWait,
	multisiteCleanup,
}

type multisiteResult struct {
	state      stageState
	reason     string
	duration   time.Duration
	attempts   int
	retries    int
	operations int
}

type multisiteCounters struct {
	operations int64
	attempts   int64
	retries    int64
	failures   int64
}

type multisiteStats map[multisitePhase]*multisiteCounters

type multisiteCycle struct {
	phases map[multisitePhase]*multisiteResult

	payloadMismatch  *float64
	replicationLagMS *float64
	phaseFailure     *float64
	rpoExceeded      *float64
	deleteLagMS      *float64
	deleteExceeded   *float64
}

func newMultisiteCycle() *multisiteCycle {
	phases := make(map[multisitePhase]*multisiteResult, len(multisitePhaseOrder))
	for _, phase := range multisitePhaseOrder {
		phases[phase] = &multisiteResult{
			state:  stateSkipped,
			reason: reasonNotRun,
		}
	}
	return &multisiteCycle{
		phases: phases,
	}
}

func newMultisiteStats() multisiteStats {
	stats := make(multisiteStats, len(multisitePhaseOrder))
	for _, phase := range multisitePhaseOrder {
		stats[phase] = &multisiteCounters{}
	}
	return stats
}

func (r *multisiteResult) succeed() {
	r.state = stateOK
	r.reason = reasonOK
}

func (r *multisiteResult) wait(reason string) {
	r.state = stateWaiting
	r.reason = reason
}

func (r *multisiteResult) fail(reason string) {
	r.state = stateFailed
	r.reason = reason
}

func (r *multisiteResult) addOperation(attempts int) {
	r.addOperations(1, attempts)
}

func (r *multisiteResult) addOperations(operations, attempts int) {
	if operations <= 0 || attempts <= 0 {
		return
	}
	r.operations += operations
	r.attempts += attempts
	if attempts > operations {
		r.retries += attempts - operations
	}
}

func (s *multisiteCounters) add(result *multisiteResult) {
	s.operations += int64(result.operations)
	s.attempts += int64(result.attempts)
	s.retries += int64(result.retries)
	if result.state == stateFailed {
		s.failures++
	}
}

func isCategoricalMultisiteFailure(reason string) bool {
	switch reason {
	case reasonRequestFailed, reasonBucketVersioned, reasonInternal, reasonOrphanCleanupPending, reasonRestartAbandoned, reasonStillPresent, reasonTimeout:
		return true
	default:
		return false
	}
}

func (c *Collector) writeMultisiteMetrics(cycle *multisiteCycle) {
	meter := c.store.Write().SnapshotMeter("")
	siteLabels := meter.LabelSet(
		metrix.Label{
			Key:   "source_site",
			Value: c.SourceSite,
		},
		metrix.Label{
			Key:   "destination_site",
			Value: c.Destination.Site,
		},
	)

	duration := meter.Gauge("multisite_phase_duration_ms")
	attempts := meter.Counter("multisite_phase_attempts_total")
	retries := meter.Counter("multisite_phase_retries_total")
	operations := meter.Counter("multisite_phase_operations_total")
	failures := meter.Counter("multisite_phase_failures_total")

	phaseFailure := float64(0)
	for _, phase := range multisitePhaseOrder {
		result := cycle.phases[phase]
		if result.state == stateFailed && isCategoricalMultisiteFailure(result.reason) {
			phaseFailure = 1
		}
	}
	cycle.phaseFailure = &phaseFailure

	for _, phase := range multisitePhaseOrder {
		result := cycle.phases[phase]
		c.multisiteStats[phase].add(result)
		counters := c.multisiteStats[phase]
		labels := []metrix.Label{
			{Key: "source_site", Value: c.SourceSite},
			{Key: "destination_site", Value: c.Destination.Site},
			{Key: "phase", Value: string(phase)},
			{Key: "reason", Value: result.reason},
		}
		labelSet := meter.LabelSet(labels...)
		meter.WithLabels(labels...).StateSet(
			"multisite_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(ownershipStates...),
		).Enable(string(result.state))
		duration.Observe(metrix.SampleValue(float64(result.duration.Nanoseconds())/1e6), labelSet)

		phaseOnly := meter.LabelSet(
			metrix.Label{
				Key:   "source_site",
				Value: c.SourceSite,
			},
			metrix.Label{
				Key:   "destination_site",
				Value: c.Destination.Site,
			},
			metrix.Label{
				Key:   "phase",
				Value: string(phase),
			},
		)
		operations.ObserveTotal(metrix.SampleValue(counters.operations), phaseOnly)
		attempts.ObserveTotal(metrix.SampleValue(counters.attempts), phaseOnly)
		retries.ObserveTotal(metrix.SampleValue(counters.retries), phaseOnly)
		failures.ObserveTotal(metrix.SampleValue(counters.failures), phaseOnly)
	}

	if cycle.phaseFailure != nil {
		meter.Gauge("multisite_phase_failure").Observe(metrix.SampleValue(*cycle.phaseFailure), siteLabels)
	}
	if cycle.payloadMismatch != nil {
		meter.Gauge("multisite_payload_mismatch").Observe(metrix.SampleValue(*cycle.payloadMismatch), siteLabels)
	}
	if cycle.replicationLagMS != nil {
		meter.Gauge("multisite_replication_lag_ms").Observe(metrix.SampleValue(*cycle.replicationLagMS), siteLabels)
	}
	if cycle.rpoExceeded != nil {
		meter.Gauge("multisite_rpo_exceeded").Observe(metrix.SampleValue(*cycle.rpoExceeded), siteLabels)
	}
	if cycle.deleteLagMS != nil {
		meter.Gauge("multisite_delete_lag_ms").Observe(metrix.SampleValue(*cycle.deleteLagMS), siteLabels)
	}
	if cycle.deleteExceeded != nil {
		meter.Gauge("multisite_delete_exceeded").Observe(metrix.SampleValue(*cycle.deleteExceeded), siteLabels)
	}
}
