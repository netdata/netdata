// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func (c *Collector) refreshTopologySnapshot() error {
	if c == nil || len(c.topologyProfiles) == 0 {
		return nil
	}

	cache, err := c.collectTopologySnapshot()
	if err != nil {
		return err
	}

	c.publishTopologySnapshot(cache)
	return nil
}

func (c *Collector) collectTopologySnapshot() (*topologyCache, error) {
	snmpClient, err := c.newConfiguredSNMPClient()
	if err != nil {
		return nil, err
	}
	if c.adjMaxRepetitions != 0 {
		snmpClient.SetMaxRepetitions(c.adjMaxRepetitions)
	}
	if err := snmpClient.Connect(); err != nil {
		return nil, err
	}
	defer func() {
		_ = snmpClient.Close()
	}()

	worker := c.newTopologyWorker(snmpClient)
	worker.resetTopologyCache()

	pms, err := worker.ddSnmpColl.Collect()
	if err != nil {
		return nil, err
	}

	worker.updateTopologyProfileTags(pms)
	worker.ingestTopologyProfileMetrics(pms)
	worker.collectTopologyVTPVLANContexts()
	worker.finalizeTopologyCache()

	return worker.topologyCache, nil
}

func (c *Collector) newTopologyWorker(snmpClient gosnmp.Handler) *Collector {
	worker := &Collector{
		Base:              c.Base,
		Config:            c.Config,
		vnode:             c.vnode,
		topologyCache:     newTopologyCache(),
		snmpClient:        snmpClient,
		newSnmpClient:     c.newSnmpClient,
		newDdSnmpColl:     c.newDdSnmpColl,
		sysInfo:           c.sysInfo,
		adjMaxRepetitions: c.adjMaxRepetitions,
		disableBulkWalk:   c.disableBulkWalk,
	}
	worker.ddSnmpColl = worker.newDdSnmpColl(ddsnmpcollector.Config{
		SnmpClient:      snmpClient,
		Profiles:        c.topologyProfiles,
		Log:             c.Logger,
		SysObjectID:     c.sysInfo.SysObjectID,
		DisableBulkWalk: c.disableBulkWalk,
	})

	return worker
}

func (c *Collector) ingestTopologyProfileMetrics(pms []*ddsnmp.ProfileMetrics) {
	for _, pm := range pms {
		for _, metric := range pm.Metrics {
			switch {
			case isTopologyMetric(metric.Name):
				c.updateTopologyCacheEntry(metric)
			case isTopologySysUptimeMetric(metric.Name):
				c.updateTopologyScalarMetric(metric)
			}
		}
	}
}

func (c *Collector) publishTopologySnapshot(cache *topologyCache) {
	if c == nil || c.topologyCache == nil || cache == nil {
		return
	}

	c.topologyCache.mu.Lock()
	c.topologyCache.replaceWith(cache)
	c.topologyCache.mu.Unlock()
}
