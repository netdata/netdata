// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import "time"

const flowSchemaVersion = "1.0"

type flowData struct {
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

type flowRecord struct {
	Timestamp    time.Time
	DurationSec  int
	Key          flowKey
	Bytes        uint64
	Packets      uint64
	Flows        uint64
	RawBytes     uint64
	RawPackets   uint64
	SamplingRate int
	Direction    string
	ExporterIP   string
	FlowVersion  string
}
