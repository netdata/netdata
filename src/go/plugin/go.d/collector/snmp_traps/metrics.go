// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type trapErrors struct {
	unknownOID         atomic.Uint64
	decodeFailed       atomic.Uint64
	templateUnresolved atomic.Uint64
	malformedPDU       atomic.Uint64
	droppedAllowlist   atomic.Uint64
	rateLimited        atomic.Uint64
	authFailures       atomic.Uint64
	usmFailures        atomic.Uint64
	unknownEngineID    atomic.Uint64
	informResponseFail atomic.Uint64
	binaryEncoded      atomic.Uint64
	profileLoadFailed  atomic.Uint64
	journalWriteFailed atomic.Uint64
	otlpExportFailed   atomic.Uint64
	listenerReadFailed atomic.Uint64
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
	enabled    atomic.Bool
	suppressed atomic.Uint64
}

type trapMetrics struct {
	mu   sync.Mutex
	jobs map[string]*perJobMetrics
}

type perJobMetrics struct {
	events     trapEvents
	errors     trapErrors
	severities trapSeverities
	dedup      trapDedupMetrics
}

var globalMetrics = &trapMetrics{
	jobs: make(map[string]*perJobMetrics),
}

func getJobMetrics(jobName string) *perJobMetrics {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	m, ok := globalMetrics.jobs[jobName]
	if !ok {
		m = &perJobMetrics{}
		globalMetrics.jobs[jobName] = m
	}
	return m
}

func removeJobMetrics(jobName string) {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	delete(globalMetrics.jobs, jobName)
}

func incTrapEvents(jobName string, category Category) {
	getJobMetrics(jobName).incEvent(category)
}

func incTrapSeverity(jobName string, severity Severity) {
	getJobMetrics(jobName).incSeverity(severity)
}

func incTrapError(jobName, dim string) {
	getJobMetrics(jobName).incError(dim)
}

func (c *Collector) incTrapEvents(category Category) {
	c.trapMetrics().incEvent(category)
}

func (c *Collector) incTrapSeverity(severity Severity) {
	c.trapMetrics().incSeverity(severity)
}

func (c *Collector) incTrapError(dim string) {
	c.trapMetrics().incError(dim)
}

func (c *Collector) trapMetrics() *perJobMetrics {
	if c.metrics != nil {
		return c.metrics
	}
	return getJobMetrics(c.jobName)
}

func (m *perJobMetrics) incEvent(category Category) {
	switch category {
	case "state_change":
		m.events.stateChange.Add(1)
	case "config_change":
		m.events.configChange.Add(1)
	case "security":
		m.events.security.Add(1)
	case "auth":
		m.events.auth.Add(1)
	case "license":
		m.events.license.Add(1)
	case "mobility":
		m.events.mobility.Add(1)
	case "diagnostic":
		m.events.diagnostic.Add(1)
	default:
		m.events.unknown.Add(1)
	}
}

func (m *perJobMetrics) incSeverity(severity Severity) {
	switch severity {
	case "emerg":
		m.severities.emerg.Add(1)
	case "alert":
		m.severities.alert.Add(1)
	case "crit":
		m.severities.crit.Add(1)
	case "err":
		m.severities.err.Add(1)
	case "warning":
		m.severities.warning.Add(1)
	case "notice":
		m.severities.notice.Add(1)
	case "info":
		m.severities.info.Add(1)
	case "debug":
		m.severities.debug.Add(1)
	default:
		m.severities.notice.Add(1)
	}
}

func (m *perJobMetrics) incError(dim string) {
	m.addError(dim, 1)
}

func (m *perJobMetrics) addError(dim string, n uint64) {
	if n == 0 {
		return
	}
	switch dim {
	case "unknown_oid":
		m.errors.unknownOID.Add(n)
	case "decode_failed":
		m.errors.decodeFailed.Add(n)
	case "template_unresolved":
		m.errors.templateUnresolved.Add(n)
	case "malformed_pdu":
		m.errors.malformedPDU.Add(n)
	case "dropped_allowlist":
		m.errors.droppedAllowlist.Add(n)
	case "rate_limited":
		m.errors.rateLimited.Add(n)
	case "auth_failures":
		m.errors.authFailures.Add(n)
	case "usm_failures":
		m.errors.usmFailures.Add(n)
	case "unknown_engine_id":
		m.errors.unknownEngineID.Add(n)
	case "inform_response_failed":
		m.errors.informResponseFail.Add(n)
	case "binary_encoded":
		m.errors.binaryEncoded.Add(n)
	case "profile_load_failed":
		m.errors.profileLoadFailed.Add(n)
	case "journal_write_failed":
		m.errors.journalWriteFailed.Add(n)
	case "otlp_export_failed":
		m.errors.otlpExportFailed.Add(n)
	case "listener_read_failed":
		m.errors.listenerReadFailed.Add(n)
	}
}

func (m *perJobMetrics) setBinaryEncoded(v uint64) {
	// Binary encoded fields are the writer's absolute cumulative total.
	m.errors.binaryEncoded.Store(v)
}

func (m *perJobMetrics) setDedupEnabled(enabled bool) {
	m.dedup.enabled.Store(enabled)
}

func (m *perJobMetrics) incDedupSuppressed() {
	m.dedup.suppressed.Add(1)
}

func incAllJobsProfileLoadFailed() {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	for _, m := range globalMetrics.jobs {
		m.errors.profileLoadFailed.Add(1)
	}
}

func collectMetrics(store metrix.CollectorStore, jobName string) {
	globalMetrics.mu.Lock()
	m, ok := globalMetrics.jobs[jobName]
	globalMetrics.mu.Unlock()
	if !ok {
		return
	}
	collectEvents(store, jobName, m)
	collectSeverities(store, jobName, m)
	collectErrors(store, jobName, m)
	collectDedup(store, jobName, m)
}

func collectEvents(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})

	meter.Counter("snmp_trap_events_state_change").ObserveTotal(float64(m.events.stateChange.Load()))
	meter.Counter("snmp_trap_events_config_change").ObserveTotal(float64(m.events.configChange.Load()))
	meter.Counter("snmp_trap_events_security").ObserveTotal(float64(m.events.security.Load()))
	meter.Counter("snmp_trap_events_auth").ObserveTotal(float64(m.events.auth.Load()))
	meter.Counter("snmp_trap_events_license").ObserveTotal(float64(m.events.license.Load()))
	meter.Counter("snmp_trap_events_mobility").ObserveTotal(float64(m.events.mobility.Load()))
	meter.Counter("snmp_trap_events_diagnostic").ObserveTotal(float64(m.events.diagnostic.Load()))
	meter.Counter("snmp_trap_events_unknown").ObserveTotal(float64(m.events.unknown.Load()))
}

func collectSeverities(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})

	meter.Counter("snmp_trap_severity_emerg").ObserveTotal(float64(m.severities.emerg.Load()))
	meter.Counter("snmp_trap_severity_alert").ObserveTotal(float64(m.severities.alert.Load()))
	meter.Counter("snmp_trap_severity_crit").ObserveTotal(float64(m.severities.crit.Load()))
	meter.Counter("snmp_trap_severity_err").ObserveTotal(float64(m.severities.err.Load()))
	meter.Counter("snmp_trap_severity_warning").ObserveTotal(float64(m.severities.warning.Load()))
	meter.Counter("snmp_trap_severity_notice").ObserveTotal(float64(m.severities.notice.Load()))
	meter.Counter("snmp_trap_severity_info").ObserveTotal(float64(m.severities.info.Load()))
	meter.Counter("snmp_trap_severity_debug").ObserveTotal(float64(m.severities.debug.Load()))
}

func collectErrors(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})

	meter.Counter("snmp_trap_errors_unknown_oid").ObserveTotal(float64(m.errors.unknownOID.Load()))
	meter.Counter("snmp_trap_errors_decode_failed").ObserveTotal(float64(m.errors.decodeFailed.Load()))
	meter.Counter("snmp_trap_errors_template_unresolved").ObserveTotal(float64(m.errors.templateUnresolved.Load()))
	meter.Counter("snmp_trap_errors_malformed_pdu").ObserveTotal(float64(m.errors.malformedPDU.Load()))
	meter.Counter("snmp_trap_errors_dropped_allowlist").ObserveTotal(float64(m.errors.droppedAllowlist.Load()))
	meter.Counter("snmp_trap_errors_rate_limited").ObserveTotal(float64(m.errors.rateLimited.Load()))
	meter.Counter("snmp_trap_errors_auth_failures").ObserveTotal(float64(m.errors.authFailures.Load()))
	meter.Counter("snmp_trap_errors_usm_failures").ObserveTotal(float64(m.errors.usmFailures.Load()))
	meter.Counter("snmp_trap_errors_unknown_engine_id").ObserveTotal(float64(m.errors.unknownEngineID.Load()))
	meter.Counter("snmp_trap_errors_inform_response_failed").ObserveTotal(float64(m.errors.informResponseFail.Load()))
	meter.Counter("snmp_trap_errors_binary_encoded").ObserveTotal(float64(m.errors.binaryEncoded.Load()))
	meter.Counter("snmp_trap_errors_profile_load_failed").ObserveTotal(float64(m.errors.profileLoadFailed.Load()))
	meter.Counter("snmp_trap_errors_journal_write_failed").ObserveTotal(float64(m.errors.journalWriteFailed.Load()))
	meter.Counter("snmp_trap_errors_otlp_export_failed").ObserveTotal(float64(m.errors.otlpExportFailed.Load()))
	meter.Counter("snmp_trap_errors_listener_read_failed").ObserveTotal(float64(m.errors.listenerReadFailed.Load()))
}

func collectDedup(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	if !m.dedup.enabled.Load() {
		return
	}
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})
	meter.Counter("snmp_trap_dedup_suppressed").ObserveTotal(float64(m.dedup.suppressed.Load()))
}
