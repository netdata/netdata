// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"
	"sort"
	"time"
)

func (c *Collector) collect() (map[string]int64, bool, error) {
	started := time.Now()
	scrape := newScrapeMetrics()
	mx := make(map[string]int64)

	families, neighbors, vnis, rpkiCaches, rpkiInventories, err := c.collectBackendData(&scrape)
	if err != nil {
		return finalizeScrapeMetrics(mx, &scrape, started), false, err
	}
	if len(families) == 0 && len(rpkiCaches) == 0 && len(rpkiInventories) == 0 {
		c.cleanupObsoleteCharts(time.Now())
		return finalizeScrapeMetrics(mx, &scrape, started), false, nil
	}

	selectedFamilies := c.selectFamilies(families)
	if c.Backend == backendFRR {
		vnis = c.collectEVPNVNIs(&scrape, families, selectedFamilies)
	}
	selectedVNIs := c.selectVNIs(vnis)
	if len(selectedFamilies) == 0 && len(selectedVNIs) == 0 && len(rpkiCaches) == 0 && len(rpkiInventories) == 0 {
		c.cleanupObsoleteCharts(time.Now())
		return finalizeScrapeMetrics(mx, &scrape, started), false, nil
	}

	selectedPeers := c.selectPeers(families, selectedFamilies)
	if c.Backend == backendFRR {
		c.collectDeepPeerPrefixMetrics(families, selectedFamilies, selectedPeers, &scrape)
	}
	selectedNeighbors := selectNeighbors(families, selectedPeers)
	now := time.Now()

	for _, family := range families {
		if !selectedFamilies[family.ID] {
			continue
		}

		family.ChartedPeers = int64(countSelectedPeers(family, selectedPeers))

		if _, ok := c.familySeen[family.ID]; !ok {
			c.addFamilyCharts(family)
		}
		c.familySeen[family.ID] = now

		familyKey := "family_" + family.ID + "_"
		mx[familyKey+"peers_established"] = family.PeersEstablished
		mx[familyKey+"peers_admin_down"] = family.PeersAdminDown
		mx[familyKey+"peers_down"] = family.PeersDown
		mx[familyKey+"peers_configured"] = family.ConfiguredPeers
		mx[familyKey+"peers_charted"] = family.ChartedPeers
		mx[familyKey+"messages_received"] = family.MessagesReceived
		mx[familyKey+"messages_sent"] = family.MessagesSent
		mx[familyKey+"prefixes_received"] = family.PrefixesReceived
		mx[familyKey+"rib_routes"] = family.RIBRoutes
		if family.HasCorrectness {
			c.addFamilyCorrectnessChart(family)
			mx[familyKey+"correctness_valid"] = family.CorrectnessValid
			mx[familyKey+"correctness_invalid"] = family.CorrectnessInvalid
			mx[familyKey+"correctness_not_found"] = family.CorrectnessNotFound
		}

		for _, peer := range family.Peers {
			if !selectedPeers[peer.ID] {
				continue
			}

			if _, ok := c.peerSeen[peer.ID]; !ok {
				c.addPeerCharts(peer)
			}
			c.peerSeen[peer.ID] = now

			peerKey := "peer_" + peer.ID + "_"
			mx[peerKey+"messages_received"] = peer.MessagesReceived
			mx[peerKey+"messages_sent"] = peer.MessagesSent
			mx[peerKey+"prefixes_received"] = peer.PrefixesReceived
			if peer.HasPrefixPolicy {
				c.addPeerPolicyCharts(peer)
			}
			if peer.HasPrefixPolicy || (peer.State != peerStateUp && c.Charts().Has(peerPolicyChartID(peer.ID))) {
				mx[peerKey+"prefixes_accepted"] = peer.PrefixesAccepted
				mx[peerKey+"prefixes_filtered"] = peer.PrefixesFiltered
			}
			if peer.HasPrefixesSent {
				c.addPeerAdvertisedPrefixesChart(peer)
				mx[peerKey+"prefixes_advertised"] = peer.PrefixesSent
			} else if peer.State != peerStateUp && c.Charts().Has(peerAdvertisedPrefixesChartID(peer.ID)) {
				mx[peerKey+"prefixes_advertised"] = 0
			}
			mx[peerKey+"uptime_seconds"] = peer.UptimeSecs
			mx[peerKey+"state"] = peer.State
		}
	}

	for _, neighbor := range neighbors {
		if !selectedNeighbors[neighbor.ID] {
			continue
		}

		if _, ok := c.neighborSeen[neighbor.ID]; !ok {
			c.addNeighborCharts(neighbor)
		}
		if neighbor.HasResetDetails {
			c.addNeighborLastResetCharts(neighbor)
		}
		c.neighborSeen[neighbor.ID] = now

		neighborKey := "neighbor_" + neighbor.ID + "_"
		if neighbor.HasTransitions {
			mx[neighborKey+"connections_established"] = neighbor.ConnectionsEstablished
			mx[neighborKey+"connections_dropped"] = neighbor.ConnectionsDropped
		}
		if neighbor.HasChurn {
			mx[neighborKey+"churn_updates_received"] = neighbor.UpdatesReceived
			mx[neighborKey+"churn_updates_sent"] = neighbor.UpdatesSent
			mx[neighborKey+"churn_withdraws_received"] = neighbor.WithdrawsReceived
			mx[neighborKey+"churn_withdraws_sent"] = neighbor.WithdrawsSent
			mx[neighborKey+"churn_notifications_received"] = neighbor.NotificationsReceived
			mx[neighborKey+"churn_notifications_sent"] = neighbor.NotificationsSent
			mx[neighborKey+"churn_route_refresh_received"] = neighbor.RouteRefreshReceived
			mx[neighborKey+"churn_route_refresh_sent"] = neighbor.RouteRefreshSent
		}
		if neighbor.HasMessageTypes {
			mx[neighborKey+"updates_received"] = neighbor.UpdatesReceived
			mx[neighborKey+"updates_sent"] = neighbor.UpdatesSent
			mx[neighborKey+"notifications_received"] = neighbor.NotificationsReceived
			mx[neighborKey+"notifications_sent"] = neighbor.NotificationsSent
			mx[neighborKey+"keepalives_received"] = neighbor.KeepalivesReceived
			mx[neighborKey+"keepalives_sent"] = neighbor.KeepalivesSent
			mx[neighborKey+"route_refresh_received"] = neighbor.RouteRefreshReceived
			mx[neighborKey+"route_refresh_sent"] = neighbor.RouteRefreshSent
		}
		if neighbor.HasResetDetails {
			resetPresent := neighbor.HasResetState || neighbor.LastResetAgeSecs > 0 || neighbor.LastResetCode > 0 || neighbor.LastErrorCode > 0 || neighbor.LastErrorSubcode > 0
			mx[neighborKey+"last_reset_never"] = boolToInt(neighbor.LastResetNever)
			mx[neighborKey+"last_reset_soft_or_unknown"] = boolToInt(resetPresent && !neighbor.LastResetNever && !neighbor.LastResetHard)
			mx[neighborKey+"last_reset_hard"] = boolToInt(resetPresent && !neighbor.LastResetNever && neighbor.LastResetHard)
			mx[neighborKey+"last_reset_age_seconds"] = neighbor.LastResetAgeSecs
			mx[neighborKey+"last_reset_code"] = neighbor.LastResetCode
			mx[neighborKey+"last_error_code"] = neighbor.LastErrorCode
			mx[neighborKey+"last_error_subcode"] = neighbor.LastErrorSubcode
		}
	}

	for _, cache := range rpkiCaches {
		if _, ok := c.rpkiSeen[cache.ID]; !ok {
			c.addRPKICacheCharts(cache)
		}
		c.rpkiSeen[cache.ID] = now

		cacheKey := "rpki_" + cache.ID + "_"
		mx[cacheKey+"up"] = boolToInt(cache.Up)
		mx[cacheKey+"down"] = boolToInt(!cache.Up)
		if cache.HasUptime {
			mx[cacheKey+"uptime_seconds"] = cache.UptimeSecs
		}
		if cache.HasRecords {
			mx[cacheKey+"record_ipv4"] = cache.RecordIPv4
			mx[cacheKey+"record_ipv6"] = cache.RecordIPv6
		}
		if cache.HasPrefixes {
			mx[cacheKey+"prefix_ipv4"] = cache.PrefixIPv4
			mx[cacheKey+"prefix_ipv6"] = cache.PrefixIPv6
		}
	}

	for _, inv := range rpkiInventories {
		if _, ok := c.rpkiInventorySeen[inv.ID]; !ok {
			c.addRPKIInventoryCharts(inv)
		}
		c.rpkiInventorySeen[inv.ID] = now

		invKey := "rpki_inventory_" + inv.ID + "_"
		mx[invKey+"prefix_ipv4"] = inv.PrefixIPv4
		mx[invKey+"prefix_ipv6"] = inv.PrefixIPv6
	}

	c.emitVNIMetrics(mx, vnis, selectedVNIs, now)

	c.cleanupObsoleteCharts(now)
	return finalizeScrapeMetrics(mx, &scrape, started), len(selectedFamilies) > 0 || len(selectedVNIs) > 0 || len(rpkiCaches) > 0 || len(rpkiInventories) > 0, nil
}

func (c *Collector) collectBackendData(scrape *scrapeMetrics) ([]familyStats, []neighborStats, []vniStats, []rpkiCacheStats, []rpkiInventoryStats, error) {
	switch c.Backend {
	case backendBIRD:
		return c.collectBIRDData(scrape)
	case backendGoBGP:
		return c.collectGoBGPData(scrape)
	case backendOpenBGPD:
		return c.collectOpenBGPDData(scrape)
	default:
		return c.collectFRRData(scrape)
	}
}

func (c *Collector) collectFRRData(scrape *scrapeMetrics) ([]familyStats, []neighborStats, []vniStats, []rpkiCacheStats, []rpkiInventoryStats, error) {
	var families []familyStats
	detailsByVRF := c.neighborDetailsCache

	client, ok := c.client.(frrClientAPI)
	if !ok {
		return nil, nil, nil, nil, nil, fmt.Errorf("backend %q client does not implement FRR API", c.Backend)
	}

	data, err := client.Neighbors()
	if err != nil {
		scrape.noteQueryError(err, false)
		c.Debugf("collect peer metadata: %v", err)
	} else {
		details, err := parseFRRNeighbors(data)
		if err != nil {
			scrape.noteParseError(false)
			c.Debugf("parse peer metadata: %v", err)
		} else {
			detailsByVRF = details
			c.neighborDetailsCache = details
		}
	}

	for _, req := range summaryRequests() {
		data, err := client.Summary(req.AFI, req.SAFI)
		if err != nil {
			scrape.noteQueryError(err, req.Required)
			if req.Required {
				return nil, nil, nil, nil, nil, fmt.Errorf("collect %s/%s summary: %v", req.AFI, req.SAFI, err)
			}
			c.Debugf("collect %s/%s summary: %v", req.AFI, req.SAFI, err)
			continue
		}

		stats, err := parseFRRSummary(data, req.AFI, req.SAFI, c.Backend, detailsByVRF)
		if err != nil {
			scrape.noteParseError(req.Required)
			if req.Required {
				return nil, nil, nil, nil, nil, fmt.Errorf("parse %s/%s summary: %v", req.AFI, req.SAFI, err)
			}
			c.Debugf("parse %s/%s summary: %v", req.AFI, req.SAFI, err)
			continue
		}
		families = append(families, stats...)
	}

	rpkiCaches := c.collectFRRRPKICaches(client, scrape)
	rpkiInventories := c.collectFRRRPKIInventory(client, scrape)

	sort.Slice(families, func(i, j int) bool {
		if families[i].VRF != families[j].VRF {
			return families[i].VRF < families[j].VRF
		}
		if families[i].AFI != families[j].AFI {
			return families[i].AFI < families[j].AFI
		}
		return families[i].SAFI < families[j].SAFI
	})

	return families, buildNeighbors(families), nil, rpkiCaches, rpkiInventories, nil
}

func (c *Collector) collectDeepPeerPrefixMetrics(families []familyStats, selectedFamilies, selectedPeers map[string]bool, scrape *scrapeMetrics) {
	if !c.DeepPeerPrefixMetrics {
		return
	}

	for familyIdx := range families {
		family := &families[familyIdx]
		if !selectedFamilies[family.ID] {
			continue
		}
		for peerIdx := range family.Peers {
			peer := &family.Peers[peerIdx]
			if !selectedPeers[peer.ID] {
				continue
			}
			if peer.State != peerStateUp {
				continue
			}

			if err := c.collectDeepPeerPolicyMetrics(peer, scrape); err != nil {
				c.Debugf("collect deep peer metrics for %s: %v", peer.Address, err)
			}
		}
	}
}

func (c *Collector) collectDeepPeerPolicyMetrics(peer *peerStats, scrape *scrapeMetrics) error {
	client, ok := c.client.(frrClientAPI)
	if !ok {
		return fmt.Errorf("backend %q client does not implement FRR API", c.Backend)
	}

	if !peer.HasPrefixPolicy {
		if !scrape.tryDeepQuery(c.MaxDeepQueriesPerScrape) {
			return nil
		}
		routesData, err := client.PeerRoutes(peer.Family.VRF, peer.Family.AFI, peer.Family.SAFI, peer.Address)
		if err != nil {
			scrape.noteDeepQueryError()
			return fmt.Errorf("accepted routes: %w", err)
		}

		accepted, err := parseFRRPrefixCounter(routesData)
		if err != nil {
			scrape.noteDeepQueryError()
			return fmt.Errorf("parse accepted routes: %w", err)
		}

		peer.HasPrefixPolicy = true
		peer.PrefixesAccepted = accepted
		if peer.PrefixesReceived > accepted {
			peer.PrefixesFiltered = peer.PrefixesReceived - accepted
		} else {
			peer.PrefixesFiltered = 0
		}
	}

	if peer.HasPrefixesSent {
		return nil
	}

	if !scrape.tryDeepQuery(c.MaxDeepQueriesPerScrape) {
		return nil
	}
	advertisedData, err := client.PeerAdvertisedRoutes(peer.Family.VRF, peer.Family.AFI, peer.Family.SAFI, peer.Address)
	if err != nil {
		scrape.noteDeepQueryError()
		return fmt.Errorf("advertised routes: %w", err)
	}

	advertised, err := parseFRRPrefixCounter(advertisedData)
	if err != nil {
		scrape.noteDeepQueryError()
		return fmt.Errorf("parse advertised routes: %w", err)
	}

	peer.HasPrefixesSent = true
	peer.PrefixesSent = advertised
	return nil
}

func (c *Collector) cleanupObsoleteCharts(now time.Time) {
	if c.cleanupLastTime.IsZero() {
		c.cleanupLastTime = now
		return
	}
	if now.Sub(c.cleanupLastTime) < c.cleanupEvery {
		return
	}
	c.cleanupLastTime = now

	for id, seen := range c.familySeen {
		if now.Sub(seen) >= c.obsoleteAfter {
			delete(c.familySeen, id)
			c.removeFamilyCharts(id)
		}
	}
	for id, seen := range c.peerSeen {
		if now.Sub(seen) >= c.obsoleteAfter {
			delete(c.peerSeen, id)
			c.removePeerCharts(id)
		}
	}
	for id, seen := range c.neighborSeen {
		if now.Sub(seen) >= c.obsoleteAfter {
			delete(c.neighborSeen, id)
			c.removeNeighborCharts(id)
		}
	}
	for id, seen := range c.vniSeen {
		if now.Sub(seen) >= c.obsoleteAfter {
			delete(c.vniSeen, id)
			c.removeVNICharts(id)
		}
	}
	for id, seen := range c.rpkiSeen {
		if now.Sub(seen) >= c.obsoleteAfter {
			delete(c.rpkiSeen, id)
			c.removeRPKICacheCharts(id)
		}
	}
	for id, seen := range c.rpkiInventorySeen {
		if now.Sub(seen) >= c.obsoleteAfter {
			delete(c.rpkiInventorySeen, id)
			c.removeRPKIInventoryCharts(id)
		}
	}
}
