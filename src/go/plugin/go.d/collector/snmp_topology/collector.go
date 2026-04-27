// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("snmp_topology", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 60,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
		Config: func() any { return &Config{} },
	})

	// Register the topology function handler and method config so the snmp module
	// can serve topology:snmp requests under the snmp:topology:snmp function name.
	ddsnmp.TopologyHandler = &funcTopology{}
	cfg := topologyMethodConfig()
	ddsnmp.TopologyMethodConfig = &cfg
}

func New() *Collector {
	return &Collector{
		deviceCaches:        make(map[string]*topologyCache),
		deviceLastCollected: make(map[string]time.Time),
		newSnmpClient:       gosnmp.NewHandler,
		newDdSnmpColl: func(cfg ddsnmpcollector.Config) ddCollector {
			return ddsnmpcollector.New(cfg)
		},
	}
}

type (
	Collector struct {
		collectorapi.Base `yaml:",inline"`
		Config            `yaml:",inline"`

		charts              *collectorapi.Charts
		deviceCaches        map[string]*topologyCache // one cache per SNMP device
		deviceLastCollected map[string]time.Time      // last collection time per device
		topologyCache       *topologyCache            // current device cache (set during refreshDeviceTopology)
		topologyChartsAdded bool

		newSnmpClient func() gosnmp.Handler
		newDdSnmpColl func(ddsnmpcollector.Config) ddCollector
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

func (c *Collector) Charts() *collectorapi.Charts {
	if c.charts == nil {
		c.charts = &collectorapi.Charts{}
	}
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	if devices := ddsnmp.DeviceRegistry.Devices(); len(devices) > 0 {
		refreshEvery := c.refreshEvery()
		now := time.Now()
		seen := make(map[string]bool, len(devices))

		for _, dev := range devices {
			key := fmt.Sprintf("%s:%d", dev.Hostname, dev.Port)
			seen[key] = true

			lastCollected, exists := c.deviceLastCollected[key]
			isNew := !exists
			isStale := exists && now.Sub(lastCollected) >= refreshEvery

			if isNew || isStale {
				c.refreshDeviceTopology(key, dev)
				c.deviceLastCollected[key] = now
			}
		}

		c.pruneStaleDeviceCaches(seen)
	}

	mx := make(map[string]int64)
	c.collectTopologyMetrics(mx)
	return mx
}

const defaultRefreshEvery = 30 * time.Minute

func (c *Collector) refreshEvery() time.Duration {
	if d := c.RefreshEvery.Duration(); d > 0 {
		return d
	}
	return defaultRefreshEvery
}

func (c *Collector) Cleanup(context.Context) {
	for key, cache := range c.deviceCaches {
		snmpTopologyRegistry.unregister(cache)
		delete(c.deviceCaches, key)
	}
}

// refreshDeviceTopology collects topology data for a single device into its own cache.
func (c *Collector) refreshDeviceTopology(key string, dev ddsnmp.DeviceConnectionInfo) {
	cache := c.getOrCreateDeviceCache(key, dev)

	snmpClient, err := newSNMPClientFromDeviceInfo(c.newSnmpClient, dev)
	if err != nil {
		c.Warningf("device '%s': failed to create SNMP client: %v", dev.Hostname, err)
		return
	}
	if dev.MaxRepetitions != 0 {
		snmpClient.SetMaxRepetitions(dev.MaxRepetitions)
	}
	if err := snmpClient.Connect(); err != nil {
		c.Warningf("device '%s': failed to connect: %v", dev.Hostname, err)
		return
	}
	defer func() { _ = snmpClient.Close() }()

	profiles := c.findTopologyProfiles(dev)
	if len(profiles) == 0 {
		return
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
		c.Warningf("device '%s': topology collection failed: %v", dev.Hostname, err)
		return
	}

	// Point c.topologyCache at this device's cache so the ingestion methods work.
	c.topologyCache = cache

	c.updateTopologyProfileTags(pms)
	c.ingestTopologyProfileMetrics(pms)
	c.collectTopologyVTPVLANContexts(dev)
	c.finalizeTopologyCache()

	c.topologyCache = nil
}

func (c *Collector) getOrCreateDeviceCache(key string, dev ddsnmp.DeviceConnectionInfo) *topologyCache {
	cache, ok := c.deviceCaches[key]
	if !ok {
		cache = newTopologyCache()
		c.deviceCaches[key] = cache
		snmpTopologyRegistry.register(cache)
	}

	// Reset cache for fresh collection cycle.
	cache.mu.Lock()
	cache.updateTime = time.Now()
	cache.lastUpdate = time.Time{}
	cache.staleAfter = c.refreshEvery() + time.Duration(c.UpdateEvery*2)*time.Second
	cache.agentID = dev.Hostname
	cache.localDevice = buildLocalTopologyDevice(dev)
	cache.lldpLocPorts = make(map[string]*lldpLocPort)
	cache.lldpRemotes = make(map[string]*lldpRemote)
	cache.cdpRemotes = make(map[string]*cdpRemote)
	cache.ifNamesByIndex = make(map[string]string)
	cache.ifStatusByIndex = make(map[string]ifStatus)
	cache.ifIndexByIP = make(map[string]string)
	cache.ifNetmaskByIP = make(map[string]string)
	cache.bridgePortToIf = make(map[string]string)
	cache.fdbEntries = make(map[string]*fdbEntry)
	cache.fdbIDToVlanID = make(map[string]string)
	cache.vlanIDToName = make(map[string]string)
	cache.vtpVersion = ""
	cache.stpBaseBridgeAddress = ""
	cache.stpDesignatedRoot = ""
	cache.stpPorts = make(map[string]*stpPortEntry)
	cache.arpEntries = make(map[string]*arpEntry)
	cache.mu.Unlock()

	return cache
}

func (c *Collector) pruneStaleDeviceCaches(seen map[string]bool) {
	for key, cache := range c.deviceCaches {
		if !seen[key] {
			snmpTopologyRegistry.unregister(cache)
			delete(c.deviceCaches, key)
			delete(c.deviceLastCollected, key)
		}
	}
}

func (c *Collector) findTopologyProfiles(dev ddsnmp.DeviceConnectionInfo) []*ddsnmp.Profile {
	return selectTopologyRefreshProfiles(ddsnmp.FindProfiles(dev.SysObjectID, dev.SysDescr, dev.ManualProfiles))
}

func (c *Collector) ingestTopologyProfileMetrics(pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		c.ingestTopologyMetricSet(pm.HiddenMetrics)
		c.ingestTopologyMetricSet(pm.Metrics)
	}
}

func (c *Collector) ingestTopologyMetricSet(metrics []ddsnmp.Metric) {
	for _, metric := range metrics {
		switch {
		case ddsnmp.IsTopologyMetric(metric.Name):
			c.updateTopologyCacheEntry(metric)
		case ddsnmp.IsTopologySysUptimeMetric(metric.Name):
			c.updateTopologyScalarMetric(metric)
		}
	}
}

// collectTopologyMetrics reads the aggregated topology from the global registry.
func (c *Collector) collectTopologyMetrics(mx map[string]int64) {
	if !c.topologyChartsAdded {
		c.addTopologyCharts()
		c.topologyChartsAdded = true
	}

	data, ok := snmpTopologyRegistry.snapshot()
	if !ok {
		mx["snmp_topology_devices_total"] = 0
		mx["snmp_topology_devices_discovered"] = 0
		mx["snmp_topology_links_total"] = 0
		mx["snmp_topology_links_lldp"] = 0
		mx["snmp_topology_links_cdp"] = 0
		mx["snmp_topology_links_stp"] = 0
		return
	}

	totalDevices := 0
	for _, actor := range data.Actors {
		if topologyengine.IsDeviceActorType(actor.ActorType) {
			totalDevices++
		}
	}

	var lldpLinks, cdpLinks, stpLinks int64
	for _, link := range data.Links {
		switch link.Protocol {
		case "lldp":
			lldpLinks++
		case "cdp":
			cdpLinks++
		case "stp":
			stpLinks++
		}
	}

	mx["snmp_topology_devices_total"] = int64(totalDevices)
	mx["snmp_topology_devices_discovered"] = int64(maxInt(totalDevices-1, 0))
	mx["snmp_topology_links_total"] = int64(len(data.Links))
	mx["snmp_topology_links_lldp"] = lldpLinks
	mx["snmp_topology_links_cdp"] = cdpLinks
	mx["snmp_topology_links_stp"] = stpLinks
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
	return &funcTopology{}
}
