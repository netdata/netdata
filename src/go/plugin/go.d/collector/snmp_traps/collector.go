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
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

const (
	trapWriteFailureJournal = "journal_write_failed"
	trapWriteFailureOTLP    = "otlp_export_failed"
)

var activeDirectJournalJobs atomic.Int64

func directJournalLogsAvailable() bool {
	return activeDirectJournalJobs.Load() > 0
}

func init() {
	collectorapi.Register("snmp_traps", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2:      func() collectorapi.CollectorV2 { return New() },
		Config:        func() any { return &Config{} },
		Methods:       snmpTrapsMethods,
		MethodHandler: snmpTrapsMethodHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			Versions: []string{"v1", "v2c"},
		},
		store: store,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	listener           *Listener
	trapWriter         TrapWriter
	journalDir         string
	store              metrix.CollectorStore
	jobName            string
	vnode              string
	versions           map[SnmpVersion]struct{}
	allowlist          *Allowlist
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
	profileCacheHeld   bool
	operatorMetrics    *operatorMetrics
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

	if err := validateEndpoints(c.Listen.Endpoints); err != nil {
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

	if err := validateRateLimit(c.RateLimit); err != nil {
		return dyncfgConfigError(err)
	}
	if err := validateDedupConfig(c.Dedup); err != nil {
		return dyncfgConfigError(err)
	}
	if _, err := validateOTLPConfig(c.OTLP); err != nil {
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
	c.operatorMetrics = nil
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
	if err := validateMetrics(c.Metrics, idx); err != nil {
		releaseProfiles()
		return dyncfgConfigError(err)
	}
	if len(c.Metrics) > 0 {
		tmpl, err := buildChartTemplateYAML(c.Metrics)
		if err != nil {
			releaseProfiles()
			return dyncfgConfigError(err)
		}
		c.dynamicChartYAML = tmpl
	}

	var journalWriter *JournalWriter
	if journalEnabled {
		if err := validatePersistentJournalRoot(); err != nil {
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

	listener, err := newListener(c.jobName, c.Listen.Endpoints)
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
		engineBootsExisted := engineStatePathExists(engineBootsPath(c.jobName))
		localEngineIDExisted := engineStatePathExists(localEngineIDPath(c.jobName))
		engineStateDirExisted := engineStatePathExists(engineBootsDir(c.jobName))
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
	var secondaryWriter TrapWriter
	if c.OTLP.Enabled {
		secondaryWriter, err = newOTLPTrapWriter(ctx, c.jobName, c.OTLP, metrics)
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
		primaryWriter = newJournalTrapWriter(journalWriter, defaultQueueCapacity)
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
	if len(c.Metrics) > 0 {
		c.operatorMetrics = newOperatorMetrics(c.Metrics, idx)
	}
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
	c.operatorMetrics = nil
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
	if c.operatorMetrics != nil {
		c.operatorMetrics.collect(c.store, c.jobName)
	}
	return nil
}

func (c *Collector) handlePacket(data []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr) {
	decodePeerIP := peerIP
	rateLimitChecked := false
	if peer != nil {
		srcAddr, ok := udpPeerAddr(peer)
		if ok {
			decodePeerIP = net.IP(srcAddr.AsSlice())
			if c.allowlist != nil && !c.allowlist.AllowedSource(srcAddr) {
				c.incTrapError("dropped_allowlist")
				return
			}
		}
	}

	if version, ok := sniffSNMPVersion(data); ok && !c.versionAllowed(version) {
		c.incTrapError("dropped_allowlist")
		return
	}

	pktCtx, err := c.decodeTrapWithSharedTable(data, decodePeerIP)
	if err != nil {
		dim := ClassifyDecodeError(err)
		if c.v3SecTable != nil {
			rawCtx, rawErr := extractRawV3Context(data)
			if rawErr == nil && rawCtx != nil {
				if c.dynamicEngineID && !rawCtx.reportable {
					retryCtx, checked, dropped := c.tryDynamicRetry(data, decodePeerIP, peer, rawCtx)
					rateLimitChecked = rateLimitChecked || checked
					if dropped {
						return
					}
					if retryCtx != nil {
						pktCtx = retryCtx
						err = nil
					}
				} else if rawCtx.discoveryProbe() && conn != nil && peer != nil {
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
			return
		}
	}

	pdu := pktCtx.PDU

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

	if c.rateLimiter != nil && peer != nil && !rateLimitChecked {
		srcAddr, ok := udpPeerAddr(peer)
		if ok {
			allowed, mode := c.rateLimiter.Allow(srcAddr)
			if !allowed {
				c.incTrapError("rate_limited")
				if mode == rateLimitModeDrop {
					return
				}
			}
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
	enrichTrapEntry(entry, c.reverseDNSEnabled, c.reverseDNS)
	renderTrapEntryTemplates(entry, td)
	if unknownOID {
		c.incTrapError("unknown_oid")
	}
	if trapEntryHasUnresolvedTemplate(entry) {
		c.incTrapError("template_unresolved")
	}
	var admission dedupAdmission
	if c.deduper != nil {
		var suppressed bool
		admission, suppressed = c.deduper.Admit(entry, td, c.Dedup.KeyVarbinds)
		if suppressed {
			return
		}
	}
	if err := c.trapWriter.Write(entry); err != nil {
		if c.deduper != nil {
			c.deduper.Rollback(admission)
		}
		c.incTrapError(c.trapWriteFailureDim())
		return
	}

	if c.operatorMetrics != nil {
		c.operatorMetrics.inc(entry.TrapOID, entry, td)
	}

	cat := Category("unknown")
	if td != nil {
		cat = Category(td.Category)
	}
	c.incTrapEvents(cat)
	c.incTrapSeverity(entry.Severity)
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

func (e *dyncfgCodedError) Error() string   { return e.err.Error() }
func (e *dyncfgCodedError) Unwrap() error   { return e.err }
func (e *dyncfgCodedError) Code() int       { return e.code }
func (e *dyncfgCodedError) Retryable() bool { return e.retryable }
