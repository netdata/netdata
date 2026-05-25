// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"errors"
	"sort"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.apiClient == nil {
		return nil, errors.New("PAN-OS API client not initialized")
	}
	defer c.logSystemInfoOnce()

	mx := c.resetMetrics()
	var errs []error

	c.beginDynamicChartCollection()

	if c.CollectSystem {
		if err := c.collectSystemMetrics(mx); err != nil {
			errs = append(errs, err)
		}
	}
	if c.CollectHA {
		if err := c.collectHAMetrics(mx); err != nil {
			errs = append(errs, err)
		}
	}
	if c.CollectEnvironment {
		if err := c.collectEnvironmentMetrics(mx); err != nil {
			errs = append(errs, err)
		}
	}
	if c.CollectLicenses {
		if err := c.collectLicenseMetrics(mx); err != nil {
			errs = append(errs, err)
		}
	}
	if c.CollectIPSec {
		if err := c.collectIPSecMetrics(mx); err != nil {
			errs = append(errs, err)
		}
	}

	if c.CollectBGP {
		peers, err := c.collectBGPPeers()
		if err != nil {
			errs = append(errs, err)
		} else {
			if len(peers) > 0 {
				monitoredPeers, peerStats := c.filterBGPPeers(peers)
				monitoredPeers, prefixStats := c.filterBGPPrefixFamilies(monitoredPeers)
				c.collectBGPPeerCardinalityMetrics(mx, peerStats)
				c.collectBGPPrefixFamilyCardinalityMetrics(mx, prefixStats)
				c.collectPeerMetrics(mx, monitoredPeers)
				seenVRs := c.collectVRMetrics(mx, peers)
				c.removeStaleCharts(monitoredPeers, seenVRs)
			} else {
				c.removeStaleCharts(nil, nil)
			}
		}
	}

	c.removeStaleDynamicCharts()

	if len(mx) == 0 {
		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		if c.CollectBGP && c.routingEngine == routingEngineNone {
			return nil, errors.New("no PAN-OS BGP peers found; BGP may be unconfigured, unsupported for this account, or the XML response shape is not recognized")
		}
		return nil, nil
	}

	return mx, errors.Join(errs...)
}

func (c *Collector) collectPeerMetrics(mx map[string]int64, peers []bgpPeer) {
	for _, peer := range peers {
		c.addPeerCharts(peer)
		key := peerKey(peer)

		for _, state := range []string{"idle", "connect", "active", "opensent", "openconfirm", "established"} {
			mx["bgp_peer_"+key+"_state_"+state] = 0
		}
		if peer.State != "" {
			mx["bgp_peer_"+key+"_state_"+peer.State] = 1
		}

		mx["bgp_peer_"+key+"_uptime"] = peer.Uptime
		mx["bgp_peer_"+key+"_messages_in"] = peer.MessagesIn
		mx["bgp_peer_"+key+"_messages_out"] = peer.MessagesOut
		mx["bgp_peer_"+key+"_updates_in"] = peer.UpdatesIn
		mx["bgp_peer_"+key+"_updates_out"] = peer.UpdatesOut
		mx["bgp_peer_"+key+"_flaps"] = peer.Flaps
		mx["bgp_peer_"+key+"_established_transitions"] = peer.Established

		for _, counter := range peer.PrefixCounters {
			c.addPrefixCharts(peer, counter)
			prefixKey := prefixKey(peer, counter)
			mx["bgp_peer_"+prefixKey+"_prefixes_received_total"] = counter.IncomingTotal
			mx["bgp_peer_"+prefixKey+"_prefixes_received_accepted"] = counter.IncomingAccepted
			mx["bgp_peer_"+prefixKey+"_prefixes_received_rejected"] = counter.IncomingRejected
			mx["bgp_peer_"+prefixKey+"_prefixes_advertised"] = counter.OutgoingAdvertised
		}
	}
}

func (c *Collector) collectVRMetrics(mx map[string]int64, peers []bgpPeer) []string {
	type vrStats struct {
		stateCounts map[string]int64
		total       int64
		established int64
	}

	stats := make(map[string]*vrStats)
	for _, peer := range peers {
		st := stats[peer.VR]
		if st == nil {
			st = &vrStats{stateCounts: make(map[string]int64)}
			stats[peer.VR] = st
		}

		st.total++
		if peer.State != "" {
			st.stateCounts[peer.State]++
		}
		if peer.State == "established" {
			st.established++
		}
	}

	vrs := make([]string, 0, len(stats))
	for vr := range stats {
		vrs = append(vrs, vr)
	}
	sort.Strings(vrs)

	cardinality := cardinalityStats{Discovered: len(vrs)}
	seenVRs := make([]string, 0, len(vrs))
	for _, vr := range vrs {
		if !matchesEntity(c.bgpVirtualRouterMatcher, vr) {
			cardinality.OmittedBySelector++
			continue
		}
		if cardinality.Monitored >= c.MaxBGPVirtualRouters {
			cardinality.OmittedByLimit++
			continue
		}
		cardinality.Monitored++

		st := stats[vr]
		c.addVRCharts(vr)
		key := cleanID(vr)
		seenVRs = append(seenVRs, key)

		for _, state := range []string{"idle", "connect", "active", "opensent", "openconfirm", "established"} {
			mx["bgp_vr_"+key+"_peers_state_"+state] = st.stateCounts[state]
		}
		mx["bgp_vr_"+key+"_peers_configured"] = st.total
		mx["bgp_vr_"+key+"_peers_established"] = st.established
	}

	c.collectBGPVirtualRouterCardinalityMetrics(mx, cardinality)
	return seenVRs
}
