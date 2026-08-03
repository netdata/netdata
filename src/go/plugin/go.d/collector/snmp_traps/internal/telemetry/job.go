// SPDX-License-Identifier: GPL-3.0-or-later

package telemetry

import (
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type ErrorKind string

const (
	ErrorUnknownOID             ErrorKind = "unknown_oid"
	ErrorDecodeFailed           ErrorKind = "decode_failed"
	ErrorTemplateUnresolved     ErrorKind = "template_unresolved"
	ErrorMalformedPDU           ErrorKind = "malformed_pdu"
	ErrorDroppedAllowlist       ErrorKind = "dropped_allowlist"
	ErrorRateLimited            ErrorKind = "rate_limited"
	ErrorAuthFailures           ErrorKind = "auth_failures"
	ErrorUSMFailures            ErrorKind = "usm_failures"
	ErrorUnknownEngineID        ErrorKind = "unknown_engine_id"
	ErrorInformResponseFailed   ErrorKind = "inform_response_failed"
	ErrorBinaryEncoded          ErrorKind = "binary_encoded"
	ErrorProfileLoadFailed      ErrorKind = "profile_load_failed"
	ErrorJournalWriteFailed     ErrorKind = "journal_write_failed"
	ErrorOTLPExportFailed       ErrorKind = "otlp_export_failed"
	ErrorListenerReadFailed     ErrorKind = "listener_read_failed"
	ErrorListenerBufferDegraded ErrorKind = "listener_buffer_degraded"
)

type trapErrors struct {
	unknownOID             atomic.Uint64
	decodeFailed           atomic.Uint64
	templateUnresolved     atomic.Uint64
	malformedPDU           atomic.Uint64
	droppedAllowlist       atomic.Uint64
	rateLimited            atomic.Uint64
	authFailures           atomic.Uint64
	usmFailures            atomic.Uint64
	unknownEngineID        atomic.Uint64
	informResponseFail     atomic.Uint64
	binaryEncoded          atomic.Uint64
	profileLoadFailed      atomic.Uint64
	journalWriteFailed     atomic.Uint64
	otlpExportFailed       atomic.Uint64
	listenerReadFailed     atomic.Uint64
	listenerBufferDegraded atomic.Uint64
}

type trapEvents struct {
	stateChange  atomic.Uint64
	configChange atomic.Uint64
	security     atomic.Uint64
	auth         atomic.Uint64
	license      atomic.Uint64
	mobility     atomic.Uint64
	diagnostic   atomic.Uint64
	unknown      atomic.Uint64
}

type trapSeverities struct {
	emerg   atomic.Uint64
	alert   atomic.Uint64
	crit    atomic.Uint64
	err     atomic.Uint64
	warning atomic.Uint64
	notice  atomic.Uint64
	info    atomic.Uint64
	debug   atomic.Uint64
}

type trapDedupMetrics struct {
	suppressed atomic.Uint64
}

type trapPipelineMetrics struct {
	received        atomic.Uint64
	decoded         atomic.Uint64
	accepted        atomic.Uint64
	committed       atomic.Uint64
	dedupSuppressed atomic.Uint64
	dropped         atomic.Uint64
	writeFailed     atomic.Uint64
}

type Job struct {
	name         string
	registry     *Registry
	dedupEnabled bool
	events       trapEvents
	errors       trapErrors
	severities   trapSeverities
	dedup        trapDedupMetrics
	pipeline     trapPipelineMetrics
}

func (j *Job) Detach() {
	if j != nil && j.registry != nil {
		j.registry.Detach(j)
	}
}

func (j *Job) Event(category model.Category) {
	switch category {
	case "state_change":
		j.events.stateChange.Add(1)
	case "config_change":
		j.events.configChange.Add(1)
	case "security":
		j.events.security.Add(1)
	case "auth":
		j.events.auth.Add(1)
	case "license":
		j.events.license.Add(1)
	case "mobility":
		j.events.mobility.Add(1)
	case "diagnostic":
		j.events.diagnostic.Add(1)
	default:
		j.events.unknown.Add(1)
	}
}

func (j *Job) Severity(severity model.Severity) {
	switch severity {
	case "emerg":
		j.severities.emerg.Add(1)
	case "alert":
		j.severities.alert.Add(1)
	case "crit":
		j.severities.crit.Add(1)
	case "err":
		j.severities.err.Add(1)
	case "warning":
		j.severities.warning.Add(1)
	case "notice":
		j.severities.notice.Add(1)
	case "info":
		j.severities.info.Add(1)
	case "debug":
		j.severities.debug.Add(1)
	default:
		j.severities.notice.Add(1)
	}
}

func (j *Job) Error(kind ErrorKind) {
	j.AddError(kind, 1)
}

func (j *Job) AddError(kind ErrorKind, n uint64) {
	if n == 0 {
		return
	}
	switch kind {
	case ErrorUnknownOID:
		j.errors.unknownOID.Add(n)
	case ErrorDecodeFailed:
		j.errors.decodeFailed.Add(n)
	case ErrorTemplateUnresolved:
		j.errors.templateUnresolved.Add(n)
	case ErrorMalformedPDU:
		j.errors.malformedPDU.Add(n)
	case ErrorDroppedAllowlist:
		j.errors.droppedAllowlist.Add(n)
	case ErrorRateLimited:
		j.errors.rateLimited.Add(n)
	case ErrorAuthFailures:
		j.errors.authFailures.Add(n)
	case ErrorUSMFailures:
		j.errors.usmFailures.Add(n)
	case ErrorUnknownEngineID:
		j.errors.unknownEngineID.Add(n)
	case ErrorInformResponseFailed:
		j.errors.informResponseFail.Add(n)
	case ErrorBinaryEncoded:
		j.errors.binaryEncoded.Add(n)
	case ErrorProfileLoadFailed:
		j.errors.profileLoadFailed.Add(n)
	case ErrorJournalWriteFailed:
		j.errors.journalWriteFailed.Add(n)
	case ErrorOTLPExportFailed:
		j.errors.otlpExportFailed.Add(n)
	case ErrorListenerReadFailed:
		j.errors.listenerReadFailed.Add(n)
	case ErrorListenerBufferDegraded:
		j.errors.listenerBufferDegraded.Add(n)
	}
}

func (j *Job) PipelineReceived()  { j.pipeline.received.Add(1) }
func (j *Job) PipelineDecoded()   { j.pipeline.decoded.Add(1) }
func (j *Job) PipelineAccepted()  { j.pipeline.accepted.Add(1) }
func (j *Job) PipelineCommitted() { j.pipeline.committed.Add(1) }
func (j *Job) PipelineDropped()   { j.pipeline.dropped.Add(1) }

func (j *Job) PipelineWriteFailed(n uint64) {
	if n > 0 {
		j.pipeline.writeFailed.Add(n)
	}
}

func (j *Job) DedupSuppressed() {
	j.dedup.suppressed.Add(1)
	j.pipeline.dedupSuppressed.Add(1)
}

func (j *Job) SetBinaryEncoded(v uint64) {
	// Binary encoded fields are the writer's absolute cumulative total.
	j.errors.binaryEncoded.Store(v)
}

func (j *Job) Collect(store metrix.CollectorStore) {
	collectPipeline(store, j.name, j)
	collectEvents(store, j.name, j)
	collectSeverities(store, j.name, j)
	collectErrors(store, j.name, j)
	if j.dedupEnabled {
		collectDedup(store, j.name, j)
	}
}

func jobMeter(store metrix.CollectorStore, jobName string) metrix.SnapshotMeter {
	return store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})
}

func collectPipeline(store metrix.CollectorStore, jobName string, j *Job) {
	meter := jobMeter(store, jobName)
	meter.Counter("snmp_trap_pipeline_received").ObserveTotal(float64(j.pipeline.received.Load()))
	meter.Counter("snmp_trap_pipeline_decoded").ObserveTotal(float64(j.pipeline.decoded.Load()))
	meter.Counter("snmp_trap_pipeline_accepted").ObserveTotal(float64(j.pipeline.accepted.Load()))
	meter.Counter("snmp_trap_pipeline_committed").ObserveTotal(float64(j.pipeline.committed.Load()))
	meter.Counter("snmp_trap_pipeline_dedup_suppressed").ObserveTotal(float64(j.pipeline.dedupSuppressed.Load()))
	meter.Counter("snmp_trap_pipeline_dropped").ObserveTotal(float64(j.pipeline.dropped.Load()))
	meter.Counter("snmp_trap_pipeline_write_failed").ObserveTotal(float64(j.pipeline.writeFailed.Load()))
}

func collectEvents(store metrix.CollectorStore, jobName string, j *Job) {
	meter := jobMeter(store, jobName)
	meter.Counter("snmp_trap_events_state_change").ObserveTotal(float64(j.events.stateChange.Load()))
	meter.Counter("snmp_trap_events_config_change").ObserveTotal(float64(j.events.configChange.Load()))
	meter.Counter("snmp_trap_events_security").ObserveTotal(float64(j.events.security.Load()))
	meter.Counter("snmp_trap_events_auth").ObserveTotal(float64(j.events.auth.Load()))
	meter.Counter("snmp_trap_events_license").ObserveTotal(float64(j.events.license.Load()))
	meter.Counter("snmp_trap_events_mobility").ObserveTotal(float64(j.events.mobility.Load()))
	meter.Counter("snmp_trap_events_diagnostic").ObserveTotal(float64(j.events.diagnostic.Load()))
	meter.Counter("snmp_trap_events_unknown").ObserveTotal(float64(j.events.unknown.Load()))
}

func collectSeverities(store metrix.CollectorStore, jobName string, j *Job) {
	meter := jobMeter(store, jobName)
	meter.Counter("snmp_trap_severity_emerg").ObserveTotal(float64(j.severities.emerg.Load()))
	meter.Counter("snmp_trap_severity_alert").ObserveTotal(float64(j.severities.alert.Load()))
	meter.Counter("snmp_trap_severity_crit").ObserveTotal(float64(j.severities.crit.Load()))
	meter.Counter("snmp_trap_severity_err").ObserveTotal(float64(j.severities.err.Load()))
	meter.Counter("snmp_trap_severity_warning").ObserveTotal(float64(j.severities.warning.Load()))
	meter.Counter("snmp_trap_severity_notice").ObserveTotal(float64(j.severities.notice.Load()))
	meter.Counter("snmp_trap_severity_info").ObserveTotal(float64(j.severities.info.Load()))
	meter.Counter("snmp_trap_severity_debug").ObserveTotal(float64(j.severities.debug.Load()))
}

func collectErrors(store metrix.CollectorStore, jobName string, j *Job) {
	meter := jobMeter(store, jobName)
	meter.Counter("snmp_trap_errors_unknown_oid").ObserveTotal(float64(j.errors.unknownOID.Load()))
	meter.Counter("snmp_trap_errors_decode_failed").ObserveTotal(float64(j.errors.decodeFailed.Load()))
	meter.Counter("snmp_trap_errors_template_unresolved").ObserveTotal(float64(j.errors.templateUnresolved.Load()))
	meter.Counter("snmp_trap_errors_malformed_pdu").ObserveTotal(float64(j.errors.malformedPDU.Load()))
	meter.Counter("snmp_trap_errors_dropped_allowlist").ObserveTotal(float64(j.errors.droppedAllowlist.Load()))
	meter.Counter("snmp_trap_errors_rate_limited").ObserveTotal(float64(j.errors.rateLimited.Load()))
	meter.Counter("snmp_trap_errors_auth_failures").ObserveTotal(float64(j.errors.authFailures.Load()))
	meter.Counter("snmp_trap_errors_usm_failures").ObserveTotal(float64(j.errors.usmFailures.Load()))
	meter.Counter("snmp_trap_errors_unknown_engine_id").ObserveTotal(float64(j.errors.unknownEngineID.Load()))
	meter.Counter("snmp_trap_errors_inform_response_failed").ObserveTotal(float64(j.errors.informResponseFail.Load()))
	meter.Counter("snmp_trap_errors_binary_encoded").ObserveTotal(float64(j.errors.binaryEncoded.Load()))
	meter.Counter("snmp_trap_errors_profile_load_failed").ObserveTotal(float64(j.errors.profileLoadFailed.Load()))
	meter.Counter("snmp_trap_errors_journal_write_failed").ObserveTotal(float64(j.errors.journalWriteFailed.Load()))
	meter.Counter("snmp_trap_errors_otlp_export_failed").ObserveTotal(float64(j.errors.otlpExportFailed.Load()))
	meter.Counter("snmp_trap_errors_listener_read_failed").ObserveTotal(float64(j.errors.listenerReadFailed.Load()))
	meter.Counter("snmp_trap_errors_listener_buffer_degraded").ObserveTotal(float64(j.errors.listenerBufferDegraded.Load()))
}

func collectDedup(store metrix.CollectorStore, jobName string, j *Job) {
	jobMeter(store, jobName).
		Counter("snmp_trap_dedup_suppressed").
		ObserveTotal(float64(j.dedup.suppressed.Load()))
}
