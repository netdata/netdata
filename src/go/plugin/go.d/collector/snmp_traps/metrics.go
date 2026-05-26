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
	sanitized          uint64
	profileLoadFailed  uint64
	journalWriteFailed uint64
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

type trapMetrics struct {
	mu   sync.Mutex
	jobs map[string]*perJobMetrics
}

type perJobMetrics struct {
	events trapEvents
	errors trapErrors
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

func incTrapError(jobName, dim string) {
	getJobMetrics(jobName).incError(dim)
}

func (c *Collector) incTrapEvents(category Category) {
	c.trapMetrics().incEvent(category)
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

func (m *perJobMetrics) incError(dim string) {
	switch dim {
	case "unknown_oid":
		atomic.AddUint64(&m.errors.unknownOID, 1)
	case "decode_failed":
		atomic.AddUint64(&m.errors.decodeFailed, 1)
	case "template_unresolved":
		atomic.AddUint64(&m.errors.templateUnresolved, 1)
	case "malformed_pdu":
		atomic.AddUint64(&m.errors.malformedPDU, 1)
	case "dropped_allowlist":
		atomic.AddUint64(&m.errors.droppedAllowlist, 1)
	case "rate_limited":
		atomic.AddUint64(&m.errors.rateLimited, 1)
	case "auth_failures":
		atomic.AddUint64(&m.errors.authFailures, 1)
	case "usm_failures":
		atomic.AddUint64(&m.errors.usmFailures, 1)
	case "unknown_engine_id":
		atomic.AddUint64(&m.errors.unknownEngineID, 1)
	case "inform_response_failed":
		atomic.AddUint64(&m.errors.informResponseFail, 1)
	case "sanitized":
		atomic.AddUint64(&m.errors.sanitized, 1)
	case "profile_load_failed":
		atomic.AddUint64(&m.errors.profileLoadFailed, 1)
	case "journal_write_failed":
		atomic.AddUint64(&m.errors.journalWriteFailed, 1)
	}
}

func (m *perJobMetrics) setSanitized(v uint64) {
	// Sanitized fields are the writer's absolute cumulative total.
	atomic.StoreUint64(&m.errors.sanitized, v)
}

func collectMetrics(store metrix.CollectorStore, jobName string) {
	globalMetrics.mu.Lock()
	m, ok := globalMetrics.jobs[jobName]
	globalMetrics.mu.Unlock()
	if !ok {
		return
	}
	collectEvents(store, jobName, m)
	collectErrors(store, jobName, m)
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
	meter.Counter("snmp_trap_errors_sanitized").ObserveTotal(float64(atomic.LoadUint64(&m.errors.sanitized)))
	meter.Counter("snmp_trap_errors_profile_load_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.profileLoadFailed)))
	meter.Counter("snmp_trap_errors_journal_write_failed").ObserveTotal(float64(atomic.LoadUint64(&m.errors.journalWriteFailed)))
}
