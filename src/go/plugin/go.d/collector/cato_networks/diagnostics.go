// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"net"
	"strings"
)

const (
	operationDiscovery   = "entityLookup"
	operationSnapshot    = "accountSnapshot"
	operationMetrics     = "accountMetrics"
	operationEvents      = "eventsFeed"
	operationBGP         = "siteBgpStatus"
	operationEventMarker = "eventsMarker"
)

const (
	normalizationSurfaceSiteConnectivity = "site_connectivity"
	normalizationSurfaceSiteOperational  = "site_operational"
	normalizationSurfaceMetrics          = "metrics"
	normalizationSurfaceBGP              = "bgp"
	normalizationSurfaceEvents           = "events"
)

const (
	normalizationIssueUnknownStatus          = "unknown_status"
	normalizationIssueUnknownTimeseriesLabel = "unknown_timeseries_label"
	normalizationIssueEmptyPeer              = "empty_peer"
	normalizationIssueParseInt               = "parse_int"
	normalizationIssueAccountError           = "account_error"
	normalizationIssueCardinalityLimit       = "cardinality_limit"
	normalizationIssuePageCap                = "page_cap"
	normalizationIssueMarkerStalled          = "marker_stalled"
	normalizationIssueEmptyEventType         = "empty_event_type"
	normalizationIssueEmptyEventSubType      = "empty_event_sub_type"
	normalizationIssueEmptyEventSeverity     = "empty_event_severity"
	normalizationIssueEmptyEventStatus       = "empty_event_status"
	normalizationIssueComplexEventField      = "complex_event_field"
)

type collectorHealth struct {
	CollectionSuccess bool
	DiscoveredSites   int64

	MarkerPersistenceAvailable bool
	BGPSitesPerCollection      int64
	BGPFullScanSeconds         int64
	BGPCachedSites             int64

	SelectedEntities        map[string]int64
	SkippedEntities         map[entitySkipKey]int64
	CardinalityLimitHits    map[string]int64
	LastOperations          map[string]operationHealth
	OperationFailures       map[operationFailureKey]int64
	OperationAffectedSites  map[operationFailureKey]int64
	CollectionFailureTotals map[string]int64
	NormalizationIssues     map[normalizationIssueKey]int64
}

type operationHealth struct {
	Success    bool
	ErrorClass string
}

type operationFailureKey struct {
	Operation  string
	ErrorClass string
}

type entitySkipKey struct {
	Entity string
	Reason string
}

type normalizationIssueKey struct {
	Surface string
	Issue   string
}

func (c *Collector) beginHealthCycle() {
	c.ensureHealth()
	c.health.CollectionSuccess = false
	c.health.MarkerPersistenceAvailable = !c.eventsEnabled() || c.markerStoreAvailable
	c.health.BGPSitesPerCollection = 0
	c.health.BGPFullScanSeconds = 0
	c.health.BGPCachedSites = 0
	c.health.SelectedEntities = make(map[string]int64)
	c.health.SkippedEntities = make(map[entitySkipKey]int64)
	c.health.CardinalityLimitHits = make(map[string]int64)
	c.updateSiteSelectionHealth()
	// LastOperations is stateful because the chart reports last observed status.
}

func (c *Collector) markOperationSuccess(operation string) {
	c.ensureHealth()
	c.health.LastOperations[operation] = operationHealth{Success: true, ErrorClass: "none"}
}

func (c *Collector) markOperationFailure(operation string, err error) {
	c.ensureHealth()
	class := classifyCatoError(err)
	c.health.LastOperations[operation] = operationHealth{ErrorClass: class}
	c.health.OperationFailures[operationFailureKey{Operation: operation, ErrorClass: class}]++
}

func (c *Collector) markOperationAffectedSites(operation string, err error, count int) {
	if count <= 0 {
		return
	}
	c.ensureHealth()
	class := classifyCatoError(err)
	c.health.OperationAffectedSites[operationFailureKey{Operation: operation, ErrorClass: class}] += int64(count)
}

func (c *Collector) markCollectionFailure(err error) {
	c.ensureHealth()
	c.health.CollectionFailureTotals[classifyCatoError(err)]++
}

func (c *Collector) markNormalizationIssue(surface, issue string) {
	c.ensureHealth()
	c.health.NormalizationIssues[normalizationIssueKey{Surface: surface, Issue: issue}]++
}

func (c *Collector) markEntitySelection(entity string, selected, skippedSelector, skippedLimit int) {
	c.ensureHealth()
	c.health.SelectedEntities[entity] = int64(selected)
	if skippedSelector > 0 {
		c.health.SkippedEntities[entitySkipKey{Entity: entity, Reason: selectionSkipSelector}] = int64(skippedSelector)
	}
	if skippedLimit > 0 {
		c.health.SkippedEntities[entitySkipKey{Entity: entity, Reason: selectionSkipLimit}] = int64(skippedLimit)
		c.health.CardinalityLimitHits[entity] = 1
	}
}

func (c *Collector) updateSiteSelectionHealth() {
	c.ensureHealth()
	total := c.discovery.totalSites
	if total == 0 && len(c.discovery.siteIDs) > 0 {
		total = len(c.discovery.siteIDs) + c.discovery.skippedBySelector + c.discovery.skippedByLimit
	}
	c.health.DiscoveredSites = int64(total)
	c.markEntitySelection(selectionEntitySite, len(c.discovery.siteIDs), c.discovery.skippedBySelector, c.discovery.skippedByLimit)
}

func (c *Collector) ensureHealth() {
	if c.health.SelectedEntities == nil {
		c.health.SelectedEntities = make(map[string]int64)
	}
	if c.health.SkippedEntities == nil {
		c.health.SkippedEntities = make(map[entitySkipKey]int64)
	}
	if c.health.CardinalityLimitHits == nil {
		c.health.CardinalityLimitHits = make(map[string]int64)
	}
	if c.health.LastOperations == nil {
		c.health.LastOperations = make(map[string]operationHealth)
	}
	if c.health.OperationFailures == nil {
		c.health.OperationFailures = make(map[operationFailureKey]int64)
	}
	if c.health.OperationAffectedSites == nil {
		c.health.OperationAffectedSites = make(map[operationFailureKey]int64)
	}
	if c.health.CollectionFailureTotals == nil {
		c.health.CollectionFailureTotals = make(map[string]int64)
	}
	if c.health.NormalizationIssues == nil {
		c.health.NormalizationIssues = make(map[normalizationIssueKey]int64)
	}
}

func cloneCollectorHealth(src collectorHealth) collectorHealth {
	dst := src
	if src.SelectedEntities != nil {
		dst.SelectedEntities = make(map[string]int64, len(src.SelectedEntities))
		for k, v := range src.SelectedEntities {
			dst.SelectedEntities[k] = v
		}
	}
	if src.SkippedEntities != nil {
		dst.SkippedEntities = make(map[entitySkipKey]int64, len(src.SkippedEntities))
		for k, v := range src.SkippedEntities {
			dst.SkippedEntities[k] = v
		}
	}
	if src.CardinalityLimitHits != nil {
		dst.CardinalityLimitHits = make(map[string]int64, len(src.CardinalityLimitHits))
		for k, v := range src.CardinalityLimitHits {
			dst.CardinalityLimitHits[k] = v
		}
	}
	if src.LastOperations != nil {
		dst.LastOperations = make(map[string]operationHealth, len(src.LastOperations))
		for k, v := range src.LastOperations {
			dst.LastOperations[k] = v
		}
	}
	if src.OperationFailures != nil {
		dst.OperationFailures = make(map[operationFailureKey]int64, len(src.OperationFailures))
		for k, v := range src.OperationFailures {
			dst.OperationFailures[k] = v
		}
	}
	if src.OperationAffectedSites != nil {
		dst.OperationAffectedSites = make(map[operationFailureKey]int64, len(src.OperationAffectedSites))
		for k, v := range src.OperationAffectedSites {
			dst.OperationAffectedSites[k] = v
		}
	}
	if src.CollectionFailureTotals != nil {
		dst.CollectionFailureTotals = make(map[string]int64, len(src.CollectionFailureTotals))
		for k, v := range src.CollectionFailureTotals {
			dst.CollectionFailureTotals[k] = v
		}
	}
	if src.NormalizationIssues != nil {
		dst.NormalizationIssues = make(map[normalizationIssueKey]int64, len(src.NormalizationIssues))
		for k, v := range src.NormalizationIssues {
			dst.NormalizationIssues[k] = v
		}
	}
	return dst
}

func classifyCatoError(err error) string {
	if err == nil {
		return "none"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg, "rate limit", "rate-limit", "ratelimit", "too many requests"),
		containsHTTPStatus(msg, "429"):
		return "rate_limit"
	case containsAny(msg, "unauthorized", "forbidden", "invalid api key"),
		containsHTTPStatus(msg, "401"),
		containsHTTPStatus(msg, "403"):
		return "auth"
	case containsAny(msg, "json", "decode", "unmarshal"):
		return "decode"
	case containsAny(msg, "deadline", "timeout", "i/o timeout"):
		return "timeout"
	case containsAny(msg, "proxyconnect", "proxy error", "proxy url", "proxy"):
		return "proxy"
	case containsAny(msg, "x509", "certificate", "tls", "handshake"):
		return "tls"
	case containsAny(msg, "no such host", "connection refused", "connection reset", "network is unreachable",
		"temporary failure in name resolution", "server misbehaving", "dial tcp", "eof"):
		return "network"
	case containsAny(msg, "no cato sites", "no sites"):
		return "empty"
	case strings.Contains(msg, "pagination"):
		return "pagination"
	case strings.Contains(msg, "graphql"):
		return "graphql"
	default:
		return "error"
	}
}

func containsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

func containsHTTPStatus(msg, code string) bool {
	return strings.Contains(msg, "http status "+code) ||
		strings.Contains(msg, "http "+code) ||
		strings.Contains(msg, "status code "+code) ||
		strings.Contains(msg, "status "+code) ||
		strings.Contains(msg, " "+code+" ")
}
