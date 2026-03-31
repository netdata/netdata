// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

const (
	defaultTopologyRefreshEvery         = 30 * time.Minute
	defaultTopologyStaleAfterMultiplier = 2
)

func (c *Collector) topologyRefreshEvery() time.Duration {
	if c == nil {
		return defaultTopologyRefreshEvery
	}
	if d := c.Topology.RefreshEvery.Duration(); d > 0 {
		return d
	}
	return defaultTopologyRefreshEvery
}

func (c *Collector) topologyRefreshEveryString() string {
	return confopt.LongDuration(c.topologyRefreshEvery()).String()
}

func (c *Collector) topologyStaleAfter() time.Duration {
	if c == nil {
		return time.Duration(defaultTopologyStaleAfterMultiplier) * defaultTopologyRefreshEvery
	}
	refreshEvery := c.topologyRefreshEvery()
	if d := c.Topology.StaleAfter.Duration(); d > 0 {
		if d < refreshEvery {
			return refreshEvery
		}
		return d
	}
	return time.Duration(defaultTopologyStaleAfterMultiplier) * refreshEvery
}

func (c *Collector) ensureTopologySchedulerStarted() {
	if c == nil || c.topologyCache == nil {
		return
	}

	c.topologyMu.Lock()
	if c.topologyRunning {
		c.topologyMu.Unlock()
		return
	}

	c.topologyProfiles = selectTopologyRefreshProfiles(c.snmpProfiles)
	if len(c.topologyProfiles) == 0 {
		c.topologyMu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	refreshEvery := c.topologyRefreshEvery()

	c.topologyCancel = cancel
	c.topologyRunning = true
	c.topologyWG.Add(1)
	go func() {
		defer c.topologyWG.Done()
		c.runTopologyRefreshLoop(ctx, refreshEvery)
	}()
	c.topologyMu.Unlock()
}

func (c *Collector) stopTopologyScheduler() {
	if c == nil {
		return
	}

	c.topologyMu.Lock()
	cancel := c.topologyCancel
	running := c.topologyRunning
	c.topologyCancel = nil
	c.topologyRunning = false
	c.topologyMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if running {
		c.topologyWG.Wait()
	}
}

func (c *Collector) runTopologyRefreshLoop(ctx context.Context, refreshEvery time.Duration) {
	c.refreshTopologySnapshotWithLogging()

	ticker := time.NewTicker(refreshEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.refreshTopologySnapshotWithLogging()
		}
	}
}

func (c *Collector) refreshTopologySnapshotWithLogging() {
	if err := c.refreshTopologySnapshot(); err != nil {
		c.Warningf("topology refresh failed: %v", err)
	}
}

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

func selectTopologyRefreshProfiles(profiles []*ddsnmp.Profile) []*ddsnmp.Profile {
	if len(profiles) == 0 {
		return nil
	}

	selected := make([]*ddsnmp.Profile, 0, len(profiles))
	for _, prof := range profiles {
		if profileContainsTopologyData(prof) {
			selected = append(selected, prof)
		}
	}

	return selected
}

func profileContainsTopologyData(prof *ddsnmp.Profile) bool {
	if prof == nil || prof.Definition == nil {
		return false
	}

	for i := range prof.Definition.Metrics {
		if metricConfigContainsTopologyData(&prof.Definition.Metrics[i]) {
			return true
		}
	}

	return false
}

func metricConfigContainsTopologyData(metric *ddprofiledefinition.MetricsConfig) bool {
	if metric == nil {
		return false
	}

	if name := firstNonEmpty(metric.Symbol.Name, metric.Name); isTopologyMetric(name) || isTopologySysUptimeMetric(name) {
		return true
	}

	for i := range metric.Symbols {
		name := metric.Symbols[i].Name
		if isTopologyMetric(name) || isTopologySysUptimeMetric(name) {
			return true
		}
	}

	return false
}

func (c *Collector) topologyRefreshDescription() string {
	return fmt.Sprintf("every %s, stale after %s", c.topologyRefreshEveryString(), confopt.LongDuration(c.topologyStaleAfter()).String())
}
