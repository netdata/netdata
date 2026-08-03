// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingOutputStarter struct {
	name  string
	order *[]string
	err   error
}

type cleanupOrderWriter struct {
	order   []string
	entries []*TrapEntry
}

func (w *cleanupOrderWriter) Write(entry *TrapEntry) error {
	w.entries = append(w.entries, entry)
	if entry.ReportType == ReportTypeDedupSummary {
		w.order = append(w.order, "summary")
	}
	return nil
}

func (*cleanupOrderWriter) Flush() error { return nil }

func (w *cleanupOrderWriter) Close() error {
	w.order = append(w.order, "close")
	return nil
}

func (s recordingOutputStarter) Start() error {
	*s.order = append(*s.order, s.name)
	return s.err
}

func TestStartOutputBackendsStartsOTLPBeforeJournal(t *testing.T) {
	var order []string
	require.NoError(t, startOutputBackends(
		recordingOutputStarter{name: "otlp", order: &order},
		recordingOutputStarter{name: "journal", order: &order},
	))
	assert.Equal(t, []string{"otlp", "journal"}, order)
}

func TestStartOutputBackendsStopsAfterFailure(t *testing.T) {
	var order []string
	wantErr := errors.New("start failed")
	err := startOutputBackends(
		recordingOutputStarter{name: "otlp", order: &order, err: wantErr},
		recordingOutputStarter{name: "journal", order: &order},
	)
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, []string{"otlp"}, order)
}

func TestHandleOutputOutcomePreservesBackendAuthorityMetrics(t *testing.T) {
	collector := &Collector{Config: Config{Name: "local"}}
	metrics := newTestJobTelemetry(t, "local", false)

	collector.handleOutputOutcome(metrics, output.Outcome{
		Backend: output.BackendOTLP, Stage: output.StageEnqueue, FailedEntries: 2,
	})
	assertJobMetric(t, metrics, "local", "snmp_trap_errors_otlp_export_failed", 2)
	assertJobMetric(t, metrics, "local", "snmp_trap_pipeline_write_failed", 0)

	collector.handleOutputOutcome(metrics, output.Outcome{
		Backend: output.BackendOTLP, Stage: output.StageExport, FailedEntries: 3, Authoritative: true,
	})
	assertJobMetric(t, metrics, "local", "snmp_trap_errors_otlp_export_failed", 5)
	assertJobMetric(t, metrics, "local", "snmp_trap_pipeline_write_failed", 3)
}

func TestCollectorCleanupWritesFinalDedupSummaryBeforeClosingOutput(t *testing.T) {
	const jobName = "cleanup-order"
	policy, err := dedup.Normalize(dedup.Config{Enabled: true, WindowSec: 3600})
	require.NoError(t, err)
	metrics := newTestJobTelemetry(t, jobName, true)
	writer := &cleanupOrderWriter{}
	d := newJobDeduper(jobName, policy, nil, writer, metrics, trapWriteFailureJournal, nil)
	d.Start()
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, decision := d.Admit(entry, nil)
	require.Equal(t, dedup.DecisionAdmit, decision)
	_, decision = d.Admit(entry, nil)
	require.Equal(t, dedup.DecisionSuppress, decision)

	c := &Collector{
		Config:     Config{Name: jobName},
		trapWriter: writer,
		telemetry:  metrics,
		deduper:    d,
	}
	c.Cleanup(context.Background())

	assert.Equal(t, []string{"summary", "close"}, writer.order)
	require.Len(t, writer.entries, 1)
	summary := writer.entries[0]
	assert.Equal(t, jobName, summary.JobName)
	assert.Equal(t, ReportTypeDedupSummary, summary.ReportType)
	assert.Equal(t, Severity("info"), summary.Severity)
	assert.Positive(t, summary.ReceivedRealtimeUsec)
	assert.Zero(t, summary.ReceivedMonotonicUsec)
	require.NotNil(t, summary.SummaryCounts)
	assert.Equal(t, int64(1), summary.SummaryCounts.TotalSuppressed)
	assert.Equal(t, int64(1), summary.SummaryCounts.Fingerprints)
	assert.Equal(t, int64(3600), summary.SummaryCounts.PeriodSec)
}

func TestFinalDedupSummaryWriteFailureOnlyRecordsBackendError(t *testing.T) {
	const jobName = "summary-write-failure"
	policy, err := dedup.Normalize(dedup.Config{Enabled: true, WindowSec: 3600})
	require.NoError(t, err)
	metrics := newTestJobTelemetry(t, jobName, true)
	writer := &mockTrapWriter{err: errors.New("write failed")}
	d := newJobDeduper(jobName, policy, nil, writer, metrics, trapWriteFailureJournal, nil)
	d.Start()
	entry := &model.TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	d.Admit(entry, nil)
	d.Admit(entry, nil)

	c := &Collector{Config: Config{Name: jobName}, trapWriter: writer, telemetry: metrics, deduper: d}
	c.Cleanup(context.Background())

	assertJobMetric(t, metrics, jobName, "snmp_trap_errors_journal_write_failed", 1)
	assertJobMetric(t, metrics, jobName, "snmp_trap_pipeline_write_failed", 0)
	assertJobMetric(t, metrics, jobName, "snmp_trap_pipeline_committed", 0)
	assertJobMetric(t, metrics, jobName, "snmp_trap_events_unknown", 0)
	assertJobMetric(t, metrics, jobName, "snmp_trap_severity_info", 0)
}
