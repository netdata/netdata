// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"fmt"
	"hash/fnv"
	"time"
)

type bgpState struct {
	noBGPUntil map[string]time.Time
}

func (c *Collector) collectBGP(ctx context.Context, sites map[string]*siteState, order []string) error {
	now := c.now()
	pruneNoBGPCache(c.bgp.noBGPUntil, order, now)

	var requestCount int
	var errCount int
	for _, siteID := range order {
		if c.isNoBGPCached(siteID, now) {
			continue
		}

		requestCount++
		raw, err := c.client.SiteBgpStatus(ctx, c.AccountID, siteID)
		if err != nil {
			errCount++
			c.Debugf("siteBgpStatus failed for one site, error_class=%s", classifyCatoError(err))
			continue
		}

		peers, issues := normalizeBGP(raw)
		for _, issue := range issues {
			c.logNormalizationIssue(normalizationSurfaceBGP, issue)
		}

		if len(peers) == 0 {
			if len(issues) > 0 {
				continue
			}
			c.bgp.noBGPUntil[siteID] = c.noBGPCacheUntil(siteID, now)
			continue
		}
		delete(c.bgp.noBGPUntil, siteID)
		if site := sites[siteID]; site != nil {
			site.BGPPeers = peers
		}
	}

	if requestCount == 0 {
		return nil
	}
	if errCount == requestCount {
		return fmt.Errorf("all siteBgpStatus requests failed")
	}
	if errCount > 0 {
		return fmt.Errorf("%d of %d siteBgpStatus requests failed", errCount, requestCount)
	}
	return nil
}

func (c *Collector) isNoBGPCached(siteID string, now time.Time) bool {
	until, ok := c.bgp.noBGPUntil[siteID]
	return ok && now.Before(until)
}

func (c *Collector) noBGPCacheUntil(siteID string, now time.Time) time.Time {
	return now.Add(seconds(defaultNoBGPCacheTTL) + c.noBGPCacheJitter(siteID))
}

func (c *Collector) noBGPCacheJitter(siteID string) time.Duration {
	interval := seconds(c.UpdateEvery)
	if interval <= 0 {
		interval = seconds(defaultUpdateEvery)
	}

	window := interval * 5
	h := fnv.New64a()
	_, _ = h.Write([]byte(siteID))
	return time.Duration(h.Sum64() % uint64(window))
}

func pruneNoBGPCache(noBGPUntil map[string]time.Time, order []string, now time.Time) {
	if len(noBGPUntil) == 0 {
		return
	}
	active := make(map[string]bool, len(order))
	for _, siteID := range order {
		active[siteID] = true
	}
	for siteID, until := range noBGPUntil {
		if !active[siteID] || !now.Before(until) {
			delete(noBGPUntil, siteID)
		}
	}
}
