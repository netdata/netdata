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
	skippedBySelector int
}

func (c *Collector) collect(ctx context.Context) error {
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
		errorClass := classifyCatoError(err)
		c.warnRecoverable(warningKeyMetrics, errorClass, "account metrics collection incomplete, error_class=%s", errorClass)
	}

	if err := c.collectBGP(ctx, sites, order); err != nil {
		errorClass := classifyCatoError(err)
		c.warnRecoverable(warningKeyBGP, errorClass, "BGP status collection incomplete, error_class=%s", errorClass)
	}

	c.pruneUnselectedSites(sites, &order)

	now := c.now()
	topo, err := buildTopology(c.AccountID, sites, order, now)
	if err != nil {
		return fmt.Errorf("build topology: %w", err)
	}
	c.topology.Publish(topo)

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
			if c.useCachedDiscoveryAfterRefreshFailure(now, force, err) {
				return nil
			}
			return err
		}

		res, err := c.client.LookupSites(ctx, c.AccountID, limit, from)
		if err != nil {
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
		skippedBySelector: skippedBySelector,
	}
	if skippedBySelector > 0 {
		c.Limit("cato:site_selector", 1, recurringLogEvery).
			Infof("selected %d of %d Cato site(s) after applying site_selector", len(selectedSiteIDs), len(siteIDs))
	}
	return nil
}

func (c *Collector) selectSites(siteIDs []string, siteNames map[string]string) ([]string, int) {
	selected := make([]string, 0, len(siteIDs))
	var skippedSelector int

	for _, siteID := range siteIDs {
		if !c.siteMatcher.MatchString(siteSelectorValue(siteID, siteNames[siteID])) {
			skippedSelector++
			continue
		}
		selected = append(selected, siteID)
	}

	return selected, skippedSelector
}

func siteSelectorValue(siteID, siteName string) string {
	if name := strings.TrimSpace(siteName); name != "" {
		return name
	}
	return strings.TrimSpace(siteID)
}

func (c *Collector) pruneUnselectedSites(sites map[string]*siteState, order *[]string) {
	active := make(map[string]bool, len(c.discovery.siteIDs))
	nextOrder := make([]string, 0, len(c.discovery.siteIDs))
	for _, siteID := range c.discovery.siteIDs {
		if sites[siteID] == nil {
			continue
		}
		active[siteID] = true
		nextOrder = append(nextOrder, siteID)
	}
	for siteID := range sites {
		if !active[siteID] {
			delete(sites, siteID)
		}
	}
	*order = nextOrder
}

func (c *Collector) useCachedDiscoveryAfterRefreshFailure(now time.Time, force bool, err error) bool {
	if force || len(c.discovery.siteIDs) == 0 {
		return false
	}

	c.discovery.fetchedAt = now
	errorClass := classifyCatoError(err)
	c.warnRecoverable(warningKeyDiscoveryCache, errorClass, "entityLookup refresh failed; using cached discovery for %d site(s), error_class=%s", len(c.discovery.siteIDs), errorClass)
	return true
}

func (c *Collector) collectSnapshot(ctx context.Context) (map[string]*siteState, []string, error) {
	res, err := c.client.AccountSnapshot(ctx, c.AccountID, c.discovery.siteIDs)
	if err != nil {
		return nil, nil, err
	}

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
