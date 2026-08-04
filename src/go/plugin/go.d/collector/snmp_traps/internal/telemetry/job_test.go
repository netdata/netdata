// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobCollectsRetainedMetricContract(t *testing.T) {
	job := NewRegistry().Attach("listener", Options{DedupEnabled: true})
	job.PipelineReceived()
	job.PipelineDecoded()
	job.PipelineAccepted()
	job.PipelineCommitted()
	job.PipelineDropped()
	job.PipelineWriteFailed(2)
	job.DedupSuppressed()
	job.Event("security")
	job.Event("not-a-category")
	job.Severity("crit")
	job.Severity("not-a-severity")
	job.Error(ErrorDecodeFailed)
	job.AddError(ErrorOTLPExportFailed, 3)
	job.SetBinaryEncoded(4)

	reader := collectJob(t, job)
	labels := metrix.Labels{"job_name": "listener"}
	want := map[string]float64{
		"snmp_trap_pipeline_received":         1,
		"snmp_trap_pipeline_decoded":          1,
		"snmp_trap_pipeline_accepted":         1,
		"snmp_trap_pipeline_committed":        1,
		"snmp_trap_pipeline_dedup_suppressed": 1,
		"snmp_trap_pipeline_dropped":          1,
		"snmp_trap_pipeline_write_failed":     2,
		"snmp_trap_events_security":           1,
		"snmp_trap_events_unknown":            1,
		"snmp_trap_severity_crit":             1,
		"snmp_trap_severity_notice":           1,
		"snmp_trap_errors_decode_failed":      1,
		"snmp_trap_errors_otlp_export_failed": 3,
		"snmp_trap_errors_binary_encoded":     4,
		"snmp_trap_dedup_suppressed":          1,
	}
	for name, expected := range want {
		value, ok := reader.Value(name, labels)
		assert.Truef(t, ok, "metric %s is missing", name)
		assert.Equalf(t, expected, value, "metric %s", name)
	}
}

func TestJobCollectsExactRetainedMetricSet(t *testing.T) {
	job := NewRegistry().Attach("listener", Options{DedupEnabled: true})
	reader := collectJob(t, job)
	labels := metrix.Labels{"job_name": "listener"}
	want := []string{
		"snmp_trap_pipeline_received",
		"snmp_trap_pipeline_decoded",
		"snmp_trap_pipeline_accepted",
		"snmp_trap_pipeline_committed",
		"snmp_trap_pipeline_dedup_suppressed",
		"snmp_trap_pipeline_dropped",
		"snmp_trap_pipeline_write_failed",
		"snmp_trap_events_state_change",
		"snmp_trap_events_config_change",
		"snmp_trap_events_security",
		"snmp_trap_events_auth",
		"snmp_trap_events_license",
		"snmp_trap_events_mobility",
		"snmp_trap_events_diagnostic",
		"snmp_trap_events_unknown",
		"snmp_trap_severity_emerg",
		"snmp_trap_severity_alert",
		"snmp_trap_severity_crit",
		"snmp_trap_severity_err",
		"snmp_trap_severity_warning",
		"snmp_trap_severity_notice",
		"snmp_trap_severity_info",
		"snmp_trap_severity_debug",
		"snmp_trap_errors_unknown_oid",
		"snmp_trap_errors_decode_failed",
		"snmp_trap_errors_template_unresolved",
		"snmp_trap_errors_malformed_pdu",
		"snmp_trap_errors_dropped_allowlist",
		"snmp_trap_errors_rate_limited",
		"snmp_trap_errors_auth_failures",
		"snmp_trap_errors_usm_failures",
		"snmp_trap_errors_unknown_engine_id",
		"snmp_trap_errors_inform_response_failed",
		"snmp_trap_errors_binary_encoded",
		"snmp_trap_errors_profile_load_failed",
		"snmp_trap_errors_journal_write_failed",
		"snmp_trap_errors_otlp_export_failed",
		"snmp_trap_errors_listener_read_failed",
		"snmp_trap_errors_listener_buffer_degraded",
		"snmp_trap_dedup_suppressed",
	}

	series := make(map[string]int)
	reader.ForEachSeries(func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
		series[name]++
	})
	require.Len(t, series, len(want))
	for _, name := range want {
		_, ok := reader.Value(name, labels)
		assert.Truef(t, ok, "metric %s with job_name label is missing", name)
		assert.Equalf(t, 1, series[name], "metric %s series count", name)
		meta, ok := reader.SeriesMeta(name, labels)
		require.Truef(t, ok, "metric %s metadata is missing", name)
		assert.Equalf(t, metrix.MetricKindCounter, meta.Kind, "metric %s kind", name)
	}
}

func TestJobOmitsDedicatedDedupMetricWhenDisabled(t *testing.T) {
	job := NewRegistry().Attach("listener", Options{})
	job.DedupSuppressed()

	reader := collectJob(t, job)
	labels := metrix.Labels{"job_name": "listener"}
	_, ok := reader.Value("snmp_trap_dedup_suppressed", labels)
	assert.False(t, ok)

	value, ok := reader.Value("snmp_trap_pipeline_dedup_suppressed", labels)
	assert.True(t, ok)
	assert.Equal(t, float64(1), value)
}

func TestJobRecordAndCollectAreConcurrentSafe(t *testing.T) {
	job := NewRegistry().Attach("listener", Options{DedupEnabled: true})
	const iterations = 100

	var wg sync.WaitGroup
	wg.Go(func() {
		for range iterations {
			job.PipelineReceived()
			job.Event("security")
			job.Error(ErrorDecodeFailed)
			job.DedupSuppressed()
		}
	})

	for range iterations {
		_ = collectJob(t, job)
	}
	wg.Wait()

	reader := collectJob(t, job)
	labels := metrix.Labels{"job_name": "listener"}
	value, ok := reader.Value("snmp_trap_pipeline_received", labels)
	require.True(t, ok)
	assert.Equal(t, float64(iterations), value)
}

func collectJob(t testing.TB, job *Job) metrix.Reader {
	t.Helper()
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	job.Collect(store)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())
	return store.Read()
}
