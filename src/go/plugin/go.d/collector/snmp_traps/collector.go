// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	_ "embed"
	"errors"
	"maps"
	"net"
	"net/netip"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
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
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2:      func() collectorapi.CollectorV2 { return New(deviceStore, topologyEnricher) },
		Config:        func() any { return &Config{} },
		Methods:       snmpTrapsMethods,
		MethodHandler: snmpTrapsMethodHandler,
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
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			Versions: []string{"v1", "v2c"},
			Listen: ListenConfig{
				ReceiveBuffer: defaultListenerReceiveBuffer,
			},
		},
		store:            store,
		deviceLookup:     deviceStore,
		topologyEnricher: topologyEnricher,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	listener           *Listener
	trapWriter         TrapWriter
	journalDir         string
	store              metrix.CollectorStore
	deviceLookup       deviceLookup
	topologyEnricher   trapTopologyEnricher
	jobName            string
	vnode              string
	versions           map[SnmpVersion]struct{}
	allowlist          *Allowlist
	trustedRelays      []netip.Prefix
	rateLimiter        *rateLimiter
	engineBoots        *EngineBoots
	localEngineID      *LocalEngineID
	v3SecTable         *gosnmp.SnmpV3SecurityParametersTable
	engineIDs          map[string]struct{}
	dynamicEngineID    bool
	dynamicEngineIDMax int
	dynamicEngineIDReg *dynamicEngineIDRegistry
	overrides          map[string]*OverrideConfig
	metrics            *perJobMetrics
	reverseDNS         *reverseDNSResolver
	reverseDNSEnabled  bool
	deduper            *trapDeduper
	packetSequence     atomic.Uint64
	profileCacheHeld   bool
	profileMetrics     *profileMetricRuntime
	dynamicChartYAML   string
	writeFailureDim    string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) SetJobName(name string) {
	c.jobName = name
}

func (c *Collector) Init(ctx context.Context) error {
	if err := validateJobName(c.jobName); err != nil {
		return dyncfgConfigError(err)
	}

	if err := validateListenConfig(c.Listen); err != nil {
		return dyncfgConfigError(err)
	}

	versions, err := validateVersions(c.Versions)
	if err != nil {
		return dyncfgConfigError(err)
	}
	v3Enabled := versionListContains(versions, "v3")

	if err := validateUSMUsers(c.USMUsers, c.DynamicEngineID); err != nil {
		return dyncfgConfigError(err)
	}

	if err := validateEngineIDWhitelist(c.EngineIDWhitelist); err != nil {
		return dyncfgConfigError(err)
	}

	if err := validateLocalEngineID(c.LocalEngineID); err != nil {
		return dyncfgConfigError(err)
	}

	allowlistPrefixes, err := validateAllowlist(c.Allowlist)
	if err != nil {
		return dyncfgConfigError(err)
	}
	trustedRelays, err := validateTrustedRelays(c.Source)
	if err != nil {
		return dyncfgConfigError(err)
	}
	c.warnCatchAllTrustedRelays(trustedRelays)

	if err := validateRateLimit(c.RateLimit); err != nil {
		return dyncfgConfigError(err)
	}
	if err := validateDedupConfig(c.Dedup); err != nil {
		return dyncfgConfigError(err)
	}
	otlpRuntime, err := validateOTLPConfig(c.OTLP)
	if err != nil {
		return dyncfgConfigError(err)
	}
	journalEnabled := c.Journal.enabled()
	if !journalEnabled && !c.OTLP.Enabled {
		return dyncfgConfigError(errors.New("at least one SNMP trap output backend must be enabled: journal.enabled or otlp.enabled"))
	}
	if journalEnabled && runtime.GOOS != "linux" {
		return dyncfgConfigError(errors.New("SNMP trap journal backend requires Linux"))
	}

	if err := validateOverrides(c.Overrides); err != nil {
		return dyncfgConfigError(err)
	}
	if err := validateDeferredConfig(c.Config); err != nil {
		return dyncfgConfigError(err)
	}
	if v3Enabled {
		if len(c.USMUsers) == 0 {
			return dyncfgConfigError(errors.New("SNMPv3 requires at least one usm_users entry"))
		}
		if !c.DynamicEngineID && len(c.EngineIDWhitelist) == 0 {
			return dyncfgConfigError(errors.New("SNMPv3 requires engine_id_whitelist when dynamic_engine_id_discovery is disabled"))
		}
	}

	var retCfg RetentionConfig
	if journalEnabled {
		ret, err := parseRetentionConfig(c.Retention)
		if err != nil {
			return dyncfgConfigError(err)
		}
		retCfg = ret
	}

	if c.listener != nil {
		return nil
	}
	c.profileMetrics = nil
	c.dynamicChartYAML = ""

	if _, err := AcquireProfileCache(); err != nil {
		return dyncfgConfigError(err)
	}
	releaseProfileCache := true
	releaseProfiles := func() {
		if releaseProfileCache {
			ReleaseProfileCache()
			releaseProfileCache = false
		}
	}

	idx := CurrentProfileIndex()
	if idx == nil {
		releaseProfiles()
		return dyncfgConfigError(errors.New("profile index not available"))
	}
	profileMetricCfg, err := normalizeProfileMetricsConfig(c.ProfileMetrics)
	if err != nil {
		releaseProfiles()
		return dyncfgConfigError(err)
	}
	if profileMetricCfg.enabled {
		rt, tmpl, err := newProfileMetricRuntime(profileMetricCfg, idx)
		if err != nil {
			releaseProfiles()
			return dyncfgConfigError(err)
		}
		c.profileMetrics = rt
		c.dynamicChartYAML = tmpl
	}

	var journalWriter *JournalWriter
	if journalEnabled {
		if err := validateNetdataLogRoot(); err != nil {
			releaseProfiles()
			return dyncfgStartupError(err)
		}
		dir := journalRoot(c.jobName)
		journalCfg := retCfg.makeJournalConfig()
		var err error
		journalWriter, err = NewJournalWriter(dir, journalCfg)
		if err != nil {
			releaseProfiles()
			return dyncfgStartupError(err)
		}
	}

	listener, err := newListener(c.jobName, c.Listen)
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
		listener.close()
	}

	var eb *EngineBoots
	var lid *LocalEngineID
	var v3Table *gosnmp.SnmpV3SecurityParametersTable
	var engineIDWhitelist map[string]struct{}
	cleanupCreatedState := func() {
		// No engine-state files exist unless SNMPv3 setup below creates them.
	}
	if v3Enabled {
		engineBootsExisted, err := engineStatePathExistsChecked(engineBootsPath(c.jobName))
		if err != nil {
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
		localEngineIDExisted, err := engineStatePathExistsChecked(localEngineIDPath(c.jobName))
		if err != nil {
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
		engineStateDirExisted, err := engineStatePathExistsChecked(engineBootsDir(c.jobName))
		if err != nil {
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
		cleanupCreatedState = func() {
			cleanupCreatedEngineState(c.jobName, !engineBootsExisted, !localEngineIDExisted, !engineStateDirExisted)
		}

		v3Table, err = buildSnmpV3SecurityTable(c.USMUsers, c.DynamicEngineID)
		if err != nil {
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgConfigError(err)
		}
		engineIDWhitelist, err = buildEngineIDWhitelist(c.EngineIDWhitelist)
		if err != nil {
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgConfigError(err)
		}

		lid, err = NewLocalEngineID(c.jobName, c.LocalEngineID)
		if err != nil {
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
		if err := registerUSMUsersWithLocalEngineID(v3Table, c.USMUsers, lid.Bytes()); err != nil {
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgConfigError(err)
		}

		eb, err = NewEngineBoots(c.jobName)
		if err != nil {
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
	}

	overrides := buildOverrideMap(c.Overrides)
	metrics := getJobMetrics(c.jobName)
	listener.metrics = metrics
	listener.onReadError = c.logListenerReadError
	var secondaryWriter TrapWriter
	if c.OTLP.Enabled {
		c.warnPlaintextOTLP(otlpRuntime)
		secondaryWriter, err = newOTLPTrapWriterWithRuntimeConfig(ctx, c.jobName, otlpRuntime, metrics, journalWriter == nil)
		if err != nil {
			removeJobMetrics(c.jobName)
			cleanupCreatedState()
			cleanupPreflight()
			return dyncfgStartupError(err)
		}
	}
	var primaryWriter TrapWriter
	writeFailureDim := trapWriteFailureJournal
	if journalWriter != nil {
		primaryWriter = newJournalTrapWriter(journalWriter, defaultQueueCapacity, func(err error) {
			c.warnf("SNMP trap journal writer stopped for job %q: %v", c.jobName, err)
		})
	}
	trapWriter := newFanoutTrapWriter(primaryWriter, secondaryWriter, metrics)
	if primaryWriter == nil {
		writeFailureDim = trapWriteFailureOTLP
	}
	metrics.setDedupEnabled(c.Dedup.Enabled)
	deduper := newTrapDeduper(c.jobName, c.Dedup, trapWriter, metrics, writeFailureDim)
	if deduper != nil {
		deduper.start()
	}

	c.Versions = versions
	c.vnode = c.Vnode
	c.versions = versionSet(versions)
	c.allowlist = NewAllowlist(allowlistPrefixes, c.Communities)
	c.trustedRelays = trustedRelays
	c.rateLimiter = newRateLimiter(c.RateLimit.Enabled, c.RateLimit.PerSourcePPS, c.RateLimit.Mode)
	c.engineBoots = eb
	c.localEngineID = lid
	c.v3SecTable = v3Table
	c.engineIDs = engineIDWhitelist
	c.dynamicEngineID = c.DynamicEngineID
	c.dynamicEngineIDMax = c.DynamicEngineIDMax
	if c.dynamicEngineIDMax == 0 {
		c.dynamicEngineIDMax = defaultDynamicEngineIDMax
	}
	c.dynamicEngineIDReg = nil
	if c.dynamicEngineID && v3Table != nil {
		known := make(map[dynamicEngineIDKey]struct{})
		for _, u := range c.USMUsers {
			if u.EngineID != "" {
				known[dynamicEngineIDKey{
					engineIDHex: strings.ToLower(strings.TrimSpace(u.EngineID)),
					username:    u.Username,
				}] = struct{}{}
			}
		}
		c.dynamicEngineIDReg = newDynamicEngineIDRegistry(v3Table, c.dynamicEngineIDMax, known, c.USMUsers)
	}
	c.overrides = overrides
	c.metrics = metrics
	c.listener = listener
	c.trapWriter = trapWriter
	c.journalDir = ""
	if journalWriter != nil {
		c.journalDir = journalWriter.JournalDirectory()
		activeDirectJournalJobs.Add(1)
	}
	c.deduper = deduper
	c.profileCacheHeld = true
	releaseProfileCache = false
	c.writeFailureDim = writeFailureDim
	c.reverseDNSEnabled = c.ReverseDNS.Enabled
	if c.reverseDNSEnabled {
		c.reverseDNS = newReverseDNSResolver()
	}

	c.listener.start(c.handlePacket)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	return c.collect(ctx)
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.listener != nil {
		c.listener.close()
		c.listener = nil
	}
	if c.deduper != nil {
		c.deduper.Close()
		c.deduper = nil
	}
	if c.trapWriter != nil {
		c.trapWriter.Close()
		c.trapWriter = nil
	}
	if c.reverseDNS != nil {
		c.reverseDNS.Close()
		c.reverseDNS = nil
		c.reverseDNSEnabled = false
	}
	if c.profileCacheHeld {
		ReleaseProfileCache()
		c.profileCacheHeld = false
	}
	if c.journalDir != "" {
		activeDirectJournalJobs.Add(-1)
	}
	removeJobMetrics(c.jobName)
	c.metrics = nil
	c.profileMetrics = nil
	c.dynamicChartYAML = ""
	c.journalDir = ""
	c.writeFailureDim = ""
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string {
	if c.dynamicChartYAML != "" {
		return c.dynamicChartYAML
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
	if c.listener == nil {
		return errors.New("listener not started")
	}
	now := time.Now()
	if c.rateLimiter != nil {
		c.rateLimiter.maybeSweep(now)
	}
	if c.reverseDNS != nil {
		c.reverseDNS.maybeSweep(now)
	}
	if c.metrics != nil {
		if w, ok := c.trapWriter.(interface{ BinaryEncodedFields() uint64 }); ok {
			c.metrics.setBinaryEncoded(w.BinaryEncodedFields())
		}
	}
	collectMetrics(c.store, c.jobName)
	if c.profileMetrics != nil {
		c.profileMetrics.collect(c.store, c.jobName)
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

	decodePeerIP := peerIP
	rateLimitChecked := false
	if srcAddr, ok := packetSourceAddr(peerIP, peer); ok {
		decodePeerIP = net.IP(srcAddr.AsSlice())
		if c.allowlist != nil && !c.allowlist.AllowedSource(srcAddr) {
			c.incTrapError("dropped_allowlist")
			return
		}
	} else if c.allowlist != nil {
		c.incTrapError("dropped_allowlist")
		return
	}

	sniffedVersion, versionKnown := sniffSNMPVersion(data)
	if versionKnown && !c.versionAllowed(sniffedVersion) {
		c.incTrapError("dropped_allowlist")
		return
	}

	trustedRelay := c.trustedRelaySource(decodePeerIP)
	pktCtx, err := c.decodeTrapWithSharedTable(data, decodePeerIP, trustedRelay)
	if err != nil {
		dim := ClassifyDecodeError(err)
		if c.v3SecTable != nil {
			rawCtx, rawErr := extractRawV3Context(data)
			if rawErr == nil && rawCtx != nil {
				if c.dynamicEngineID && !rawCtx.reportable {
					retryCtx, checked, dropped := c.tryDynamicRetry(data, decodePeerIP, peer, rawCtx, trustedRelay)
					rateLimitChecked = rateLimitChecked || checked
					if dropped {
						return
					}
					if retryCtx != nil {
						pktCtx = retryCtx
						err = nil
					}
				} else if rawCtx.discoveryProbe() && conn != nil && peer != nil {
					allowed, checked := c.allowRateLimitedPacket(peer)
					rateLimitChecked = rateLimitChecked || checked
					if !allowed {
						return
					}
					c.sendDiscoveryReport(rawCtx, conn, peer)
				}
			}
		}
		if err != nil {
			if shouldExtractEngineIDOnDecodeError(dim) {
				engineIDHex, ok, extractErr := extractSNMPv3EngineIDHex(data)
				if extractErr == nil && ok && !engineIDHexAllowed(engineIDHex, c.engineIDs) {
					dim = "unknown_engine_id"
				}
			}
			c.incTrapError(dim)
			c.writeDecodeErrorEntry(decodeErrorRecord{
				data:           data,
				peerIP:         decodePeerIP,
				conn:           conn,
				peer:           peer,
				packetSequence: packetSequence,
				kind:           dim,
				err:            err,
				sniffedVersion: sniffedVersion,
				versionKnown:   versionKnown,
			})
			return
		}
	}

	pdu := pktCtx.PDU
	metrics.incPipelineDecoded()

	if !c.versionAllowed(pdu.Version) {
		c.incTrapError("dropped_allowlist")
		return
	}

	if pdu.Version != SnmpVersionV3 && !c.communityAllowed(pdu.Community) {
		c.incTrapError("dropped_allowlist")
		return
	}

	if !c.ensureDynamicEngineIDRegistered(pktCtx) {
		return
	}

	if pktCtx.Packet != nil && pdu.Version == SnmpVersionV3 {
		if pdu.PduType == PduTypeInform {
			if !c.localEngineIDMatches(pktCtx.Packet.SecurityParameters) {
				c.incTrapError("unknown_engine_id")
				return
			}
		} else {
			if !isEngineIDAllowed(pktCtx.Packet.SecurityParameters, c.engineIDs) {
				c.incTrapError("unknown_engine_id")
				return
			}
		}
	}

	if pdu.PduType == PduTypeInform {
		if pktCtx.Packet != nil && conn != nil && peer != nil {
			var localEID []byte
			if c.localEngineID != nil {
				localEID = c.localEngineID.Bytes()
			}
			if sendErr := sendInformResponse(conn, peer, pktCtx.Packet, c.engineBoots, localEID); sendErr != nil {
				c.warnf("SNMP trap INFORM response failed: %v", sendErr)
				c.incTrapError("inform_response_failed")
			}
		}
	}

	if !rateLimitChecked {
		allowed, _ := c.allowRateLimitedPacket(peer)
		if !allowed {
			return
		}
	}

	idx := CurrentProfileIndex()
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

	entry := trapEntryFromPDU(c.jobName, pdu, td, time.Now().UnixMicro(), monotonicUsec())
	entry.PacketSequence = packetSequence
	c.enrichTrapEntry(entry, c.reverseDNSEnabled, c.reverseDNS)
	renderTrapEntryTemplates(entry, td)
	if unknownOID {
		c.incTrapError("unknown_oid")
	}
	if trapEntryHasUnresolvedTemplate(entry) {
		c.incTrapError("template_unresolved")
	}
	metrics.recordSourceAccepted(entry)
	if profileLookupErr != nil {
		metrics.recordSourceError(entry, "profile_load_failed")
	}
	if unknownOID {
		metrics.recordSourceError(entry, "unknown_oid")
	}
	if trapEntryHasUnresolvedTemplate(entry) {
		metrics.recordSourceError(entry, "template_unresolved")
	}
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
		metrics.recordWriteFailure(entry, c.trapWriteFailureDim())
		packetFinished = true
		return
	}
	packetFinished = true

	if c.profileMetrics != nil {
		c.profileMetrics.update(entry)
	}

	cat := Category("unknown")
	if td != nil {
		cat = Category(td.Category)
	}
	metrics.recordSourceCommitted(entry)
	metrics.incEvent(cat)
	metrics.incSeverity(entry.Severity)
}

func udpPeerAddr(peer *net.UDPAddr) (netip.Addr, bool) {
	if peer == nil {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(peer.IP)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func packetSourceAddr(peerIP net.IP, peer *net.UDPAddr) (netip.Addr, bool) {
	if addr, ok := udpPeerAddr(peer); ok {
		return addr, true
	}
	if peerIP == nil {
		return netip.Addr{}, false
	}
	addr, err := netip.ParseAddr(peerIP.String())
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func (c *Collector) trustedRelaySource(peerIP net.IP) bool {
	if len(c.trustedRelays) == 0 || peerIP == nil {
		return false
	}
	addr, ok := netip.AddrFromSlice(peerIP)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range c.trustedRelays {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func shouldExtractEngineIDOnDecodeError(dim string) bool {
	switch dim {
	case "auth_failures", "usm_failures", "unknown_engine_id":
		return true
	default:
		return false
	}
}

func (c *Collector) localEngineIDMatches(sp gosnmp.SnmpV3SecurityParameters) bool {
	if c.localEngineID == nil || sp == nil {
		return false
	}
	usp, ok := sp.(*gosnmp.UsmSecurityParameters)
	if !ok {
		return false
	}
	return c.localEngineID.EqualRaw(usp.AuthoritativeEngineID)
}

func (c *Collector) warnf(format string, args ...any) {
	if c.Logger != nil {
		c.Warningf(format, args...)
	}
}

func (c *Collector) warnPlaintextOTLP(runtimeCfg otlpRuntimeConfig) {
	if !runtimeCfg.insecure || otlpTargetIsLoopback(runtimeCfg.target) {
		return
	}
	c.warnf("SNMP trap OTLP endpoint %q uses plaintext transport; use https:// for remote collectors", runtimeCfg.target)
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

func (c *Collector) logListenerReadError(ep EndpointConfig, err error) {
	if c.Logger == nil {
		return
	}
	endpoint := listenerEndpointLogName(ep)
	c.Limit(listenerReadErrorLogKeyPrefix+endpoint, 1, listenerReadErrorLogEvery).
		Warningf("SNMP trap listener read failed (endpoint=%s): %v", endpoint, err)
}

func (c *Collector) applyOverrides(td *TrapDef) *TrapDef {
	if td == nil || len(c.overrides) == 0 {
		return td
	}
	ov, ok := c.overrides[td.OID]
	if !ok {
		return td
	}
	cp := *td
	if td.Labels != nil {
		cp.Labels = make(map[string]string, len(td.Labels)+len(ov.Labels))
		maps.Copy(cp.Labels, td.Labels)
	}
	if ov.Category != "" {
		cp.Category = ov.Category
	}
	if ov.Severity != "" {
		cp.Severity = ov.Severity
	}
	if ov.Labels != nil {
		if cp.Labels == nil {
			cp.Labels = make(map[string]string, len(ov.Labels))
		}
		maps.Copy(cp.Labels, ov.Labels)
	}
	return &cp
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

func (c *Collector) versionAllowed(version SnmpVersion) bool {
	if len(c.versions) == 0 {
		return true
	}
	_, ok := c.versions[version]
	return ok
}

func (c *Collector) communityAllowed(community string) bool {
	if c.allowlist != nil && !c.allowlist.AllowedCommunity(community) {
		return false
	}
	return true
}

func versionSet(versions []string) map[SnmpVersion]struct{} {
	set := make(map[SnmpVersion]struct{}, len(versions))
	for _, version := range versions {
		set[SnmpVersion(version)] = struct{}{}
	}
	return set
}

func versionListContains(versions []string, version string) bool {
	return slices.Contains(versions, version)
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
