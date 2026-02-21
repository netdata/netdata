// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

// TopologyDataOptions controls conversion from Result to topology.Data.
type TopologyDataOptions struct {
	SchemaVersion string
	Source        string
	Layer         string
	View          string
	AgentID       string
	LocalDeviceID string
	CollectedAt   time.Time
}

type endpointActorAccumulator struct {
	endpointID string
	mac        string
	ips        map[string]netip.Addr
	sources    map[string]struct{}
	ifIndexes  map[string]struct{}
	ifNames    map[string]struct{}
}

type builtAdjacencyLink struct {
	adj      Adjacency
	protocol string
	link     topology.Link
}

type pairedLinkAccumulator struct {
	source *builtAdjacencyLink
	target *builtAdjacencyLink
}

type projectedLinks struct {
	links               []topology.Link
	lldp                int
	cdp                 int
	bidirectionalCount  int
	unidirectionalCount int
}

// ToTopologyData converts an engine result to the shared topology schema.
func ToTopologyData(result Result, opts TopologyDataOptions) topology.Data {
	schemaVersion := strings.TrimSpace(opts.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = "2.0"
	}

	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "snmp"
	}

	layer := strings.TrimSpace(opts.Layer)
	if layer == "" {
		layer = "2"
	}

	view := strings.TrimSpace(opts.View)
	if view == "" {
		view = "summary"
	}

	collectedAt := opts.CollectedAt
	if collectedAt.IsZero() {
		collectedAt = result.CollectedAt
	}
	if collectedAt.IsZero() {
		collectedAt = time.Now().UTC()
	}

	deviceByID := make(map[string]Device, len(result.Devices))
	ifaceByDeviceIndex := make(map[string]Interface, len(result.Interfaces))
	ifIndexByDeviceName := make(map[string]int, len(result.Interfaces))

	for _, dev := range result.Devices {
		deviceByID[dev.ID] = dev
	}

	for _, iface := range result.Interfaces {
		if iface.IfIndex <= 0 {
			continue
		}
		ifaceByDeviceIndex[deviceIfIndexKey(iface.DeviceID, iface.IfIndex)] = iface
		ifName := strings.TrimSpace(iface.IfName)
		if ifName != "" {
			ifIndexByDeviceName[deviceIfNameKey(iface.DeviceID, ifName)] = iface.IfIndex
		}
	}

	actors := make([]topology.Actor, 0, len(result.Devices))
	actorIndex := make(map[string]struct{}, len(result.Devices)*2)
	for _, dev := range result.Devices {
		actor := deviceToTopologyActor(dev, source, layer, opts.LocalDeviceID)
		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) == 0 {
			continue
		}
		if topologyIdentityIndexOverlaps(actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(actorIndex, keys)
		actors = append(actors, actor)
	}

	projected := projectAdjacencyLinks(result.Adjacencies, layer, collectedAt, deviceByID, ifIndexByDeviceName)
	links := projected.links
	linksLLDP := projected.lldp
	linksCDP := projected.cdp

	endpointActors := buildEndpointActors(result.Attachments, result.Enrichments, ifaceByDeviceIndex, source, layer, actorIndex)
	actors = append(actors, endpointActors.actors...)
	sortTopologyActors(actors)

	stats := cloneAnyMap(result.Stats)
	if stats == nil {
		stats = make(map[string]any)
	}
	stats["devices_total"] = len(result.Devices)
	stats["devices_discovered"] = discoveredDeviceCount(result.Devices, opts.LocalDeviceID)
	stats["links_total"] = len(links)
	stats["links_lldp"] = linksLLDP
	stats["links_cdp"] = linksCDP
	stats["links_bidirectional"] = projected.bidirectionalCount
	stats["links_unidirectional"] = projected.unidirectionalCount
	stats["links_fdb"] = 0
	stats["links_arp"] = 0
	stats["actors_total"] = len(actors)
	stats["endpoints_total"] = endpointActors.count

	return topology.Data{
		SchemaVersion: schemaVersion,
		Source:        source,
		Layer:         layer,
		AgentID:       opts.AgentID,
		CollectedAt:   collectedAt,
		View:          view,
		Actors:        actors,
		Links:         links,
		Stats:         stats,
	}
}

func projectAdjacencyLinks(
	adjacencies []Adjacency,
	layer string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
) projectedLinks {
	out := projectedLinks{
		links: make([]topology.Link, 0, len(adjacencies)),
	}
	if len(adjacencies) == 0 {
		return out
	}

	pairs := make(map[string]*pairedLinkAccumulator)
	pairOrder := make([]string, 0)

	for _, adj := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		link := adjacencyToTopologyLink(adj, protocol, layer, collectedAt, deviceByID, ifIndexByDeviceName)

		pairID := strings.TrimSpace(adj.Labels[adjacencyLabelPairID])
		pairSide := strings.TrimSpace(adj.Labels[adjacencyLabelPairSide])
		if pairID != "" && (pairSide == adjacencyPairSideSource || pairSide == adjacencyPairSideTarget) {
			acc := pairs[pairID]
			if acc == nil {
				acc = &pairedLinkAccumulator{}
				pairs[pairID] = acc
				pairOrder = append(pairOrder, pairID)
			}

			entry := &builtAdjacencyLink{
				adj:      adj,
				protocol: protocol,
				link:     link,
			}
			switch pairSide {
			case adjacencyPairSideSource:
				if acc.source == nil {
					acc.source = entry
					continue
				}
			case adjacencyPairSideTarget:
				if acc.target == nil {
					acc.target = entry
					continue
				}
			}
		}

		out.links = append(out.links, link)
		incrementProjectedProtocolCounters(&out, protocol, false)
	}

	for _, pairID := range pairOrder {
		acc := pairs[pairID]
		if acc == nil {
			continue
		}

		if acc.source != nil && acc.target != nil {
			merged := acc.source.link
			merged.Direction = "bidirectional"
			merged.Src = mergeEndpointIPHints(acc.source.link.Src, acc.target.link.Dst)
			merged.Dst = mergeEndpointIPHints(acc.target.link.Src, acc.source.link.Dst)
			merged.Metrics = buildPairedLinkMetrics(acc.source.adj.Labels, acc.target.adj.Labels)
			out.links = append(out.links, merged)
			incrementProjectedProtocolCounters(&out, acc.source.protocol, true)
			continue
		}

		if acc.source != nil {
			out.links = append(out.links, acc.source.link)
			incrementProjectedProtocolCounters(&out, acc.source.protocol, false)
		}
		if acc.target != nil {
			out.links = append(out.links, acc.target.link)
			incrementProjectedProtocolCounters(&out, acc.target.protocol, false)
		}
	}

	sortTopologyLinks(out.links)
	return out
}

func adjacencyToTopologyLink(
	adj Adjacency,
	protocol string,
	layer string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
) topology.Link {
	src := adjacencySideToEndpoint(deviceByID[adj.SourceID], adj.SourcePort, ifIndexByDeviceName)
	dst := adjacencySideToEndpoint(deviceByID[adj.TargetID], adj.TargetPort, ifIndexByDeviceName)
	if rawAddress := strings.TrimSpace(adj.Labels["remote_address_raw"]); rawAddress != "" {
		dst.Match.IPAddresses = uniqueTopologyStrings(append(dst.Match.IPAddresses, rawAddress))
	}

	link := topology.Link{
		Layer:        layer,
		Protocol:     protocol,
		Direction:    "unidirectional",
		Src:          src,
		Dst:          dst,
		DiscoveredAt: topologyTimePtr(collectedAt),
		LastSeen:     topologyTimePtr(collectedAt),
	}
	if len(adj.Labels) > 0 {
		link.Metrics = mapStringStringToAny(adj.Labels)
	}
	return link
}

func buildPairedLinkMetrics(sourceLabels, targetLabels map[string]string) map[string]any {
	metrics := make(map[string]any)

	pairID := strings.TrimSpace(sourceLabels[adjacencyLabelPairID])
	if pairID == "" {
		pairID = strings.TrimSpace(targetLabels[adjacencyLabelPairID])
	}
	if pairID != "" {
		metrics[adjacencyLabelPairID] = pairID
	}

	pairPass := strings.TrimSpace(sourceLabels[adjacencyLabelPairPass])
	if pairPass == "" {
		pairPass = strings.TrimSpace(targetLabels[adjacencyLabelPairPass])
	}
	if pairPass != "" {
		metrics[adjacencyLabelPairPass] = pairPass
	}
	metrics["pair_consistent"] = true

	for key, value := range sourceLabels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || isPairLabelKey(key) {
			continue
		}
		metrics["src_"+key] = value
	}
	for key, value := range targetLabels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || isPairLabelKey(key) {
			continue
		}
		metrics["dst_"+key] = value
	}

	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

func mergeEndpointIPHints(base, extra topology.LinkEndpoint) topology.LinkEndpoint {
	if len(extra.Match.IPAddresses) == 0 {
		return base
	}
	base.Match.IPAddresses = uniqueTopologyStrings(append(base.Match.IPAddresses, extra.Match.IPAddresses...))
	return base
}

func isPairLabelKey(key string) bool {
	return key == adjacencyLabelPairID || key == adjacencyLabelPairSide || key == adjacencyLabelPairPass
}

func incrementProjectedProtocolCounters(out *projectedLinks, protocol string, bidirectional bool) {
	if out == nil {
		return
	}
	switch protocol {
	case "lldp":
		out.lldp++
	case "cdp":
		out.cdp++
	}
	if bidirectional {
		out.bidirectionalCount++
		return
	}
	out.unidirectionalCount++
}

func deviceToTopologyActor(dev Device, source, layer, localDeviceID string) topology.Actor {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}

	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			match.MacAddresses = []string{mac}
		}
	}

	if len(dev.Addresses) > 0 {
		ips := make([]string, 0, len(dev.Addresses))
		for _, addr := range dev.Addresses {
			if !addr.IsValid() {
				continue
			}
			ips = append(ips, addr.String())
		}
		match.IPAddresses = uniqueTopologyStrings(ips)
	}

	discovered := true
	if strings.TrimSpace(localDeviceID) != "" && dev.ID == localDeviceID {
		discovered = false
	}

	attrs := map[string]any{
		"device_id":              dev.ID,
		"discovered":             discovered,
		"management_ip":          firstAddress(dev.Addresses),
		"management_addresses":   addressStrings(dev.Addresses),
		"capabilities":           labelsCSVToSlice(dev.Labels, "capabilities"),
		"capabilities_supported": labelsCSVToSlice(dev.Labels, "capabilities_supported"),
		"capabilities_enabled":   labelsCSVToSlice(dev.Labels, "capabilities_enabled"),
	}

	return topology.Actor{
		ActorType:  "device",
		Layer:      layer,
		Source:     source,
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
		Labels:     cloneStringMap(dev.Labels),
	}
}

func adjacencySideToEndpoint(dev Device, port string, ifIndexByDeviceName map[string]int) topology.LinkEndpoint {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}
	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			match.MacAddresses = []string{mac}
		}
	}
	for _, addr := range dev.Addresses {
		if !addr.IsValid() {
			continue
		}
		match.IPAddresses = append(match.IPAddresses, addr.String())
	}
	match.IPAddresses = uniqueTopologyStrings(match.IPAddresses)

	port = strings.TrimSpace(port)
	ifName := port
	ifIndex := 0
	if port != "" {
		ifIndex = ifIndexByDeviceName[deviceIfNameKey(dev.ID, port)]
		if ifIndex == 0 {
			if n, err := strconv.Atoi(port); err == nil && n > 0 {
				ifIndex = n
			}
		}
	}
	if ifIndex > 0 && ifName == "" {
		ifName = strconv.Itoa(ifIndex)
	}

	attrs := map[string]any{
		"if_index":      ifIndex,
		"if_name":       ifName,
		"port_id":       port,
		"sys_name":      strings.TrimSpace(dev.Hostname),
		"management_ip": firstAddress(dev.Addresses),
	}

	return topology.LinkEndpoint{
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
	}
}

type builtEndpointActors struct {
	actors []topology.Actor
	count  int
}

func buildEndpointActors(
	attachments []Attachment,
	enrichments []Enrichment,
	ifaceByDeviceIndex map[string]Interface,
	source string,
	layer string,
	actorIndex map[string]struct{},
) builtEndpointActors {
	accumulators := make(map[string]*endpointActorAccumulator)

	for _, attachment := range attachments {
		endpointID := strings.TrimSpace(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		acc := ensureEndpointActorAccumulator(accumulators, endpointID)
		addEndpointIDIdentity(acc, endpointID)
		if method := strings.TrimSpace(attachment.Method); method != "" {
			acc.sources[strings.ToLower(method)] = struct{}{}
		}
		if attachment.IfIndex > 0 {
			acc.ifIndexes[strconv.Itoa(attachment.IfIndex)] = struct{}{}
			iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(strings.TrimSpace(attachment.DeviceID), attachment.IfIndex)]
			if ok {
				if ifName := strings.TrimSpace(iface.IfName); ifName != "" {
					acc.ifNames[ifName] = struct{}{}
				}
			}
		}
		if ifName := strings.TrimSpace(attachment.Labels["if_name"]); ifName != "" {
			acc.ifNames[ifName] = struct{}{}
		}
	}

	for _, enrichment := range enrichments {
		endpointID := strings.TrimSpace(enrichment.EndpointID)
		if endpointID == "" {
			continue
		}
		acc := ensureEndpointActorAccumulator(accumulators, endpointID)
		addEndpointIDIdentity(acc, endpointID)

		if mac := normalizeMAC(enrichment.MAC); mac != "" {
			acc.mac = mac
		}
		for _, ip := range enrichment.IPs {
			if ip.IsValid() {
				acc.ips[ip.String()] = ip.Unmap()
			}
		}
		for _, sourceName := range csvToSet(enrichment.Labels["sources"]) {
			acc.sources[sourceName] = struct{}{}
		}
		for _, ifIndex := range csvToSet(enrichment.Labels["if_indexes"]) {
			acc.ifIndexes[ifIndex] = struct{}{}
		}
		for _, ifName := range csvToSet(enrichment.Labels["if_names"]) {
			acc.ifNames[ifName] = struct{}{}
		}
	}

	if len(accumulators) == 0 {
		return builtEndpointActors{}
	}

	keys := make([]string, 0, len(accumulators))
	for endpointID := range accumulators {
		keys = append(keys, endpointID)
	}
	sort.Strings(keys)

	actors := make([]topology.Actor, 0, len(keys))
	endpointCount := 0
	for _, endpointID := range keys {
		acc := accumulators[endpointID]
		if acc == nil {
			continue
		}

		match := topology.Match{}
		if acc.mac != "" {
			match.ChassisIDs = []string{acc.mac}
			match.MacAddresses = []string{acc.mac}
		}
		match.IPAddresses = sortedEndpointIPs(acc.ips)

		attrs := map[string]any{
			"discovered":         true,
			"learned_sources":    sortedTopologySet(acc.sources),
			"learned_if_indexes": sortedTopologySet(acc.ifIndexes),
			"learned_if_names":   sortedTopologySet(acc.ifNames),
		}
		actor := topology.Actor{
			ActorType:  "endpoint",
			Layer:      layer,
			Source:     source,
			Match:      match,
			Attributes: pruneTopologyAttributes(attrs),
		}

		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) == 0 {
			continue
		}
		if topologyIdentityIndexOverlaps(actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(actorIndex, keys)

		actors = append(actors, actor)
		endpointCount++
	}

	return builtEndpointActors{actors: actors, count: endpointCount}
}

func ensureEndpointActorAccumulator(accumulators map[string]*endpointActorAccumulator, endpointID string) *endpointActorAccumulator {
	acc := accumulators[endpointID]
	if acc != nil {
		return acc
	}
	acc = &endpointActorAccumulator{
		endpointID: endpointID,
		ips:        make(map[string]netip.Addr),
		sources:    make(map[string]struct{}),
		ifIndexes:  make(map[string]struct{}),
		ifNames:    make(map[string]struct{}),
	}
	accumulators[endpointID] = acc
	return acc
}

func addEndpointIDIdentity(acc *endpointActorAccumulator, endpointID string) {
	if acc == nil {
		return
	}
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		if mac := normalizeMAC(value); mac != "" {
			acc.mac = mac
		}
	case "ip":
		if addr := parseAddr(value); addr.IsValid() {
			acc.ips[addr.String()] = addr.Unmap()
		}
	}
}

func discoveredDeviceCount(devices []Device, localDeviceID string) int {
	if len(devices) == 0 {
		return 0
	}

	localDeviceID = strings.TrimSpace(localDeviceID)
	if localDeviceID == "" {
		return maxIntValue(len(devices)-1, 0)
	}

	count := 0
	for _, dev := range devices {
		if strings.TrimSpace(dev.ID) == "" {
			continue
		}
		if dev.ID == localDeviceID {
			continue
		}
		count++
	}
	return count
}

func maxIntValue(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func addressStrings(addresses []netip.Addr) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if !addr.IsValid() {
			continue
		}
		out = append(out, addr.String())
	}
	out = uniqueTopologyStrings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstAddress(addresses []netip.Addr) string {
	values := addressStrings(addresses)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func uniqueTopologyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedEndpointIPs(in map[string]netip.Addr) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		addr, ok := in[key]
		if !ok || !addr.IsValid() {
			continue
		}
		out = append(out, addr.String())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedTopologySet(in map[string]struct{}) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func csvToSet(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func labelsCSVToSlice(labels map[string]string, key string) []string {
	if len(labels) == 0 {
		return nil
	}
	return csvToSet(labels[key])
}

func pruneTopologyAttributes(attrs map[string]any) map[string]any {
	for key, value := range attrs {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				delete(attrs, key)
			}
		case []string:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case map[string]string:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case map[string]any:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case int:
			if typed == 0 {
				delete(attrs, key)
			}
		case nil:
			delete(attrs, key)
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func mapStringStringToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func deviceIfNameKey(deviceID, ifName string) string {
	return fmt.Sprintf("%s|%s", strings.TrimSpace(deviceID), strings.ToLower(strings.TrimSpace(ifName)))
}

func topologyIdentityIndexOverlaps(index map[string]struct{}, keys []string) bool {
	if len(index) == 0 || len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := index[key]; ok {
			return true
		}
	}
	return false
}

func addTopologyIdentityKeys(index map[string]struct{}, keys []string) {
	if index == nil || len(keys) == 0 {
		return
	}
	for _, key := range keys {
		index[key] = struct{}{}
	}
}

func topologyMatchIdentityKeys(match topology.Match) []string {
	seen := make(map[string]struct{}, 8)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + ":" + value
		seen[key] = struct{}{}
	}

	for _, value := range match.ChassisIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("chassis", strings.ToLower(value))
	}

	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
		}
	}
	for _, value := range match.IPAddresses {
		if ip := normalizeTopologyIP(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("ipraw", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.Hostnames {
		add("hostname", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.DNSNames {
		add("dns", strings.ToLower(strings.TrimSpace(value)))
	}
	if sysName := strings.TrimSpace(match.SysName); sysName != "" {
		add("sysname", strings.ToLower(sysName))
	}

	if len(seen) == 0 {
		return nil
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeTopologyIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	addr := parseAddr(value)
	if !addr.IsValid() {
		return ""
	}
	return addr.Unmap().String()
}

func canonicalTopologyMatchKey(match topology.Match) string {
	if key := canonicalTopologyHardwareKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalTopologyMACListKey(match.MacAddresses); key != "" {
		return "mac:" + key
	}
	if key := canonicalTopologyIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalTopologyStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalTopologyStringListKey(match.DNSNames); key != "" {
		return "dns:" + key
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func canonicalTopologyHardwareKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyMACListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyIPListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyStringListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func topologyLinkSortKey(link topology.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalTopologyMatchKey(link.Src.Match),
		canonicalTopologyMatchKey(link.Dst.Match),
		topologyAttrKey(link.Src.Attributes, "if_index"),
		topologyAttrKey(link.Src.Attributes, "if_name"),
		topologyAttrKey(link.Src.Attributes, "port_id"),
		topologyAttrKey(link.Dst.Attributes, "if_index"),
		topologyAttrKey(link.Dst.Attributes, "if_name"),
		topologyAttrKey(link.Dst.Attributes, "port_id"),
		link.State,
	}, "|")
}

func topologyAttrKey(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func sortTopologyActors(actors []topology.Actor) {
	sort.SliceStable(actors, func(i, j int) bool {
		a, b := actors[i], actors[j]
		if a.ActorType != b.ActorType {
			return a.ActorType < b.ActorType
		}
		ak := canonicalTopologyMatchKey(a.Match)
		bk := canonicalTopologyMatchKey(b.Match)
		if ak != bk {
			return ak < bk
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Layer < b.Layer
	})
}

func sortTopologyLinks(links []topology.Link) {
	sort.SliceStable(links, func(i, j int) bool {
		return topologyLinkSortKey(links[i]) < topologyLinkSortKey(links[j])
	})
}

func topologyTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	out := t
	return &out
}
