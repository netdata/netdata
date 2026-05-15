// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type bgpPeerCache struct {
	mu          sync.RWMutex
	lastUpdate  time.Time
	lastFailure time.Time
	lastError   string
	staleAfter  time.Duration
	updateTime  time.Time
	entries     map[string]*bgpPeerEntry
}

type bgpPeerCacheSnapshot struct {
	entries []*bgpPeerEntry
	stale   bool
	expired bool
	message string
}

type bgpPeerEntry struct {
	key     string
	scope   string
	source  string
	tags    map[string]string
	updated bool
	stale   bool

	lastUpdate  time.Time
	lastFailure time.Time
	lastError   string

	adminStatus   string
	state         string
	previousState string

	establishedUptime    *int64
	lastReceivedUpdate   *int64
	establishedCount     *int64
	downTransitions      *int64
	upTransitions        *int64
	flaps                *int64
	lastErrorCode        *int64
	lastErrorSubcode     *int64
	lastErrorText        string
	lastDownReason       string
	lastRecvNotify       string
	lastSentNotify       string
	gracefulRestart      string
	unavailabilityReason string

	updateCounts         map[string]int64
	messageCounts        map[string]int64
	notificationCounts   map[string]int64
	routeRefreshCounts   map[string]int64
	openCounts           map[string]int64
	keepaliveCounts      map[string]int64
	routeCounts          map[string]int64
	routeTotals          map[string]int64
	routeLimits          map[string]int64
	routeLimitThresholds map[string]int64
}

var (
	bgpPeerIdentityTags               = []string{"routing_instance", "neighbor", "remote_as"}
	bgpPeerRequiredIdentityTags       = []string{"neighbor", "remote_as"}
	bgpPeerFamilyIdentityTags         = []string{"routing_instance", "neighbor", "remote_as", "address_family", "subsequent_address_family"}
	bgpPeerFamilyRequiredIdentityTags = []string{"neighbor", "remote_as", "address_family", "subsequent_address_family"}
)

func newBGPPeerCache() *bgpPeerCache {
	return &bgpPeerCache{
		staleAfter: 30 * time.Second,
		entries:    make(map[string]*bgpPeerEntry),
	}
}

func (c *bgpPeerCache) setStaleAfter(d time.Duration) {
	if c == nil || d <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.staleAfter = d
}

func (entry *bgpPeerEntry) reset() {
	key := entry.key
	scope := entry.scope
	source := entry.source
	tags := entry.tags
	if tags == nil {
		tags = make(map[string]string)
	} else {
		clear(tags)
	}

	*entry = bgpPeerEntry{
		key:    key,
		scope:  scope,
		source: source,
		tags:   tags,
	}
}

func (c *bgpPeerCache) reset() {
	c.resetExceptSources(nil)
}

func (c *bgpPeerCache) resetExceptSources(excludedSources map[string]bool) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.updateTime = time.Now()
	for _, entry := range c.entries {
		if len(excludedSources) > 0 && excludedSources[entry.source] {
			entry.stale = true
			continue
		}
		entry.reset()
	}
}

func (c *bgpPeerCache) markCollectFailed(err error) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastFailure = time.Now()
	if err == nil {
		c.lastError = ""
		return
	}
	c.lastError = err.Error()
}

func (c *bgpPeerCache) markSourcesCollectFailed(sources map[string]bool, err error) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	marked := 0
	now := time.Now()
	lastError := ""
	if err != nil {
		lastError = err.Error()
	}
	for _, entry := range c.entries {
		if len(sources) == 0 || sources[entry.source] {
			entry.stale = true
			entry.lastFailure = now
			entry.lastError = lastError
			marked++
		}
	}
	if len(sources) > 0 && marked == 0 {
		return
	}

	if len(sources) == 0 {
		c.lastFailure = now
		c.lastError = lastError
	}
}

func (c *bgpPeerCache) updateRow(source string, row ddsnmp.BGPRow) {
	if c == nil {
		return
	}

	scope := bgpPeerEntryScopeFromRow(row)
	if scope == "" {
		return
	}

	key := row.StructuralID
	if key == "" {
		key = bgpPeerEntryKey(scope, bgpPeerEntryTagsFromRow(row))
	}
	if key == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[key]
	if entry == nil {
		entry = &bgpPeerEntry{
			key:    key,
			scope:  scope,
			source: source,
			tags:   make(map[string]string),
		}
		c.entries[key] = entry
	}
	entry.scope = scope
	entry.source = firstNonEmpty(source, entry.source)
	maps.Copy(entry.tags, bgpPeerEntryTagsFromRow(row))

	if row.Admin.Enabled.Has {
		if row.Admin.Enabled.Value {
			entry.adminStatus = "enabled"
		} else {
			entry.adminStatus = "disabled"
		}
	}
	if row.State.Has {
		entry.state = string(row.State.State)
	}
	if row.Previous.Has {
		entry.previousState = string(row.Previous.State)
	}
	entry.establishedUptime = bgpValuePtr(row.Connection.EstablishedUptime)
	entry.lastReceivedUpdate = bgpValuePtr(row.Connection.LastReceivedUpdateAge)
	entry.establishedCount = bgpValuePtr(row.Transitions.Established)
	entry.downTransitions = bgpValuePtr(row.Transitions.Down)
	entry.upTransitions = bgpValuePtr(row.Transitions.Up)
	entry.flaps = bgpValuePtr(row.Transitions.Flaps)
	entry.updateCounts = bgpDirectionalMap(row.Traffic.Updates)
	entry.messageCounts = bgpDirectionalMap(row.Traffic.Messages)
	entry.notificationCounts = bgpDirectionalMap(row.Traffic.Notifications)
	entry.routeRefreshCounts = bgpDirectionalMap(row.Traffic.RouteRefreshes)
	entry.openCounts = bgpDirectionalMap(row.Traffic.Opens)
	entry.keepaliveCounts = bgpDirectionalMap(row.Traffic.Keepalives)
	setBGPPeerLastErrorFromRow(entry, row.LastError)
	entry.lastDownReason = humanizeBGPLabel(row.Reasons.LastDown.Value)
	entry.lastRecvNotify = humanizeBGPLabel(row.LastNotify.Received.Reason.Value)
	entry.lastSentNotify = humanizeBGPLabel(row.LastNotify.Sent.Reason.Value)
	entry.gracefulRestart = humanizeBGPLabel(row.Restart.State.Value)
	entry.unavailabilityReason = humanizeBGPLabel(row.Reasons.Unavailability.Value)
	entry.routeCounts = bgpRouteCountersMap(row.Routes.Current)
	entry.routeTotals = bgpRouteCountersMap(row.Routes.Total)
	entry.routeLimits = bgpRouteLimitsMap(row.RouteLimits)
	entry.routeLimitThresholds = bgpRouteLimitThresholdsMap(row.RouteLimits)
	entry.updated = true
	entry.stale = false
}

func bgpPeerEntryScopeFromRow(row ddsnmp.BGPRow) string {
	switch row.Kind {
	case ddprofiledefinition.BGPRowKindPeer:
		return bgpPeersViewPeers
	case ddprofiledefinition.BGPRowKindPeerFamily:
		return bgpPeersViewFamilies
	default:
		return ""
	}
}

func bgpPeerEntryTagsFromRow(row ddsnmp.BGPRow) map[string]string {
	tags := maps.Clone(row.Tags)
	if tags == nil {
		tags = make(map[string]string)
	}
	add := func(key, value string) {
		if value != "" {
			tags[key] = value
		}
	}
	add("routing_instance", bgpRoutingInstance(row))
	add("neighbor", row.Identity.Neighbor)
	add("remote_as", row.Identity.RemoteAS)
	add("address_family", string(row.Identity.AddressFamily))
	add("subsequent_address_family", string(row.Identity.SubsequentAddressFamily))
	add("local_address", row.Descriptors.LocalAddress)
	add("local_as", row.Descriptors.LocalAS)
	add("local_identifier", row.Descriptors.LocalIdentifier)
	add("peer_identifier", row.Descriptors.PeerIdentifier)
	add("peer_type", row.Descriptors.PeerType)
	add("bgp_version", row.Descriptors.BGPVersion)
	add("peer_description", row.Descriptors.Description)
	return tags
}

func bgpValuePtr(value ddsnmp.BGPInt64) *int64 {
	if !value.Has {
		return nil
	}
	return int64Ptr(value.Value)
}

func bgpDirectionalMap(value ddsnmp.BGPDirectional) map[string]int64 {
	result := make(map[string]int64)
	if value.Received.Has {
		result["received"] = value.Received.Value
	}
	if value.Sent.Has {
		result["sent"] = value.Sent.Value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func setBGPPeerLastErrorFromRow(entry *bgpPeerEntry, value ddsnmp.BGPLastError) {
	code := bgpValuePtr(value.Code)
	subcode := bgpValuePtr(value.Subcode)
	if isNoBGPLastError(code, subcode) {
		entry.lastErrorCode = nil
		entry.lastErrorSubcode = nil
		entry.lastErrorText = ""
		return
	}
	entry.lastErrorCode = code
	entry.lastErrorSubcode = subcode
	entry.lastErrorText = bgpLastErrorText(code, subcode)
}

func bgpRouteCountersMap(value ddsnmp.BGPRouteCounters) map[string]int64 {
	result := make(map[string]int64)
	add := func(key string, value ddsnmp.BGPInt64) {
		if value.Has {
			result[key] = value.Value
		}
	}
	add("received", value.Received)
	add("accepted", value.Accepted)
	add("rejected", value.Rejected)
	add("active", value.Active)
	add("advertised", value.Advertised)
	add("suppressed", value.Suppressed)
	add("withdrawn", value.Withdrawn)
	if len(result) == 0 {
		return nil
	}
	return result
}

func bgpRouteLimitsMap(value ddsnmp.BGPRouteLimits) map[string]int64 {
	if !value.Limit.Has {
		return nil
	}
	return map[string]int64{"admin_limit": value.Limit.Value}
}

func bgpRouteLimitThresholdsMap(value ddsnmp.BGPRouteLimits) map[string]int64 {
	result := make(map[string]int64)
	if value.Threshold.Has {
		result["threshold"] = value.Threshold.Value
	}
	if value.ClearThreshold.Has {
		result["clear_threshold"] = value.ClearThreshold.Value
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (c *bgpPeerCache) finalize() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.updateTime
	if now.IsZero() {
		now = time.Now()
	}

	hadUpdatedEntries := false
	for key, entry := range c.entries {
		if !entry.updated && !entry.stale {
			delete(c.entries, key)
			continue
		}
		if !entry.updated && entry.stale && !entry.lastUpdate.IsZero() && c.staleAfter > 0 && now.Sub(entry.lastUpdate) >= c.staleAfter {
			delete(c.entries, key)
			continue
		}
		if entry.updated {
			hadUpdatedEntries = true
			entry.lastUpdate = now
			entry.lastFailure = time.Time{}
			entry.lastError = ""
		}
		entry.updated = false
	}

	if hadUpdatedEntries || len(c.entries) == 0 {
		c.lastUpdate = now
	}
	c.lastFailure = time.Time{}
	c.lastError = ""
}

func (c *bgpPeerCache) snapshot(now time.Time) bgpPeerCacheSnapshot {
	if c == nil {
		return bgpPeerCacheSnapshot{expired: true, message: "BGP peer data not available yet, please retry after data collection"}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	state := c.snapshotState(now)
	entries := make([]*bgpPeerEntry, 0, len(c.entries))
	hasEntryStale := false
	nonExpiredEntries := 0
	entryStaleMessage := ""
	entryExpired := make(map[string]bool, len(c.entries))
	for _, entry := range c.entries {
		if entry.stale {
			hasEntryStale = true
			expired, message := entry.staleSnapshot(now, c.staleAfter)
			entryExpired[entry.key] = expired
			if message != "" && entryStaleMessage == "" {
				entryStaleMessage = message
			}
			if !expired {
				nonExpiredEntries++
			}
		} else {
			nonExpiredEntries++
		}
	}
	for _, entry := range c.entries {
		if hasEntryStale && entryExpired[entry.key] && nonExpiredEntries > 0 {
			continue
		}
		clone := entry.clone()
		clone.stale = entry.stale || (!hasEntryStale && state.stale)
		entries = append(entries, clone)
	}
	if hasEntryStale {
		state.stale = nonExpiredEntries > 0
		state.expired = nonExpiredEntries == 0
		state.message = entryStaleMessage
	}
	state.entries = entries
	return state
}

func (c *bgpPeerCache) snapshotState(now time.Time) bgpPeerCacheSnapshot {
	if len(c.entries) == 0 {
		return bgpPeerCacheSnapshot{message: "no BGP peer data is available for this device"}
	}
	if c.lastFailure.IsZero() || (!c.lastUpdate.IsZero() && !c.lastFailure.After(c.lastUpdate)) {
		return bgpPeerCacheSnapshot{}
	}
	if c.lastUpdate.IsZero() {
		return bgpPeerCacheSnapshot{expired: true, message: bgpStaleMessage("no successful BGP collection yet", c.lastError)}
	}

	age := now.Sub(c.lastUpdate)
	if c.staleAfter > 0 && age >= c.staleAfter {
		return bgpPeerCacheSnapshot{expired: true, message: bgpStaleMessage(fmt.Sprintf("last successful BGP collection was %s ago", age.Round(time.Second)), c.lastError)}
	}
	return bgpPeerCacheSnapshot{stale: true, message: bgpStaleMessage(fmt.Sprintf("showing stale BGP rows from %s ago", age.Round(time.Second)), c.lastError)}
}

func bgpStaleMessage(prefix, err string) string {
	if err == "" {
		return prefix
	}
	return prefix + ": " + err
}

func (entry *bgpPeerEntry) staleSnapshot(now time.Time, staleAfter time.Duration) (bool, string) {
	if entry == nil || !entry.stale {
		return false, ""
	}
	if entry.lastUpdate.IsZero() {
		return true, bgpStaleMessage("no successful BGP collection yet", entry.lastError)
	}
	age := now.Sub(entry.lastUpdate)
	if staleAfter > 0 && age >= staleAfter {
		return true, bgpStaleMessage(fmt.Sprintf("last successful BGP collection was %s ago", age.Round(time.Second)), entry.lastError)
	}
	return false, bgpStaleMessage(fmt.Sprintf("showing stale BGP rows from %s ago", age.Round(time.Second)), entry.lastError)
}

func (entry *bgpPeerEntry) clone() *bgpPeerEntry {
	if entry == nil {
		return nil
	}
	clone := *entry
	clone.tags = maps.Clone(entry.tags)
	clone.updateCounts = maps.Clone(entry.updateCounts)
	clone.messageCounts = maps.Clone(entry.messageCounts)
	clone.notificationCounts = maps.Clone(entry.notificationCounts)
	clone.routeRefreshCounts = maps.Clone(entry.routeRefreshCounts)
	clone.openCounts = maps.Clone(entry.openCounts)
	clone.keepaliveCounts = maps.Clone(entry.keepaliveCounts)
	clone.routeCounts = maps.Clone(entry.routeCounts)
	clone.routeTotals = maps.Clone(entry.routeTotals)
	clone.routeLimits = maps.Clone(entry.routeLimits)
	clone.routeLimitThresholds = maps.Clone(entry.routeLimitThresholds)
	return &clone
}

func bgpPeerEntryKey(scope string, tags map[string]string) string {
	identityTags, requiredTags := bgpPeerIdentityKeyTags(scope)
	if len(identityTags) == 0 {
		return ""
	}

	for _, key := range requiredTags {
		if tags[key] == "" {
			return ""
		}
	}

	var sb strings.Builder
	bgpWritePeerKeyPart(&sb, scope)
	for _, key := range identityTags {
		bgpWritePeerKeyPart(&sb, key)
		bgpWritePeerKeyPart(&sb, tags[key])
	}
	return sb.String()
}

func bgpPeerIdentityKeyTags(scope string) ([]string, []string) {
	switch scope {
	case "peers":
		return bgpPeerIdentityTags, bgpPeerRequiredIdentityTags
	case "peer_families":
		return bgpPeerFamilyIdentityTags, bgpPeerFamilyRequiredIdentityTags
	default:
		return nil, nil
	}
}

func isNoBGPLastError(code, subcode *int64) bool {
	return (code == nil || *code == 0) && (subcode == nil || *subcode == 0)
}

func int64Ptr(v int64) *int64 {
	value := v
	return &value
}

func mapValuePtr(m map[string]int64, key string) *int64 {
	if len(m) == 0 {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	return int64Ptr(v)
}

func humanizeBGPLabel(value string) string {
	if value == "" {
		return ""
	}
	return strings.ReplaceAll(value, "_", " ")
}

func bgpWritePeerKeyPart(sb *strings.Builder, value string) {
	sb.WriteString(strconv.Itoa(len(value)))
	sb.WriteByte(':')
	sb.WriteString(value)
}
