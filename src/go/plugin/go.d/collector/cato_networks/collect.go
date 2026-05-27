// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (c *Collector) refreshDiscovery(ctx context.Context, force bool) error {
	now := c.now()
	if !force && len(c.discovery.siteIDs) > 0 {
		if now.Sub(c.discovery.fetchedAt) < seconds(c.Discovery.RefreshEvery) {
			return nil
		}
	}

	limit := int64(c.Discovery.PageLimit)
	var from int64
	siteNames := make(map[string]string)
	var siteIDs []string

	for {
		if from/limit >= maxDiscoveryPages {
			err := fmt.Errorf("entityLookup pagination exceeded %d pages", maxDiscoveryPages)
			c.markOperationFailure(operationDiscovery, err)
			if c.useCachedDiscoveryAfterRefreshFailure(now, force, err) {
				return nil
			}
			return err
		}

		res, err := c.client.LookupSites(ctx, c.AccountID, limit, from)
		if err != nil {
			c.markOperationFailure(operationDiscovery, err)
			if c.useCachedDiscoveryAfterRefreshFailure(now, force, err) {
				return nil
			}
			return err
		}

		items := res.GetEntityLookup().GetItems()
		for _, item := range items {
			entity := item.GetEntity()
			siteID := strings.TrimSpace(entity.GetID())
			if siteID == "" {
				continue
			}
			siteIDs = append(siteIDs, siteID)
			if name := ptrString(entity.GetName()); name != "" {
				siteNames[siteID] = name
			}
		}

		total := ptrInt64(res.GetEntityLookup().GetTotal())
		from += int64(len(items))
		if len(items) == 0 || (total > 0 && from >= total) || int64(len(items)) < limit {
			break
		}
	}

	selectedSiteIDs, skippedBySelector, skippedByLimit := c.selectSites(siteIDs, siteNames)
	c.discovery = discoveryState{
		siteIDs:           selectedSiteIDs,
		siteNames:         siteNames,
		fetchedAt:         now,
		totalSites:        len(siteIDs),
		skippedBySelector: skippedBySelector,
		skippedByLimit:    skippedByLimit,
	}
	c.markOperationSuccess(operationDiscovery)
	c.clearRecoverableWarning(warningKeyDiscoveryCache)
	return nil
}

func (c *Collector) useCachedDiscoveryAfterRefreshFailure(now time.Time, force bool, err error) bool {
	if force || len(c.discovery.siteIDs) == 0 {
		return false
	}

	c.discovery.fetchedAt = now
	c.warnRecoverable(warningKeyDiscoveryCache, classifyCatoError(err), "entityLookup refresh failed; using cached discovery for %d site(s), error_class=%s", len(c.discovery.siteIDs), classifyCatoError(err))
	return true
}

func (c *Collector) collectSnapshot(ctx context.Context) (map[string]*siteState, []string, error) {
	res, err := c.client.AccountSnapshot(ctx, c.AccountID, c.discovery.siteIDs)
	if err != nil {
		c.markOperationFailure(operationSnapshot, err)
		return nil, nil, err
	}
	c.markOperationSuccess(operationSnapshot)

	sites, order := normalizeSnapshot(res, c.discovery.siteNames)
	if len(order) == 0 {
		return sites, order, nil
	}

	seen := make(map[string]bool, len(order))
	for _, siteID := range order {
		seen[siteID] = true
	}
	for _, siteID := range c.discovery.siteIDs {
		if seen[siteID] {
			continue
		}
		sites[siteID] = &siteState{
			ID:                 siteID,
			Name:               siteDisplayName(siteID, c.discovery.siteNames, "", ""),
			ConnectivityStatus: "unknown",
			OperationalStatus:  "unknown",
			Interfaces:         make(map[string]*interfaceState),
		}
		order = append(order, siteID)
	}

	return sites, order, nil
}

func (c *Collector) collectMetrics(ctx context.Context, sites map[string]*siteState) error {
	ids := append([]string(nil), c.discovery.siteIDs...)
	if len(ids) == 0 {
		for id := range sites {
			ids = append(ids, id)
		}
		sort.Strings(ids)
	}

	var errCount int
	var successCount int
	batches := chunkStrings(ids, c.Metrics.MaxSitesPerQuery)
	for _, batch := range batches {
		res, err := c.client.AccountMetrics(ctx, c.AccountID, batch, c.Metrics.TimeFrame, c.Metrics.Buckets, c.groupInterfaces())
		if err != nil {
			errCount++
			c.markOperationFailure(operationMetrics, err)
			c.markOperationAffectedSites(operationMetrics, err, len(batch))
			c.Debugf("accountMetrics batch failed for %d site(s), error_class=%s", len(batch), classifyCatoError(err))
			continue
		}
		successCount++
		for _, issue := range mergeMetrics(res, sites) {
			c.markNormalizationIssue(normalizationSurfaceMetrics, issue)
		}
	}

	if errCount == 0 && successCount > 0 {
		c.markOperationSuccess(operationMetrics)
	}
	if errCount > 0 && errCount == len(batches) {
		return fmt.Errorf("all accountMetrics batches failed")
	}
	if errCount > 0 {
		return fmt.Errorf("%d of %d accountMetrics batches failed", errCount, len(batches))
	}
	return nil
}

func (c *Collector) collectBGP(ctx context.Context, sites map[string]*siteState, order []string) error {
	now := c.now()
	pruneBGPState(c.bgp.bySite, order)
	if now.Before(c.bgp.nextRefresh) && len(c.bgp.bySite) > 0 {
		c.updateBGPPollingHealth(len(order), c.bgpSitesPerCollectionLimit(len(order)))
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
	c.updateBGPPollingHealth(len(order), limit)

	var errCount int
	var successCount int
	for i := 0; i < limit; i++ {
		idx := (c.bgp.nextIndex + i) % len(order)
		siteID := order[idx]
		raw, err := c.client.SiteBgpStatus(ctx, c.AccountID, siteID)
		if err != nil {
			errCount++
			c.markOperationFailure(operationBGP, err)
			c.markOperationAffectedSites(operationBGP, err, 1)
			c.Debugf("siteBgpStatus failed for one site, error_class=%s", classifyCatoError(err))
			continue
		}
		successCount++
		peers, issues := normalizeBGP(raw)
		for _, issue := range issues {
			c.markNormalizationIssue(normalizationSurfaceBGP, issue)
		}
		c.bgp.bySite[siteID] = peers
	}
	c.health.BGPCachedSites = int64(len(c.bgp.bySite))

	if errCount == limit {
		mergeBGPState(sites, c.bgp.bySite)
		return fmt.Errorf("all siteBgpStatus requests failed")
	}
	c.bgp.nextIndex = (c.bgp.nextIndex + limit) % len(order)
	c.bgp.nextRefresh = now.Add(seconds(c.BGP.RefreshEvery))
	mergeBGPState(sites, c.bgp.bySite)

	if errCount == 0 && successCount > 0 {
		c.markOperationSuccess(operationBGP)
	}
	if errCount > 0 {
		return fmt.Errorf("%d of %d siteBgpStatus requests failed", errCount, limit)
	}
	return nil
}

func (c *Collector) bgpSitesPerCollectionLimit(siteCount int) int {
	limit := c.BGP.MaxSitesPerCollection
	if limit > siteCount {
		limit = siteCount
	}
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

func (c *Collector) updateBGPPollingHealth(siteCount, sitesPerCollection int) {
	if siteCount <= 0 || sitesPerCollection <= 0 {
		return
	}
	c.ensureHealth()
	cycles := (siteCount + sitesPerCollection - 1) / sitesPerCollection
	c.health.BGPSitesPerCollection = int64(sitesPerCollection)
	c.health.BGPFullScanSeconds = int64(cycles * c.BGP.RefreshEvery)
	c.health.BGPCachedSites = int64(len(c.bgp.bySite))
}

func seconds(v int) time.Duration {
	return time.Duration(v) * time.Second
}

func chunkStrings(values []string, size int) [][]string {
	if size <= 0 {
		size = len(values)
	}
	if size <= 0 {
		return nil
	}
	chunks := make([][]string, 0, (len(values)+size-1)/size)
	for len(values) > 0 {
		n := size
		if n > len(values) {
			n = len(values)
		}
		chunks = append(chunks, values[:n])
		values = values[n:]
	}
	return chunks
}
