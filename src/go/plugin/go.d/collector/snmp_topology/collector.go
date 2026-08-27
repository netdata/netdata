// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"net/netip"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

//go:embed "config_schema.json"
var configSchema string

// Register registers the SNMP topology collector with shared SNMP-family state.
func Register(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) {
	collectorapi.Register("snmp_topology", newCreator(deviceStore, trapEnrichment, reverseDNS))
}

func newCreator(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) collectorapi.Creator {
	if deviceStore == nil {
		panic("snmp_topology Register requires a non-nil device store")
	}
	if trapEnrichment == nil {
		panic("snmp_topology Register requires a non-nil trap enrichment handle")
	}
	if reverseDNS == nil {
		panic("snmp_topology Register requires a non-nil reverse DNS resolver")
	}
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 60,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return newCollector(deviceStore, trapEnrichment, reverseDNS) },
		Config:          func() any { return &Config{} },
		InstancePolicy:  collectorapi.InstancePolicySingle,
		SharedFunctions: topologyMethods,
		MethodHandler:   topologyFunctionHandler,
	}
}

// New returns an SNMP topology collector using the provided SNMP-family state.
func New(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) *Collector {
	return newCollector(deviceStore, trapEnrichment, reverseDNS)
}

func newCollector(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle, reverseDNS *reversedns.Resolver) *Collector {
	if deviceStore == nil {
		panic("snmp_topology New requires a non-nil device store")
	}
	if trapEnrichment == nil {
		panic("snmp_topology New requires a non-nil trap enrichment handle")
	}
	if reverseDNS == nil {
		panic("snmp_topology New requires a non-nil reverse DNS resolver")
	}
	metricStore := metrix.NewCollectorStore()
	return &Collector{
		deviceStates:     make(map[ddsnmp.DeviceRegistrationID]deviceRefreshState),
		topologyRegistry: newTopologyRegistryWithResolver(reverseDNS),
		deviceSource:     deviceStore,
		trapEnrichment:   trapEnrichment,
		newSnmpClient:    gosnmp.NewHandler,
		newDdSnmpColl: func(cfg ddsnmpcollector.Config) ddCollector {
			return ddsnmpcollector.New(cfg)
		},
		resolveTargetIPs: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
		now:     time.Now,
		store:   metricStore,
		metrics: newCollectorMetrics(metricStore),
	}
}

type (
	Collector struct {
		collectorapi.Base `yaml:",inline"`
		Config            `yaml:",inline"`

		deviceStates       map[ddsnmp.DeviceRegistrationID]deviceRefreshState
		generationSequence uint64
		topologyRegistry   *topologyRegistry
		deviceSource       deviceSource
		trapEnrichment     *TrapEnrichmentHandle

		refreshMu sync.Mutex
		statsMu   sync.RWMutex
		stats     collectorRuntimeStats

		store   metrix.CollectorStore
		metrics *collectorMetrics

		topologyProfiles   func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile
		newSnmpClient      func() gosnmp.Handler
		newDdSnmpColl      func(ddsnmpcollector.Config) ddCollector
		resolveTargetIPs   func(context.Context, string) ([]netip.Addr, error)
		now                func() time.Time
		diagnosticRecorder *diagnostic.Recorder
	}
	deviceSource interface {
		Entries() []ddsnmp.DeviceEntry
	}
	ddCollector interface {
		Collect() ([]*ddsnmp.ProfileMetrics, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	return nil
}

func (c *Collector) Check(context.Context) error {
	return nil
}

func (c *Collector) Collect(context.Context) error {
	c.writeInternalMetrics(time.Now())
	return nil
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) Run(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return nil
	}
	c.publishTrapTopologyEnrichment()
	defer c.unpublishTrapTopologyEnrichment()
	c.topologyRegistry.setReverseDNSWarmContext(ctx)
	defer c.topologyRegistry.setReverseDNSWarmContext(nil)

	c.refreshTopologyRecovering(ctx)
	c.topologyRegistry.enqueueReverseDNSWarmFromDefaultSnapshot()

	ticker := time.NewTicker(c.deviceCheckEvery())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.refreshTopologyRecovering(ctx)
			c.topologyRegistry.enqueueReverseDNSWarmFromDefaultSnapshot()
		}
	}
}

const (
	defaultDeviceCheckEvery        = time.Minute
	defaultRefreshEvery            = 30 * time.Minute
	topologyTargetLookupMaxTimeout = 5 * time.Second
	topologyTargetLookupMaxWorkers = 8
	topologyRefreshWarningEvery    = time.Hour
)

const (
	topologyRefreshFailureClient     = "client"
	topologyRefreshFailureConnect    = "connect"
	topologyRefreshFailureCollection = "collection"
)

type topologyRefreshDevicePlan struct {
	registrationID      ddsnmp.DeviceRegistrationID
	device              ddsnmp.DeviceConnectionInfo
	targetManagementIPs []netip.Addr
	targetResolution    string
	diagnosticIndex     int
}

type deviceRefreshOutcome uint8

const (
	deviceRefreshOutcomeUnknown deviceRefreshOutcome = iota
	deviceRefreshOutcomeSuccess
	deviceRefreshOutcomeNoProfiles
	deviceRefreshOutcomeFailed
)

type deviceRefreshState struct {
	generation          *topologyDeviceGeneration
	lastAttempt         time.Time
	lastSuccess         time.Time
	nextRetry           time.Time
	outcome             deviceRefreshOutcome
	consecutiveFailures uint32
}

func (c *Collector) deviceCheckEvery() time.Duration {
	if c.UpdateEvery > 0 {
		return time.Duration(c.UpdateEvery) * time.Second
	}
	return defaultDeviceCheckEvery
}

func (c *Collector) refreshEvery() time.Duration {
	if d := c.RefreshEvery.Duration(); d > 0 {
		return d
	}
	return defaultRefreshEvery
}

func (c *Collector) Cleanup(context.Context) {
	c.unpublishTrapTopologyEnrichment()

	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.deviceStates = make(map[ddsnmp.DeviceRegistrationID]deviceRefreshState)
	c.topologyRegistry.publishGeneration(nil)
	c.recordCleanupStats()
}

func (c *Collector) refreshTopology(ctx context.Context) refreshStats {
	start := c.currentTime()
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	entries := c.getRegisteredDevices()
	previousGeneration := c.topologyRegistry.acquireGeneration()
	diagnosticSweep := newTopologyDiagnosticRefreshSweep(c.diagnosticRecorder, start, entries, previousGeneration)
	defer func() {
		if recovered := recover(); recovered != nil {
			diagnosticSweep.terminalizePending(
				diagnostic.RefreshOutcomePanicInFlight,
				diagnostic.RefreshOutcomePanicNotStarted,
			)
			diagnosticSweep.finishUnpublished(c.currentTime(), diagnostic.RefreshPublicationPanic)
			panic(recovered)
		}
	}()
	refreshEvery := c.refreshEvery()
	now := c.currentTime()
	seen := make(map[ddsnmp.DeviceRegistrationID]bool, len(entries))
	stats := refreshStats{
		hasDeviceCounts:   true,
		registeredDevices: len(entries),
	}
	nextStates := cloneDeviceRefreshStates(c.deviceStates)
	successfulSnapshots := make(map[ddsnmp.DeviceRegistrationID]*topologyDeviceSnapshot)

	plans := make([]topologyRefreshDevicePlan, 0, len(entries))
	for index, entry := range entries {
		registrationID := entry.RegistrationID
		dev := entry.Info
		seen[registrationID] = true

		state, exists := nextStates[registrationID]
		if !exists || state.nextRetry.IsZero() || !now.Before(state.nextRetry) {
			diagnosticSweep.markDue(index)
			plans = append(plans, topologyRefreshDevicePlan{
				registrationID:   registrationID,
				device:           dev,
				targetResolution: diagnostic.TargetResolutionNotAttempted,
				diagnosticIndex:  index,
			})
		}
	}

	c.resolveTopologyTargetManagementIPs(
		ctx,
		plans,
		topologyTargetLookupMaxTimeout,
		topologyTargetLookupMaxWorkers,
	)
	for _, plan := range plans {
		diagnosticSweep.setTargetResolution(plan.diagnosticIndex, plan.targetResolution)
	}

	for _, plan := range plans {
		if ctx.Err() != nil {
			break
		}
		diagnosticSweep.start(plan.diagnosticIndex)
		attemptedAt := c.currentTime()
		state := nextStates[plan.registrationID]
		state.lastAttempt = attemptedAt
		snapshot, outcome, terminalOutcome := c.refreshDeviceTopology(
			ctx,
			plan.registrationID,
			plan.device,
			plan.targetManagementIPs,
		)
		diagnosticSweep.complete(plan.diagnosticIndex, terminalOutcome)
		if ctx.Err() != nil {
			break
		}
		completedAt := c.currentTime()

		state.outcome = outcome
		switch outcome {
		case deviceRefreshOutcomeSuccess:
			successfulSnapshots[plan.registrationID] = snapshot
			state.lastSuccess = completedAt
			state.consecutiveFailures = 0
			state.nextRetry = completedAt.Add(refreshEvery)
		case deviceRefreshOutcomeNoProfiles:
			state.consecutiveFailures = 0
			state.nextRetry = completedAt.Add(refreshEvery)
		default:
			if ctx.Err() != nil {
				break
			}
			stats.errors++
			state.consecutiveFailures++
			state.nextRetry = completedAt.Add(failedRefreshRetryDelay(
				c.deviceCheckEvery(),
				refreshEvery,
				state.consecutiveFailures,
			))
		}
		nextStates[plan.registrationID] = state
	}

	if ctx.Err() != nil {
		diagnosticSweep.terminalizePending(
			diagnostic.RefreshOutcomeCanceledInFlight,
			diagnostic.RefreshOutcomeCanceledNotStarted,
		)
		diagnosticSweep.finishUnpublished(c.currentTime(), diagnostic.RefreshPublicationCanceled)
		stats.cachedDevices = previousGeneration.deviceCount()
		stats.completedAt = c.currentTime()
		stats.duration = stats.completedAt.Sub(start)
		return stats
	}

	pruneUnregisteredDeviceStates(nextStates, seen)
	publishedAt := c.currentTime()
	for registrationID, snapshot := range successfulSnapshots {
		state := nextStates[registrationID]
		state.generation = activateTopologyDeviceSnapshot(registrationID, publishedAt, snapshot)
		nextStates[registrationID] = state
	}
	c.deviceStates = nextStates
	c.generationSequence++
	generation := newTopologyGeneration(c.generationSequence, publishedAt, nextStates)
	generation.diagnosticMember = diagnosticSweep.finishPublished(c, publishedAt, generation, nextStates, successfulSnapshots)
	c.topologyRegistry.publishGeneration(generation)
	stats.cachedDevices = generation.deviceCount()
	stats.completedAt = c.currentTime()
	stats.duration = stats.completedAt.Sub(start)
	return stats
}

func (c *Collector) getRegisteredDevices() []ddsnmp.DeviceEntry {
	if c.deviceSource == nil {
		return nil
	}
	return c.deviceSource.Entries()
}

func (c *Collector) refreshTopologyRecovering(ctx context.Context) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			c.recordRefreshStats(refreshStats{
				errors:      1,
				completedAt: time.Now(),
				duration:    time.Since(start),
			})
			c.Errorf("PANIC: %v", r)
			if logger.Level.Enabled(slog.LevelDebug) {
				c.Errorf("STACK: %s", debug.Stack())
			}
		}
	}()

	c.recordRefreshStats(c.refreshTopology(ctx))
}

// refreshDeviceTopology builds one immutable collected snapshot without
// changing the generation currently visible to readers.
func (c *Collector) refreshDeviceTopology(
	ctx context.Context,
	registrationID ddsnmp.DeviceRegistrationID,
	dev ddsnmp.DeviceConnectionInfo,
	targetManagementIPs []netip.Addr,
) (*topologyDeviceSnapshot, deviceRefreshOutcome, string) {
	if ctx.Err() != nil {
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
	}
	diagnosticCapture := c.beginTopologyDiagnosticCapture(registrationID)
	if diagnosticCapture != nil {
		defer diagnosticCapture.abort()
	}

	snmpClient, err := newSNMPClientFromDeviceInfo(c.newSnmpClient, dev)
	if err != nil {
		c.warnTopologyRefreshFailure(registrationID, topologyRefreshFailureClient,
			"device '%s': failed to create SNMP client: %v", dev.Hostname, err)
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeClientFailed
	}
	if dev.MaxRepetitions != 0 {
		snmpClient.SetMaxRepetitions(dev.MaxRepetitions)
	}
	if err := snmpClient.Connect(); err != nil {
		if ctx.Err() != nil {
			return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
		}
		c.warnTopologyRefreshFailure(registrationID, topologyRefreshFailureConnect,
			"device '%s': failed to connect: %v", dev.Hostname, err)
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeConnectFailed
	}
	stopContextClose := closeSNMPClientOnContextCancel(ctx, snmpClient)
	defer stopContextClose()
	defer func() { _ = snmpClient.Close() }()

	if ctx.Err() != nil {
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
	}

	profiles := c.getTopologyProfiles(dev)
	if len(profiles) == 0 {
		return nil, deviceRefreshOutcomeNoProfiles, diagnostic.RefreshOutcomeNoProfiles
	}

	if ctx.Err() != nil {
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
	}

	coll := c.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient:      snmpClient,
		Profiles:        profiles,
		Log:             c.Logger,
		SysObjectID:     dev.SysObjectID,
		DisableBulkWalk: dev.DisableBulkWalk,
	})

	pms, err := coll.Collect()
	if err != nil {
		if ctx.Err() != nil {
			return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
		}
		c.warnTopologyRefreshFailure(registrationID, topologyRefreshFailureCollection,
			"device '%s': topology collection failed: %v", dev.Hostname, err)
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCollectionFailed
	}

	if ctx.Err() != nil {
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
	}

	sysUptime, err := snmputils.GetSysUptime(snmpClient)
	if err != nil && ctx.Err() == nil {
		c.Debugf("device '%s': failed to query system uptime: %v", dev.Hostname, err)
	}

	if ctx.Err() != nil {
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
	}

	// Build the next device generation off-registry. Function readers keep
	// seeing the previous global generation until collection is fully ingested.
	next := c.newDeviceTopologyBuilder(dev)
	next.targetManagementIPs = append([]netip.Addr(nil), targetManagementIPs...)
	if diagnosticCapture != nil {
		diagnosticCapture.setDevice(next, targetManagementIPs, sysUptime, profiles)
	}

	var semanticObserver topologySemanticObserver
	if diagnosticCapture != nil {
		semanticObserver = diagnosticCapture.observe
	}
	applyTopologySemanticStream(next, newTopologyMainSemanticStream(sysUptime, pms), semanticObserver)
	var vlanOutcomeObserver func(topologyVLANContextResult)
	if diagnosticCapture != nil {
		vlanOutcomeObserver = diagnosticCapture.captureVLANContext
	}
	c.collectTopologyVTPVLANContexts(ctx, next.vtpVLANContexts(), dev, func(result topologyVLANContextResult) {
		consumeTopologySemanticEvent(next, topologySemanticEvent{
			kind: topologySemanticEventVLANContext,
			vlan: result,
		}, nil)
	}, vlanOutcomeObserver)
	if ctx.Err() != nil {
		return nil, deviceRefreshOutcomeFailed, diagnostic.RefreshOutcomeCanceledInFlight
	}
	snapshot := c.freezeTopologyBuilder(next)
	if diagnosticCapture != nil {
		diagnosticCapture.commit(snapshot)
	}
	return snapshot, deviceRefreshOutcomeSuccess, diagnostic.RefreshOutcomeSuccess
}

func (c *Collector) warnTopologyRefreshFailure(registrationID ddsnmp.DeviceRegistrationID, class, format string, args ...any) {
	c.Limit("snmp_topology:refresh:"+registrationID.String()+":"+class, 1, topologyRefreshWarningEvery).Warningf(format, args...)
}

func (c *Collector) resolveTopologyTargetManagementIPs(
	ctx context.Context,
	plans []topologyRefreshDevicePlan,
	budget time.Duration,
	maxWorkers int,
) {
	type lookupJob struct {
		planIndex      int
		registrationID ddsnmp.DeviceRegistrationID
	}

	jobs := make([]lookupJob, 0, len(plans))
	for i := range plans {
		host := strings.TrimSpace(plans[i].device.Hostname)
		if host == "" {
			plans[i].targetResolution = diagnostic.TargetResolutionFailed
			continue
		}
		if addr, isIP := parseTopologyIPAddress(host); isIP {
			plans[i].targetManagementIPs = normalizeTargetManagementIPs([]netip.Addr{addr})
			plans[i].targetResolution = diagnostic.TargetResolutionLiteral
			continue
		}
		if c.resolveTargetIPs != nil {
			jobs = append(jobs, lookupJob{planIndex: i, registrationID: plans[i].registrationID})
		}
	}
	if len(jobs) == 0 || budget <= 0 || maxWorkers <= 0 || ctx.Err() != nil {
		return
	}

	sort.SliceStable(jobs, func(i, j int) bool {
		if jobs[i].registrationID != jobs[j].registrationID {
			return jobs[i].registrationID < jobs[j].registrationID
		}
		return jobs[i].planIndex < jobs[j].planIndex
	})

	lookupCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	jobCh := make(chan lookupJob, len(jobs))
	for _, job := range jobs {
		jobCh <- job
	}
	close(jobCh)

	var wg sync.WaitGroup
	for range min(maxWorkers, len(jobs)) {
		wg.Go(func() {
			for job := range jobCh {
				if lookupCtx.Err() != nil {
					if ctx.Err() == nil {
						plans[job.planIndex].targetResolution = diagnostic.TargetResolutionTimedOut
					}
					continue
				}
				plans[job.planIndex].targetManagementIPs, plans[job.planIndex].targetResolution = c.resolveDeviceTargetManagementIPsWithState(
					lookupCtx,
					plans[job.planIndex].device,
				)
			}
		})
	}
	wg.Wait()
}

func closeSNMPClientOnContextCancel(ctx context.Context, client gosnmp.Handler) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func (c *Collector) newDeviceTopologyBuilder(dev ddsnmp.DeviceConnectionInfo) *topologyBuilder {
	cache := newTopologyBuilder()
	cache.updateTime = c.currentTime()
	cache.staleAfter = c.refreshEvery() + 2*c.deviceCheckEvery()
	cache.agentID = dev.Hostname
	cache.localDevice = buildLocalTopologyDevice(dev)
	return cache
}

func (c *Collector) currentTime() time.Time {
	if c != nil && c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Collector) resolveDeviceTargetManagementIPsWithState(
	ctx context.Context,
	dev ddsnmp.DeviceConnectionInfo,
) ([]netip.Addr, string) {
	host := strings.TrimSpace(dev.Hostname)
	if host == "" {
		return nil, diagnostic.TargetResolutionFailed
	}
	if addr, isIP := parseTopologyIPAddress(host); isIP {
		return normalizeTargetManagementIPs([]netip.Addr{addr}), diagnostic.TargetResolutionLiteral
	}
	if c.resolveTargetIPs == nil {
		return nil, diagnostic.TargetResolutionNotAttempted
	}

	timeout := topologyTargetLookupMaxTimeout
	if dev.Timeout > 0 {
		timeout = min(time.Duration(dev.Timeout)*time.Second, topologyTargetLookupMaxTimeout)
	}
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addrs, err := c.resolveTargetIPs(lookupCtx, host)
	if err != nil {
		if ctx.Err() == nil {
			c.Debugf("device '%s': failed to resolve topology management target: %v", host, err)
		}
		if errors.Is(err, context.DeadlineExceeded) || (lookupCtx.Err() != nil && ctx.Err() == nil) {
			return nil, diagnostic.TargetResolutionTimedOut
		}
		return nil, diagnostic.TargetResolutionFailed
	}
	return normalizeTargetManagementIPs(addrs), diagnostic.TargetResolutionResolved
}

func cloneDeviceRefreshStates(states map[ddsnmp.DeviceRegistrationID]deviceRefreshState) map[ddsnmp.DeviceRegistrationID]deviceRefreshState {
	cloned := make(map[ddsnmp.DeviceRegistrationID]deviceRefreshState, len(states))
	maps.Copy(cloned, states)
	return cloned
}

func pruneUnregisteredDeviceStates(states map[ddsnmp.DeviceRegistrationID]deviceRefreshState, seen map[ddsnmp.DeviceRegistrationID]bool) {
	for registrationID := range states {
		if !seen[registrationID] {
			delete(states, registrationID)
		}
	}
}

func failedRefreshRetryDelay(checkEvery, refreshEvery time.Duration, consecutiveFailures uint32) time.Duration {
	if checkEvery <= 0 {
		checkEvery = defaultDeviceCheckEvery
	}
	if refreshEvery <= 0 {
		refreshEvery = defaultRefreshEvery
	}
	if checkEvery >= refreshEvery {
		return refreshEvery
	}

	delay := checkEvery
	for failure := uint32(1); failure < consecutiveFailures; failure++ {
		if delay >= refreshEvery/2 {
			return refreshEvery
		}
		delay *= 2
	}
	return min(delay, refreshEvery)
}

func (c *Collector) findTopologyProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	return ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID:    dev.SysObjectID,
		SysDescr:       dev.SysDescr,
		ManualProfiles: dev.ManualProfiles,
		ManualPolicy:   ddsnmp.ManualProfileAugment,
	}).Project(ddsnmp.ConsumerTopology, ddsnmp.ConsumerBGP).FilterBGPToTopologyPeers().Profiles()
}

func (c *Collector) getTopologyProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	if c.topologyProfiles != nil {
		return c.topologyProfiles(dev)
	}
	return c.findTopologyProfiles(dev)
}

func newSNMPClientFromDeviceInfo(newClient func() gosnmp.Handler, dev ddsnmp.DeviceConnectionInfo) (gosnmp.Handler, error) {
	client := newClient()

	client.SetTarget(dev.Hostname)
	client.SetPort(uint16(dev.Port))
	client.SetRetries(dev.Retries)
	client.SetTimeout(time.Duration(dev.Timeout) * time.Second)
	client.SetMaxOids(dev.MaxOIDs)
	client.SetMaxRepetitions(uint32(dev.MaxRepetitions))

	ver := snmputils.ParseSNMPVersion(dev.SNMPVersion)

	switch ver {
	case gosnmp.Version1:
		client.SetCommunity(dev.Community)
		client.SetVersion(gosnmp.Version1)
	case gosnmp.Version2c:
		client.SetCommunity(dev.Community)
		client.SetVersion(gosnmp.Version2c)
	case gosnmp.Version3:
		if dev.V3User == "" {
			return nil, fmt.Errorf("username is required for SNMPv3")
		}
		client.SetVersion(gosnmp.Version3)
		client.SetSecurityModel(gosnmp.UserSecurityModel)
		client.SetMsgFlags(snmputils.ParseSNMPv3SecurityLevel(dev.V3SecurityLevel))
		client.SetSecurityParameters(&gosnmp.UsmSecurityParameters{
			UserName:                 dev.V3User,
			AuthenticationProtocol:   snmputils.ParseSNMPv3AuthProtocol(dev.V3AuthProto),
			AuthenticationPassphrase: dev.V3AuthKey,
			PrivacyProtocol:          snmputils.ParseSNMPv3PrivProtocol(dev.V3PrivProto),
			PrivacyPassphrase:        dev.V3PrivKey,
		})
		client.SetContextName(dev.V3ContextName)
	default:
		return nil, fmt.Errorf("invalid SNMP version: %s", dev.SNMPVersion)
	}

	return client, nil
}
