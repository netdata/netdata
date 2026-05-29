// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"fmt"
	"time"
)

type bgpState struct {
	bySite      map[string][]bgpPeerState
	nextRefresh time.Time
	nextIndex   int
}

func (c *Collector) collectBGP(ctx context.Context, sites map[string]*siteState, order []string) error {
	now := c.now()
	pruneBGPState(c.bgp.bySite, order)
	if now.Before(c.bgp.nextRefresh) && len(c.bgp.bySite) > 0 {
		mergeBGPState(sites, c.bgp.bySite)
		return nil
	}

	if c.bgp.bySite == nil {
		c.bgp.bySite = make(map[string][]bgpPeerState)
	}

	limit := c.bgpSitesPerCollectionLimit(len(order))
	if limit == 0 {
		return nil
	}

	var errCount int
	var successCount int
	for i := range limit {
		idx := (c.bgp.nextIndex + i) % len(order)
		siteID := order[idx]
		raw, err := c.client.SiteBgpStatus(ctx, c.AccountID, siteID)
		if err != nil {
			errCount++
			c.Debugf("siteBgpStatus failed for one site, error_class=%s", classifyCatoError(err))
			continue
		}
		successCount++
		peers, issues := normalizeBGP(raw)
		for _, issue := range issues {
			c.logNormalizationIssue(normalizationSurfaceBGP, issue)
		}
		c.bgp.bySite[siteID] = peers
	}

	if errCount == limit {
		mergeBGPState(sites, c.bgp.bySite)
		return fmt.Errorf("all siteBgpStatus requests failed")
	}
	c.bgp.nextIndex = (c.bgp.nextIndex + limit) % len(order)
	c.bgp.nextRefresh = now.Add(seconds(defaultBGPRefreshEvery))
	mergeBGPState(sites, c.bgp.bySite)

	if errCount > 0 {
		return fmt.Errorf("%d of %d siteBgpStatus requests failed", errCount, limit)
	}
	return nil
}

func (c *Collector) bgpSitesPerCollectionLimit(siteCount int) int {
	limit := min(defaultBGPMaxSites, siteCount)
	if limit < 0 {
		return 0
	}
	return limit
}

func pruneBGPState(bySite map[string][]bgpPeerState, order []string) {
	if len(bySite) == 0 {
		return
	}
	active := make(map[string]bool, len(order))
	for _, siteID := range order {
		active[siteID] = true
	}
	for siteID := range bySite {
		if !active[siteID] {
			delete(bySite, siteID)
		}
	}
}

func mergeBGPState(sites map[string]*siteState, bySite map[string][]bgpPeerState) {
	for siteID, peers := range bySite {
		if site := sites[siteID]; site != nil {
			site.BGPPeers = append([]bgpPeerState(nil), peers...)
		}
	}
}
