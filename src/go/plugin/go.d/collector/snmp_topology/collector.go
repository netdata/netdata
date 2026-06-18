// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

//go:embed "config_schema.json"
var configSchema string

// Register registers the SNMP topology collector with shared SNMP-family state.
func Register(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle) {
	collectorapi.Register("snmp_topology", newCreator(deviceStore, trapEnrichment))
}

func newCreator(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle) collectorapi.Creator {
	if deviceStore == nil {
		panic("snmp_topology Register requires a non-nil device store")
	}
	if trapEnrichment == nil {
		panic("snmp_topology Register requires a non-nil trap enrichment handle")
	}
	return collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 60,
		},
		CreateV2:       func() collectorapi.CollectorV2 { return New(deviceStore, trapEnrichment) },
		Config:         func() any { return &Config{} },
		InstancePolicy: collectorapi.InstancePolicySingle,
		Methods:        topologyMethods,
		MethodHandler:  topologyFunctionHandler,
	}
}

// New returns an SNMP topology collector. Nil dependencies create isolated state for direct use.
func New(deviceStore *ddsnmp.DeviceStore, trapEnrichment *TrapEnrichmentHandle) *Collector {
	if deviceStore == nil {
		deviceStore = ddsnmp.NewDeviceStore()
	}
	if trapEnrichment == nil {
		trapEnrichment = NewTrapEnrichmentHandle()
	}
	metricStore := metrix.NewCollectorStore()
	return &Collector{
		deviceCaches:        make(map[string]*topologyCache),
		deviceLastCollected: make(map[string]time.Time),
		topologyRegistry:    newTopologyRegistry(),
		deviceSource:        deviceStore,
		trapEnrichment:      trapEnrichment,
		newSnmpClient:       gosnmp.NewHandler,
		newDdSnmpColl: func(cfg ddsnmpcollector.Config) ddCollector {
			return ddsnmpcollector.New(cfg)
		},
		store:   metricStore,
		metrics: newCollectorMetrics(metricStore),
	}
}

type (
	Collector struct {
		collectorapi.Base `yaml:",inline"`
		Config            `yaml:",inline"`

		deviceCaches        map[string]*topologyCache // one cache per SNMP device
		deviceLastCollected map[string]time.Time      // last collection time per device
		topologyCache       *topologyCache            // current device cache (set during refreshDeviceTopology)
		topologyRegistry    *topologyRegistry
		deviceSource        deviceSource
		trapEnrichment      *TrapEnrichmentHandle

		refreshMu sync.Mutex
		statsMu   sync.RWMutex
		stats     collectorRuntimeStats

		store   metrix.CollectorStore
		metrics *collectorMetrics

		topologyProfiles func(ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile
		newSnmpClient    func() gosnmp.Handler
		newDdSnmpColl    func(ddsnmpcollector.Config) ddCollector
	}
	deviceSource interface {
		Devices() []ddsnmp.DeviceConnectionInfo
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

	c.refreshTopologyRecovering(ctx)

	ticker := time.NewTicker(c.deviceCheckEvery())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c.refreshTopologyRecovering(ctx)
		}
	}
}

const (
	defaultDeviceCheckEvery = time.Minute
	defaultRefreshEvery     = 30 * time.Minute
)

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

	for key, cache := range c.deviceCaches {
		c.topologyRegistry.unregister(cache)
		delete(c.deviceCaches, key)
		delete(c.deviceLastCollected, key)
	}
	c.recordCleanupStats()
}

func (c *Collector) refreshTopology(ctx context.Context) refreshStats {
	start := time.Now()
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	devices := c.getRegisteredDevices()
	refreshEvery := c.refreshEvery()
	now := time.Now()
	seen := make(map[string]bool, len(devices))
	stats := refreshStats{
		hasDeviceCounts:   true,
		registeredDevices: len(devices),
	}

	for _, dev := range devices {
		if ctx.Err() != nil {
			break
		}

		key := fmt.Sprintf("%s:%d", dev.Hostname, dev.Port)
		seen[key] = true

		lastCollected, exists := c.deviceLastCollected[key]
		isNew := !exists
		isStale := exists && now.Sub(lastCollected) >= refreshEvery

		if isNew || isStale {
			if !c.refreshDeviceTopology(ctx, key, dev) {
				if ctx.Err() != nil {
					break
				}
				stats.errors++
			}
			c.deviceLastCollected[key] = now
		}
	}

	if ctx.Err() == nil {
		c.pruneStaleDeviceCaches(seen)
	}
	stats.cachedDevices = len(c.deviceCaches)
	stats.completedAt = time.Now()
	stats.duration = stats.completedAt.Sub(start)
	return stats
}

func (c *Collector) getRegisteredDevices() []ddsnmp.DeviceConnectionInfo {
	if c.deviceSource == nil {
		return nil
	}
	return c.deviceSource.Devices()
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

// refreshDeviceTopology collects topology data for a single device into its own cache.
func (c *Collector) refreshDeviceTopology(ctx context.Context, key string, dev ddsnmp.DeviceConnectionInfo) bool {
	if ctx.Err() != nil {
		return false
	}

	snmpClient, err := newSNMPClientFromDeviceInfo(c.newSnmpClient, dev)
	if err != nil {
		c.Warningf("device '%s': failed to create SNMP client: %v", dev.Hostname, err)
		return false
	}
	if dev.MaxRepetitions != 0 {
		snmpClient.SetMaxRepetitions(dev.MaxRepetitions)
	}
	if err := snmpClient.Connect(); err != nil {
		if ctx.Err() != nil {
			return false
		}
		c.Warningf("device '%s': failed to connect: %v", dev.Hostname, err)
		return false
	}
	stopContextClose := closeSNMPClientOnContextCancel(ctx, snmpClient)
	defer stopContextClose()
	defer func() { _ = snmpClient.Close() }()

	if ctx.Err() != nil {
		return false
	}

	profiles := c.getTopologyProfiles(dev)
	if len(profiles) == 0 {
		return true
	}

	if ctx.Err() != nil {
		return false
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
			return false
		}
		c.Warningf("device '%s': topology collection failed: %v", dev.Hostname, err)
		return false
	}

	if ctx.Err() != nil {
		return false
	}

	sysUptime, err := snmputils.GetSysUptime(snmpClient)
	if err != nil && ctx.Err() == nil {
		c.Debugf("device '%s': failed to query system uptime: %v", dev.Hostname, err)
	}

	if ctx.Err() != nil {
		return false
	}

	// Build the next snapshot off-registry. Function readers keep seeing the
	// previous complete snapshot until this collection is fully ingested.
	next := c.newDeviceCollectionCache(dev)
	c.topologyCache = next
	defer func() { c.topologyCache = nil }()

	c.updateTopologySysUptime(sysUptime)
	c.updateTopologyProfileTags(pms)
	c.ingestTopologyProfileMetrics(pms)
	c.collectTopologyVTPVLANContexts(ctx, dev)
	if ctx.Err() != nil {
		return false
	}
	c.finalizeTopologyCache()

	cache := c.getOrCreateDeviceCache(key)
	cache.mu.Lock()
	cache.replaceWith(next)
	cache.mu.Unlock()
	return true
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

func (c *Collector) getOrCreateDeviceCache(key string) *topologyCache {
	cache, ok := c.deviceCaches[key]
	if !ok {
		cache = newTopologyCache()
		c.deviceCaches[key] = cache
		c.topologyRegistry.register(cache)
	}
	return cache
}

func (c *Collector) newDeviceCollectionCache(dev ddsnmp.DeviceConnectionInfo) *topologyCache {
	cache := newTopologyCache()
	cache.updateTime = time.Now()
	cache.staleAfter = c.refreshEvery() + 2*c.deviceCheckEvery()
	cache.agentID = dev.Hostname
	cache.localDevice = buildLocalTopologyDevice(dev)
	return cache
}

func (c *Collector) pruneStaleDeviceCaches(seen map[string]bool) {
	for key, cache := range c.deviceCaches {
		if !seen[key] {
			c.topologyRegistry.unregister(cache)
			delete(c.deviceCaches, key)
			delete(c.deviceLastCollected, key)
		}
	}
}

func (c *Collector) findTopologyProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	return ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID:    dev.SysObjectID,
		SysDescr:       dev.SysDescr,
		ManualProfiles: dev.ManualProfiles,
		ManualPolicy:   ddsnmp.ManualProfileAugment,
	}).Project(ddsnmp.ConsumerTopology).Profiles()
}

func (c *Collector) getTopologyProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	if c.topologyProfiles != nil {
		return c.topologyProfiles(dev)
	}
	return c.findTopologyProfiles(dev)
}

func (c *Collector) ingestTopologyProfileMetrics(pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		c.ingestTopologyMetricSet(pm.TopologyMetrics)
	}
}

func (c *Collector) ingestTopologyMetricSet(metrics []ddsnmp.Metric) {
	for _, metric := range metrics {
		c.updateTopologyCacheEntry(metric)
	}
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

func topologyMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		topologyMethodConfig(),
	}
}

func topologyFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	if job == nil {
		return nil
	}
	coll, ok := job.Collector().(*Collector)
	if !ok || coll == nil {
		return nil
	}
	return &funcTopology{registry: coll.topologyRegistry}
}
