// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

type bgpPeerCache struct {
	mu         sync.RWMutex
	lastUpdate time.Time
	updateTime time.Time
	entries    map[string]*bgpPeerEntry
}

type bgpPeerEntry struct {
	key     string
	scope   string
	tags    map[string]string
	updated bool

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
		entries: make(map[string]*bgpPeerEntry),
	}
}

func (entry *bgpPeerEntry) reset() {
	key := entry.key
	scope := entry.scope
	tags := entry.tags
	if tags == nil {
		tags = make(map[string]string)
	} else {
		clear(tags)
	}

	*entry = bgpPeerEntry{
		key:   key,
		scope: scope,
		tags:  tags,
	}
}

func (c *bgpPeerCache) reset() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.updateTime = time.Now()
	for _, entry := range c.entries {
		entry.reset()
	}
}

func (c *bgpPeerCache) updateEntry(metric ddsnmp.Metric) {
	if c == nil || !metric.IsTable || !isBGPPeerFunctionMetric(metric.Name) || len(metric.Tags) == 0 {
		return
	}

	scope := bgpPeerEntryScope(metric.Name)
	if scope == "" {
		return
	}

	key := bgpPeerEntryKey(scope, metric.Tags)
	if key == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[key]
	if entry == nil {
		entry = &bgpPeerEntry{
			key:   key,
			scope: scope,
			tags:  make(map[string]string, len(metric.Tags)),
		}
		c.entries[key] = entry
	}
	mergeBGPPeerEntryTags(entry, metric.Tags)

	switch bgpPeerMetricLeaf(metric.Name) {
	case "availability":
		entry.adminStatus = bgpAdminStatus(metric.MultiValue)
	case "connection_state":
		entry.state = activeMultiValueDimension(metric.MultiValue)
	case "previous_connection_state":
		entry.previousState = activeMultiValueDimension(metric.MultiValue)
	case "established_uptime":
		entry.establishedUptime = metricDimValuePtr(metric.MultiValue, "uptime")
	case "update_traffic":
		entry.updateCounts = maps.Clone(metric.MultiValue)
	case "message_traffic":
		entry.messageCounts = maps.Clone(metric.MultiValue)
	case "notification_traffic":
		entry.notificationCounts = maps.Clone(metric.MultiValue)
	case "route_refresh_traffic":
		entry.routeRefreshCounts = maps.Clone(metric.MultiValue)
	case "open_traffic":
		entry.openCounts = maps.Clone(metric.MultiValue)
	case "keepalive_traffic":
		entry.keepaliveCounts = maps.Clone(metric.MultiValue)
	case "established_transitions":
		entry.establishedCount = metricDimValuePtr(metric.MultiValue, "transitions")
	case "down_transitions":
		entry.downTransitions = metricDimValuePtr(metric.MultiValue, "transitions")
	case "up_transitions":
		entry.upTransitions = metricDimValuePtr(metric.MultiValue, "transitions")
	case "flaps":
		entry.flaps = metricDimValuePtr(metric.MultiValue, "flaps")
	case "last_received_update_age":
		entry.lastReceivedUpdate = metricDimValuePtr(metric.MultiValue, "age")
	case "last_error":
		setBGPPeerLastError(entry, metric.MultiValue)
	case "last_down_reason":
		entry.lastDownReason = humanizeBGPLabel(activeMultiValueDimension(metric.MultiValue))
	case "last_received_notification_reason":
		entry.lastRecvNotify = humanizeBGPLabel(activeMultiValueDimension(metric.MultiValue))
	case "last_sent_notification_reason":
		entry.lastSentNotify = humanizeBGPLabel(activeMultiValueDimension(metric.MultiValue))
	case "graceful_restart_state":
		entry.gracefulRestart = humanizeBGPLabel(activeMultiValueDimension(metric.MultiValue))
	case "unavailability_reason":
		entry.unavailabilityReason = humanizeBGPLabel(activeMultiValueDimension(metric.MultiValue))
	case "route_counts.current":
		entry.routeCounts = maps.Clone(metric.MultiValue)
	case "route_totals":
		entry.routeTotals = maps.Clone(metric.MultiValue)
	case "route_limits":
		entry.routeLimits = maps.Clone(metric.MultiValue)
	case "route_limit_thresholds":
		entry.routeLimitThresholds = maps.Clone(metric.MultiValue)
	}

	entry.updated = true
}

func mergeBGPPeerEntryTags(entry *bgpPeerEntry, tags map[string]string) {
	for key, value := range tags {
		if value == "" || strings.HasPrefix(key, "_") {
			continue
		}
		entry.tags[key] = value
	}

	for key, value := range tags {
		if value == "" || !strings.HasPrefix(key, "_") {
			continue
		}
		normalizedKey := strings.TrimPrefix(key, "_")
		if normalizedKey == "" {
			continue
		}
		if tags[normalizedKey] != "" {
			continue
		}
		entry.tags[normalizedKey] = value
	}
}

func (c *bgpPeerCache) finalize() {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if !entry.updated {
			delete(c.entries, key)
		}
	}

	c.lastUpdate = c.updateTime
}

func bgpPeerEntryScope(name string) string {
	switch {
	case strings.HasPrefix(name, "bgp.peers."):
		return "peers"
	case strings.HasPrefix(name, "bgp.peer_families."):
		return "peer_families"
	default:
		return ""
	}
}

func bgpPeerMetricLeaf(name string) string {
	switch {
	case strings.HasPrefix(name, "bgp.peers."):
		return strings.TrimPrefix(name, "bgp.peers.")
	case strings.HasPrefix(name, "bgp.peer_families."):
		return strings.TrimPrefix(name, "bgp.peer_families.")
	default:
		return name
	}
}

func bgpPeerEntryKey(scope string, tags map[string]string) string {
	identityTags, requiredTags := bgpPeerIdentityKeyTags(scope)
	if len(identityTags) == 0 {
		return ""
	}

	for _, key := range requiredTags {
		if bgpTagValue(tags, key) == "" {
			return ""
		}
	}

	var sb strings.Builder
	bgpWritePeerKeyPart(&sb, scope)
	for _, key := range identityTags {
		bgpWritePeerKeyPart(&sb, key)
		bgpWritePeerKeyPart(&sb, bgpTagValue(tags, key))
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

func bgpAdminStatus(mv map[string]int64) string {
	if len(mv) == 0 {
		return ""
	}
	if v, ok := mv["admin_enabled"]; ok {
		if v == 1 {
			return "enabled"
		}
		if v == 0 {
			return "disabled"
		}
	}
	if mv["admin_disabled"] == 1 {
		return "disabled"
	}
	return ""
}

func activeMultiValueDimension(mv map[string]int64) string {
	if len(mv) == 0 {
		return ""
	}

	keys := make([]string, 0, len(mv))
	for key := range mv {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	for _, key := range keys {
		if mv[key] == 1 {
			return key
		}
	}
	return ""
}

func metricDimValuePtr(mv map[string]int64, key string) *int64 {
	if len(mv) == 0 {
		return nil
	}
	v, ok := mv[key]
	if !ok {
		return nil
	}
	return int64Ptr(v)
}

func setBGPPeerLastError(entry *bgpPeerEntry, mv map[string]int64) {
	code := metricDimValuePtr(mv, "code")
	subcode := metricDimValuePtr(mv, "subcode")
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
