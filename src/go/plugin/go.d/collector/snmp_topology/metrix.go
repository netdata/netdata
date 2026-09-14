// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const internalMetricPrefix = "netdata.go.plugin.collector.snmp_topology"

type collectorMetrics struct {
	devicesRegistered       metrix.SnapshotGauge
	devicesCached           metrix.SnapshotGauge
	lastRefreshAgeSeconds   metrix.SnapshotGauge
	lastRefreshDurationSecs metrix.SnapshotGauge
	refreshRuns             metrix.SnapshotCounter
	refreshErrors           metrix.SnapshotCounter
}

type collectorRuntimeStats struct {
	registeredDevices int
	cachedDevices     int
	lastRefresh       time.Time
	lastDuration      time.Duration
	refreshRuns       uint64
	refreshErrors     uint64
}

type refreshStats struct {
	hasDeviceCounts   bool
	registeredDevices int
	cachedDevices     int
	errors            int
	completedAt       time.Time
	duration          time.Duration
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter(internalMetricPrefix)
	return &collectorMetrics{
		devicesRegistered:       meter.Gauge("devices_registered"),
		devicesCached:           meter.Gauge("devices_cached"),
		lastRefreshAgeSeconds:   meter.Gauge("last_refresh_age_seconds"),
		lastRefreshDurationSecs: meter.Gauge("last_refresh_duration_seconds"),
		refreshRuns:             meter.Counter("refresh_runs_total"),
		refreshErrors:           meter.Counter("refresh_errors_total"),
	}
}

func (c *Collector) recordRefreshStats(stats refreshStats) {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	if stats.hasDeviceCounts {
		c.stats.registeredDevices = stats.registeredDevices
		c.stats.cachedDevices = stats.cachedDevices
	}
	c.stats.lastRefresh = stats.completedAt
	c.stats.lastDuration = stats.duration
	c.stats.refreshRuns++
	c.stats.refreshErrors += uint64(stats.errors)
}

func (c *Collector) recordCleanupStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()

	c.stats.registeredDevices = 0
	c.stats.cachedDevices = 0
}

func (c *Collector) writeInternalMetrics(now time.Time) {
	stats := c.runtimeStatsSnapshot(now)

	c.metrics.devicesRegistered.Observe(float64(stats.registeredDevices))
	c.metrics.devicesCached.Observe(float64(stats.cachedDevices))
	c.metrics.lastRefreshAgeSeconds.Observe(stats.lastRefreshAgeSeconds)
	c.metrics.lastRefreshDurationSecs.Observe(stats.lastRefreshDurationSeconds)
	c.metrics.refreshRuns.ObserveTotal(float64(stats.refreshRuns))
	c.metrics.refreshErrors.ObserveTotal(float64(stats.refreshErrors))
}

type runtimeStatsSnapshot struct {
	registeredDevices          int
	cachedDevices              int
	lastRefreshAgeSeconds      float64
	lastRefreshDurationSeconds float64
	refreshRuns                uint64
	refreshErrors              uint64
}

func (c *Collector) runtimeStatsSnapshot(now time.Time) runtimeStatsSnapshot {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()

	var ageSeconds float64
	if !c.stats.lastRefresh.IsZero() {
		ageSeconds = now.Sub(c.stats.lastRefresh).Seconds()
		if ageSeconds < 0 {
			ageSeconds = 0
		}
	}

	return runtimeStatsSnapshot{
		registeredDevices:          c.stats.registeredDevices,
		cachedDevices:              c.stats.cachedDevices,
		lastRefreshAgeSeconds:      ageSeconds,
		lastRefreshDurationSeconds: c.stats.lastDuration.Seconds(),
		refreshRuns:                c.stats.refreshRuns,
		refreshErrors:              c.stats.refreshErrors,
	}
}
