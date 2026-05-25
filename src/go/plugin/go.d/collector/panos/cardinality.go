// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"fmt"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	defaultMaxBGPPeers                 = 512
	defaultMaxBGPPrefixFamiliesPerPeer = 4
	defaultMaxBGPVirtualRouters        = 256
	defaultMaxEnvironmentSensors       = 512
	defaultMaxLicenses                 = 64
	defaultMaxIPSecTunnels             = 1024
)

type cardinalityStats struct {
	Discovered        int
	Monitored         int
	OmittedBySelector int
	OmittedByLimit    int
}

type environmentSensor struct {
	kind  string
	entry environmentEntry
}

func (c *Collector) filterBGPPeers(peers []bgpPeer) ([]bgpPeer, cardinalityStats) {
	items := append([]bgpPeer(nil), peers...)
	sort.SliceStable(items, func(i, j int) bool {
		return bgpPeerSelectorValue(items[i]) < bgpPeerSelectorValue(items[j])
	})

	var stats cardinalityStats
	monitored := make([]bgpPeer, 0, len(items))
	for _, peer := range items {
		stats.Discovered++
		if !matchesEntity(c.bgpPeerMatcher, bgpPeerSelectorValue(peer)) {
			stats.OmittedBySelector++
			continue
		}
		if stats.Monitored >= c.MaxBGPPeers {
			stats.OmittedByLimit++
			continue
		}
		monitored = append(monitored, peer)
		stats.Monitored++
	}
	return monitored, stats
}

func (c *Collector) filterBGPPrefixFamilies(peers []bgpPeer) ([]bgpPeer, cardinalityStats) {
	var stats cardinalityStats
	monitoredPeers := make([]bgpPeer, 0, len(peers))

	for _, peer := range peers {
		counters := append([]bgpPrefixCounter(nil), peer.PrefixCounters...)
		sort.SliceStable(counters, func(i, j int) bool {
			return bgpPrefixFamilySelectorValue(peer, counters[i]) < bgpPrefixFamilySelectorValue(peer, counters[j])
		})

		monitoredPeer := peer
		monitoredPeer.PrefixCounters = nil
		var monitoredForPeer int
		for _, counter := range counters {
			stats.Discovered++
			if !matchesEntity(c.bgpPrefixFamilyMatcher, bgpPrefixFamilySelectorValue(peer, counter)) {
				stats.OmittedBySelector++
				continue
			}
			if monitoredForPeer >= c.MaxBGPPrefixFamiliesPerPeer {
				stats.OmittedByLimit++
				continue
			}
			monitoredPeer.PrefixCounters = append(monitoredPeer.PrefixCounters, counter)
			monitoredForPeer++
			stats.Monitored++
		}
		monitoredPeers = append(monitoredPeers, monitoredPeer)
	}

	return monitoredPeers, stats
}

func (c *Collector) filterEnvironmentSensors(env environmentMetrics) ([]environmentSensor, cardinalityStats) {
	items := env.sensors()
	sort.SliceStable(items, func(i, j int) bool {
		return environmentSensorSelectorValue(items[i].kind, items[i].entry) < environmentSensorSelectorValue(items[j].kind, items[j].entry)
	})

	var stats cardinalityStats
	monitored := make([]environmentSensor, 0, len(items))
	for _, sensor := range items {
		stats.Discovered++
		if !matchesEntity(c.environmentSensorMatcher, environmentSensorSelectorValue(sensor.kind, sensor.entry)) {
			stats.OmittedBySelector++
			continue
		}
		if stats.Monitored >= c.MaxEnvironmentSensors {
			stats.OmittedByLimit++
			continue
		}
		monitored = append(monitored, sensor)
		stats.Monitored++
	}
	return monitored, stats
}

func (c *Collector) filterLicenses(licenses []licenseEntry) ([]licenseEntry, cardinalityStats) {
	items := append([]licenseEntry(nil), licenses...)
	sort.SliceStable(items, func(i, j int) bool {
		return licenseSelectorValue(items[i]) < licenseSelectorValue(items[j])
	})

	var stats cardinalityStats
	monitored := make([]licenseEntry, 0, len(items))
	for _, license := range items {
		stats.Discovered++
		if !matchesEntity(c.licenseMatcher, licenseSelectorValue(license)) {
			stats.OmittedBySelector++
			continue
		}
		if stats.Monitored >= c.MaxLicenses {
			stats.OmittedByLimit++
			continue
		}
		monitored = append(monitored, license)
		stats.Monitored++
	}
	return monitored, stats
}

func (c *Collector) filterIPSecTunnels(tunnels []ipsecTunnel) ([]ipsecTunnel, cardinalityStats) {
	items := append([]ipsecTunnel(nil), tunnels...)
	sort.SliceStable(items, func(i, j int) bool {
		return ipsecTunnelSelectorValue(items[i]) < ipsecTunnelSelectorValue(items[j])
	})

	var stats cardinalityStats
	monitored := make([]ipsecTunnel, 0, len(items))
	for _, tunnel := range items {
		stats.Discovered++
		if !matchesEntity(c.ipsecTunnelMatcher, ipsecTunnelSelectorValue(tunnel)) {
			stats.OmittedBySelector++
			continue
		}
		if stats.Monitored >= c.MaxIPSecTunnels {
			stats.OmittedByLimit++
			continue
		}
		monitored = append(monitored, tunnel)
		stats.Monitored++
	}
	return monitored, stats
}

func (m environmentMetrics) sensors() []environmentSensor {
	total := len(m.ThermalEntries) + len(m.FanEntries) + len(m.VoltageEntries) + len(m.PowerSupplyEntries)
	sensors := make([]environmentSensor, 0, total)
	for _, entry := range m.ThermalEntries {
		sensors = append(sensors, environmentSensor{kind: "temperature", entry: entry})
	}
	for _, entry := range m.FanEntries {
		sensors = append(sensors, environmentSensor{kind: "fan", entry: entry})
	}
	for _, entry := range m.VoltageEntries {
		sensors = append(sensors, environmentSensor{kind: "voltage", entry: entry})
	}
	for _, entry := range m.PowerSupplyEntries {
		sensors = append(sensors, environmentSensor{kind: "power_supply", entry: entry})
	}
	return sensors
}

func matchesEntity(m matcher.Matcher, value string) bool {
	return m == nil || m.MatchString(value)
}

func bgpPeerSelectorValue(peer bgpPeer) string {
	return firstNonEmpty(peer.VR, "default") + "/" + firstNonEmpty(peer.PeerAddress, "unknown")
}

func bgpPrefixFamilySelectorValue(peer bgpPeer, counter bgpPrefixCounter) string {
	return bgpPeerSelectorValue(peer) + "/" + firstNonEmpty(counter.AFI, "unknown") + "/" + firstNonEmpty(counter.SAFI, "unknown")
}

func environmentSensorSelectorValue(kind string, entry environmentEntry) string {
	return kind + "/" + firstNonEmpty(entry.Slot, "unknown") + "/" + environmentSensorName(entry)
}

func licenseSelectorValue(entry licenseEntry) string {
	return firstNonEmpty(entry.Feature, "unknown")
}

func ipsecTunnelSelectorValue(tunnel ipsecTunnel) string {
	return firstNonEmpty(tunnel.Name, "unknown") + "/" + tunnel.Gateway + "/" + tunnel.Remote + "/" + firstNonEmpty(tunnel.TID, tunnel.ISPI, tunnel.OSPI)
}

func (c *Collector) collectBGPPeerCardinalityMetrics(mx map[string]int64, stats cardinalityStats) {
	c.addBGPPeersCollectionChart()
	writeCardinalityMetrics(mx, "bgp_peers_collection", stats)
	c.logCardinalityCap("BGP peers", "max_bgp_peers", "bgp_peers", c.MaxBGPPeers, stats)
}

func (c *Collector) collectBGPPrefixFamilyCardinalityMetrics(mx map[string]int64, stats cardinalityStats) {
	c.addBGPPrefixFamiliesCollectionChart()
	writeCardinalityMetrics(mx, "bgp_prefix_families_collection", stats)
	c.logCardinalityCap("BGP prefix families", "max_bgp_prefix_families_per_peer", "bgp_prefix_families", c.MaxBGPPrefixFamiliesPerPeer, stats)
}

func (c *Collector) collectBGPVirtualRouterCardinalityMetrics(mx map[string]int64, stats cardinalityStats) {
	c.addBGPVirtualRoutersCollectionChart()
	writeCardinalityMetrics(mx, "bgp_virtual_routers_collection", stats)
	c.logCardinalityCap("BGP virtual routers", "max_bgp_virtual_routers", "bgp_virtual_routers", c.MaxBGPVirtualRouters, stats)
}

func (c *Collector) collectEnvironmentSensorCardinalityMetrics(mx map[string]int64, stats cardinalityStats) {
	c.addEnvironmentSensorsCollectionChart()
	writeCardinalityMetrics(mx, "env_sensors_collection", stats)
	c.logCardinalityCap("environment sensors", "max_environment_sensors", "environment_sensors", c.MaxEnvironmentSensors, stats)
}

func (c *Collector) collectLicenseCardinalityMetrics(mx map[string]int64, stats cardinalityStats) {
	c.addLicensesCollectionChart()
	writeCardinalityMetrics(mx, "license_collection", stats)
	c.logCardinalityCap("licenses", "max_licenses", "licenses", c.MaxLicenses, stats)
}

func (c *Collector) collectIPSecTunnelCardinalityMetrics(mx map[string]int64, stats cardinalityStats) {
	c.addIPSecTunnelsCollectionChart()
	writeCardinalityMetrics(mx, "ipsec_tunnels_collection", stats)
	c.logCardinalityCap("IPsec tunnels", "max_ipsec_tunnels", "ipsec_tunnels", c.MaxIPSecTunnels, stats)
}

func writeCardinalityMetrics(mx map[string]int64, prefix string, stats cardinalityStats) {
	mx[prefix+"_discovered"] = int64(stats.Discovered)
	mx[prefix+"_monitored"] = int64(stats.Monitored)
	mx[prefix+"_omitted_by_selector"] = int64(stats.OmittedBySelector)
	mx[prefix+"_omitted_by_limit"] = int64(stats.OmittedByLimit)
}

func (c *Collector) logCardinalityCap(family, maxName, selectorName string, max int, stats cardinalityStats) {
	if stats.OmittedByLimit == 0 {
		delete(c.cardinalityLogState, family)
		return
	}
	if c.cardinalityLogState == nil {
		c.cardinalityLogState = make(map[string]string)
	}

	msg := fmt.Sprintf(
		"PAN-OS %s chart cap reached: discovered=%d monitored=%d omitted_by_selector=%d omitted_by_limit=%d (%s=%d); use %s selector to choose monitored entities or raise the cap",
		family,
		stats.Discovered,
		stats.Monitored,
		stats.OmittedBySelector,
		stats.OmittedByLimit,
		maxName,
		max,
		selectorName,
	)
	if c.cardinalityLogState[family] == msg {
		return
	}
	c.cardinalityLogState[family] = msg
	c.Warning(msg)
}
