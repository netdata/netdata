// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

type topologyData struct {
	SchemaVersion string           `json:"schema_version"`
	AgentID       string           `json:"agent_id"`
	CollectedAt   time.Time        `json:"collected_at"`
	Devices       []topologyDevice `json:"devices"`
	Links         []topologyLink   `json:"links"`
	Stats         map[string]any   `json:"stats,omitempty"`
	Metrics       map[string]any   `json:"metrics,omitempty"`
}

type topologyDevice struct {
	ChassisID     string            `json:"chassis_id"`
	ChassisIDType string            `json:"chassis_id_type"`
	SysObjectID   string            `json:"sys_object_id,omitempty"`
	SysName       string            `json:"sys_name,omitempty"`
	SysDescr      string            `json:"sys_descr,omitempty"`
	SysLocation   string            `json:"sys_location,omitempty"`
	ManagementIP  string            `json:"management_ip,omitempty"`
	AgentID       string            `json:"agent_id,omitempty"`
	AgentJobID    string            `json:"agent_job_id,omitempty"`
	Vendor        string            `json:"vendor,omitempty"`
	Model         string            `json:"model,omitempty"`
	Capabilities  []string          `json:"capabilities,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	Discovered    bool              `json:"discovered,omitempty"`
}

type topologyEndpoint struct {
	ChassisID     string `json:"chassis_id"`
	ChassisIDType string `json:"chassis_id_type"`
	IfIndex       int    `json:"if_index,omitempty"`
	IfName        string `json:"if_name,omitempty"`
	PortID        string `json:"port_id,omitempty"`
	PortIDType    string `json:"port_id_type,omitempty"`
	PortDescr     string `json:"port_descr,omitempty"`
	SysName       string `json:"sys_name,omitempty"`
	ManagementIP  string `json:"management_ip,omitempty"`
	AgentID       string `json:"agent_id,omitempty"`
}

type topologyLink struct {
	Protocol      string           `json:"protocol"`
	Src           topologyEndpoint `json:"src"`
	Dst           topologyEndpoint `json:"dst"`
	DiscoveredAt  time.Time        `json:"discovered_at,omitempty"`
	LastSeen      time.Time        `json:"last_seen,omitempty"`
	Bidirectional bool             `json:"bidirectional,omitempty"`
	Validated     bool             `json:"validated,omitempty"`
}

type flowsData struct {
	SchemaVersion string         `json:"schema_version"`
	AgentID       string         `json:"agent_id"`
	PeriodStart   time.Time      `json:"period_start"`
	PeriodEnd     time.Time      `json:"period_end"`
	Exporters     []flowExporter `json:"exporters,omitempty"`
	Buckets       []flowBucket   `json:"buckets,omitempty"`
	Summaries     map[string]any `json:"summaries,omitempty"`
	Metrics       map[string]any `json:"metrics,omitempty"`
}

type flowExporter struct {
	ExporterIP   string         `json:"exporter_ip"`
	ExporterName string         `json:"exporter_name,omitempty"`
	SamplingRate int            `json:"sampling_rate,omitempty"`
	FlowVersion  string         `json:"flow_version,omitempty"`
	Interfaces   map[string]any `json:"interfaces,omitempty"`
}

type flowKey struct {
	SrcPrefix string `json:"src_prefix,omitempty"`
	DstPrefix string `json:"dst_prefix,omitempty"`
	SrcPort   int    `json:"src_port,omitempty"`
	DstPort   int    `json:"dst_port,omitempty"`
	Protocol  int    `json:"protocol,omitempty"`
	SrcAS     int    `json:"src_as,omitempty"`
	DstAS     int    `json:"dst_as,omitempty"`
	InIf      int    `json:"in_if,omitempty"`
	OutIf     int    `json:"out_if,omitempty"`
}

type flowBucket struct {
	Timestamp    time.Time `json:"timestamp"`
	DurationSec  int       `json:"duration_sec,omitempty"`
	Key          *flowKey  `json:"key,omitempty"`
	Bytes        uint64    `json:"bytes"`
	Packets      uint64    `json:"packets"`
	Flows        uint64    `json:"flows,omitempty"`
	RawBytes     uint64    `json:"raw_bytes,omitempty"`
	RawPackets   uint64    `json:"raw_packets,omitempty"`
	SamplingRate int       `json:"sampling_rate,omitempty"`
	Direction    string    `json:"direction,omitempty"`
	ExporterIP   string    `json:"exporter_ip,omitempty"`
	AgentID      string    `json:"agent_id,omitempty"`
}

func mergeTopology(inputs []topologyData) topologyData {
	if len(inputs) == 0 {
		return topologyData{}
	}

	deviceMap := make(map[string]topologyDevice)
	linkMap := make(map[string]topologyLink)

	var collectedAt time.Time
	for _, data := range inputs {
		if data.CollectedAt.After(collectedAt) {
			collectedAt = data.CollectedAt
		}
		for _, device := range data.Devices {
			key := deviceKey(device)
			if key == "" {
				continue
			}
			if existing, ok := deviceMap[key]; ok {
				deviceMap[key] = mergeDevice(existing, device)
				continue
			}
			deviceMap[key] = device
		}
		for _, link := range data.Links {
			key := linkKey(link)
			if key == "" {
				continue
			}
			if existing, ok := linkMap[key]; ok {
				linkMap[key] = mergeLink(existing, link)
				continue
			}
			linkMap[key] = link
		}
	}

	devices := make([]topologyDevice, 0, len(deviceMap))
	for _, device := range deviceMap {
		devices = append(devices, device)
	}
	sort.Slice(devices, func(i, j int) bool {
		return devices[i].ChassisID < devices[j].ChassisID
	})

	links := make([]topologyLink, 0, len(linkMap))
	for _, link := range linkMap {
		links = append(links, link)
	}

	stats := map[string]any{
		"devices": len(devices),
		"links":   len(links),
	}

	return topologyData{
		SchemaVersion: inputs[0].SchemaVersion,
		AgentID:       "merged",
		CollectedAt:   collectedAt,
		Devices:       devices,
		Links:         links,
		Stats:         stats,
	}
}

func deviceKey(device topologyDevice) string {
	if device.ChassisID != "" {
		return strings.ToLower(device.ChassisIDType + ":" + device.ChassisID)
	}
	if device.SysObjectID != "" && device.SysName != "" {
		return strings.ToLower(device.SysObjectID + ":" + device.SysName)
	}
	if device.ManagementIP != "" {
		return strings.ToLower("ip:" + device.ManagementIP)
	}
	if device.AgentID != "" && device.AgentJobID != "" {
		return strings.ToLower(device.AgentID + ":" + device.AgentJobID)
	}
	return ""
}

func mergeDevice(a, b topologyDevice) topologyDevice {
	if a.SysObjectID == "" {
		a.SysObjectID = b.SysObjectID
	}
	if a.SysName == "" {
		a.SysName = b.SysName
	}
	if a.SysDescr == "" {
		a.SysDescr = b.SysDescr
	}
	if a.SysLocation == "" {
		a.SysLocation = b.SysLocation
	}
	if a.ManagementIP == "" {
		a.ManagementIP = b.ManagementIP
	}
	if a.AgentID == "" {
		a.AgentID = b.AgentID
	}
	if a.AgentJobID == "" {
		a.AgentJobID = b.AgentJobID
	}
	if a.Vendor == "" {
		a.Vendor = b.Vendor
	}
	if a.Model == "" {
		a.Model = b.Model
	}
	if len(a.Capabilities) == 0 {
		a.Capabilities = b.Capabilities
	}
	if len(a.Labels) == 0 && len(b.Labels) > 0 {
		a.Labels = b.Labels
	} else if len(b.Labels) > 0 {
		if a.Labels == nil {
			a.Labels = make(map[string]string)
		}
		for k, v := range b.Labels {
			if _, ok := a.Labels[k]; !ok {
				a.Labels[k] = v
			}
		}
	}
	a.Discovered = a.Discovered || b.Discovered
	return a
}

func linkKey(link topologyLink) string {
	src := endpointKey(link.Src)
	dst := endpointKey(link.Dst)
	if src == "" || dst == "" {
		return ""
	}
	if src < dst {
		return link.Protocol + ":" + src + "|" + dst
	}
	return link.Protocol + ":" + dst + "|" + src
}

func endpointKey(ep topologyEndpoint) string {
	key := ep.ChassisIDType + ":" + ep.ChassisID
	if ep.PortID != "" {
		return key + ":port:" + ep.PortID
	}
	if ep.IfIndex != 0 {
		return key + ":if:" + fmtInt(ep.IfIndex)
	}
	if ep.IfName != "" {
		return key + ":ifname:" + ep.IfName
	}
	return key
}

func mergeLink(a, b topologyLink) topologyLink {
	a.Bidirectional = a.Bidirectional || b.Bidirectional || (linkKey(a) == linkKey(b))
	a.Validated = a.Validated || b.Validated
	if b.DiscoveredAt.After(a.DiscoveredAt) {
		a.DiscoveredAt = b.DiscoveredAt
	}
	if b.LastSeen.After(a.LastSeen) {
		a.LastSeen = b.LastSeen
	}
	return a
}

func mergeFlows(inputs []flowsData) flowsData {
	if len(inputs) == 0 {
		return flowsData{}
	}

	bucketMap := make(map[flowBucketKey]flowBucket)
	exporterMap := make(map[string]flowExporter)

	periodStart := inputs[0].PeriodStart
	periodEnd := inputs[0].PeriodEnd

	for _, data := range inputs {
		if data.PeriodStart.Before(periodStart) {
			periodStart = data.PeriodStart
		}
		if data.PeriodEnd.After(periodEnd) {
			periodEnd = data.PeriodEnd
		}

		for _, exporter := range data.Exporters {
			if exporter.ExporterIP == "" {
				continue
			}
			if existing, ok := exporterMap[exporter.ExporterIP]; ok {
				if existing.ExporterName == "" {
					existing.ExporterName = exporter.ExporterName
				}
				if existing.SamplingRate == 0 {
					existing.SamplingRate = exporter.SamplingRate
				}
				if existing.FlowVersion == "" {
					existing.FlowVersion = exporter.FlowVersion
				}
				exporterMap[exporter.ExporterIP] = existing
				continue
			}
			exporterMap[exporter.ExporterIP] = exporter
		}

		for _, bucket := range data.Buckets {
			key := flowBucketKey{Timestamp: bucket.Timestamp, Direction: bucket.Direction}
			if bucket.Key != nil {
				key.Key = *bucket.Key
			}
			if existing, ok := bucketMap[key]; ok {
				existing.Bytes += bucket.Bytes
				existing.Packets += bucket.Packets
				existing.Flows += bucket.Flows
				existing.RawBytes += bucket.RawBytes
				existing.RawPackets += bucket.RawPackets
				bucketMap[key] = existing
				continue
			}
			bucketMap[key] = bucket
		}
	}

	buckets := make([]flowBucket, 0, len(bucketMap))
	for _, bucket := range bucketMap {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Timestamp.Before(buckets[j].Timestamp)
	})

	exporters := make([]flowExporter, 0, len(exporterMap))
	for _, exporter := range exporterMap {
		exporters = append(exporters, exporter)
	}
	sort.Slice(exporters, func(i, j int) bool { return exporters[i].ExporterIP < exporters[j].ExporterIP })

	summaries := map[string]any{
		"total_bytes":   uint64(0),
		"total_packets": uint64(0),
		"total_flows":   uint64(0),
		"raw_bytes":     uint64(0),
		"raw_packets":   uint64(0),
	}
	for _, bucket := range buckets {
		summaries["total_bytes"] = summaries["total_bytes"].(uint64) + bucket.Bytes
		summaries["total_packets"] = summaries["total_packets"].(uint64) + bucket.Packets
		summaries["total_flows"] = summaries["total_flows"].(uint64) + bucket.Flows
		summaries["raw_bytes"] = summaries["raw_bytes"].(uint64) + bucket.RawBytes
		summaries["raw_packets"] = summaries["raw_packets"].(uint64) + bucket.RawPackets
	}

	metrics := map[string]any{
		"agents": len(inputs),
	}

	return flowsData{
		SchemaVersion: inputs[0].SchemaVersion,
		AgentID:       "merged",
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Exporters:     exporters,
		Buckets:       buckets,
		Summaries:     summaries,
		Metrics:       metrics,
	}
}

type flowBucketKey struct {
	Timestamp time.Time
	Direction string
	Key       flowKey
}

func fmtInt(value int) string {
	return strconv.Itoa(value)
}
