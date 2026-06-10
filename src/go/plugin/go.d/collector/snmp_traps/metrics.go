// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type trapErrors struct {
	unknownOID         uint64
	decodeFailed       uint64
	templateUnresolved uint64
	malformedPDU       uint64
	droppedAllowlist   uint64
	rateLimited        uint64
	authFailures       uint64
	usmFailures        uint64
	unknownEngineID    uint64
	informResponseFail uint64
	binaryEncoded      uint64
	profileLoadFailed  uint64
	journalWriteFailed uint64
	otlpExportFailed   uint64
	listenerReadFailed uint64
}

type trapEvents struct {
	stateChange  uint64
	configChange uint64
	security     uint64
	auth         uint64
	license      uint64
	mobility     uint64
	diagnostic   uint64
	unknown      uint64
}

type trapSeverities struct {
	emerg   uint64
	alert   uint64
	crit    uint64
	err     uint64
	warning uint64
	notice  uint64
	info    uint64
	debug   uint64
}

type trapDedupMetrics struct {
	enabled    atomic.Bool
	suppressed uint64
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
		atomic.AddUint64(&m.events.stateChange, 1)
	case "config_change":
		atomic.AddUint64(&m.events.configChange, 1)
	case "security":
		atomic.AddUint64(&m.events.security, 1)
	case "auth":
		atomic.AddUint64(&m.events.auth, 1)
	case "license":
		atomic.AddUint64(&m.events.license, 1)
	case "mobility":
		atomic.AddUint64(&m.events.mobility, 1)
	case "diagnostic":
		atomic.AddUint64(&m.events.diagnostic, 1)
	default:
		atomic.AddUint64(&m.events.unknown, 1)
	}
}

func (m *perJobMetrics) incSeverity(severity Severity) {
	switch severity {
	case "emerg":
		atomic.AddUint64(&m.severities.emerg, 1)
	case "alert":
		atomic.AddUint64(&m.severities.alert, 1)
	case "crit":
		atomic.AddUint64(&m.severities.crit, 1)
	case "err":
		atomic.AddUint64(&m.severities.err, 1)
	case "warning":
		atomic.AddUint64(&m.severities.warning, 1)
	case "notice":
		atomic.AddUint64(&m.severities.notice, 1)
	case "info":
		atomic.AddUint64(&m.severities.info, 1)
	case "debug":
		atomic.AddUint64(&m.severities.debug, 1)
	default:
		atomic.AddUint64(&m.severities.notice, 1)
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
		atomic.AddUint64(&m.errors.unknownOID, n)
	case "decode_failed":
		atomic.AddUint64(&m.errors.decodeFailed, n)
	case "template_unresolved":
		atomic.AddUint64(&m.errors.templateUnresolved, n)
	case "malformed_pdu":
		atomic.AddUint64(&m.errors.malformedPDU, n)
	case "dropped_allowlist":
		atomic.AddUint64(&m.errors.droppedAllowlist, n)
	case "rate_limited":
		atomic.AddUint64(&m.errors.rateLimited, n)
	case "auth_failures":
		atomic.AddUint64(&m.errors.authFailures, n)
	case "usm_failures":
		atomic.AddUint64(&m.errors.usmFailures, n)
	case "unknown_engine_id":
		atomic.AddUint64(&m.errors.unknownEngineID, n)
	case "inform_response_failed":
		atomic.AddUint64(&m.errors.informResponseFail, n)
	case "binary_encoded":
		atomic.AddUint64(&m.errors.binaryEncoded, n)
	case "profile_load_failed":
		atomic.AddUint64(&m.errors.profileLoadFailed, n)
	case "journal_write_failed":
		atomic.AddUint64(&m.errors.journalWriteFailed, n)
	case "otlp_export_failed":
		atomic.AddUint64(&m.errors.otlpExportFailed, n)
	case "listener_read_failed":
		atomic.AddUint64(&m.errors.listenerReadFailed, n)
	}
}

func (m *perJobMetrics) setBinaryEncoded(v uint64) {
	// Binary encoded fields are the writer's absolute cumulative total.
	atomic.StoreUint64(&m.errors.binaryEncoded, v)
}

func (m *perJobMetrics) setDedupEnabled(enabled bool) {
	m.dedup.enabled.Store(enabled)
}

func (m *perJobMetrics) incDedupSuppressed() {
	atomic.AddUint64(&m.dedup.suppressed, 1)
}

func incAllJobsProfileLoadFailed() {
	globalMetrics.mu.Lock()
	defer globalMetrics.mu.Unlock()
	for _, m := range globalMetrics.jobs {
		atomic.AddUint64(&m.errors.profileLoadFailed, 1)
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

	meter.Counter("snmp_trap_events_state_change").ObserveTotal(float64(atomic.LoadUint64(&m.events.stateChange)))
	meter.Counter("snmp_trap_events_config_change").ObserveTotal(float64(atomic.LoadUint64(&m.events.configChange)))
	meter.Counter("snmp_trap_events_security").ObserveTotal(float64(atomic.LoadUint64(&m.events.security)))
	meter.Counter("snmp_trap_events_auth").ObserveTotal(float64(atomic.LoadUint64(&m.events.auth)))
	meter.Counter("snmp_trap_events_license").ObserveTotal(float64(atomic.LoadUint64(&m.events.license)))
	meter.Counter("snmp_trap_events_mobility").ObserveTotal(float64(atomic.LoadUint64(&m.events.mobility)))
	meter.Counter("snmp_trap_events_diagnostic").ObserveTotal(float64(atomic.LoadUint64(&m.events.diagnostic)))
	meter.Counter("snmp_trap_events_unknown").ObserveTotal(float64(atomic.LoadUint64(&m.events.unknown)))
}

func collectSeverities(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})

	meter.Counter("snmp_trap_severity_emerg").ObserveTotal(float64(atomic.LoadUint64(&m.severities.emerg)))
	meter.Counter("snmp_trap_severity_alert").ObserveTotal(float64(atomic.LoadUint64(&m.severities.alert)))
	meter.Counter("snmp_trap_severity_crit").ObserveTotal(float64(atomic.LoadUint64(&m.severities.crit)))
	meter.Counter("snmp_trap_severity_err").ObserveTotal(float64(atomic.LoadUint64(&m.severities.err)))
	meter.Counter("snmp_trap_severity_warning").ObserveTotal(float64(atomic.LoadUint64(&m.severities.warning)))
	meter.Counter("snmp_trap_severity_notice").ObserveTotal(float64(atomic.LoadUint64(&m.severities.notice)))
	meter.Counter("snmp_trap_severity_info").ObserveTotal(float64(atomic.LoadUint64(&m.severities.info)))
	meter.Counter("snmp_trap_severity_debug").ObserveTotal(float64(atomic.LoadUint64(&m.severities.debug)))
}

func collectErrors(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})

	meter.Counter("snmp_trap_errors_unknown_oid").ObserveTotal(float64(atomic.LoadUint64(&m.errors.unknownOID)))
	meter.Counter("snmp_trap_errors_decode_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.decodeFailed)))
	meter.Counter("snmp_trap_errors_template_unresolved").ObserveTotal(float64(atomic.LoadUint64(&m.errors.templateUnresolved)))
	meter.Counter("snmp_trap_errors_malformed_pdu").ObserveTotal(float64(atomic.LoadUint64(&m.errors.malformedPDU)))
	meter.Counter("snmp_trap_errors_dropped_allowlist").ObserveTotal(float64(atomic.LoadUint64(&m.errors.droppedAllowlist)))
	meter.Counter("snmp_trap_errors_rate_limited").ObserveTotal(float64(atomic.LoadUint64(&m.errors.rateLimited)))
	meter.Counter("snmp_trap_errors_auth_failures").ObserveTotal(float64(atomic.LoadUint64(&m.errors.authFailures)))
	meter.Counter("snmp_trap_errors_usm_failures").ObserveTotal(float64(atomic.LoadUint64(&m.errors.usmFailures)))
	meter.Counter("snmp_trap_errors_unknown_engine_id").ObserveTotal(float64(atomic.LoadUint64(&m.errors.unknownEngineID)))
	meter.Counter("snmp_trap_errors_inform_response_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.informResponseFail)))
	meter.Counter("snmp_trap_errors_binary_encoded").ObserveTotal(float64(atomic.LoadUint64(&m.errors.binaryEncoded)))
	meter.Counter("snmp_trap_errors_profile_load_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.profileLoadFailed)))
	meter.Counter("snmp_trap_errors_journal_write_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.journalWriteFailed)))
	meter.Counter("snmp_trap_errors_otlp_export_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.otlpExportFailed)))
	meter.Counter("snmp_trap_errors_listener_read_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.listenerReadFailed)))
}

func collectDedup(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	if !m.dedup.enabled.Load() {
		return
	}
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})
	meter.Counter("snmp_trap_dedup_suppressed").ObserveTotal(float64(atomic.LoadUint64(&m.dedup.suppressed)))
}
