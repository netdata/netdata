// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const (
	defaultPipelineMetricMaxSources        = 2000
	defaultPipelineMetricExpireAfterCycles = 60
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

type trapPipelineMetrics struct {
	received        atomic.Uint64
	decoded         atomic.Uint64
	accepted        atomic.Uint64
	committed       atomic.Uint64
	dedupSuppressed atomic.Uint64
	dropped         atomic.Uint64
	writeFailed     atomic.Uint64
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
	pipeline   trapPipelineMetrics

	sourceMu           sync.Mutex
	sourceCollectCycle uint64
	sourceHashSalt     string
	sources            map[trapMetricSourceIdentityKey]*perSourceMetrics
	sourceRoutes       map[string]string
	sourceRouteSeen    map[string]time.Time
	sourceDiagnostics  trapSourceDiagnostics
}

type trapSourceDiagnostics struct {
	vnode             uint64
	fallback          uint64
	ambiguous         uint64
	attributionFailed uint64
	overflowDropped   uint64
	sourceTransitions uint64
}

type perSourceMetrics struct {
	key       trapMetricSourceIdentityKey
	scope     metrix.HostScope
	labels    []metrix.Label
	lastSeen  time.Time
	lastCycle uint64

	pipeline trapSourcePipeline
	errors   trapSourceErrors
}

type trapSourcePipeline struct {
	accepted        uint64
	committed       uint64
	dedupSuppressed uint64
	writeFailed     uint64
}

type trapSourceErrors struct {
	unknownOID         uint64
	templateUnresolved uint64
	profileLoadFailed  uint64
	journalWriteFailed uint64
	otlpExportFailed   uint64
}

type trapSourceMetricsSnapshot struct {
	scope              metrix.HostScope
	labels             []metrix.Label
	pipeline           trapSourcePipeline
	errors             trapSourceErrors
	lastSeenSecondsAgo float64
}

type trapSourceDiagnosticsSnapshot struct {
	activeSources int
	diagnostics   trapSourceDiagnostics
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

func (m *perJobMetrics) incPipelineReceived() {
	m.pipeline.received.Add(1)
}

func (m *perJobMetrics) incPipelineDecoded() {
	m.pipeline.decoded.Add(1)
}

func (m *perJobMetrics) incPipelineAccepted() {
	m.pipeline.accepted.Add(1)
}

func (m *perJobMetrics) incPipelineCommitted() {
	m.pipeline.committed.Add(1)
}

func (m *perJobMetrics) incPipelineDedupSuppressed() {
	m.pipeline.dedupSuppressed.Add(1)
}

func (m *perJobMetrics) incPipelineDropped() {
	m.pipeline.dropped.Add(1)
}

func (m *perJobMetrics) incPipelineWriteFailed() {
	m.pipeline.writeFailed.Add(1)
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

func (m *perJobMetrics) recordSourceAccepted(entry *TrapEntry) {
	if m == nil {
		return
	}
	m.incPipelineAccepted()
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	src := m.sourceForEntryLocked(entry, time.Now(), true)
	if src == nil {
		return
	}
	src.pipeline.accepted++
}

func (m *perJobMetrics) recordSourceCommitted(entry *TrapEntry) {
	if m == nil {
		return
	}
	m.incPipelineCommitted()
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	src := m.sourceForEntryLocked(entry, time.Now(), false)
	if src == nil {
		return
	}
	src.pipeline.committed++
}

func (m *perJobMetrics) recordSourceError(entry *TrapEntry, dim string) {
	if m == nil {
		return
	}
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	src := m.sourceForEntryLocked(entry, time.Now(), false)
	if src == nil {
		return
	}
	src.incError(dim)
}

func (m *perJobMetrics) recordSourceDedupSuppressed(entry *TrapEntry) {
	if m == nil {
		return
	}
	m.incPipelineDedupSuppressed()
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	src := m.sourceForEntryLocked(entry, time.Now(), false)
	if src == nil {
		return
	}
	src.pipeline.dedupSuppressed++
}

func (m *perJobMetrics) recordWriteFailure(entry *TrapEntry, dim string) {
	if m == nil {
		return
	}
	m.incPipelineWriteFailed()
	m.sourceMu.Lock()
	defer m.sourceMu.Unlock()
	src := m.sourceForEntryLocked(entry, time.Now(), false)
	if src == nil {
		return
	}
	src.pipeline.writeFailed++
	src.incError(dim)
}

func (m *perJobMetrics) initSourceMetricsLocked() {
	if m.sourceHashSalt == "" {
		m.sourceHashSalt = profileMetricSourceHashSalt()
	}
	if m.sources == nil {
		m.sources = make(map[trapMetricSourceIdentityKey]*perSourceMetrics)
	}
	if m.sourceRoutes == nil {
		m.sourceRoutes = make(map[string]string)
	}
	if m.sourceRouteSeen == nil {
		m.sourceRouteSeen = make(map[string]time.Time)
	}
}

func (m *perJobMetrics) sourceForEntryLocked(entry *TrapEntry, now time.Time, countDiagnostics bool) *perSourceMetrics {
	if entry == nil {
		if countDiagnostics {
			m.sourceDiagnostics.attributionFailed++
		}
		return nil
	}
	m.initSourceMetricsLocked()
	identity := ProfileMetricIdentityConfig{
		Device:           profileMetricIdentitySource,
		UnresolvedSource: profileMetricUnresolvedSourceLabel,
		SourceIDPrivacy:  profileMetricSourceIDHash,
	}
	source, ok := resolveTrapMetricSourceIdentity(entry, entry.JobName, identity, m.sourceHashSalt)
	if !ok {
		if countDiagnostics {
			m.sourceDiagnostics.attributionFailed++
		}
		return nil
	}
	if countDiagnostics {
		if source.key.sourceKind == "vnode" {
			m.sourceDiagnostics.vnode++
		} else {
			m.sourceDiagnostics.fallback++
		}
		if trapEntryHasAmbiguousSourceEvidence(entry) {
			m.sourceDiagnostics.ambiguous++
		}
		m.noteSourceRouteTransitionLocked(source.rawRouteKey, source.routeKey, now)
	}
	src, ok := m.sources[source.key]
	if !ok {
		if len(m.sources) >= defaultPipelineMetricMaxSources {
			if countDiagnostics {
				m.sourceDiagnostics.overflowDropped++
			}
			return nil
		}
		src = &perSourceMetrics{key: source.key}
		m.sources[source.key] = src
	}
	src.scope = source.scope
	src.labels = slices.Clone(source.labels)
	src.lastSeen = now
	src.lastCycle = m.sourceCollectCycle
	return src
}

func trapEntryHasAmbiguousSourceEvidence(entry *TrapEntry) bool {
	if entry == nil || entry.Enrichment == nil {
		return false
	}
	if src := entry.Enrichment.Source; src != nil && len(src.RejectedCandidates) > 0 {
		return true
	}
	return trapLookupIsAmbiguous(entry.Enrichment.Registry) || trapLookupIsAmbiguous(entry.Enrichment.Topology)
}

func trapLookupIsAmbiguous(lookup *TrapEnrichmentLookup) bool {
	if lookup == nil {
		return false
	}
	switch lookup.Status {
	case "ambiguous", "conflict":
		return true
	default:
		return lookup.Reason == "ambiguous_source" || lookup.Reason == "vnode_mismatch"
	}
}

func (m *perJobMetrics) noteSourceRouteTransitionLocked(rawRouteKey, routeKey string, now time.Time) {
	if rawRouteKey == "" || routeKey == "" {
		return
	}
	if previous := m.sourceRoutes[rawRouteKey]; previous != "" && previous != routeKey {
		m.sourceDiagnostics.sourceTransitions++
	}
	if _, ok := m.sourceRoutes[rawRouteKey]; !ok && len(m.sourceRoutes) >= defaultPipelineMetricMaxSources {
		m.pruneSourceRoutesLocked(1)
	}
	m.sourceRoutes[rawRouteKey] = routeKey
	m.sourceRouteSeen[rawRouteKey] = now
}

func (m *perJobMetrics) pruneSourceRoutesLocked(need int) {
	for need > 0 && len(m.sourceRoutes) >= defaultPipelineMetricMaxSources {
		var oldestKey string
		var oldestSeen time.Time
		for key := range m.sourceRoutes {
			seen := m.sourceRouteSeen[key]
			if oldestKey == "" || seen.Before(oldestSeen) || (seen.Equal(oldestSeen) && key < oldestKey) {
				oldestKey = key
				oldestSeen = seen
			}
		}
		if oldestKey == "" {
			return
		}
		delete(m.sourceRoutes, oldestKey)
		delete(m.sourceRouteSeen, oldestKey)
		need--
	}
}

func (s *perSourceMetrics) incError(dim string) {
	switch dim {
	case "unknown_oid":
		s.errors.unknownOID++
	case "template_unresolved":
		s.errors.templateUnresolved++
	case "profile_load_failed":
		s.errors.profileLoadFailed++
	case "journal_write_failed":
		s.errors.journalWriteFailed++
	case "otlp_export_failed":
		s.errors.otlpExportFailed++
	}
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
	collectPipeline(store, jobName, m)
	collectEvents(store, jobName, m)
	collectSeverities(store, jobName, m)
	collectErrors(store, jobName, m)
	collectDedup(store, jobName, m)
	collectSourceMetrics(store, jobName, m)
}

func collectPipeline(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})

	meter.Counter("snmp_trap_pipeline_received").ObserveTotal(float64(m.pipeline.received.Load()))
	meter.Counter("snmp_trap_pipeline_decoded").ObserveTotal(float64(m.pipeline.decoded.Load()))
	meter.Counter("snmp_trap_pipeline_accepted").ObserveTotal(float64(m.pipeline.accepted.Load()))
	meter.Counter("snmp_trap_pipeline_committed").ObserveTotal(float64(m.pipeline.committed.Load()))
	meter.Counter("snmp_trap_pipeline_dedup_suppressed").ObserveTotal(float64(m.pipeline.dedupSuppressed.Load()))
	meter.Counter("snmp_trap_pipeline_dropped").ObserveTotal(float64(m.pipeline.dropped.Load()))
	meter.Counter("snmp_trap_pipeline_write_failed").ObserveTotal(float64(m.pipeline.writeFailed.Load()))
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

func collectSourceMetrics(store metrix.CollectorStore, jobName string, m *perJobMetrics) {
	now := time.Now()
	m.sourceMu.Lock()
	m.sourceCollectCycle++
	m.sweepSourceMetricsLocked()
	snapshots := make([]trapSourceMetricsSnapshot, 0, len(m.sources))
	for _, src := range m.sources {
		age := now.Sub(src.lastSeen).Seconds()
		if src.lastSeen.IsZero() || age < 0 {
			age = 0
		}
		snapshots = append(snapshots, trapSourceMetricsSnapshot{
			scope:              src.scope,
			labels:             slices.Clone(src.labels),
			pipeline:           src.pipeline,
			errors:             src.errors,
			lastSeenSecondsAgo: age,
		})
	}
	diag := trapSourceDiagnosticsSnapshot{
		activeSources: len(m.sources),
		diagnostics:   m.sourceDiagnostics,
	}
	m.sourceMu.Unlock()

	for _, snapshot := range snapshots {
		collectSourceSnapshot(store, snapshot)
	}
	collectSourceDiagnostics(store, jobName, diag)
}

func (m *perJobMetrics) sweepSourceMetricsLocked() {
	if defaultPipelineMetricExpireAfterCycles <= 0 {
		return
	}
	for key, src := range m.sources {
		if m.sourceCollectCycle-src.lastCycle > uint64(defaultPipelineMetricExpireAfterCycles) {
			delete(m.sources, key)
		}
	}
}

func collectSourceSnapshot(store metrix.CollectorStore, s trapSourceMetricsSnapshot) {
	meter := store.Write().SnapshotMeter("").WithHostScope(s.scope).WithLabels(s.labels...)

	meter.Counter("snmp_trap_source_pipeline_accepted").ObserveTotal(float64(s.pipeline.accepted))
	meter.Counter("snmp_trap_source_pipeline_committed").ObserveTotal(float64(s.pipeline.committed))
	meter.Counter("snmp_trap_source_pipeline_dedup_suppressed").ObserveTotal(float64(s.pipeline.dedupSuppressed))
	meter.Counter("snmp_trap_source_pipeline_write_failed").ObserveTotal(float64(s.pipeline.writeFailed))

	meter.Counter("snmp_trap_source_errors_unknown_oid").ObserveTotal(float64(s.errors.unknownOID))
	meter.Counter("snmp_trap_source_errors_template_unresolved").ObserveTotal(float64(s.errors.templateUnresolved))
	meter.Counter("snmp_trap_source_errors_profile_load_failed").ObserveTotal(float64(s.errors.profileLoadFailed))
	meter.Counter("snmp_trap_source_errors_journal_write_failed").ObserveTotal(float64(s.errors.journalWriteFailed))
	meter.Counter("snmp_trap_source_errors_otlp_export_failed").ObserveTotal(float64(s.errors.otlpExportFailed))

	meter.Gauge("snmp_trap_source_last_seen_seconds").Observe(metrix.SampleValue(s.lastSeenSecondsAgo))
}

func collectSourceDiagnostics(store metrix.CollectorStore, jobName string, diag trapSourceDiagnosticsSnapshot) {
	meter := store.Write().SnapshotMeter("").WithLabels(metrix.Label{Key: "job_name", Value: jobName})
	meter.Gauge("snmp_trap_sources_active").Observe(metrix.SampleValue(diag.activeSources))
	meter.Counter("snmp_trap_source_attribution_vnode").ObserveTotal(float64(diag.diagnostics.vnode))
	meter.Counter("snmp_trap_source_attribution_fallback").ObserveTotal(float64(diag.diagnostics.fallback))
	meter.Counter("snmp_trap_source_attribution_ambiguous").ObserveTotal(float64(diag.diagnostics.ambiguous))
	meter.Counter("snmp_trap_source_attribution_failed").ObserveTotal(float64(diag.diagnostics.attributionFailed))
	meter.Counter("snmp_trap_source_attribution_overflow_dropped").ObserveTotal(float64(diag.diagnostics.overflowDropped))
	meter.Counter("snmp_trap_source_attribution_source_transitions").ObserveTotal(float64(diag.diagnostics.sourceTransitions))
}
