// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"slices"
	"strings"
)

type BGPRowKind string

const (
	BGPRowKindDevice     BGPRowKind = "device"
	BGPRowKindPeer       BGPRowKind = "peer"
	BGPRowKindPeerFamily BGPRowKind = "peer_family"
)

var validBGPRowKinds = map[BGPRowKind]struct{}{
	BGPRowKindDevice:     {},
	BGPRowKindPeer:       {},
	BGPRowKindPeerFamily: {},
}

func IsValidBGPRowKind(kind BGPRowKind) bool {
	_, ok := validBGPRowKinds[kind]
	return ok
}

type BGPPeerState string

const (
	BGPPeerStateNone        BGPPeerState = "none"
	BGPPeerStateIdle        BGPPeerState = "idle"
	BGPPeerStateConnect     BGPPeerState = "connect"
	BGPPeerStateActive      BGPPeerState = "active"
	BGPPeerStateOpenSent    BGPPeerState = "opensent"
	BGPPeerStateOpenConfirm BGPPeerState = "openconfirm"
	BGPPeerStateEstablished BGPPeerState = "established"
)

var validBGPPeerStates = map[BGPPeerState]struct{}{
	BGPPeerStateNone:        {},
	BGPPeerStateIdle:        {},
	BGPPeerStateConnect:     {},
	BGPPeerStateActive:      {},
	BGPPeerStateOpenSent:    {},
	BGPPeerStateOpenConfirm: {},
	BGPPeerStateEstablished: {},
}

var requiredBGPPeerStates = []BGPPeerState{
	BGPPeerStateIdle,
	BGPPeerStateConnect,
	BGPPeerStateActive,
	BGPPeerStateOpenSent,
	BGPPeerStateOpenConfirm,
	BGPPeerStateEstablished,
}

func IsValidBGPPeerState(state BGPPeerState) bool {
	_, ok := validBGPPeerStates[state]
	return ok
}

type BGPAddressFamily string

const (
	BGPAddressFamilyIPv4  BGPAddressFamily = "ipv4"
	BGPAddressFamilyIPv6  BGPAddressFamily = "ipv6"
	BGPAddressFamilyL2VPN BGPAddressFamily = "l2vpn"
)

var validBGPAddressFamilies = map[BGPAddressFamily]struct{}{
	BGPAddressFamilyIPv4:  {},
	BGPAddressFamilyIPv6:  {},
	BGPAddressFamilyL2VPN: {},
}

func IsValidBGPAddressFamily(family BGPAddressFamily) bool {
	_, ok := validBGPAddressFamilies[family]
	return ok
}

type BGPSubsequentAddressFamily string

const (
	BGPSubsequentAddressFamilyUnicast        BGPSubsequentAddressFamily = "unicast"
	BGPSubsequentAddressFamilyMulticast      BGPSubsequentAddressFamily = "multicast"
	BGPSubsequentAddressFamilyLabeledUnicast BGPSubsequentAddressFamily = "labeled_unicast"
	BGPSubsequentAddressFamilyMVPN           BGPSubsequentAddressFamily = "mvpn"
	BGPSubsequentAddressFamilyVPLS           BGPSubsequentAddressFamily = "vpls"
	BGPSubsequentAddressFamilyEVPN           BGPSubsequentAddressFamily = "evpn"
	BGPSubsequentAddressFamilyVPN            BGPSubsequentAddressFamily = "vpn"
	BGPSubsequentAddressFamilyRTFilter       BGPSubsequentAddressFamily = "rtfilter"
	BGPSubsequentAddressFamilyFlow           BGPSubsequentAddressFamily = "flow"
)

var validBGPSubsequentAddressFamilies = map[BGPSubsequentAddressFamily]struct{}{
	BGPSubsequentAddressFamilyUnicast:        {},
	BGPSubsequentAddressFamilyMulticast:      {},
	BGPSubsequentAddressFamilyLabeledUnicast: {},
	BGPSubsequentAddressFamilyMVPN:           {},
	BGPSubsequentAddressFamilyVPLS:           {},
	BGPSubsequentAddressFamilyEVPN:           {},
	BGPSubsequentAddressFamilyVPN:            {},
	BGPSubsequentAddressFamilyRTFilter:       {},
	BGPSubsequentAddressFamilyFlow:           {},
}

func IsValidBGPSubsequentAddressFamily(family BGPSubsequentAddressFamily) bool {
	_, ok := validBGPSubsequentAddressFamilies[family]
	return ok
}

type BGPConfig struct {
	OriginProfileID string       `yaml:"-" json:"-"`
	ID              string       `yaml:"id,omitempty" json:"id,omitempty"`
	MIB             string       `yaml:"MIB,omitempty" json:"MIB,omitempty"`
	Kind            BGPRowKind   `yaml:"kind,omitempty" json:"kind,omitempty"`
	Table           SymbolConfig `yaml:"table,omitempty" json:"table"`

	Identity    BGPIdentityConfig        `yaml:"identity,omitempty" json:"identity"`
	Descriptors BGPDescriptorsConfig     `yaml:"descriptors,omitempty" json:"descriptors"`
	Admin       BGPAdminConfig           `yaml:"admin,omitempty" json:"admin"`
	State       BGPStateConfig           `yaml:"state,omitempty" json:"state"`
	Previous    BGPStateConfig           `yaml:"previous_state,omitempty" json:"previous_state"`
	Connection  BGPConnectionConfig      `yaml:"connection,omitempty" json:"connection"`
	Traffic     BGPTrafficConfig         `yaml:"traffic,omitempty" json:"traffic"`
	Transitions BGPTransitionsConfig     `yaml:"transitions,omitempty" json:"transitions"`
	Timers      BGPTimersConfig          `yaml:"timers,omitempty" json:"timers"`
	LastError   BGPLastErrorConfig       `yaml:"last_error,omitempty" json:"last_error"`
	LastNotify  BGPLastNotifyConfig      `yaml:"last_notifications,omitempty" json:"last_notifications"`
	Reasons     BGPReasonsConfig         `yaml:"reasons,omitempty" json:"reasons"`
	Restart     BGPGracefulRestartConfig `yaml:"graceful_restart,omitempty" json:"graceful_restart"`
	Routes      BGPRoutesConfig          `yaml:"routes,omitempty" json:"routes"`
	RouteLimits BGPRouteLimitsConfig     `yaml:"route_limits,omitempty" json:"route_limits"`
	Device      BGPDeviceCountsConfig    `yaml:"device_counts,omitempty" json:"device_counts"`

	StaticTags []StaticMetricTagConfig `yaml:"static_tags,omitempty" json:"-"`
	MetricTags MetricTagConfigList     `yaml:"metric_tags,omitempty" json:"metric_tags,omitempty"`
}

func (c BGPConfig) Clone() BGPConfig {
	return BGPConfig{
		OriginProfileID: c.OriginProfileID,
		ID:              c.ID,
		MIB:             c.MIB,
		Kind:            c.Kind,
		Table:           c.Table.Clone(),
		Identity:        c.Identity.Clone(),
		Descriptors:     c.Descriptors.Clone(),
		Admin:           c.Admin.Clone(),
		State:           c.State.Clone(),
		Previous:        c.Previous.Clone(),
		Connection:      c.Connection.Clone(),
		Traffic:         c.Traffic.Clone(),
		Transitions:     c.Transitions.Clone(),
		Timers:          c.Timers.Clone(),
		LastError:       c.LastError.Clone(),
		LastNotify:      c.LastNotify.Clone(),
		Reasons:         c.Reasons.Clone(),
		Restart:         c.Restart.Clone(),
		Routes:          c.Routes.Clone(),
		RouteLimits:     c.RouteLimits.Clone(),
		Device:          c.Device.Clone(),
		StaticTags:      slices.Clone(c.StaticTags),
		MetricTags:      cloneSlice(c.MetricTags),
	}
}

type BGPIdentityConfig struct {
	RoutingInstance         BGPValueConfig                        `yaml:"routing_instance,omitempty" json:"routing_instance"`
	Neighbor                BGPValueConfig                        `yaml:"neighbor,omitempty" json:"neighbor"`
	RemoteAS                BGPValueConfig                        `yaml:"remote_as,omitempty" json:"remote_as"`
	AddressFamily           BGPAddressFamilyValueConfig           `yaml:"address_family,omitempty" json:"address_family"`
	SubsequentAddressFamily BGPSubsequentAddressFamilyValueConfig `yaml:"subsequent_address_family,omitempty" json:"subsequent_address_family"`
}

func (c BGPIdentityConfig) Clone() BGPIdentityConfig {
	return BGPIdentityConfig{
		RoutingInstance:         c.RoutingInstance.Clone(),
		Neighbor:                c.Neighbor.Clone(),
		RemoteAS:                c.RemoteAS.Clone(),
		AddressFamily:           c.AddressFamily.Clone(),
		SubsequentAddressFamily: c.SubsequentAddressFamily.Clone(),
	}
}

type BGPDescriptorsConfig struct {
	LocalAddress    BGPValueConfig `yaml:"local_address,omitempty" json:"local_address"`
	LocalAS         BGPValueConfig `yaml:"local_as,omitempty" json:"local_as"`
	LocalIdentifier BGPValueConfig `yaml:"local_identifier,omitempty" json:"local_identifier"`
	PeerIdentifier  BGPValueConfig `yaml:"peer_identifier,omitempty" json:"peer_identifier"`
	PeerType        BGPValueConfig `yaml:"peer_type,omitempty" json:"peer_type"`
	BGPVersion      BGPValueConfig `yaml:"bgp_version,omitempty" json:"bgp_version"`
	Description     BGPValueConfig `yaml:"description,omitempty" json:"description"`
}

func (c BGPDescriptorsConfig) Clone() BGPDescriptorsConfig {
	return BGPDescriptorsConfig{
		LocalAddress:    c.LocalAddress.Clone(),
		LocalAS:         c.LocalAS.Clone(),
		LocalIdentifier: c.LocalIdentifier.Clone(),
		PeerIdentifier:  c.PeerIdentifier.Clone(),
		PeerType:        c.PeerType.Clone(),
		BGPVersion:      c.BGPVersion.Clone(),
		Description:     c.Description.Clone(),
	}
}

type BGPStateConfig struct {
	BGPValueConfig `yaml:",inline" json:",inline"`
	Partial        bool           `yaml:"partial,omitempty" json:"partial,omitempty"`
	PartialStates  []BGPPeerState `yaml:"partial_states,omitempty" json:"partial_states,omitempty"`
}

func (c BGPStateConfig) Clone() BGPStateConfig {
	return BGPStateConfig{
		BGPValueConfig: c.BGPValueConfig.Clone(),
		Partial:        c.Partial,
		PartialStates:  slices.Clone(c.PartialStates),
	}
}

type BGPAdminConfig struct {
	Enabled BGPValueConfig `yaml:"enabled,omitempty" json:"enabled"`
}

func (c BGPAdminConfig) Clone() BGPAdminConfig {
	return BGPAdminConfig{Enabled: c.Enabled.Clone()}
}

type BGPConnectionConfig struct {
	EstablishedUptime     BGPValueConfig `yaml:"established_uptime,omitempty" json:"established_uptime"`
	LastReceivedUpdateAge BGPValueConfig `yaml:"last_received_update_age,omitempty" json:"last_received_update_age"`
}

func (c BGPConnectionConfig) Clone() BGPConnectionConfig {
	return BGPConnectionConfig{
		EstablishedUptime:     c.EstablishedUptime.Clone(),
		LastReceivedUpdateAge: c.LastReceivedUpdateAge.Clone(),
	}
}

type BGPDirectionalConfig struct {
	Received BGPValueConfig `yaml:"received,omitempty" json:"received"`
	Sent     BGPValueConfig `yaml:"sent,omitempty" json:"sent"`
}

func (c BGPDirectionalConfig) Clone() BGPDirectionalConfig {
	return BGPDirectionalConfig{
		Received: c.Received.Clone(),
		Sent:     c.Sent.Clone(),
	}
}

type BGPTrafficConfig struct {
	Messages       BGPDirectionalConfig `yaml:"messages,omitempty" json:"messages"`
	Updates        BGPDirectionalConfig `yaml:"updates,omitempty" json:"updates"`
	Notifications  BGPDirectionalConfig `yaml:"notifications,omitempty" json:"notifications"`
	RouteRefreshes BGPDirectionalConfig `yaml:"route_refreshes,omitempty" json:"route_refreshes"`
	Opens          BGPDirectionalConfig `yaml:"opens,omitempty" json:"opens"`
	Keepalives     BGPDirectionalConfig `yaml:"keepalives,omitempty" json:"keepalives"`
}

func (c BGPTrafficConfig) Clone() BGPTrafficConfig {
	return BGPTrafficConfig{
		Messages:       c.Messages.Clone(),
		Updates:        c.Updates.Clone(),
		Notifications:  c.Notifications.Clone(),
		RouteRefreshes: c.RouteRefreshes.Clone(),
		Opens:          c.Opens.Clone(),
		Keepalives:     c.Keepalives.Clone(),
	}
}

type BGPTransitionsConfig struct {
	Established BGPValueConfig `yaml:"established,omitempty" json:"established"`
	Down        BGPValueConfig `yaml:"down,omitempty" json:"down"`
	Up          BGPValueConfig `yaml:"up,omitempty" json:"up"`
	Flaps       BGPValueConfig `yaml:"flaps,omitempty" json:"flaps"`
}

func (c BGPTransitionsConfig) Clone() BGPTransitionsConfig {
	return BGPTransitionsConfig{
		Established: c.Established.Clone(),
		Down:        c.Down.Clone(),
		Up:          c.Up.Clone(),
		Flaps:       c.Flaps.Clone(),
	}
}

type BGPTimersConfig struct {
	Negotiated BGPTimerPairConfig `yaml:"negotiated,omitempty" json:"negotiated"`
	Configured BGPTimerPairConfig `yaml:"configured,omitempty" json:"configured"`
}

func (c BGPTimersConfig) Clone() BGPTimersConfig {
	return BGPTimersConfig{
		Negotiated: c.Negotiated.Clone(),
		Configured: c.Configured.Clone(),
	}
}

type BGPTimerPairConfig struct {
	ConnectRetry                  BGPValueConfig `yaml:"connect_retry,omitempty" json:"connect_retry"`
	HoldTime                      BGPValueConfig `yaml:"hold_time,omitempty" json:"hold_time"`
	KeepaliveTime                 BGPValueConfig `yaml:"keepalive_time,omitempty" json:"keepalive_time"`
	MinASOriginationInterval      BGPValueConfig `yaml:"min_as_origination_interval,omitempty" json:"min_as_origination_interval"`
	MinRouteAdvertisementInterval BGPValueConfig `yaml:"min_route_advertisement_interval,omitempty" json:"min_route_advertisement_interval"`
}

func (c BGPTimerPairConfig) Clone() BGPTimerPairConfig {
	return BGPTimerPairConfig{
		ConnectRetry:                  c.ConnectRetry.Clone(),
		HoldTime:                      c.HoldTime.Clone(),
		KeepaliveTime:                 c.KeepaliveTime.Clone(),
		MinASOriginationInterval:      c.MinASOriginationInterval.Clone(),
		MinRouteAdvertisementInterval: c.MinRouteAdvertisementInterval.Clone(),
	}
}

type BGPLastErrorConfig struct {
	Code    BGPValueConfig `yaml:"code,omitempty" json:"code"`
	Subcode BGPValueConfig `yaml:"subcode,omitempty" json:"subcode"`
}

func (c BGPLastErrorConfig) Clone() BGPLastErrorConfig {
	return BGPLastErrorConfig{
		Code:    c.Code.Clone(),
		Subcode: c.Subcode.Clone(),
	}
}

type BGPLastNotificationConfig struct {
	Code    BGPValueConfig `yaml:"code,omitempty" json:"code"`
	Subcode BGPValueConfig `yaml:"subcode,omitempty" json:"subcode"`
	Reason  BGPValueConfig `yaml:"reason,omitempty" json:"reason"`
}

func (c BGPLastNotificationConfig) Clone() BGPLastNotificationConfig {
	return BGPLastNotificationConfig{
		Code:    c.Code.Clone(),
		Subcode: c.Subcode.Clone(),
		Reason:  c.Reason.Clone(),
	}
}

type BGPLastNotifyConfig struct {
	Received BGPLastNotificationConfig `yaml:"received,omitempty" json:"received"`
	Sent     BGPLastNotificationConfig `yaml:"sent,omitempty" json:"sent"`
}

func (c BGPLastNotifyConfig) Clone() BGPLastNotifyConfig {
	return BGPLastNotifyConfig{
		Received: c.Received.Clone(),
		Sent:     c.Sent.Clone(),
	}
}

type BGPReasonsConfig struct {
	LastDown       BGPValueConfig `yaml:"last_down,omitempty" json:"last_down"`
	Unavailability BGPValueConfig `yaml:"unavailability,omitempty" json:"unavailability"`
}

func (c BGPReasonsConfig) Clone() BGPReasonsConfig {
	return BGPReasonsConfig{
		LastDown:       c.LastDown.Clone(),
		Unavailability: c.Unavailability.Clone(),
	}
}

type BGPGracefulRestartConfig struct {
	State BGPValueConfig `yaml:"state,omitempty" json:"state"`
}

func (c BGPGracefulRestartConfig) Clone() BGPGracefulRestartConfig {
	return BGPGracefulRestartConfig{State: c.State.Clone()}
}

type BGPRoutesConfig struct {
	Current BGPRouteCountersConfig `yaml:"current,omitempty" json:"current"`
	Total   BGPRouteCountersConfig `yaml:"total,omitempty" json:"total"`
}

func (c BGPRoutesConfig) Clone() BGPRoutesConfig {
	return BGPRoutesConfig{
		Current: c.Current.Clone(),
		Total:   c.Total.Clone(),
	}
}

type BGPRouteCountersConfig struct {
	Received   BGPValueConfig `yaml:"received,omitempty" json:"received"`
	Accepted   BGPValueConfig `yaml:"accepted,omitempty" json:"accepted"`
	Rejected   BGPValueConfig `yaml:"rejected,omitempty" json:"rejected"`
	Active     BGPValueConfig `yaml:"active,omitempty" json:"active"`
	Advertised BGPValueConfig `yaml:"advertised,omitempty" json:"advertised"`
	Suppressed BGPValueConfig `yaml:"suppressed,omitempty" json:"suppressed"`
	Withdrawn  BGPValueConfig `yaml:"withdrawn,omitempty" json:"withdrawn"`
}

func (c BGPRouteCountersConfig) Clone() BGPRouteCountersConfig {
	return BGPRouteCountersConfig{
		Received:   c.Received.Clone(),
		Accepted:   c.Accepted.Clone(),
		Rejected:   c.Rejected.Clone(),
		Active:     c.Active.Clone(),
		Advertised: c.Advertised.Clone(),
		Suppressed: c.Suppressed.Clone(),
		Withdrawn:  c.Withdrawn.Clone(),
	}
}

type BGPRouteLimitsConfig struct {
	Limit          BGPValueConfig `yaml:"limit,omitempty" json:"limit"`
	Threshold      BGPValueConfig `yaml:"threshold,omitempty" json:"threshold"`
	ClearThreshold BGPValueConfig `yaml:"clear_threshold,omitempty" json:"clear_threshold"`
}

func (c BGPRouteLimitsConfig) Clone() BGPRouteLimitsConfig {
	return BGPRouteLimitsConfig{
		Limit:          c.Limit.Clone(),
		Threshold:      c.Threshold.Clone(),
		ClearThreshold: c.ClearThreshold.Clone(),
	}
}

type BGPDeviceCountsConfig struct {
	Peers         BGPValueConfig      `yaml:"peers,omitempty" json:"peers"`
	InternalPeers BGPValueConfig      `yaml:"ibgp_peers,omitempty" json:"ibgp_peers"`
	ExternalPeers BGPValueConfig      `yaml:"ebgp_peers,omitempty" json:"ebgp_peers"`
	States        BGPPeerStatesConfig `yaml:"states,omitempty" json:"states"`
}

func (c BGPDeviceCountsConfig) Clone() BGPDeviceCountsConfig {
	return BGPDeviceCountsConfig{
		Peers:         c.Peers.Clone(),
		InternalPeers: c.InternalPeers.Clone(),
		ExternalPeers: c.ExternalPeers.Clone(),
		States:        c.States.Clone(),
	}
}

type BGPPeerStatesConfig struct {
	Idle        BGPValueConfig `yaml:"idle,omitempty" json:"idle"`
	Connect     BGPValueConfig `yaml:"connect,omitempty" json:"connect"`
	Active      BGPValueConfig `yaml:"active,omitempty" json:"active"`
	OpenSent    BGPValueConfig `yaml:"opensent,omitempty" json:"opensent"`
	OpenConfirm BGPValueConfig `yaml:"openconfirm,omitempty" json:"openconfirm"`
	Established BGPValueConfig `yaml:"established,omitempty" json:"established"`
}

func (c BGPPeerStatesConfig) Clone() BGPPeerStatesConfig {
	return BGPPeerStatesConfig{
		Idle:        c.Idle.Clone(),
		Connect:     c.Connect.Clone(),
		Active:      c.Active.Clone(),
		OpenSent:    c.OpenSent.Clone(),
		OpenConfirm: c.OpenConfirm.Clone(),
		Established: c.Established.Clone(),
	}
}

type BGPAddressFamilyValueConfig struct {
	BGPValueConfig `yaml:",inline" json:",inline"`
	AllowPrivate   bool `yaml:"allow_private,omitempty" json:"allow_private,omitempty"`
}

func (c BGPAddressFamilyValueConfig) Clone() BGPAddressFamilyValueConfig {
	return BGPAddressFamilyValueConfig{
		BGPValueConfig: c.BGPValueConfig.Clone(),
		AllowPrivate:   c.AllowPrivate,
	}
}

type BGPSubsequentAddressFamilyValueConfig struct {
	BGPValueConfig `yaml:",inline" json:",inline"`
	AllowPrivate   bool `yaml:"allow_private,omitempty" json:"allow_private,omitempty"`
}

func (c BGPSubsequentAddressFamilyValueConfig) Clone() BGPSubsequentAddressFamilyValueConfig {
	return BGPSubsequentAddressFamilyValueConfig{
		BGPValueConfig: c.BGPValueConfig.Clone(),
		AllowPrivate:   c.AllowPrivate,
	}
}

type BGPValueConfig struct {
	Value string `yaml:"value,omitempty" json:"value,omitempty"`
	From  string `yaml:"from,omitempty" json:"from,omitempty"`
	Table string `yaml:"table,omitempty" json:"table,omitempty"`

	Index          uint                   `yaml:"index,omitempty" json:"index,omitempty"`
	IndexFromEnd   uint                   `yaml:"index_from_end,omitempty" json:"index_from_end,omitempty"`
	IndexTransform []MetricIndexTransform `yaml:"index_transform,omitempty" json:"index_transform,omitempty"`

	Symbol SymbolConfig `yaml:"symbol,omitempty" json:"symbol"`
	OID    string       `yaml:"OID,omitempty" json:"OID,omitempty" jsonschema:"-"`
	Name   string       `yaml:"name,omitempty" json:"name,omitempty" jsonschema:"-"`

	LookupSymbol SymbolConfigCompat `yaml:"lookup_symbol,omitempty" json:"lookup_symbol"`

	Format  string        `yaml:"format,omitempty" json:"format,omitempty"`
	Mapping MappingConfig `yaml:"mapping,omitempty" json:"mapping"`
}

func (c BGPValueConfig) IsSet() bool {
	return c.Value != "" ||
		c.From != "" ||
		c.Table != "" ||
		c.Index != 0 ||
		c.IndexFromEnd != 0 ||
		len(c.IndexTransform) > 0 ||
		c.Symbol.OID != "" ||
		c.Symbol.Name != "" ||
		c.OID != "" ||
		c.Name != "" ||
		c.LookupSymbol.OID != "" ||
		c.LookupSymbol.Name != "" ||
		c.Format != "" ||
		c.Mapping.HasItems() ||
		c.Mapping.Mode != ""
}

func (c BGPValueConfig) Clone() BGPValueConfig {
	return BGPValueConfig{
		Value:          c.Value,
		From:           c.From,
		Table:          c.Table,
		Index:          c.Index,
		IndexFromEnd:   c.IndexFromEnd,
		IndexTransform: slices.Clone(c.IndexTransform),
		Symbol:         c.Symbol.Clone(),
		OID:            c.OID,
		Name:           c.Name,
		LookupSymbol:   c.LookupSymbol.Clone(),
		Format:         c.Format,
		Mapping:        c.Mapping.Clone(),
	}
}

func ForEachBGPSignalValue(row BGPConfig, add func(path string, value BGPValueConfig)) {
	addValue := func(path string, value BGPValueConfig) {
		if value.IsSet() {
			add(path, value)
		}
	}
	addState := func(path string, value BGPStateConfig) {
		if value.BGPValueConfig.IsSet() {
			add(path, value.BGPValueConfig)
		}
	}
	addDirectional := func(prefix string, value BGPDirectionalConfig) {
		addValue(prefix+".received", value.Received)
		addValue(prefix+".sent", value.Sent)
	}
	addTimerPair := func(prefix string, value BGPTimerPairConfig) {
		addValue(prefix+".connect_retry", value.ConnectRetry)
		addValue(prefix+".hold_time", value.HoldTime)
		addValue(prefix+".keepalive_time", value.KeepaliveTime)
		addValue(prefix+".min_as_origination_interval", value.MinASOriginationInterval)
		addValue(prefix+".min_route_advertisement_interval", value.MinRouteAdvertisementInterval)
	}
	addNotification := func(prefix string, value BGPLastNotificationConfig) {
		addValue(prefix+".code", value.Code)
		addValue(prefix+".subcode", value.Subcode)
		addValue(prefix+".reason", value.Reason)
	}
	addRoutes := func(prefix string, value BGPRouteCountersConfig) {
		addValue(prefix+".received", value.Received)
		addValue(prefix+".accepted", value.Accepted)
		addValue(prefix+".rejected", value.Rejected)
		addValue(prefix+".active", value.Active)
		addValue(prefix+".advertised", value.Advertised)
		addValue(prefix+".suppressed", value.Suppressed)
		addValue(prefix+".withdrawn", value.Withdrawn)
	}

	addValue("admin.enabled", row.Admin.Enabled)
	addState("state", row.State)
	addState("previous_state", row.Previous)
	addValue("connection.established_uptime", row.Connection.EstablishedUptime)
	addValue("connection.last_received_update_age", row.Connection.LastReceivedUpdateAge)
	addDirectional("traffic.messages", row.Traffic.Messages)
	addDirectional("traffic.updates", row.Traffic.Updates)
	addDirectional("traffic.notifications", row.Traffic.Notifications)
	addDirectional("traffic.route_refreshes", row.Traffic.RouteRefreshes)
	addDirectional("traffic.opens", row.Traffic.Opens)
	addDirectional("traffic.keepalives", row.Traffic.Keepalives)
	addValue("transitions.established", row.Transitions.Established)
	addValue("transitions.down", row.Transitions.Down)
	addValue("transitions.up", row.Transitions.Up)
	addValue("transitions.flaps", row.Transitions.Flaps)
	addTimerPair("timers.negotiated", row.Timers.Negotiated)
	addTimerPair("timers.configured", row.Timers.Configured)
	addValue("last_error.code", row.LastError.Code)
	addValue("last_error.subcode", row.LastError.Subcode)
	addNotification("last_notifications.received", row.LastNotify.Received)
	addNotification("last_notifications.sent", row.LastNotify.Sent)
	addValue("reasons.last_down", row.Reasons.LastDown)
	addValue("reasons.unavailability", row.Reasons.Unavailability)
	addValue("graceful_restart.state", row.Restart.State)
	addRoutes("routes.current", row.Routes.Current)
	addRoutes("routes.total", row.Routes.Total)
	addValue("route_limits.limit", row.RouteLimits.Limit)
	addValue("route_limits.threshold", row.RouteLimits.Threshold)
	addValue("route_limits.clear_threshold", row.RouteLimits.ClearThreshold)
	addValue("device_counts.peers", row.Device.Peers)
	addValue("device_counts.ibgp_peers", row.Device.InternalPeers)
	addValue("device_counts.ebgp_peers", row.Device.ExternalPeers)
	addValue("device_counts.states.idle", row.Device.States.Idle)
	addValue("device_counts.states.connect", row.Device.States.Connect)
	addValue("device_counts.states.active", row.Device.States.Active)
	addValue("device_counts.states.opensent", row.Device.States.OpenSent)
	addValue("device_counts.states.openconfirm", row.Device.States.OpenConfirm)
	addValue("device_counts.states.established", row.Device.States.Established)
}

func BGPStructuralIdentity(row BGPConfig) string {
	origin := row.OriginProfileID
	if origin == "" {
		origin = "<profile>"
	}
	return strings.Join([]string{origin, BGPMergeIdentity(row)}, "|")
}

func BGPMergeIdentity(row BGPConfig) string {
	if row.Table.OID != "" {
		parts := []string{"table", string(row.Kind), TrimBGPOID(row.Table.OID)}
		if row.ID != "" {
			parts = append(parts, row.ID)
		}
		return strings.Join(parts, "|")
	}
	if row.ID != "" {
		return strings.Join([]string{"scalar-group", string(row.Kind), row.ID}, "|")
	}
	if oid := firstBGPSignalSourceOID(row); oid != "" {
		return strings.Join([]string{"scalar", string(row.Kind), TrimBGPOID(oid)}, "|")
	}
	return strings.Join([]string{"scalar", string(row.Kind), "<missing-source>"}, "|")
}

func firstBGPSignalSourceOID(row BGPConfig) string {
	var first string
	ForEachBGPSignalValue(row, func(_ string, value BGPValueConfig) {
		if first != "" {
			return
		}
		if oid := BGPValueSourceOID(value); oid != "" {
			first = oid
			return
		}
	})
	return first
}

func BGPValueSourceOID(value BGPValueConfig) string {
	switch {
	case value.From != "":
		return value.From
	case value.Symbol.OID != "":
		return value.Symbol.OID
	case value.OID != "":
		return value.OID
	default:
		return ""
	}
}

func TrimBGPOID(oid string) string {
	return strings.TrimPrefix(strings.TrimSpace(oid), ".")
}
