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

type eventsCollection struct {
	counts []eventCount
	marker string
}

func (c *Collector) collectEvents(ctx context.Context) (eventsCollection, error) {
	currentMarker := strings.TrimSpace(c.eventMarker)
	var marker *string
	if currentMarker != "" {
		marker = &currentMarker
	}

	counts := make(map[eventKey]int64)
	seenEventIDs := make(map[string]bool)
	var finalMarker string
	var cardinalityLimited bool

	for page := 0; page < c.Events.MaxPagesPerCycle; page++ {
		res, err := c.client.EventsFeed(ctx, c.AccountID, marker)
		if err != nil {
			c.markOperationFailure(operationEvents, err)
			return eventsCollection{}, err
		}
		c.markOperationSuccess(operationEvents)

		feed := res.GetEventsFeed()
		newMarker := strings.TrimSpace(ptrString(feed.GetMarker()))
		if marker != nil && newMarker == *marker {
			c.markNormalizationIssue(normalizationSurfaceEvents, normalizationIssueMarkerStalled)
			break
		}
		if newMarker != "" {
			finalMarker = newMarker
		}

		for _, account := range feed.GetAccounts() {
			if errString := ptrString(account.GetErrorString()); errString != "" {
				c.markNormalizationIssue(normalizationSurfaceEvents, normalizationIssueAccountError)
				c.warnRecoverable(warningKeyEventAccountErr, "account_error", "eventsFeed returned an account-level error; skipping account events")
				c.Debugf("eventsFeed account-level error metadata: page=%d error_size=%d", page+1, len(errString))
				err := fmt.Errorf("eventsFeed account error")
				c.markOperationFailure(operationEvents, err)
				return eventsCollection{}, err
			}
			for _, record := range account.GetRecords() {
				fields := record.GetFieldsMap()
				if eventID := eventRecordID(fields); eventID != "" {
					if seenEventIDs[eventID] {
						continue
					}
					seenEventIDs[eventID] = true
				}
				eventType, typeIssue := eventField(fields, "event_type")
				eventSubType, subTypeIssue := eventField(fields, "event_sub_type")
				severity, severityIssue := eventField(fields, "severity")
				status, statusIssue := eventField(fields, "status")
				for _, issue := range []string{typeIssue, subTypeIssue, severityIssue, statusIssue} {
					if issue != "" {
						c.markNormalizationIssue(normalizationSurfaceEvents, issue)
					}
				}
				key := eventKey{
					EventType:    eventType,
					EventSubType: eventSubType,
					Severity:     severity,
					Status:       status,
				}
				if addEventCount(counts, key, c.Events.MaxCardinality) {
					cardinalityLimited = true
				}
			}
		}

		if feed.GetFetchedCount() < eventsFeedMaxFetchSize || newMarker == "" {
			break
		}
		if page == c.Events.MaxPagesPerCycle-1 {
			c.markNormalizationIssue(normalizationSurfaceEvents, normalizationIssuePageCap)
			break
		}
		currentMarker = newMarker
		marker = &currentMarker
	}

	if cardinalityLimited {
		c.markNormalizationIssue(normalizationSurfaceEvents, normalizationIssueCardinalityLimit)
	}

	out := make([]eventCount, 0, len(counts))
	for key, count := range counts {
		out = append(out, eventCount{
			EventType:    key.EventType,
			EventSubType: key.EventSubType,
			Severity:     key.Severity,
			Status:       key.Status,
			Count:        count,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].EventType != out[j].EventType {
			return out[i].EventType < out[j].EventType
		}
		if out[i].EventSubType != out[j].EventSubType {
			return out[i].EventSubType < out[j].EventSubType
		}
		if out[i].Severity != out[j].Severity {
			return out[i].Severity < out[j].Severity
		}
		return out[i].Status < out[j].Status
	})

	return eventsCollection{counts: out, marker: finalMarker}, nil
}

func addEventCount(counts map[eventKey]int64, key eventKey, maxCardinality int) bool {
	key = normalizedEventKey(key)
	if _, ok := counts[key]; ok {
		counts[key]++
		return false
	}

	other := eventKey{EventType: "other", EventSubType: "other", Severity: "other", Status: "other"}
	if len(counts) >= maxCardinality {
		counts[other]++
		return true
	}

	counts[key] = 1
	return false
}

func normalizedEventKey(key eventKey) eventKey {
	key.EventType = normalizedEventField(key.EventType)
	key.EventSubType = normalizedEventField(key.EventSubType)
	key.Severity = normalizedEventField(key.Severity)
	key.Status = normalizedEventField(key.Status)
	return key
}

func normalizedEventField(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func eventField(fields map[string]any, key string) (string, string) {
	v, ok := lookupEventField(fields, key)
	if !ok || v == nil {
		return "", emptyEventFieldIssue(key)
	}
	switch value := v.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return "", emptyEventFieldIssue(key)
		}
		return value, ""
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(value)), ""
	default:
		return "", normalizationIssueComplexEventField
	}
}

func lookupEventField(fields map[string]any, key string) (any, bool) {
	for _, candidate := range eventFieldCandidates(key) {
		if v, ok := fields[candidate]; ok {
			return v, true
		}
	}
	return nil, false
}

func eventRecordID(fields map[string]any) string {
	v, ok := lookupEventField(fields, "event_id")
	if !ok || v == nil {
		return ""
	}
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(value))
	default:
		return ""
	}
}

func eventFieldCandidates(key string) []string {
	switch key {
	case "event_id":
		return []string{"event_id", "eventId"}
	case "event_type":
		return []string{"event_type", "eventType"}
	case "event_sub_type":
		return []string{"event_sub_type", "eventSubType", "event_subtype"}
	case "severity":
		return []string{"severity"}
	case "status":
		return []string{"status"}
	default:
		return []string{key}
	}
}

func emptyEventFieldIssue(key string) string {
	switch key {
	case "event_type":
		return normalizationIssueEmptyEventType
	case "event_sub_type":
		return normalizationIssueEmptyEventSubType
	case "severity":
		return normalizationIssueEmptyEventSeverity
	case "status":
		return normalizationIssueEmptyEventStatus
	default:
		return normalizationIssueComplexEventField
	}
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
