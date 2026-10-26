// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"

// Typed rows keep topology construction independent of compact-table column order.
type topologyActorRow struct {
	Type               string
	Layer              string
	ID                 string
	DisplayName        string
	Description        string
	AccountID          string
	SiteID             string
	DeviceID           string
	PopName            string
	RemoteIP           string
	RemoteASN          string
	ConnectivityStatus string
	OperationalStatus  string
	SiteType           string
	ConnectionType     string
	CountryCode        string
	CountryName        string
	Region             string
	HostCount          *int64
	Connected          *bool
	HARole             string
	SocketSerial       string
	SocketVersion      string
	InternalIP         *string
}

func (row topologyActorRow) addTo(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary) int {
	return table.Add(
		dict.Ref(row.Type),
		dict.Ref(row.Layer),
		row.ID,
		row.DisplayName,
		row.Description,
		row.AccountID,
		row.SiteID,
		row.DeviceID,
		row.PopName,
		row.RemoteIP,
		row.RemoteASN,
		row.ConnectivityStatus,
		row.OperationalStatus,
		row.SiteType,
		row.ConnectionType,
		row.CountryCode,
		row.CountryName,
		row.Region,
		optionalValue(row.HostCount),
		optionalValue(row.Connected),
		row.HARole,
		row.SocketSerial,
		row.SocketVersion,
		optionalValue(row.InternalIP),
	)
}

func catoTopologyActorColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("layer", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("id", "string", topologyv1.WithRole("identity")),
		topologyv1.NewColumn("display_name", "string"),
		topologyv1.NewColumn("description", "string"),
		topologyv1.NewColumn("account_id", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("site_id", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("device_id", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("pop_name", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("remote_ip", "ip", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("remote_asn", "string", topologyv1.WithRole("merge_identity")),
		topologyv1.NewColumn("connectivity_status", "string"),
		topologyv1.NewColumn("operational_status", "string"),
		topologyv1.NewColumn("site_type", "string"),
		topologyv1.NewColumn("connection_type", "string"),
		topologyv1.NewColumn("country_code", "string"),
		topologyv1.NewColumn("country_name", "string"),
		topologyv1.NewColumn("region", "string"),
		topologyv1.NewColumn("host_count", "uint", topologyv1.WithNullable(), topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("connected", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("ha_role", "string"),
		topologyv1.NewColumn("socket_serial", "string"),
		topologyv1.NewColumn("socket_version", "string"),
		topologyv1.NewColumn("internal_ip", "ip", topologyv1.WithNullable()),
	}
}

type topologyLinkRow struct {
	Type                    string
	SrcActor                int
	DstActor                int
	Protocol                string
	State                   string
	Direction               string
	EvidenceCount           int
	BytesUpstreamMax        *float64
	BytesDownstreamMax      *float64
	LostUpstreamPercent     *float64
	RTTMS                   *float64
	Routes                  *int64
	RoutesLimit             *int64
	RoutesLimitExceeded     *bool
	RIBOutRoutes            *int64
	IncomingConnectionState *string
	OutgoingConnectionState *string
	InterfaceID             *string
	InterfaceName           *string
	SiteActor               int
	Observation             string
	LocalIP                 *string
	LocalASN                *string
}

func (row topologyLinkRow) addTo(table *topologyv1.TableBuilder, dict *topologyv1.StringDictionary) int {
	return table.Add(
		dict.Ref(row.Type),
		row.SrcActor,
		row.DstActor,
		dict.Ref(row.Protocol),
		dict.Ref(row.State),
		dict.Ref(row.Direction),
		row.EvidenceCount,
		optionalValue(row.BytesUpstreamMax),
		optionalValue(row.BytesDownstreamMax),
		optionalValue(row.LostUpstreamPercent),
		optionalValue(row.RTTMS),
		optionalValue(row.Routes),
		optionalValue(row.RoutesLimit),
		optionalValue(row.RoutesLimitExceeded),
		optionalValue(row.RIBOutRoutes),
		optionalValue(row.IncomingConnectionState),
		optionalValue(row.OutgoingConnectionState),
		optionalValue(row.InterfaceID),
		optionalValue(row.InterfaceName),
		row.SiteActor,
		row.Observation,
		optionalValue(row.LocalIP),
		optionalValue(row.LocalASN),
	)
}

func catoTopologyLinkColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("src_actor", "actor_ref"),
		topologyv1.NewColumn("dst_actor", "actor_ref"),
		topologyv1.NewColumn("protocol", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("state", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("direction", "string_ref", topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("evidence_count", "uint", topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn(
			"bytes_upstream_max",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("max"),
		),
		topologyv1.NewColumn(
			"bytes_downstream_max",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("max"),
		),
		topologyv1.NewColumn(
			"lost_upstream_percent",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("avg"),
		),
		topologyv1.NewColumn(
			"rtt_ms",
			"float",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithUnit("ms"),
			topologyv1.WithAggregation("avg"),
		),
		topologyv1.NewColumn(
			"routes",
			"int",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("sum"),
		),
		topologyv1.NewColumn(
			"routes_limit",
			"int",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("max"),
		),
		topologyv1.NewColumn("routes_limit_exceeded", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn(
			"rib_out_routes",
			"int",
			topologyv1.WithNullable(),
			topologyv1.WithRole("metric"),
			topologyv1.WithAggregation("sum"),
		),
		topologyv1.NewColumn("incoming_connection_state", "string", topologyv1.WithNullable()),
		topologyv1.NewColumn("outgoing_connection_state", "string", topologyv1.WithNullable()),
		topologyv1.NewColumn("interface_id", "string", topologyv1.WithNullable(), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("interface_name", "string", topologyv1.WithNullable(), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("site_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("observation", "string"),
		topologyv1.NewColumn("local_ip", "ip", topologyv1.WithNullable()),
		topologyv1.NewColumn("local_asn", "string", topologyv1.WithNullable()),
	}
}

type topologyInterfaceRow struct {
	Actor                 int
	ID                    string
	Name                  string
	Type                  string
	Connected             *bool
	PopName               string
	TunnelRemoteIP        string
	TunnelUptime          *int64
	UpstreamBandwidth     *float64
	DownstreamBandwidth   *float64
	TunnelConnected       *bool
	LinkUp                *bool
	Link                  *int
	SiteActor             int
	PopActor              *int
	RTTMS                 *float64
	LostUpstreamPercent   *float64
	LostDownstreamPercent *float64
	JitterUpstreamMS      *float64
	JitterDownstreamMS    *float64
	LastMileLatencyMS     *float64
	LastMileLossPercent   *float64
	TunnelState           string
	PhysicalLinkState     string
}

func (row topologyInterfaceRow) addTo(table *topologyv1.TableBuilder) int {
	return table.Add(
		row.Actor,
		row.ID,
		row.Name,
		row.Type,
		optionalValue(row.Connected),
		row.PopName,
		row.TunnelRemoteIP,
		optionalValue(row.TunnelUptime),
		optionalValue(row.UpstreamBandwidth),
		optionalValue(row.DownstreamBandwidth),
		optionalValue(row.TunnelConnected),
		optionalValue(row.LinkUp),
		optionalValue(row.Link),
		row.SiteActor,
		optionalValue(row.PopActor),
		optionalValue(row.RTTMS),
		optionalValue(row.LostUpstreamPercent),
		optionalValue(row.LostDownstreamPercent),
		optionalValue(row.JitterUpstreamMS),
		optionalValue(row.JitterDownstreamMS),
		optionalValue(row.LastMileLatencyMS),
		optionalValue(row.LastMileLossPercent),
		row.TunnelState,
		row.PhysicalLinkState,
	)
}

func catoTopologyInterfaceColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref"),
		topologyv1.NewColumn("id", "string"),
		topologyv1.NewColumn("name", "string"),
		topologyv1.NewColumn("type", "string"),
		topologyv1.NewColumn("connected", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("pop_name", "string"),
		topologyv1.NewColumn("tunnel_remote_ip", "ip"),
		topologyv1.NewColumn("tunnel_uptime", "duration", topologyv1.WithNullable()),
		topologyv1.NewColumn("upstream_bandwidth", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("downstream_bandwidth", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("tunnel_connected", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("link_up", "bool", topologyv1.WithNullable()),
		topologyv1.NewColumn("link", "link_ref", topologyv1.WithNullable()),
		topologyv1.NewColumn("site_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("pop_actor", "actor_ref", topologyv1.WithRole("reference"), topologyv1.WithNullable()),
		topologyv1.NewColumn("rtt_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("lost_upstream_percent", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("lost_downstream_percent", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("jitter_upstream_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("jitter_downstream_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("last_mile_latency_ms", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("last_mile_loss_percent", "float", topologyv1.WithNullable()),
		topologyv1.NewColumn("tunnel_state", "string"),
		topologyv1.NewColumn("physical_link_state", "string"),
	}
}

func optionalValue[T any](value *T) any {
	if value == nil {
		return nil
	}
	return *value
}
