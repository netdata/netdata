// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"math"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// Interface metric names we track for the function.
var ifaceMetricNames = map[string]bool{
	"ifTraffic":          true,
	"ifPacketsUcast":     true,
	"ifPacketsBroadcast": true,
	"ifPacketsMulticast": true,
	"ifAdminStatus":      true,
	"ifOperStatus":       true,
}

// Tag keys used to identify interfaces.
const (
	tagInterface = "interface"
	tagIfType    = "_if_type"
	tagIfTypeGrp = "_if_type_group"
)

// ifaceCache holds interface metrics between collections for function queries.
type ifaceCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	updateTime time.Time              // current collection cycle time
	interfaces map[string]*ifaceEntry // key: interface name from m.Tags["interface"]
}

// ifaceEntry holds metrics for a single network interface.
type ifaceEntry struct {
	// Identity
	name        string // interface name (from Tags["interface"])
	ifType      string // interface type (from Tags["_if_type"])
	ifTypeGroup string // interface type group (from Tags["_if_type_group"])

	// Status (text values extracted from MultiValue)
	adminStatus string
	operStatus  string

	// Raw counter values (cumulative, stored for next delta calculation)
	counters ifaceCounters

	// Previous counter values (for delta calculation)
	prevCounters ifaceCounters
	prevTime     time.Time
	hasPrev      bool // true if we have previous values for delta calculation

	// Computed rates (per-second, nil if not yet calculable)
	rates ifaceRates

	// Tracking
	updated bool // true if seen in current collection cycle
}

// ifaceCounters holds raw cumulative counter values.
type ifaceCounters struct {
	trafficIn    int64
	trafficOut   int64
	ucastPktsIn  int64
	ucastPktsOut int64
	bcastPktsIn  int64
	bcastPktsOut int64
	mcastPktsIn  int64
	mcastPktsOut int64
}

// ifaceRates holds computed per-second rates.
type ifaceRates struct {
	trafficIn    *float64
	trafficOut   *float64
	ucastPktsIn  *float64
	ucastPktsOut *float64
	bcastPktsIn  *float64
	bcastPktsOut *float64
	mcastPktsIn  *float64
	mcastPktsOut *float64
}

// newIfaceCache creates a new interface cache.
func newIfaceCache() *ifaceCache {
	return &ifaceCache{
		interfaces: make(map[string]*ifaceEntry),
	}
}

// isIfaceMetric returns true if the metric name is one we track for interface function.
func isIfaceMetric(name string) bool {
	return ifaceMetricNames[name]
}

// resetIfaceCache prepares the cache for a new collection cycle.
// Must be called before processing metrics.
func (c *Collector) resetIfaceCache() {
	if c.ifaceCache == nil {
		return
	}

	c.ifaceCache.mu.Lock()
	defer c.ifaceCache.mu.Unlock()

	c.ifaceCache.updateTime = time.Now()

	for _, entry := range c.ifaceCache.interfaces {
		entry.updated = false
	}
}

// updateIfaceCacheEntry updates the cache with a single interface metric.
// Called during collectProfileTableMetrics for matching metrics.
// Caller must ensure m.IsTable is true and m.Tags["interface"] is not empty.
func (c *Collector) updateIfaceCacheEntry(m ddsnmp.Metric) {
	if c.ifaceCache == nil {
		return
	}

	ifaceName := m.Tags[tagInterface]
	if ifaceName == "" {
		return
	}

	c.ifaceCache.mu.Lock()
	defer c.ifaceCache.mu.Unlock()

	entry := c.ifaceCache.interfaces[ifaceName]
	if entry == nil {
		entry = &ifaceEntry{
			name: ifaceName,
		}
		c.ifaceCache.interfaces[ifaceName] = entry
	}

	if ifType := m.Tags[tagIfType]; ifType != "" {
		entry.ifType = ifType
	}
	if ifTypeGroup := m.Tags[tagIfTypeGrp]; ifTypeGroup != "" {
		entry.ifTypeGroup = ifTypeGroup
	}

	switch m.Name {
	case "ifTraffic":
		if v, ok := m.MultiValue["in"]; ok {
			entry.counters.trafficIn = v
		}
		if v, ok := m.MultiValue["out"]; ok {
			entry.counters.trafficOut = v
		}
	case "ifPacketsUcast":
		if v, ok := m.MultiValue["in"]; ok {
			entry.counters.ucastPktsIn = v
		}
		if v, ok := m.MultiValue["out"]; ok {
			entry.counters.ucastPktsOut = v
		}
	case "ifPacketsBroadcast":
		if v, ok := m.MultiValue["in"]; ok {
			entry.counters.bcastPktsIn = v
		}
		if v, ok := m.MultiValue["out"]; ok {
			entry.counters.bcastPktsOut = v
		}
	case "ifPacketsMulticast":
		if v, ok := m.MultiValue["in"]; ok {
			entry.counters.mcastPktsIn = v
		}
		if v, ok := m.MultiValue["out"]; ok {
			entry.counters.mcastPktsOut = v
		}
	case "ifAdminStatus":
		entry.adminStatus = extractStatus(m.MultiValue)
	case "ifOperStatus":
		entry.operStatus = extractStatus(m.MultiValue)
	}

	entry.updated = true
}

// finalizeIfaceCache removes stale entries and calculates rates.
// Must be called after all metrics have been processed.
func (c *Collector) finalizeIfaceCache() {
	if c.ifaceCache == nil {
		return
	}

	c.ifaceCache.mu.Lock()
	defer c.ifaceCache.mu.Unlock()

	now := c.ifaceCache.updateTime

	for name, entry := range c.ifaceCache.interfaces {
		if !entry.updated {
			delete(c.ifaceCache.interfaces, name)
			continue
		}

		if entry.hasPrev {
			elapsed := now.Sub(entry.prevTime)
			entry.rates.trafficIn = calcRate(entry.counters.trafficIn, entry.prevCounters.trafficIn, elapsed)
			entry.rates.trafficOut = calcRate(entry.counters.trafficOut, entry.prevCounters.trafficOut, elapsed)
			entry.rates.ucastPktsIn = calcRate(entry.counters.ucastPktsIn, entry.prevCounters.ucastPktsIn, elapsed)
			entry.rates.ucastPktsOut = calcRate(entry.counters.ucastPktsOut, entry.prevCounters.ucastPktsOut, elapsed)
			entry.rates.bcastPktsIn = calcRate(entry.counters.bcastPktsIn, entry.prevCounters.bcastPktsIn, elapsed)
			entry.rates.bcastPktsOut = calcRate(entry.counters.bcastPktsOut, entry.prevCounters.bcastPktsOut, elapsed)
			entry.rates.mcastPktsIn = calcRate(entry.counters.mcastPktsIn, entry.prevCounters.mcastPktsIn, elapsed)
			entry.rates.mcastPktsOut = calcRate(entry.counters.mcastPktsOut, entry.prevCounters.mcastPktsOut, elapsed)
		}

		entry.prevCounters = entry.counters
		entry.prevTime = now
		entry.hasPrev = true
	}

	c.ifaceCache.lastUpdate = now
}

// calcRate computes per-second rate from counter delta.
// Returns nil if rate cannot be calculated (zero or negative elapsed time).
// Handles counter wrap by treating values as unsigned.
func calcRate(current, previous int64, elapsed time.Duration) *float64 {
	if elapsed <= 0 {
		return nil
	}

	// Treat as unsigned for proper counter wrap handling
	ucurrent := uint64(current)
	uprevious := uint64(previous)

	var delta uint64
	if ucurrent >= uprevious {
		delta = ucurrent - uprevious
	} else {
		// Counter wrap - calculate wrapped delta
		delta = (math.MaxUint64 - uprevious) + ucurrent + 1
	}

	rate := float64(delta) / elapsed.Seconds()
	return &rate
}

// extractStatus finds the active status from a MultiValue map.
// Returns the key where value == 1, or "unknown" if none found.
func extractStatus(mv map[string]int64) string {
	for k, v := range mv {
		if v == 1 {
			return k
		}
	}
	return "unknown"
}
