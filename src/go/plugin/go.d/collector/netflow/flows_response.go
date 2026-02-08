// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

const (
	flowsSchemaVersion = "2.0"
	flowsSource        = "netflow"
	flowsLayer         = "3"

	viewSummary = "aggregated"
	viewLive    = "detailed"

	summaryAggIP         = "ip"
	summaryAggIPProtocol = "ip_protocol"
	summaryAggFiveTuple  = "five_tuple"
)

type endpointInfo struct {
	match     topology.Match
	attrs     map[string]any
	aggregate bool
}

type linkAggregate struct {
	src       endpointInfo
	dst       endpointInfo
	proto     int
	direction string
	srcPort   int
	dstPort   int
	firstSeen time.Time
	lastSeen  time.Time
	metrics   map[string]uint64
}

func (c *Collector) buildFlowsResponse(snapshot flowData, view string) topology.Data {
	if view != viewLive {
		view = viewSummary
	}

	collectedAt := snapshot.PeriodEnd
	if collectedAt.IsZero() {
		collectedAt = time.Now()
	}

	ipPolicy := &topology.IPPolicy{
		PublicAllowlist: c.Flows.PublicAllowlist,
		LiveTopN: &topology.LiveTopN{
			Enabled: c.Flows.LiveTopN.Enabled,
			Limit:   c.Flows.LiveTopN.Limit,
			SortBy:  c.Flows.LiveTopN.SortBy,
		},
	}

	data := topology.Data{
		SchemaVersion: flowsSchemaVersion,
		Source:        flowsSource,
		Layer:         flowsLayer,
		AgentID:       snapshot.AgentID,
		CollectedAt:   collectedAt,
		View:          view,
		IPPolicy:      ipPolicy,
		Actors:        []topology.Actor{},
		Links:         []topology.Link{},
		Flows:         []topology.Flow{},
		Stats:         snapshot.Summaries,
		Metrics:       snapshot.Metrics,
	}

	exporters := c.exporterActors(snapshot.Exporters)
	actorIndex := make(map[string]topology.Actor)
	for _, actor := range exporters {
		key := actorKey(actor)
		if key == "" {
			continue
		}
		actorIndex[key] = actor
	}

	switch view {
	case viewLive:
		flows, flowActors := c.buildLiveFlows(snapshot)
		data.Flows = flows
		for _, actor := range flowActors {
			c.mergeActor(actorIndex, actor)
		}
	case viewSummary:
		links, linkActors := c.buildSummaryLinks(snapshot)
		data.Links = links
		for _, actor := range linkActors {
			c.mergeActor(actorIndex, actor)
		}
	}

	data.Actors = mapActors(actorIndex)
	return data
}

func (c *Collector) mergeActor(idx map[string]topology.Actor, actor topology.Actor) {
	key := actorKey(actor)
	if key == "" {
		return
	}
	if _, ok := idx[key]; ok {
		return
	}
	idx[key] = actor
}

func (c *Collector) exporterActors(exporters []flowExporter) []topology.Actor {
	actors := make([]topology.Actor, 0, len(exporters))
	for _, exp := range exporters {
		if exp.ExporterIP == "" {
			continue
		}
		match := topology.Match{IPAddresses: []string{exp.ExporterIP}}
		attrs := map[string]any{}
		if exp.ExporterName != "" {
			attrs["name"] = exp.ExporterName
		}
		if exp.SamplingRate > 0 {
			attrs["sampling_rate"] = exp.SamplingRate
		}
		if exp.FlowVersion != "" {
			attrs["flow_version"] = exp.FlowVersion
		}
		actors = append(actors, topology.Actor{
			ActorType:  "exporter",
			Layer:      flowsLayer,
			Source:     flowsSource,
			Match:      match,
			Attributes: attrs,
		})
	}
	return actors
}

func (c *Collector) buildLiveFlows(snapshot flowData) ([]topology.Flow, []topology.Actor) {
	latest := latestBuckets(snapshot.Buckets)
	if len(latest) == 0 {
		return nil, nil
	}

	sortBy := strings.ToLower(c.Flows.LiveTopN.SortBy)
	buckets := append([]flowBucket(nil), latest...)
	sort.Slice(buckets, func(i, j int) bool {
		left := buckets[i]
		right := buckets[j]
		if sortBy == "packets" {
			if left.Packets == right.Packets {
				return left.Bytes > right.Bytes
			}
			return left.Packets > right.Packets
		}
		if left.Bytes == right.Bytes {
			return left.Packets > right.Packets
		}
		return left.Bytes > right.Bytes
	})

	limit := c.Flows.LiveTopN.Limit
	if c.Flows.LiveTopN.Enabled && limit > 0 && len(buckets) > limit {
		buckets = buckets[:limit]
	}

	exporterMap := exporterByIP(snapshot.Exporters)
	flows := make([]topology.Flow, 0, len(buckets))
	actors := make([]topology.Actor, 0, len(buckets)*2)
	actorIndex := make(map[string]struct{})

	for _, bucket := range buckets {
		flow, flowActors := c.flowFromBucket(bucket, exporterMap)
		if flow == nil {
			continue
		}
		flows = append(flows, *flow)
		for _, actor := range flowActors {
			key := actorKey(actor)
			if key == "" {
				continue
			}
			if _, ok := actorIndex[key]; ok {
				continue
			}
			actorIndex[key] = struct{}{}
			actors = append(actors, actor)
		}
	}

	return flows, actors
}

func (c *Collector) buildSummaryLinks(snapshot flowData) ([]topology.Link, []topology.Actor) {
	if len(snapshot.Buckets) == 0 {
		return nil, nil
	}

	aggregation := c.Flows.SummaryAggregation
	if aggregation == "" {
		aggregation = summaryAggIPProtocol
	}

	aggregates := make(map[string]*linkAggregate)
	actorIndex := make(map[string]struct{})
	actors := make([]topology.Actor, 0)

	for _, bucket := range snapshot.Buckets {
		key := bucket.Key
		if key == nil {
			continue
		}

		src := c.endpointFromPrefix(key.SrcPrefix)
		dst := c.endpointFromPrefix(key.DstPrefix)
		if len(src.match.IPAddresses) == 0 || len(dst.match.IPAddresses) == 0 {
			continue
		}

		linkKey := buildLinkKey(aggregation, src, dst, key, bucket.Direction)
		if linkKey == "" {
			continue
		}

		agg := aggregates[linkKey]
		if agg == nil {
			agg = &linkAggregate{
				src:       src,
				dst:       dst,
				proto:     key.Protocol,
				direction: bucket.Direction,
				srcPort:   key.SrcPort,
				dstPort:   key.DstPort,
				firstSeen: bucket.Timestamp,
				lastSeen:  bucket.Timestamp,
				metrics: map[string]uint64{
					"bytes":   bucket.Bytes,
					"packets": bucket.Packets,
					"flows":   bucket.Flows,
				},
			}
			if bucket.RawBytes > 0 {
				agg.metrics["raw_bytes"] = bucket.RawBytes
			}
			if bucket.RawPackets > 0 {
				agg.metrics["raw_packets"] = bucket.RawPackets
			}
			aggregates[linkKey] = agg
		} else {
			if bucket.Timestamp.Before(agg.firstSeen) || agg.firstSeen.IsZero() {
				agg.firstSeen = bucket.Timestamp
			}
			if bucket.Timestamp.After(agg.lastSeen) {
				agg.lastSeen = bucket.Timestamp
			}
			agg.metrics["bytes"] += bucket.Bytes
			agg.metrics["packets"] += bucket.Packets
			agg.metrics["flows"] += bucket.Flows
			if bucket.RawBytes > 0 {
				agg.metrics["raw_bytes"] += bucket.RawBytes
			}
			if bucket.RawPackets > 0 {
				agg.metrics["raw_packets"] += bucket.RawPackets
			}
		}

		for _, actor := range []topology.Actor{ipActorFromEndpoint(src), ipActorFromEndpoint(dst)} {
			key := actorKey(actor)
			if key == "" {
				continue
			}
			if _, ok := actorIndex[key]; ok {
				continue
			}
			actorIndex[key] = struct{}{}
			actors = append(actors, actor)
		}

	}

	links := make([]topology.Link, 0, len(aggregates))
	for _, agg := range aggregates {
		protocol := protocolName(agg.proto)
		if aggregation == summaryAggIP {
			protocol = "ip"
		}

		srcAttrs := cloneAttrs(agg.src.attrs)
		dstAttrs := cloneAttrs(agg.dst.attrs)
		if aggregation == summaryAggFiveTuple {
			if agg.srcPort > 0 {
				srcAttrs["port"] = agg.srcPort
			}
			if agg.dstPort > 0 {
				dstAttrs["port"] = agg.dstPort
			}
		}

		link := topology.Link{
			Layer:     flowsLayer,
			Protocol:  protocol,
			Direction: agg.direction,
			Src:       topology.LinkEndpoint{Match: agg.src.match, Attributes: srcAttrs},
			Dst:       topology.LinkEndpoint{Match: agg.dst.match, Attributes: dstAttrs},
			Metrics:   mapStringUint64(agg.metrics),
		}
		if !agg.firstSeen.IsZero() {
			link.DiscoveredAt = timePtr(agg.firstSeen)
		}
		if !agg.lastSeen.IsZero() {
			link.LastSeen = timePtr(agg.lastSeen)
		}
		links = append(links, link)
	}

	return links, actors
}

func (c *Collector) flowFromBucket(bucket flowBucket, exporters map[string]flowExporter) (*topology.Flow, []topology.Actor) {
	if bucket.Key == nil {
		return nil, nil
	}

	src := c.endpointFromPrefix(bucket.Key.SrcPrefix)
	dst := c.endpointFromPrefix(bucket.Key.DstPrefix)
	if len(src.match.IPAddresses) == 0 || len(dst.match.IPAddresses) == 0 {
		return nil, nil
	}

	exporter := exporterInfo(bucket, exporters)
	flow := topology.Flow{
		Timestamp:   bucket.Timestamp,
		DurationSec: bucket.DurationSec,
		Exporter:    exporter,
		Src:         topology.LinkEndpoint{Match: src.match, Attributes: cloneAttrs(src.attrs)},
		Dst:         topology.LinkEndpoint{Match: dst.match, Attributes: cloneAttrs(dst.attrs)},
		Key:         flowKeyAttributes(bucket),
		Metrics:     flowMetrics(bucket),
	}

	actors := []topology.Actor{ipActorFromEndpoint(src), ipActorFromEndpoint(dst)}
	return &flow, actors
}

func flowMetrics(bucket flowBucket) map[string]any {
	metrics := map[string]any{
		"bytes":   bucket.Bytes,
		"packets": bucket.Packets,
		"flows":   bucket.Flows,
	}
	if bucket.RawBytes > 0 {
		metrics["raw_bytes"] = bucket.RawBytes
	}
	if bucket.RawPackets > 0 {
		metrics["raw_packets"] = bucket.RawPackets
	}
	if bucket.SamplingRate > 0 {
		metrics["sampling_rate"] = bucket.SamplingRate
	}
	return metrics
}

func flowKeyAttributes(bucket flowBucket) map[string]any {
	key := map[string]any{}
	if bucket.Key == nil {
		return key
	}
	if bucket.Key.SrcPrefix != "" {
		key["src_prefix"] = bucket.Key.SrcPrefix
	}
	if bucket.Key.DstPrefix != "" {
		key["dst_prefix"] = bucket.Key.DstPrefix
	}
	if bucket.Key.SrcPort > 0 {
		key["src_port"] = bucket.Key.SrcPort
	}
	if bucket.Key.DstPort > 0 {
		key["dst_port"] = bucket.Key.DstPort
	}
	if bucket.Key.Protocol > 0 {
		key["protocol"] = bucket.Key.Protocol
	}
	if bucket.Key.SrcAS > 0 {
		key["src_as"] = bucket.Key.SrcAS
	}
	if bucket.Key.DstAS > 0 {
		key["dst_as"] = bucket.Key.DstAS
	}
	if bucket.Key.InIf > 0 {
		key["in_if"] = bucket.Key.InIf
	}
	if bucket.Key.OutIf > 0 {
		key["out_if"] = bucket.Key.OutIf
	}
	if bucket.Direction != "" {
		key["direction"] = bucket.Direction
	}
	return key
}

func exporterByIP(exporters []flowExporter) map[string]flowExporter {
	out := make(map[string]flowExporter, len(exporters))
	for _, exp := range exporters {
		if exp.ExporterIP == "" {
			continue
		}
		out[exp.ExporterIP] = exp
	}
	return out
}

func exporterInfo(bucket flowBucket, exporters map[string]flowExporter) *topology.FlowExporter {
	if bucket.ExporterIP == "" {
		return nil
	}
	exp := exporters[bucket.ExporterIP]
	info := topology.FlowExporter{IP: bucket.ExporterIP}
	if exp.ExporterName != "" {
		info.Name = exp.ExporterName
	}
	if exp.SamplingRate > 0 {
		info.SamplingRate = exp.SamplingRate
	} else if bucket.SamplingRate > 0 {
		info.SamplingRate = bucket.SamplingRate
	}
	if exp.FlowVersion != "" {
		info.FlowVersion = exp.FlowVersion
	}
	return &info
}

func buildLinkKey(aggregation string, src, dst endpointInfo, key *flowKey, direction string) string {
	if key == nil {
		return ""
	}
	srcID := endpointKey(src)
	dstID := endpointKey(dst)
	if srcID == "" || dstID == "" {
		return ""
	}

	switch aggregation {
	case summaryAggIP:
		return fmt.Sprintf("%s|%s|%s", srcID, dstID, direction)
	case summaryAggFiveTuple:
		return fmt.Sprintf("%s|%s|%d|%d|%d|%s", srcID, dstID, key.Protocol, key.SrcPort, key.DstPort, direction)
	default:
		return fmt.Sprintf("%s|%s|%d|%s", srcID, dstID, key.Protocol, direction)
	}
}

func endpointKey(ep endpointInfo) string {
	if len(ep.match.IPAddresses) > 0 {
		return "ip:" + strings.Join(ep.match.IPAddresses, ",")
	}
	if len(ep.match.Hostnames) > 0 {
		return "host:" + strings.Join(ep.match.Hostnames, ",")
	}
	return ""
}

func ipActorFromEndpoint(ep endpointInfo) topology.Actor {
	actorType := "ip"
	attrs := cloneAttrs(ep.attrs)
	if ep.aggregate {
		actorType = "ip-aggregate"
		attrs["synthetic"] = true
	}
	return topology.Actor{
		ActorType:  actorType,
		Layer:      flowsLayer,
		Source:     flowsSource,
		Match:      ep.match,
		Attributes: attrs,
	}
}

func (c *Collector) endpointFromPrefix(prefix string) endpointInfo {
	addr, ipStr := parsePrefixIP(prefix)
	if ipStr == "" {
		return endpointInfo{}
	}

	aggregate := false
	matchIP := ipStr
	if isPublicIP(addr) && !c.isAllowlisted(addr) {
		matchIP = "public:other"
		aggregate = true
	}

	match := topology.Match{IPAddresses: []string{matchIP}}
	attrs := map[string]any{}
	if aggregate {
		attrs["aggregate"] = "public:other"
	}
	return endpointInfo{match: match, attrs: attrs, aggregate: aggregate}
}

func parsePrefixIP(prefix string) (netip.Addr, string) {
	if prefix == "" {
		return netip.Addr{}, ""
	}
	ipStr := prefix
	if idx := strings.Index(prefix, "/"); idx >= 0 {
		ipStr = prefix[:idx]
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(ipStr))
	if err != nil || !addr.IsValid() {
		return netip.Addr{}, ""
	}
	return addr, addr.String()
}

func isPublicIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	if addr.IsLoopback() || addr.IsMulticast() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified() {
		return false
	}
	if addr.IsPrivate() {
		return false
	}
	return addr.IsGlobalUnicast()
}

func (c *Collector) isAllowlisted(addr netip.Addr) bool {
	if !addr.IsValid() || len(c.allowlist) == 0 {
		return false
	}
	for _, r := range c.allowlist {
		if r.Contains(addr) {
			return true
		}
	}
	return false
}

func latestBuckets(buckets []flowBucket) []flowBucket {
	var latest time.Time
	for _, bucket := range buckets {
		if bucket.Timestamp.After(latest) {
			latest = bucket.Timestamp
		}
	}
	if latest.IsZero() {
		return nil
	}
	var out []flowBucket
	for _, bucket := range buckets {
		if bucket.Timestamp.Equal(latest) {
			out = append(out, bucket)
		}
	}
	return out
}

func actorMatchKey(match topology.Match) string {
	if len(match.IPAddresses) > 0 {
		return "ip:" + strings.Join(match.IPAddresses, ",")
	}
	if len(match.Hostnames) > 0 {
		return "host:" + strings.Join(match.Hostnames, ",")
	}
	if match.SysName != "" {
		return "sys:" + match.SysName
	}
	return ""
}

func actorKey(actor topology.Actor) string {
	matchKey := actorMatchKey(actor.Match)
	if matchKey == "" {
		return ""
	}
	if actor.ActorType == "" {
		return matchKey
	}
	return actor.ActorType + "|" + matchKey
}

func protocolName(proto int) string {
	switch proto {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 47:
		return "gre"
	case 58:
		return "icmpv6"
	case 132:
		return "sctp"
	default:
		if proto <= 0 {
			return "unknown"
		}
		return fmt.Sprintf("%d", proto)
	}
}

func timePtr(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	return &ts
}

func mapStringUint64(src map[string]uint64) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneAttrs(src map[string]any) map[string]any {
	if len(src) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func mapActors(idx map[string]topology.Actor) []topology.Actor {
	if len(idx) == 0 {
		return nil
	}
	actors := make([]topology.Actor, 0, len(idx))
	for _, actor := range idx {
		actors = append(actors, actor)
	}
	sort.Slice(actors, func(i, j int) bool {
		return actorKey(actors[i]) < actorKey(actors[j])
	})
	return actors
}
