// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	_ "embed"
	"errors"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profilemetrics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

const (
	trapWriteFailureJournal = "journal_write_failed"
	trapWriteFailureOTLP    = "otlp_export_failed"

	listenerReadErrorLogEvery     = time.Hour
	listenerReadErrorLogKeyPrefix = "snmp_traps:listener_read_failed:"
	listenerBufferDegradedMetric  = "listener_buffer_degraded"
)

var activeDirectJournalJobs atomic.Int64

func directJournalLogsAvailable() bool {
	return activeDirectJournalJobs.Load() > 0
}

// Register registers the SNMP traps collector with shared SNMP-family enrichment state.
func Register(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle) {
	collectorapi.Register("snmp_traps", newCreator(deviceStore, topologyEnricher))
}

func newCreator(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle) collectorapi.Creator {
	if deviceStore == nil {
		panic("snmp_traps Register requires a non-nil device store")
	}
	if topologyEnricher == nil {
		panic("snmp_traps Register requires a non-nil trap enrichment handle")
	}
	hostIdentity := newHostIdentityService()
	var profileCatalogOnce sync.Once
	var profileCatalog *catalog.Manager
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 {
			profileCatalogOnce.Do(func() {
				profileCatalog = catalog.NewManager(defaultProfileCatalogPaths())
			})
			return newCollector(deviceStore, topologyEnricher, hostIdentity, profileCatalog, netdataEngineStateRoot)
		},
		Config:         func() any { return &Config{} },
		AgentFunctions: snmpTrapsMethods,
		MethodHandler:  snmpTrapsMethodHandler,
	}
}

// New returns an SNMP traps collector using the provided SNMP-family enrichment state.
func New(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle) *Collector {
	if deviceStore == nil {
		panic("snmp_traps New requires a non-nil device store")
	}
	if topologyEnricher == nil {
		panic("snmp_traps New requires a non-nil trap enrichment handle")
	}
	return newCollector(
		deviceStore,
		topologyEnricher,
		newHostIdentityService(),
		nil,
		netdataEngineStateRoot,
	)
}

func newCollector(
	deviceStore *ddsnmp.DeviceStore,
	topologyEnricher *snmptopology.TrapEnrichmentHandle,
	hostIdentity *hostidentity.Service,
	profileCatalog *catalog.Manager,
	engineStateRoot func() string,
) *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			Versions: []string{"v1", "v2c"},
			Listen: ListenConfig{
				ReceiveBuffer: receiver.DefaultReceiveBuffer,
			},
		},
		store:            store,
		deviceLookup:     deviceStore,
		topologyEnricher: topologyEnricher,
		hostIdentity:     hostIdentity,
		profileCatalog:   profileCatalog,
		engineStateRoot:  engineStateRoot,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	receiver          *receiver.Receiver
	trapWriter        output.Writer
	journalHost       journalHostProvider
	journalDir        string
	store             metrix.CollectorStore
	deviceLookup      deviceLookup
	topologyEnricher  trapTopologyEnricher
	hostIdentity      *hostidentity.Service
	profileCatalog    *catalog.Manager
	profileLease      *catalog.Lease
	profileIndex      *catalog.Epoch
	engineStateRoot   func() string
	vnode             string
	overrides         map[string]*OverrideConfig
	metrics           *perJobMetrics
	reverseDNS        *reverseDNSResolver
	reverseDNSEnabled bool
	deduper           *trapDeduper
	packetSequence    atomic.Uint64
	profileMetrics    *profilemetrics.Runtime
	writeFailureDim   string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(ctx context.Context) error {
	validated, err := validateConfig(c.Config)
	if err != nil {
		return dyncfgConfigError(err)
	}
	c.warnCatchAllTrustedRelays(validated.trustedRelays)

	if c.receiver != nil {
		return nil
	}
	c.profileMetrics = nil

	profileCatalog := c.profileCatalog
	if profileCatalog == nil {
		profileCatalog = catalog.NewManager(defaultProfileCatalogPaths())
	}
	profileLease, err := profileCatalog.Acquire()
	if err != nil {
		return dyncfgConfigError(err)
	}
	releaseProfileLease := true
	releaseProfiles := func() {
		if releaseProfileLease {
			profileLease.Close()
			releaseProfileLease = false
		}
	}

	idx := profileLease.Epoch()
	if idx == nil {
		releaseProfiles()
		return dyncfgConfigError(errors.New("profile index not available"))
	}
	if validated.profileMetrics.Enabled() {
		rt, err := profilemetrics.New(validated.profileMetrics, idx, profilemetrics.Options{
			BaseChartTemplateYAML: chartTemplateYAML,
			SourceHashSalt:        c.sourceHashSalt(),
		})
		if err != nil {
			releaseProfiles()
			return dyncfgConfigError(err)
		}
		c.profileMetrics = rt
	}

	var metrics *perJobMetrics
	reportReceiver := receiver.Reporter(func(event receiver.Event) {
		c.handleReceiverEvent(metrics, event)
	})
	reportOutput := output.OutcomeReporter(func(outcome output.Outcome) {
		c.handleOutputOutcome(metrics, outcome)
	})
	var journalWriter *journal.Writer
	var otlpWriter *otlp.Writer
	var journalHost journalHostProvider
	if validated.journalEnabled {
		if err := journal.ValidateLogRoot(); err != nil {
			releaseProfiles()
			return dyncfgStartupError(err)
		}
		dir := journal.Root(c.Name)
		journalCfg := validated.retention.Config()
		host, err := c.hostIdentity.FreshJournal()
		if err != nil {
			releaseProfiles()
			return dyncfgStartupError(err)
		}
		journalHost = host
		journalWriter, err = journal.Prepare(dir, journalCfg, host, journal.Options{Report: reportOutput})
		if err != nil {
			releaseProfiles()
			return dyncfgStartupError(err)
		}
	}

	recv := receiver.New(validated.receiver, reportReceiver)
	err = recv.Bind()
	if err != nil {
		releaseProfiles()
		if journalWriter != nil {
			journalWriter.Close()
		}
		return dyncfgStartupError(err)
	}
	cleanupPreflight := func() {
		releaseProfiles()
		if journalWriter != nil {
			journalWriter.Close()
		}
		if otlpWriter != nil {
			otlpWriter.Close()
		}
		recv.Close()
	}

	cleanupCreatedState := recv.RollbackPreparedState
	if validated.receiver.V3Enabled() {
		if c.engineStateRoot == nil {
			cleanupPreflight()
			return dyncfgStartupError(errors.New("SNMP engine state root is not configured"))
		}
		if err := recv.PrepareV3(c.engineStateRoot(), c.Name); err != nil {
			cleanupPreflight()
			if receiver.IsConfigPreparationError(err) {
				return dyncfgConfigError(err)
			}
			return dyncfgStartupError(err)
		}
	}

	overrides := buildOverrideMap(c.Overrides)
	metrics = getJobMetrics(c.Name)
	recv.ReportBindWarnings()
	var secondaryWriter output.Writer
	if c.OTLP.Enabled {
		c.warnPlaintextOTLP(validated.otlp)
		otlpWriter, err = otlp.Prepare(ctx, c.Name, validated.otlp, otlp.Options{
			Authoritative: journalWriter == nil,
			Report:        reportOutput,
		})
		if err != nil {
			removeJobMetrics(c.Name)
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
		secondaryWriter = otlpWriter
	}
	var primaryWriter output.Writer
	writeFailureDim := trapWriteFailureJournal
	if journalWriter != nil {
		primaryWriter = journalWriter
	}
	trapWriter := output.NewCoordinator(primaryWriter, secondaryWriter, output.BackendOTLP, reportOutput)
	if primaryWriter == nil {
		writeFailureDim = trapWriteFailureOTLP
	}
	metrics.setDedupEnabled(c.Dedup.Enabled)
	deduper := newTrapDeduper(c.Name, c.Dedup, trapWriter, metrics, writeFailureDim, c.monotonicUsec)
	if deduper != nil {
		deduper.profiles = idx
	}

	var otlpStarter, journalStarter outputStarter
	if otlpWriter != nil {
		otlpStarter = otlpWriter
	}
	if journalWriter != nil {
		journalStarter = journalWriter
	}
	if err := startOutputBackends(otlpStarter, journalStarter); err != nil {
		removeJobMetrics(c.Name)
		cleanupCreatedState()
		cleanupPreflight()
		return dyncfgStartupError(err)
	}

	c.Versions = validated.versions
	c.vnode = c.Vnode
	c.overrides = overrides
	c.metrics = metrics
	c.receiver = recv
	c.trapWriter = trapWriter
	c.journalHost = nil
	c.journalDir = ""
	if journalWriter != nil {
		c.journalHost = journalHost
		c.journalDir = journalWriter.Directory()
		activeDirectJournalJobs.Add(1)
	}
	c.deduper = deduper
	c.profileCatalog = profileCatalog
	c.profileLease = profileLease
	c.profileIndex = idx
	releaseProfileLease = false
	c.writeFailureDim = writeFailureDim
	c.reverseDNSEnabled = c.ReverseDNS.Enabled
	if c.reverseDNSEnabled {
		c.reverseDNS = newReverseDNSResolver()
	}

	if deduper != nil {
		deduper.start()
	}

	recv.CommitPreparedState()
	c.receiver.Start(func(datagram receiver.Datagram) {
		c.handlePacket(datagram.Data, datagram.PeerIP, datagram.Conn, datagram.Peer)
	})

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.receiver != nil {
		c.receiver.Close()
		c.receiver = nil
	}
	if c.deduper != nil {
		c.deduper.Close()
		c.deduper = nil
	}
	if c.trapWriter != nil {
		c.trapWriter.Close()
		c.trapWriter = nil
	}
	c.journalHost = nil
	if c.reverseDNS != nil {
		c.reverseDNS.Close()
		c.reverseDNS = nil
		c.reverseDNSEnabled = false
	}
	if c.profileLease != nil {
		c.profileLease.Close()
		c.profileLease = nil
		c.profileIndex = nil
	}
	if c.journalDir != "" {
		activeDirectJournalJobs.Add(-1)
	}
	removeJobMetrics(c.Name)
	c.metrics = nil
	c.profileMetrics = nil
	c.journalDir = ""
	c.writeFailureDim = ""
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string {
	if c.profileMetrics != nil {
		return c.profileMetrics.ChartTemplateYAML()
	}
	return chartTemplateYAML
}

func (c *Collector) trapWriteFailureDim() string {
	if c.writeFailureDim != "" {
		return c.writeFailureDim
	}
	return trapWriteFailureJournal
}

func (c *Collector) collect(ctx context.Context) error {
	if c.receiver == nil || !c.receiver.Ready() {
		return errors.New("receiver not ready")
	}
	now := time.Now()
	c.receiver.Sweep(now)
	if c.reverseDNS != nil {
		c.reverseDNS.maybeSweep(now)
	}
	if c.metrics != nil {
		if w, ok := c.trapWriter.(output.BinaryFieldCounter); ok {
			c.metrics.setBinaryEncoded(w.BinaryEncodedFields())
		}
	}
	collectMetrics(c.store, c.Name)
	if c.profileMetrics != nil {
		c.profileMetrics.Collect(c.store, c.Name)
	}
	return nil
}

func (c *Collector) handlePacket(data []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr) {
	packetSequence := c.packetSequence.Add(1)
	metrics := c.trapMetrics()
	metrics.incPipelineReceived()
	packetFinished := false

	defer func() {
		if v := recover(); v != nil {
			c.incTrapError("decode_failed")
			if !packetFinished {
				metrics.incPipelineDropped()
			}
			c.Errorf("SNMP trap packet handling panic from %s: %v", peerIP, v)
			return
		}
		if !packetFinished {
			metrics.incPipelineDropped()
		}
	}()

	result := c.receiver.Process(receiver.Datagram{Data: data, PeerIP: peerIP, Conn: conn, Peer: peer})
	if failure := result.DecodeFailure; failure != nil {
		if c.trapWriter != nil && c.receiver.AdmitDecodeErrorAudit(failure) {
			c.writeDecodeErrorEntry(failure, packetSequence)
		}
		return
	}
	if result.PDU == nil {
		return
	}

	pdu := result.PDU

	idx := c.profileIndex
	var td *TrapDef
	var profileLookupErr error
	if idx != nil {
		td, profileLookupErr = idx.LookupWithError(pdu.OID)
		if profileLookupErr != nil {
			c.warnf("SNMP trap profile lookup failed for OID %s: %v", pdu.OID, profileLookupErr)
			c.incTrapError("profile_load_failed")
		}
	}
	unknownOID := td == nil && profileLookupErr == nil
	if td != nil {
		td = c.applyOverrides(td)
	}

	entry := trapEntryFromPDU(c.Name, pdu, td, time.Now().UnixMicro(), c.monotonicUsec())
	entry.PacketSequence = packetSequence
	c.enrichTrapEntry(entry, c.reverseDNSEnabled, c.reverseDNS)
	renderTrapEntryTemplates(entry, td)
	if unknownOID {
		c.incTrapError("unknown_oid")
	}
	if trapEntryHasUnresolvedTemplate(entry) {
		c.incTrapError("template_unresolved")
	}
	metrics.incPipelineAccepted()
	var admission dedupAdmission
	if c.deduper != nil {
		var suppressed bool
		admission, suppressed = c.deduper.Admit(entry, td, c.Dedup.KeyVarbinds)
		if suppressed {
			packetFinished = true
			return
		}
	}
	if err := c.trapWriter.Write(entry); err != nil {
		if c.deduper != nil {
			c.deduper.Rollback(admission)
		}
		c.incTrapError(c.trapWriteFailureDim())
		metrics.incPipelineWriteFailed()
		packetFinished = true
		return
	}
	packetFinished = true

	if c.profileMetrics != nil {
		c.profileMetrics.Update(entry)
	}

	cat := Category("unknown")
	if td != nil {
		cat = Category(td.Category)
	}
	metrics.incPipelineCommitted()
	metrics.incEvent(cat)
	metrics.incSeverity(entry.Severity)
}

func (c *Collector) warnf(format string, args ...any) {
	if c.Logger != nil {
		c.Warningf(format, args...)
	}
}

type outputStarter interface {
	Start() error
}

func startOutputBackends(otlpBackend, journalBackend outputStarter) error {
	if otlpBackend != nil {
		if err := otlpBackend.Start(); err != nil {
			return err
		}
	}
	if journalBackend != nil {
		if err := journalBackend.Start(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) handleOutputOutcome(metrics *perJobMetrics, outcome output.Outcome) {
	if outcome.Backend == output.BackendJournal && outcome.Err != nil {
		c.warnf("SNMP trap journal writer stopped for job %q: %v", c.Name, outcome.Err)
	}
	if metrics == nil || outcome.FailedEntries == 0 {
		return
	}
	switch outcome.Backend {
	case output.BackendJournal:
		metrics.addError(trapWriteFailureJournal, outcome.FailedEntries)
	case output.BackendOTLP:
		metrics.addError(trapWriteFailureOTLP, outcome.FailedEntries)
	}
	if outcome.Authoritative {
		metrics.addPipelineWriteFailed(outcome.FailedEntries)
	}
}

func (c *Collector) warnPlaintextOTLP(policy otlp.Policy) {
	if !policy.PlaintextRemote() {
		return
	}
	c.warnf("SNMP trap OTLP endpoint %q uses plaintext transport; use https:// for remote collectors", policy.Target())
}

func (c *Collector) warnCatchAllTrustedRelays(prefixes []netip.Prefix) {
	for _, prefix := range prefixes {
		if trustedRelayPrefixIsCatchAll(prefix) {
			c.warnf("SNMP trap source.trusted_relays contains catch-all prefix %s; every UDP peer in this address family may override source identity via snmpTrapAddress.0", prefix)
		}
	}
}

func trustedRelayPrefixIsCatchAll(prefix netip.Prefix) bool {
	return prefix.Bits() == 0
}

func (c *Collector) handleReceiverEvent(metrics *perJobMetrics, event receiver.Event) {
	switch event.Type {
	case receiver.EventError:
		if metrics != nil {
			metrics.incError(string(event.ErrorKind))
		}
	case receiver.EventDecoded:
		if metrics != nil {
			metrics.incPipelineDecoded()
		}
	case receiver.EventListenerReadFailed:
		if metrics != nil {
			metrics.incError("listener_read_failed")
		}
		if c.Logger != nil {
			endpoint := event.Endpoint.LogName()
			c.Limit(listenerReadErrorLogKeyPrefix+endpoint, 1, listenerReadErrorLogEvery).
				Warningf("SNMP trap listener read failed (endpoint=%s): %v", endpoint, event.Err)
		}
	case receiver.EventListenerBufferDegraded:
		if metrics != nil {
			metrics.incError(listenerBufferDegradedMetric)
		}
		c.warnf(
			"SNMP trap listener receive buffer request degraded (endpoint=%s, requested=%d bytes): %v; continuing with the OS socket buffer, high trap bursts may be dropped",
			event.Endpoint.LogName(), event.Requested, event.Err,
		)
	case receiver.EventDynamicEngineIDRegistered:
		if metrics != nil {
			metrics.incError("unknown_engine_id")
		}
		c.warnf("Dynamic SNMPv3 engine ID registered: engineID=%s username=%s. This sender was not in the static configuration. Every first-time dynamic (engineID, username) pair is accepted and logged once per job lifetime.",
			event.EngineID, event.Username)
	case receiver.EventInformResponseFailed:
		if metrics != nil {
			metrics.incError("inform_response_failed")
		}
		c.warnf("SNMP trap INFORM response failed: %v", event.Err)
	case receiver.EventDiscoveryReportFailed:
		if metrics != nil {
			metrics.incError("inform_response_failed")
		}
		c.warnf("SNMP trap INFORM discovery Report failed: %v", event.Err)
	}
}

func (c *Collector) applyOverrides(td *TrapDef) *TrapDef {
	if td == nil || len(c.overrides) == 0 {
		return td
	}
	ov, ok := c.overrides[td.OID]
	if !ok {
		return td
	}
	return td.WithOverrides(ov.Category, ov.Severity, ov.Labels)
}

func buildOverrideMap(overrides []OverrideConfig) map[string]*OverrideConfig {
	if len(overrides) == 0 {
		return nil
	}
	m := make(map[string]*OverrideConfig, len(overrides))
	for i := range overrides {
		ov := overrides[i]
		m[ov.OID] = &ov
	}
	return m
}

type dyncfgCodedError struct {
	err       error
	code      int
	retryable bool
}

func dyncfgConfigError(err error) *dyncfgCodedError {
	return &dyncfgCodedError{err: err, code: 422}
}

func dyncfgStartupError(err error) *dyncfgCodedError {
	return &dyncfgCodedError{err: err, code: 503, retryable: true}
}

func (e *dyncfgCodedError) Error() string         { return e.err.Error() }
func (e *dyncfgCodedError) Unwrap() error         { return e.err }
func (e *dyncfgCodedError) DyncfgCode() int       { return e.code }
func (e *dyncfgCodedError) DyncfgRetryable() bool { return e.retryable }
