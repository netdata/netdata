// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"context"
	"fmt"
	"strings"
)

// Metric names. These are the selectors charts.yaml binds to, so they are a stable contract.
const (
	metricMemoryErrors        = "memory_errors"
	metricMCEvents            = "mc_events"
	metricAEREvents           = "aer_events"
	metricMCERecords          = "mce_records"
	metricMemoryFailureEvents = "memory_failure_events"
	metricClassEvents         = "class_events"
)

// Canonical severity buckets.
//
// The severity strings come from the rasdaemon database and vary by kernel, EDAC driver and
// rasdaemon version. They are folded onto a fixed set so the summary charts have a stable,
// bounded dimension list that can be emitted even when a machine has recorded nothing at all.
const (
	sevCorrected   = "corrected"
	sevUncorrected = "uncorrected"
	sevFatal       = "fatal"
	sevOther       = "other"
)

// memorySeverities / aerSeverities are emitted on EVERY cycle, including as zeros.
//
// This is deliberate. A healthy machine produces an entirely empty `ras-mc-ctl --summary`, and
// if the collector wrote nothing then no charts would exist at all: an operator could not tell a
// healthy machine from a broken collector, and health alerts cannot attach to a chart that has
// never appeared. Emitting a steady zero makes "no errors" an observable, alertable state and
// makes the first error a visible transition rather than a chart springing into existence.
var (
	memorySeverities = []string{sevCorrected, sevUncorrected, sevFatal, sevOther}
	aerSeverities    = []string{sevCorrected, sevUncorrected, sevFatal, sevOther}
	// allClasses is the fixed set of aggregate error classes, also always emitted.
	allClasses = []string{classExtlog, classDevlink, classDisk, classCXL, classARM, classSignal}
)

// canonicalSeverity folds a rasdaemon severity string onto a fixed bucket.
func canonicalSeverity(s string) string {
	switch t := strings.ToLower(strings.TrimSpace(s)); {
	case strings.Contains(t, "uncorrected"), strings.Contains(t, "uncorrectable"),
		strings.Contains(t, "non-fatal"), strings.Contains(t, "nonfatal"):
		// Checked before "corrected" because "uncorrected" contains it as a substring.
		return sevUncorrected
	case strings.Contains(t, "fatal"):
		return sevFatal
	case strings.Contains(t, "corrected"), strings.Contains(t, "correctable"):
		return sevCorrected
	default:
		return sevOther
	}
}

func (c *Collector) collect(ctx context.Context) error {
	sm, err := c.fetchSummary()
	if err != nil {
		return err
	}
	// Abort the cycle rather than commit a partial frame if the runtime is shutting down.
	if err := ctx.Err(); err != nil {
		return err
	}

	c.writeSummary(sm)
	return nil
}

func (c *Collector) fetchSummary() (*summary, error) {
	if c.exec == nil {
		return nil, fmt.Errorf("ras-mc-ctl client not initialized")
	}

	out, err := c.exec.summary()
	if err != nil {
		return nil, fmt.Errorf("running ras-mc-ctl --summary: %w", err)
	}

	sm, err := parseSummary(out)
	if err != nil {
		return nil, fmt.Errorf("parsing ras-mc-ctl --summary: %w", err)
	}
	return sm, nil
}

// writeSummary maps the parsed summary onto metrix instruments.
//
// Every value is written with ObserveTotal because ras-mc-ctl reports cumulative counts over an
// append-only database. The charts declare `algorithm: incremental`, so netdata derives the rate
// and the series stays correct across collector and agent restarts. Computing deltas here instead
// would re-emit the whole history as a spike after any restart.
func (c *Collector) writeSummary(sm *summary) {
	meter := c.store.Write().SnapshotMeter("")

	// ── Always-present summary series (zero-filled) ──────────────────────────
	memTotals := make(map[string]int64, len(memorySeverities))
	for _, s := range memorySeverities {
		memTotals[s] = 0
	}
	for _, e := range sm.memory {
		memTotals[canonicalSeverity(e.errType)] += e.count
	}
	memErrs := meter.Vec("severity").Counter(metricMemoryErrors)
	for _, s := range memorySeverities {
		memErrs.WithLabelValues(s).ObserveTotal(float64(memTotals[s]))
	}

	aerTotals := make(map[string]int64, len(aerSeverities))
	for _, s := range aerSeverities {
		aerTotals[s] = 0
	}
	for _, e := range sm.aer {
		aerTotals[canonicalSeverity(e.errType)] += e.count
	}
	aer := meter.Vec("severity").Counter(metricAEREvents)
	for _, s := range aerSeverities {
		aer.WithLabelValues(s).ObserveTotal(float64(aerTotals[s]))
	}

	// MCE and memory-failure messages are free-form vendor text with unbounded cardinality,
	// so they are reported as a single total each rather than as labeled series.
	var mceTotal, memFailTotal int64
	for _, e := range sm.mce {
		mceTotal += e.count
	}
	for _, e := range sm.memoryFailure {
		memFailTotal += e.count
	}
	meter.Counter(metricMCERecords).ObserveTotal(float64(mceTotal))
	meter.Counter(metricMemoryFailureEvents).ObserveTotal(float64(memFailTotal))

	class := meter.Vec("class").Counter(metricClassEvents)
	for _, name := range allClasses {
		class.WithLabelValues(name).ObserveTotal(float64(sm.classes[name]))
	}

	// ── Per-DIMM detail (sparse by design) ───────────────────────────────────
	// Instances materialize when a DIMM first records an error. A machine with 4 healthy sticks
	// should not carry 4 permanently-zero per-DIMM charts; the always-present summary above is
	// what makes "healthy" observable.
	mc := meter.Vec("dimm", "severity").Counter(metricMCEvents)
	for _, e := range sm.memory {
		mc.WithLabelValues(e.dimm, canonicalSeverity(e.errType)).ObserveTotal(float64(e.count))
	}
}
