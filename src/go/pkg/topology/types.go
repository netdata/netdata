// SPDX-License-Identifier: GPL-3.0-or-later

package topology

import "time"

type Match struct {
	ChassisIDs         []string `json:"chassis_ids,omitempty"`
	MacAddresses       []string `json:"mac_addresses,omitempty"`
	IPAddresses        []string `json:"ip_addresses,omitempty"`
	Hostnames          []string `json:"hostnames,omitempty"`
	DNSNames           []string `json:"dns_names,omitempty"`
	SysObjectID        string   `json:"sys_object_id,omitempty"`
	SysName            string   `json:"sys_name,omitempty"`
	NetdataNodeID      string   `json:"netdata_node_id,omitempty"`
	NetdataMachineGUID string   `json:"netdata_machine_guid,omitempty"`
	CloudInstanceID    string   `json:"cloud_instance_id,omitempty"`
	CloudAccountID     string   `json:"cloud_account_id,omitempty"`
	ContainerIDs       []string `json:"container_ids,omitempty"`
	PodNames           []string `json:"pod_names,omitempty"`
	NamespaceIDs       []string `json:"namespace_ids,omitempty"`
}

type Actor struct {
	ActorType   string            `json:"actor_type"`
	Layer       string            `json:"layer"`
	Source      string            `json:"source"`
	Match       Match             `json:"match"`
	ParentMatch *Match            `json:"parent_match,omitempty"`
	Attributes  map[string]any    `json:"attributes,omitempty"`
	Derived     map[string]any    `json:"derived,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

type LinkEndpoint struct {
	Match      Match          `json:"match,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type Link struct {
	Layer        string          `json:"layer"`
	Protocol     string          `json:"protocol"`
	Direction    string          `json:"direction,omitempty"`
	State        string          `json:"state,omitempty"`
	Src          LinkEndpoint    `json:"src"`
	Dst          LinkEndpoint    `json:"dst"`
	DiscoveredAt *time.Time      `json:"discovered_at,omitempty"`
	LastSeen     *time.Time      `json:"last_seen,omitempty"`
	Metrics      map[string]any  `json:"metrics,omitempty"`
}

type FlowExporter struct {
	IP           string `json:"ip,omitempty"`
	Name         string `json:"name,omitempty"`
	SamplingRate int    `json:"sampling_rate,omitempty"`
	FlowVersion  string `json:"flow_version,omitempty"`
}

type Flow struct {
	Timestamp   time.Time      `json:"timestamp"`
	DurationSec int            `json:"duration_sec,omitempty"`
	Exporter    *FlowExporter  `json:"exporter,omitempty"`
	Src         LinkEndpoint   `json:"src,omitempty"`
	Dst         LinkEndpoint   `json:"dst,omitempty"`
	Key         map[string]any `json:"key,omitempty"`
	Metrics     map[string]any `json:"metrics,omitempty"`
}

type LiveTopN struct {
	Enabled bool   `json:"enabled,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	SortBy  string `json:"sort_by,omitempty"`
}

type IPPolicy struct {
	PublicAllowlist []string  `json:"public_allowlist,omitempty"`
	LiveTopN        *LiveTopN `json:"live_top_n,omitempty"`
}

type Data struct {
	SchemaVersion string         `json:"schema_version"`
	Source        string         `json:"source,omitempty"`
	Layer         string         `json:"layer,omitempty"`
	AgentID       string         `json:"agent_id"`
	CollectedAt   time.Time      `json:"collected_at"`
	View          string         `json:"view,omitempty"`
	IPPolicy      *IPPolicy      `json:"ip_policy,omitempty"`
	Actors        []Actor        `json:"actors,omitempty"`
	Links         []Link         `json:"links,omitempty"`
	Flows         []Flow         `json:"flows,omitempty"`
	Stats         map[string]any `json:"stats,omitempty"`
	Metrics       map[string]any `json:"metrics,omitempty"`
}
