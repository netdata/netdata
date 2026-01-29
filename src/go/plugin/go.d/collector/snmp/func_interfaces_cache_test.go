// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestCalcRate(t *testing.T) {
	tests := map[string]struct {
		current      int64
		previous     int64
		elapsed      time.Duration
		wantNil      bool
		wantRate     float64
		wantPositive bool // for wrap cases where exact value is hard to predict
	}{
		"simple rate": {
			current:  1000,
			previous: 0,
			elapsed:  time.Second,
			wantRate: 1000.0,
		},
		"rate over 10 seconds": {
			current:  1000,
			previous: 0,
			elapsed:  10 * time.Second,
			wantRate: 100.0,
		},
		"rate with non-zero previous": {
			current:  5000,
			previous: 1000,
			elapsed:  2 * time.Second,
			wantRate: 2000.0,
		},
		"zero delta": {
			current:  1000,
			previous: 1000,
			elapsed:  time.Second,
			wantRate: 0.0,
		},
		"counter wrap small": {
			current:      100,
			previous:     math.MaxInt64 - 100,
			elapsed:      time.Second,
			wantPositive: true,
		},
		"counter wrap from max to zero": {
			current:      0,
			previous:     math.MaxInt64,
			elapsed:      time.Second,
			wantPositive: true,
		},
		"zero elapsed": {
			current:  1000,
			previous: 0,
			elapsed:  0,
			wantNil:  true,
		},
		"negative elapsed": {
			current:  1000,
			previous: 0,
			elapsed:  -time.Second,
			wantNil:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := calcRate(tc.current, tc.previous, tc.elapsed)
			if tc.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				if tc.wantPositive {
					assert.Greater(t, *result, 0.0)
				} else {
					assert.InDelta(t, tc.wantRate, *result, 0.001)
				}
			}
		})
	}
}

func TestExtractStatus(t *testing.T) {
	tests := map[string]struct {
		mv       map[string]int64
		expected string
	}{
		"single active up": {
			mv:       map[string]int64{"up": 1, "down": 0, "testing": 0},
			expected: "up",
		},
		"single active down": {
			mv:       map[string]int64{"up": 0, "down": 1, "testing": 0},
			expected: "down",
		},
		"none active": {
			mv:       map[string]int64{"up": 0, "down": 0, "testing": 0},
			expected: "unknown",
		},
		"empty map": {
			mv:       map[string]int64{},
			expected: "unknown",
		},
		"nil map": {
			mv:       nil,
			expected: "unknown",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, extractStatus(tc.mv))
		})
	}
}

func TestIsIfaceMetric(t *testing.T) {
	tests := map[string]struct {
		name     string
		expected bool
	}{
		"ifTraffic":          {name: "ifTraffic", expected: true},
		"ifPacketsUcast":     {name: "ifPacketsUcast", expected: true},
		"ifPacketsBroadcast": {name: "ifPacketsBroadcast", expected: true},
		"ifPacketsMulticast": {name: "ifPacketsMulticast", expected: true},
		"ifErrors":           {name: "ifErrors", expected: true},
		"ifDiscards":         {name: "ifDiscards", expected: true},
		"ifAdminStatus":      {name: "ifAdminStatus", expected: true},
		"ifOperStatus":       {name: "ifOperStatus", expected: true},
		"sysUptime":          {name: "sysUptime", expected: false},
		"empty":              {name: "", expected: false},
		"random":             {name: "someOtherMetric", expected: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isIfaceMetric(tc.name))
		})
	}
}

func TestIfaceCache(t *testing.T) {
	tests := map[string]struct {
		setup    func(c *Collector)
		validate func(t *testing.T, c *Collector)
	}{
		"new interface": {
			setup: func(c *Collector) {
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0", tagIfType: "ethernetCsmacd"},
					MultiValue: map[string]int64{"in": 1000, "out": 2000},
				})
				c.finalizeIfaceCache()
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				require.Len(t, c.ifaceCache.interfaces, 1)
				entry := c.ifaceCache.interfaces["eth0"]
				require.NotNil(t, entry)

				assert.Equal(t, "eth0", entry.name)
				assert.Equal(t, "ethernetCsmacd", entry.ifType)
				assert.Equal(t, int64(1000), entry.counters.trafficIn)
				assert.Equal(t, int64(2000), entry.counters.trafficOut)
				assert.True(t, entry.hasPrev)
				assert.Nil(t, entry.rates.trafficIn)
				assert.Nil(t, entry.rates.trafficOut)
			},
		},
		"update existing with rates": {
			setup: func(c *Collector) {
				// First collection
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0"},
					MultiValue: map[string]int64{"in": 1000, "out": 2000},
				})
				c.finalizeIfaceCache()

				time.Sleep(10 * time.Millisecond)

				// Second collection
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0"},
					MultiValue: map[string]int64{"in": 2000, "out": 4000},
				})
				c.finalizeIfaceCache()
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				entry := c.ifaceCache.interfaces["eth0"]
				require.NotNil(t, entry)

				assert.Equal(t, int64(2000), entry.counters.trafficIn)
				assert.Equal(t, int64(4000), entry.counters.trafficOut)
				require.NotNil(t, entry.rates.trafficIn)
				require.NotNil(t, entry.rates.trafficOut)
				assert.Greater(t, *entry.rates.trafficIn, 0.0)
				assert.Greater(t, *entry.rates.trafficOut, 0.0)
			},
		},
		"remove stale interface": {
			setup: func(c *Collector) {
				// First collection with two interfaces
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0"},
					MultiValue: map[string]int64{"in": 1000, "out": 2000},
				})
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth1"},
					MultiValue: map[string]int64{"in": 3000, "out": 4000},
				})
				c.finalizeIfaceCache()

				// Second collection with only eth0
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0"},
					MultiValue: map[string]int64{"in": 2000, "out": 3000},
				})
				c.finalizeIfaceCache()
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				assert.Len(t, c.ifaceCache.interfaces, 1)
				_, ok := c.ifaceCache.interfaces["eth0"]
				assert.True(t, ok)
				_, ok = c.ifaceCache.interfaces["eth1"]
				assert.False(t, ok)
			},
		},
		"multiple metrics for same interface": {
			setup: func(c *Collector) {
				c.resetIfaceCache()
				metrics := []ddsnmp.Metric{
					{Name: "ifTraffic", IsTable: true, Tags: map[string]string{tagInterface: "eth0", tagIfType: "ethernetCsmacd"}, MultiValue: map[string]int64{"in": 1000, "out": 2000}},
					{Name: "ifPacketsUcast", IsTable: true, Tags: map[string]string{tagInterface: "eth0"}, MultiValue: map[string]int64{"in": 100, "out": 200}},
					{Name: "ifPacketsBroadcast", IsTable: true, Tags: map[string]string{tagInterface: "eth0"}, MultiValue: map[string]int64{"in": 10, "out": 20}},
					{Name: "ifPacketsMulticast", IsTable: true, Tags: map[string]string{tagInterface: "eth0"}, MultiValue: map[string]int64{"in": 5, "out": 15}},
					{Name: "ifAdminStatus", IsTable: true, Tags: map[string]string{tagInterface: "eth0"}, MultiValue: map[string]int64{"up": 1, "down": 0}},
					{Name: "ifOperStatus", IsTable: true, Tags: map[string]string{tagInterface: "eth0"}, MultiValue: map[string]int64{"up": 1, "down": 0}},
				}
				for _, m := range metrics {
					c.updateIfaceCacheEntry(m)
				}
				c.finalizeIfaceCache()
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				require.Len(t, c.ifaceCache.interfaces, 1)
				entry := c.ifaceCache.interfaces["eth0"]
				require.NotNil(t, entry)

				assert.Equal(t, int64(1000), entry.counters.trafficIn)
				assert.Equal(t, int64(2000), entry.counters.trafficOut)
				assert.Equal(t, int64(100), entry.counters.ucastPktsIn)
				assert.Equal(t, int64(200), entry.counters.ucastPktsOut)
				assert.Equal(t, int64(10), entry.counters.bcastPktsIn)
				assert.Equal(t, int64(20), entry.counters.bcastPktsOut)
				assert.Equal(t, int64(5), entry.counters.mcastPktsIn)
				assert.Equal(t, int64(15), entry.counters.mcastPktsOut)
				assert.Equal(t, "up", entry.adminStatus)
				assert.Equal(t, "up", entry.operStatus)
				assert.Equal(t, "ethernetCsmacd", entry.ifType)
			},
		},
		"first collection rates nil": {
			setup: func(c *Collector) {
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0"},
					MultiValue: map[string]int64{"in": 1000, "out": 2000},
				})
				c.finalizeIfaceCache()
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				entry := c.ifaceCache.interfaces["eth0"]
				require.NotNil(t, entry)

				assert.Nil(t, entry.rates.trafficIn)
				assert.Nil(t, entry.rates.trafficOut)
				assert.Nil(t, entry.rates.ucastPktsIn)
				assert.Nil(t, entry.rates.ucastPktsOut)
				assert.Nil(t, entry.rates.bcastPktsIn)
				assert.Nil(t, entry.rates.bcastPktsOut)
				assert.Nil(t, entry.rates.mcastPktsIn)
				assert.Nil(t, entry.rates.mcastPktsOut)
			},
		},
		"missing interface tag ignored": {
			setup: func(c *Collector) {
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{},
					MultiValue: map[string]int64{"in": 1000, "out": 2000},
				})
				c.finalizeIfaceCache()
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				assert.Len(t, c.ifaceCache.interfaces, 0)
			},
		},
		"reset marks all as not updated": {
			setup: func(c *Collector) {
				c.resetIfaceCache()
				c.updateIfaceCacheEntry(ddsnmp.Metric{
					Name:       "ifTraffic",
					IsTable:    true,
					Tags:       map[string]string{tagInterface: "eth0"},
					MultiValue: map[string]int64{"in": 1000, "out": 2000},
				})
				c.finalizeIfaceCache()
				c.resetIfaceCache() // reset again without finalize
			},
			validate: func(t *testing.T, c *Collector) {
				c.ifaceCache.mu.RLock()
				defer c.ifaceCache.mu.RUnlock()

				entry := c.ifaceCache.interfaces["eth0"]
				require.NotNil(t, entry)
				assert.False(t, entry.updated)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{
				ifaceCache: newIfaceCache(),
			}
			tc.setup(c)
			tc.validate(t, c)
		})
	}
}

func TestIfaceCacheNilSafety(t *testing.T) {
	c := &Collector{
		ifaceCache: nil,
	}

	// None of these should panic
	c.resetIfaceCache()
	c.updateIfaceCacheEntry(ddsnmp.Metric{
		Name:       "ifTraffic",
		IsTable:    true,
		Tags:       map[string]string{tagInterface: "eth0"},
		MultiValue: map[string]int64{"in": 1000, "out": 2000},
	})
	c.finalizeIfaceCache()
}

func TestNewIfaceCache(t *testing.T) {
	cache := newIfaceCache()
	require.NotNil(t, cache)
	require.NotNil(t, cache.interfaces)
	assert.Len(t, cache.interfaces, 0)
}
