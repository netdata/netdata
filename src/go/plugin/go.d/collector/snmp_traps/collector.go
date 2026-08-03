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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/dedup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment/netdataadapter"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/otlp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profilemetrics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

const (
	trapWriteFailureJournal = telemetry.ErrorJournalWriteFailed
	trapWriteFailureOTLP    = telemetry.ErrorOTLPExportFailed

	listenerReadErrorLogEvery     = time.Hour
	listenerReadErrorLogKeyPrefix = "snmp_traps:listener_read_failed:"
	listenerBufferDegradedMetric  = telemetry.ErrorListenerBufferDegraded
)

var activeDirectJournalJobs atomic.Int64

func directJournalLogsAvailable() bool {
	return activeDirectJournalJobs.Load() > 0
}

// Register registers the SNMP traps collector with shared SNMP-family enrichment state.
func Register(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) {
	collectorapi.Register("snmp_traps", newCreator(deviceStore, topologyEnricher, reverseDNS))
}

func newCreator(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) collectorapi.Creator {
	if deviceStore == nil {
		panic("snmp_traps Register requires a non-nil device store")
	}
	if topologyEnricher == nil {
		panic("snmp_traps Register requires a non-nil trap enrichment handle")
	}
	if reverseDNS == nil {
		panic("snmp_traps Register requires a non-nil reverse DNS resolver")
	}
	trapEnricher := enrichment.New(
		netdataadapter.RegistryLookup(deviceStore),
		netdataadapter.TopologyLookup(topologyEnricher),
		reverseDNS,
	)
	hostIdentity := newHostIdentityService()
	telemetryRegistry := telemetry.NewRegistry()
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
			return newCollector(trapEnricher, hostIdentity, profileCatalog, telemetryRegistry, netdataEngineStateRoot)
		},
		Config:         func() any { return &Config{} },
		AgentFunctions: snmpTrapsMethods,
		MethodHandler:  snmpTrapsMethodHandler,
	}
}

// New returns an SNMP traps collector using the provided SNMP-family enrichment state.
func New(deviceStore *ddsnmp.DeviceStore, topologyEnricher *snmptopology.TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) *Collector {
	if deviceStore == nil {
		panic("snmp_traps New requires a non-nil device store")
	}
	if topologyEnricher == nil {
		panic("snmp_traps New requires a non-nil trap enrichment handle")
	}
	if reverseDNS == nil {
		panic("snmp_traps New requires a non-nil reverse DNS resolver")
	}
	return newCollector(
		enrichment.New(
			netdataadapter.RegistryLookup(deviceStore),
			netdataadapter.TopologyLookup(topologyEnricher),
			reverseDNS,
		),
		newHostIdentityService(),
		nil,
		telemetry.NewRegistry(),
		netdataEngineStateRoot,
	)
}

func newCollector(
	trapEnricher *enrichment.Enricher,
	hostIdentity *hostidentity.Service,
	profileCatalog *catalog.Manager,
	telemetryRegistry *telemetry.Registry,
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
		store:             store,
		enricher:          trapEnricher,
		hostIdentity:      hostIdentity,
		profileCatalog:    profileCatalog,
		telemetryRegistry: telemetryRegistry,
		engineStateRoot:   engineStateRoot,
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
	enricher          *enrichment.Enricher
	hostIdentity      *hostidentity.Service
	profileCatalog    *catalog.Manager
	telemetryRegistry *telemetry.Registry
	profileLease      *catalog.Lease
	profileIndex      *catalog.Epoch
	engineStateRoot   func() string
	vnode             string
	overrides         map[string]*OverrideConfig
	telemetry         *telemetry.Job
	reverseDNSEnabled bool
	deduper           *dedup.Deduper
	dedupKeyVarbinds  []string
	packetSequence    atomic.Uint64
	profileMetrics    *profilemetrics.Runtime
	writeFailureDim   telemetry.ErrorKind
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

	var jobTelemetry *telemetry.Job
	reportReceiver := receiver.Reporter(func(event receiver.Event) {
		c.handleReceiverEvent(jobTelemetry, event)
	})
	reportOutput := output.OutcomeReporter(func(outcome output.Outcome) {
		c.handleOutputOutcome(jobTelemetry, outcome)
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
	bindEvents, err := recv.Bind()
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
	jobTelemetry = c.attachTelemetryAndHandleInitEvents(bindEvents, validated.dedup.Enabled())
	var secondaryWriter output.Writer
	if c.OTLP.Enabled {
		c.warnPlaintextOTLP(validated.otlp)
		otlpWriter, err = otlp.Prepare(ctx, c.Name, validated.otlp, otlp.Options{
			Authoritative: journalWriter == nil,
			Report:        reportOutput,
		})
		if err != nil {
			jobTelemetry.Detach()
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
	jobName := c.Name
	deduper := newJobDeduper(jobName, validated.dedup, idx, trapWriter, jobTelemetry, writeFailureDim, c.monotonicUsec)
	dedupKeyVarbinds := validated.dedup.KeyVarbinds()

	var otlpStarter, journalStarter outputStarter
	if otlpWriter != nil {
		otlpStarter = otlpWriter
	}
	if journalWriter != nil {
		journalStarter = journalWriter
	}
	if err := startOutputBackends(otlpStarter, journalStarter); err != nil {
		jobTelemetry.Detach()
		cleanupCreatedState()
		cleanupPreflight()
		return dyncfgStartupError(err)
	}

	c.Versions = validated.versions
	c.vnode = c.Vnode
	c.overrides = overrides
	c.telemetry = jobTelemetry
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
	c.dedupKeyVarbinds = dedupKeyVarbinds
	c.profileCatalog = profileCatalog
	c.profileLease = profileLease
	c.profileIndex = idx
	releaseProfileLease = false
	c.writeFailureDim = writeFailureDim
	c.reverseDNSEnabled = c.ReverseDNS.Enabled

	if deduper != nil {
		deduper.Start()
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
	c.dedupKeyVarbinds = nil
	if c.trapWriter != nil {
		c.trapWriter.Close()
		c.trapWriter = nil
	}
	c.journalHost = nil
	c.reverseDNSEnabled = false
	if c.profileLease != nil {
		c.profileLease.Close()
		c.profileLease = nil
		c.profileIndex = nil
	}
	if c.journalDir != "" {
		activeDirectJournalJobs.Add(-1)
	}
	if c.telemetry != nil {
		c.telemetry.Detach()
		c.telemetry = nil
	}
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

func (c *Collector) trapWriteFailureDim() telemetry.ErrorKind {
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
	if c.telemetry != nil {
		if w, ok := c.trapWriter.(output.BinaryFieldCounter); ok {
			c.telemetry.SetBinaryEncoded(w.BinaryEncodedFields())
		}
		c.telemetry.Collect(c.store)
	}
	if c.profileMetrics != nil {
		c.profileMetrics.Collect(c.store, c.Name)
	}
	return nil
}

func (c *Collector) handlePacket(data []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr) {
	packetSequence := c.packetSequence.Add(1)
	jobTelemetry := c.telemetry
	jobTelemetry.PipelineReceived()
	packetFinished := false

	defer func() {
		if v := recover(); v != nil {
			jobTelemetry.Error(telemetry.ErrorDecodeFailed)
			if !packetFinished {
				jobTelemetry.PipelineDropped()
			}
			c.Errorf("SNMP trap packet handling panic from %s: %v", peerIP, v)
			return
		}
		if !packetFinished {
			jobTelemetry.PipelineDropped()
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
			jobTelemetry.Error(telemetry.ErrorProfileLoadFailed)
		}
	}
	unknownOID := td == nil && profileLookupErr == nil
	if td != nil {
		td = c.applyOverrides(td)
	}

	entry := trapEntryFromPDU(c.Name, pdu, td, time.Now().UnixMicro(), c.monotonicUsec())
	entry.PacketSequence = packetSequence
	c.enrichTrapEntry(entry)
	renderTrapEntryTemplates(entry, td)
	if unknownOID {
		jobTelemetry.Error(telemetry.ErrorUnknownOID)
	}
	if trapEntryHasUnresolvedTemplate(entry) {
		jobTelemetry.Error(telemetry.ErrorTemplateUnresolved)
	}
	jobTelemetry.PipelineAccepted()
	var admission dedup.Admission
	if c.deduper != nil {
		var decision dedup.Decision
		admission, decision = c.deduper.Admit(entry, selectDedupKeyVarbinds(td, c.dedupKeyVarbinds))
		if decision == dedup.DecisionSuppress {
			jobTelemetry.DedupSuppressed()
			packetFinished = true
			return
		}
	}
	if err := c.trapWriter.Write(entry); err != nil {
		if c.deduper != nil {
			c.deduper.Rollback(admission)
		}
		jobTelemetry.Error(c.trapWriteFailureDim())
		jobTelemetry.PipelineWriteFailed(1)
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
	jobTelemetry.PipelineCommitted()
	jobTelemetry.Event(cat)
	jobTelemetry.Severity(entry.Severity)
}

func selectDedupKeyVarbinds(td *TrapDef, jobKeys []string) []string {
	if td != nil && len(td.DedupKeyVarbinds) > 0 {
		return td.DedupKeyVarbinds
	}
	return jobKeys
}

func (c *Collector) warnf(format string, args ...any) {
	if c.Logger != nil {
		c.Warningf(format, args...)
	}
}

func (c *Collector) attachTelemetryAndHandleInitEvents(events []receiver.Event, dedupEnabled bool) *telemetry.Job {
	jobTelemetry := c.telemetryRegistry.Attach(c.Name, telemetry.Options{DedupEnabled: dedupEnabled})
	for _, event := range events {
		c.handleReceiverEvent(jobTelemetry, event)
	}
	return jobTelemetry
}

func newJobDeduper(
	jobName string,
	policy dedup.Policy,
	idx *catalog.Epoch,
	writer output.Writer,
	jobTelemetry *telemetry.Job,
	writeFailureDim telemetry.ErrorKind,
	monotonicNow func() int64,
) *dedup.Deduper {
	return dedup.New(policy, dedup.Options{
		MonotonicNow: monotonicNow,
		ResolveName: func(oid string) string {
			if idx != nil {
				if td := idx.Lookup(oid); td != nil && td.Name != "" {
					return td.Name
				}
			}
			return oid
		},
		OnSummary: func(summary dedup.Summary) {
			entry := &TrapEntry{
				JobName:               jobName,
				ReportType:            ReportTypeDedupSummary,
				ReceivedRealtimeUsec:  summary.ReceivedRealtimeUsec,
				ReceivedMonotonicUsec: summary.ReceivedMonotonicUsec,
				Message:               summary.Message,
				Severity:              "info",
				SummaryCounts:         summary.Counts,
			}
			if err := writer.Write(entry); err != nil {
				jobTelemetry.Error(writeFailureDim)
			}
		},
	})
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

func (c *Collector) handleOutputOutcome(jobTelemetry *telemetry.Job, outcome output.Outcome) {
	if outcome.Backend == output.BackendJournal && outcome.Err != nil {
		c.warnf("SNMP trap journal writer stopped for job %q: %v", c.Name, outcome.Err)
	}
	if jobTelemetry == nil || outcome.FailedEntries == 0 {
		return
	}
	switch outcome.Backend {
	case output.BackendJournal:
		jobTelemetry.AddError(trapWriteFailureJournal, outcome.FailedEntries)
	case output.BackendOTLP:
		jobTelemetry.AddError(trapWriteFailureOTLP, outcome.FailedEntries)
	}
	if outcome.Authoritative {
		jobTelemetry.PipelineWriteFailed(outcome.FailedEntries)
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

func (c *Collector) handleReceiverEvent(jobTelemetry *telemetry.Job, event receiver.Event) {
	switch event.Type {
	case receiver.EventError:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorKind(event.ErrorKind))
		}
	case receiver.EventDecoded:
		if jobTelemetry != nil {
			jobTelemetry.PipelineDecoded()
		}
	case receiver.EventListenerReadFailed:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorListenerReadFailed)
		}
		if c.Logger != nil {
			endpoint := event.Endpoint.LogName()
			c.Limit(listenerReadErrorLogKeyPrefix+endpoint, 1, listenerReadErrorLogEvery).
				Warningf("SNMP trap listener read failed (endpoint=%s): %v", endpoint, event.Err)
		}
	case receiver.EventListenerBufferDegraded:
		if jobTelemetry != nil {
			jobTelemetry.Error(listenerBufferDegradedMetric)
		}
		c.warnf(
			"SNMP trap listener receive buffer request degraded (endpoint=%s, requested=%d bytes): %v; continuing with the OS socket buffer, high trap bursts may be dropped",
			event.Endpoint.LogName(), event.Requested, event.Err,
		)
	case receiver.EventDynamicEngineIDRegistered:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorUnknownEngineID)
		}
		c.warnf("Dynamic SNMPv3 engine ID registered: engineID=%s username=%s. This sender was not in the static configuration. Every first-time dynamic (engineID, username) pair is accepted and logged once per job lifetime.",
			event.EngineID, event.Username)
	case receiver.EventInformResponseFailed:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorInformResponseFailed)
		}
		c.warnf("SNMP trap INFORM response failed: %v", event.Err)
	case receiver.EventDiscoveryReportFailed:
		if jobTelemetry != nil {
			jobTelemetry.Error(telemetry.ErrorInformResponseFailed)
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
