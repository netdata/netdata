// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"sort"
)

func (c *Collector) collect(ctx context.Context) (bool, error) {
	if c.apiClient == nil {
		return false, errors.New("PAN-OS API client not initialized")
	}
	defer c.logSystemInfo()

	var result collectResult

	if result.addContextError(ctx) {
		return result.hasMetrics, errors.Join(result.errs...)
	}
	result.add(c.collectSystemMetrics(ctx))
	if result.addContextError(ctx) {
		return result.hasMetrics, errors.Join(result.errs...)
	}
	result.add(c.collectHAMetrics(ctx))
	if result.addContextError(ctx) {
		return result.hasMetrics, errors.Join(result.errs...)
	}
	result.add(c.collectEnvironmentMetrics(ctx))
	if result.addContextError(ctx) {
		return result.hasMetrics, errors.Join(result.errs...)
	}
	result.add(c.collectLicenseMetrics(ctx))
	if result.addContextError(ctx) {
		return result.hasMetrics, errors.Join(result.errs...)
	}
	result.add(c.collectIPSecMetrics(ctx))
	if result.addContextError(ctx) {
		return result.hasMetrics, errors.Join(result.errs...)
	}

	peers, err := c.collectBGPPeers(ctx)
	result.add(false, err)
	if len(peers) > 0 {
		monitoredPeers := orderedBGPPeers(peers)
		result.add(c.collectPeerMetrics(monitoredPeers), nil)
		result.add(c.collectVRMetrics(peers), nil)
	}

	return result.hasMetrics, errors.Join(result.errs...)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

type collectResult struct {
	hasMetrics bool
	errs       []error
}

func (r *collectResult) add(hasMetrics bool, err error) {
	if hasMetrics {
		r.hasMetrics = true
	}
	if err != nil {
		r.errs = append(r.errs, err)
	}
}

func (r *collectResult) addContextError(ctx context.Context) bool {
	if err := contextError(ctx); err != nil {
		r.add(false, err)
		return true
	}
	return false
}

func orderedBGPPeers(peers []bgpPeer) []bgpPeer {
	items := append([]bgpPeer(nil), peers...)
	sort.SliceStable(items, func(i, j int) bool {
		return bgpPeerOrderKey(items[i]) < bgpPeerOrderKey(items[j])
	})

	for i := range items {
		counters := append([]bgpPrefixCounter(nil), items[i].PrefixCounters...)
		sort.SliceStable(counters, func(j, k int) bool {
			return bgpPrefixCounterOrderKey(counters[j]) < bgpPrefixCounterOrderKey(counters[k])
		})
		items[i].PrefixCounters = counters
	}

	return items
}

func bgpPeerOrderKey(peer bgpPeer) string {
	return firstNonEmpty(peer.VR, "default") + "/" + firstNonEmpty(peer.PeerAddress, "unknown")
}

func bgpPrefixCounterOrderKey(counter bgpPrefixCounter) string {
	return firstNonEmpty(counter.AFI, "unknown") + "/" + firstNonEmpty(counter.SAFI, "unknown")
}

func (c *Collector) collectPeerMetrics(peers []bgpPeer) bool {
	if len(peers) == 0 {
		return false
	}

	for _, peer := range peers {
		labels := peerLabelValues(peer)
		observeStateSetVec(c.metrics.bgp.peerState, peer.State, labels...)
		c.metrics.bgp.peerUptime.WithLabelValues(labels...).Observe(float64(peer.Uptime))
		c.metrics.bgp.peerMessagesIn.WithLabelValues(labels...).ObserveTotal(float64(peer.MessagesIn))
		c.metrics.bgp.peerMessagesOut.WithLabelValues(labels...).ObserveTotal(float64(peer.MessagesOut))
		c.metrics.bgp.peerUpdatesIn.WithLabelValues(labels...).ObserveTotal(float64(peer.UpdatesIn))
		c.metrics.bgp.peerUpdatesOut.WithLabelValues(labels...).ObserveTotal(float64(peer.UpdatesOut))
		c.metrics.bgp.peerFlaps.WithLabelValues(labels...).ObserveTotal(float64(peer.Flaps))
		c.metrics.bgp.peerEstablishedTransitions.WithLabelValues(labels...).ObserveTotal(float64(peer.Established))

		for _, counter := range peer.PrefixCounters {
			prefixLabels := prefixLabelValues(peer, counter)
			c.metrics.bgp.peerPrefixesReceivedTotal.WithLabelValues(prefixLabels...).Observe(float64(counter.IncomingTotal))
			c.metrics.bgp.peerPrefixesReceivedAccepted.WithLabelValues(prefixLabels...).Observe(float64(counter.IncomingAccepted))
			c.metrics.bgp.peerPrefixesReceivedRejected.WithLabelValues(prefixLabels...).Observe(float64(counter.IncomingRejected))
			c.metrics.bgp.peerPrefixesAdvertised.WithLabelValues(prefixLabels...).Observe(float64(counter.OutgoingAdvertised))
		}
	}

	return true
}

func (c *Collector) collectVRMetrics(peers []bgpPeer) bool {
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

	for _, vr := range vrs {
		st := stats[vr]
		labels := []string{vr}
		for _, state := range bgpStates {
			c.metrics.bgp.vrPeersByState[state].WithLabelValues(labels...).Observe(float64(st.stateCounts[state]))
		}
		c.metrics.bgp.vrPeersConfigured.WithLabelValues(labels...).Observe(float64(st.total))
		c.metrics.bgp.vrPeersEstablished.WithLabelValues(labels...).Observe(float64(st.established))
	}

	return len(vrs) > 0
}
