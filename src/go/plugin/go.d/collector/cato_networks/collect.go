// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type discoveryState struct {
	siteIDs           []string
	siteNames         map[string]string
	fetchedAt         time.Time
	totalSites        int
	skippedBySelector int
}

func (c *Collector) collect(ctx context.Context) (err error) {
	c.beginHealthCycle()
	defer func() {
		if contextErr(ctx) != nil {
			return
		}
		c.health.CollectionSuccess = err == nil
		c.updateSiteSelectionHealth()
		if err != nil {
			c.markCollectionFailure(err)
		}
		c.writeCollectorHealth()
	}()

	if c.client == nil {
		return errors.New("Cato client is not initialized")
	}

	if err := c.refreshDiscovery(ctx, false); err != nil {
		return wrapCatoOperationError("site discovery", err)
	}
	if len(c.discovery.siteIDs) == 0 {
		return errors.New("no Cato sites discovered")
	}

	sites, order, err := c.collectSnapshot(ctx)
	if err != nil {
		return wrapCatoOperationError("account snapshot", err)
	}
	if len(sites) == 0 {
		return errors.New("no Cato sites returned by account snapshot")
	}
	c.pruneUnselectedSites(sites, &order)

	if err := c.collectMetrics(ctx, sites); err != nil {
		c.warnRecoverable(warningKeyMetrics, classifyCatoError(err), "account metrics collection incomplete, error_class=%s", classifyCatoError(err))
	} else {
		c.clearRecoverableWarning(warningKeyMetrics)
	}

	if err := c.collectBGP(ctx, sites, order); err != nil {
		c.warnRecoverable(warningKeyBGP, classifyCatoError(err), "BGP status collection incomplete, error_class=%s", classifyCatoError(err))
	} else {
		c.clearRecoverableWarning(warningKeyBGP)
	}

	c.pruneUnselectedSites(sites, &order)
	c.updateSiteSelectionHealth()

	now := c.now()
	topo := buildTopology(c.AccountID, sites, order, now)

	c.mu.Lock()
	c.topology = topo
	c.mu.Unlock()

	c.writeMetrics(sites, order)

	return nil
}

func (c *Collector) refreshDiscovery(ctx context.Context, force bool) error {
	now := c.now()
	if !force && len(c.discovery.siteIDs) > 0 {
		if now.Sub(c.discovery.fetchedAt) < seconds(defaultDiscoveryEvery) {
			return nil
		}
	}

	limit := int64(defaultDiscoveryLimit)
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
			if name := derefZero(entity.GetName()); name != "" {
				siteNames[siteID] = name
			}
		}

		total := derefZero(res.GetEntityLookup().GetTotal())
		from += int64(len(items))
		if len(items) == 0 || (total > 0 && from >= total) || int64(len(items)) < limit {
			break
		}
	}

	selectedSiteIDs, skippedBySelector := c.selectSites(siteIDs, siteNames)
	c.discovery = discoveryState{
		siteIDs:           selectedSiteIDs,
		siteNames:         siteNames,
		fetchedAt:         now,
		totalSites:        len(siteIDs),
		skippedBySelector: skippedBySelector,
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

func seconds(v int) time.Duration {
	return time.Duration(v) * time.Second
}
