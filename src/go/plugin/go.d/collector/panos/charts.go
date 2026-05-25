// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioPeerState = collectorapi.Priority + iota
	prioPeerUptime
	prioPeerMessages
	prioPeerUpdates
	prioPeerFlaps
	prioPeerEstablishedTransitions
	prioPeerPrefixesReceived
	prioPeerPrefixesAdvertised
	prioVRPeersByState
	prioVRPeersTotal
	prioSystemUptime
	prioSystemCertificateStatus
	prioSystemOperationalMode
	prioHAEnabled
	prioHALocalState
	prioHAPeerState
	prioHAPeerConnectionStatus
	prioHAStateSync
	prioHALinksStatus
	prioHAPriority
	prioEnvironmentTemperature
	prioEnvironmentFanSpeed
	prioEnvironmentVoltage
	prioEnvironmentSensorAlarm
	prioEnvironmentPowerSupplyStatus
	prioLicenseCount
	prioLicenseStatus
	prioLicenseExpiration
	prioIPSecTunnels
	prioIPSecTunnelSALifetime
	prioBGPPeersCollection
	prioBGPPrefixFamiliesCollection
	prioBGPVirtualRoutersCollection
	prioEnvironmentSensorsCollection
	prioLicensesCollection
	prioIPSecTunnelsCollection
)

var (
	peerChartTemplates = collectorapi.Charts{
		{
			ID:       "bgp_peer_%s_state",
			Title:    "BGP Peer State",
			Units:    "state",
			Fam:      "bgp peers",
			Ctx:      "panos.bgp.peer.state",
			Type:     collectorapi.Stacked,
			Priority: prioPeerState,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_state_idle", Name: "idle"},
				{ID: "bgp_peer_%s_state_connect", Name: "connect"},
				{ID: "bgp_peer_%s_state_active", Name: "active"},
				{ID: "bgp_peer_%s_state_opensent", Name: "opensent"},
				{ID: "bgp_peer_%s_state_openconfirm", Name: "openconfirm"},
				{ID: "bgp_peer_%s_state_established", Name: "established"},
			},
		},
		{
			ID:       "bgp_peer_%s_uptime",
			Title:    "BGP Peer Uptime",
			Units:    "seconds",
			Fam:      "bgp peers",
			Ctx:      "panos.bgp.peer.uptime",
			Type:     collectorapi.Line,
			Priority: prioPeerUptime,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_uptime", Name: "uptime"},
			},
		},
		{
			ID:       "bgp_peer_%s_messages",
			Title:    "BGP Peer Messages",
			Units:    "messages/s",
			Fam:      "bgp peers",
			Ctx:      "panos.bgp.peer.messages",
			Type:     collectorapi.Line,
			Priority: prioPeerMessages,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_messages_in", Name: "in", Algo: collectorapi.Incremental},
				{ID: "bgp_peer_%s_messages_out", Name: "out", Algo: collectorapi.Incremental},
			},
		},
		{
			ID:       "bgp_peer_%s_updates",
			Title:    "BGP Peer Updates",
			Units:    "messages/s",
			Fam:      "bgp peers",
			Ctx:      "panos.bgp.peer.updates",
			Type:     collectorapi.Line,
			Priority: prioPeerUpdates,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_updates_in", Name: "in", Algo: collectorapi.Incremental},
				{ID: "bgp_peer_%s_updates_out", Name: "out", Algo: collectorapi.Incremental},
			},
		},
		{
			ID:       "bgp_peer_%s_flaps",
			Title:    "BGP Peer Flaps",
			Units:    "flaps/s",
			Fam:      "bgp peers",
			Ctx:      "panos.bgp.peer.flaps",
			Type:     collectorapi.Line,
			Priority: prioPeerFlaps,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_flaps", Name: "flaps", Algo: collectorapi.Incremental},
			},
		},
		{
			ID:       "bgp_peer_%s_established_transitions",
			Title:    "BGP Peer Established Transitions",
			Units:    "transitions/s",
			Fam:      "bgp peers",
			Ctx:      "panos.bgp.peer.established_transitions",
			Type:     collectorapi.Line,
			Priority: prioPeerEstablishedTransitions,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_established_transitions", Name: "established", Algo: collectorapi.Incremental},
			},
		},
	}

	prefixChartTemplates = collectorapi.Charts{
		{
			ID:       "bgp_peer_%s_prefixes_received",
			Title:    "BGP Peer Received Prefixes",
			Units:    "prefixes",
			Fam:      "bgp prefixes",
			Ctx:      "panos.bgp.peer.prefixes_received",
			Type:     collectorapi.Line,
			Priority: prioPeerPrefixesReceived,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_prefixes_received_total", Name: "total"},
				{ID: "bgp_peer_%s_prefixes_received_accepted", Name: "accepted"},
				{ID: "bgp_peer_%s_prefixes_received_rejected", Name: "rejected"},
			},
		},
		{
			ID:       "bgp_peer_%s_prefixes_advertised",
			Title:    "BGP Peer Advertised Prefixes",
			Units:    "prefixes",
			Fam:      "bgp prefixes",
			Ctx:      "panos.bgp.peer.prefixes_advertised",
			Type:     collectorapi.Line,
			Priority: prioPeerPrefixesAdvertised,
			Dims: collectorapi.Dims{
				{ID: "bgp_peer_%s_prefixes_advertised", Name: "advertised"},
			},
		},
	}

	vrChartTemplates = collectorapi.Charts{
		{
			ID:       "bgp_vr_%s_peers_by_state",
			Title:    "BGP Peers by State",
			Units:    "peers",
			Fam:      "bgp virtual routers",
			Ctx:      "panos.bgp.vr.peers_by_state",
			Type:     collectorapi.Stacked,
			Priority: prioVRPeersByState,
			Dims: collectorapi.Dims{
				{ID: "bgp_vr_%s_peers_state_idle", Name: "idle"},
				{ID: "bgp_vr_%s_peers_state_connect", Name: "connect"},
				{ID: "bgp_vr_%s_peers_state_active", Name: "active"},
				{ID: "bgp_vr_%s_peers_state_opensent", Name: "opensent"},
				{ID: "bgp_vr_%s_peers_state_openconfirm", Name: "openconfirm"},
				{ID: "bgp_vr_%s_peers_state_established", Name: "established"},
			},
		},
		{
			ID:       "bgp_vr_%s_peers_total",
			Title:    "BGP Peers Total",
			Units:    "peers",
			Fam:      "bgp virtual routers",
			Ctx:      "panos.bgp.vr.peers_total",
			Type:     collectorapi.Line,
			Priority: prioVRPeersTotal,
			Dims: collectorapi.Dims{
				{ID: "bgp_vr_%s_peers_configured", Name: "configured"},
				{ID: "bgp_vr_%s_peers_established", Name: "established"},
			},
		},
	}

	systemChartTemplates = collectorapi.Charts{
		{
			ID:       "system_uptime",
			Title:    "System Uptime",
			Units:    "seconds",
			Fam:      "system",
			Ctx:      "panos.system.uptime",
			Type:     collectorapi.Line,
			Priority: prioSystemUptime,
			Dims: collectorapi.Dims{
				{ID: "system_uptime", Name: "uptime"},
			},
		},
		{
			ID:       "system_device_certificate_status",
			Title:    "Device Certificate Status",
			Units:    "status",
			Fam:      "system",
			Ctx:      "panos.system.device_certificate_status",
			Type:     collectorapi.Stacked,
			Priority: prioSystemCertificateStatus,
			Dims: collectorapi.Dims{
				{ID: "system_device_certificate_status_valid", Name: "valid"},
				{ID: "system_device_certificate_status_invalid", Name: "invalid"},
			},
		},
		{
			ID:       "system_operational_mode",
			Title:    "Operational Mode",
			Units:    "mode",
			Fam:      "system",
			Ctx:      "panos.system.operational_mode",
			Type:     collectorapi.Stacked,
			Priority: prioSystemOperationalMode,
			Dims: collectorapi.Dims{
				{ID: "system_operational_mode_normal", Name: "normal"},
				{ID: "system_operational_mode_other", Name: "other"},
			},
		},
	}

	haChartTemplates = collectorapi.Charts{
		{
			ID:       "ha_enabled",
			Title:    "HA Enabled",
			Units:    "status",
			Fam:      "ha",
			Ctx:      "panos.ha.enabled",
			Type:     collectorapi.Line,
			Priority: prioHAEnabled,
			Dims: collectorapi.Dims{
				{ID: "ha_enabled", Name: "enabled"},
			},
		},
		{
			ID:       "ha_local_state",
			Title:    "Local HA State",
			Units:    "state",
			Fam:      "ha",
			Ctx:      "panos.ha.local.state",
			Type:     collectorapi.Stacked,
			Priority: prioHALocalState,
			Dims: collectorapi.Dims{
				{ID: "ha_local_state_active", Name: "active"},
				{ID: "ha_local_state_passive", Name: "passive"},
				{ID: "ha_local_state_non_functional", Name: "non_functional"},
				{ID: "ha_local_state_suspended", Name: "suspended"},
				{ID: "ha_local_state_unknown", Name: "unknown"},
			},
		},
		{
			ID:       "ha_peer_state",
			Title:    "Peer HA State",
			Units:    "state",
			Fam:      "ha",
			Ctx:      "panos.ha.peer.state",
			Type:     collectorapi.Stacked,
			Priority: prioHAPeerState,
			Dims: collectorapi.Dims{
				{ID: "ha_peer_state_active", Name: "active"},
				{ID: "ha_peer_state_passive", Name: "passive"},
				{ID: "ha_peer_state_non_functional", Name: "non_functional"},
				{ID: "ha_peer_state_suspended", Name: "suspended"},
				{ID: "ha_peer_state_unknown", Name: "unknown"},
			},
		},
		{
			ID:       "ha_peer_connection_status",
			Title:    "HA Peer Connection Status",
			Units:    "status",
			Fam:      "ha",
			Ctx:      "panos.ha.peer.connection_status",
			Type:     collectorapi.Line,
			Priority: prioHAPeerConnectionStatus,
			Dims: collectorapi.Dims{
				{ID: "ha_peer_connection_status_up", Name: "up"},
			},
		},
		{
			ID:       "ha_state_sync",
			Title:    "HA State Synchronization",
			Units:    "status",
			Fam:      "ha",
			Ctx:      "panos.ha.state_sync",
			Type:     collectorapi.Line,
			Priority: prioHAStateSync,
			Dims: collectorapi.Dims{
				{ID: "ha_state_sync_synchronized", Name: "synchronized"},
			},
		},
		{
			ID:       "ha_links_status",
			Title:    "HA Links Status",
			Units:    "status",
			Fam:      "ha",
			Ctx:      "panos.ha.links_status",
			Type:     collectorapi.Line,
			Priority: prioHALinksStatus,
			Dims: collectorapi.Dims{
				{ID: "ha_links_status_ha1", Name: "ha1"},
				{ID: "ha_links_status_ha1_backup", Name: "ha1_backup"},
				{ID: "ha_links_status_ha2", Name: "ha2"},
				{ID: "ha_links_status_ha2_backup", Name: "ha2_backup"},
			},
		},
		{
			ID:       "ha_priority",
			Title:    "HA Priority",
			Units:    "priority",
			Fam:      "ha",
			Ctx:      "panos.ha.priority",
			Type:     collectorapi.Line,
			Priority: prioHAPriority,
			Dims: collectorapi.Dims{
				{ID: "ha_priority_local", Name: "local"},
				{ID: "ha_priority_peer", Name: "peer"},
			},
		},
	}

	environmentTemperatureChartTmpl = collectorapi.Chart{
		ID:       "env_temperature_%s",
		Title:    "Environment Temperature",
		Units:    "Celsius",
		Fam:      "environment temperature",
		Ctx:      "panos.environment.temperature",
		Type:     collectorapi.Line,
		Priority: prioEnvironmentTemperature,
		Dims: collectorapi.Dims{
			{ID: "env_temperature_%s", Name: "temperature", Div: 1000},
		},
	}
	environmentFanChartTmpl = collectorapi.Chart{
		ID:       "env_fan_%s_speed",
		Title:    "Environment Fan Speed",
		Units:    "RPM",
		Fam:      "environment fans",
		Ctx:      "panos.environment.fan_speed",
		Type:     collectorapi.Line,
		Priority: prioEnvironmentFanSpeed,
		Dims: collectorapi.Dims{
			{ID: "env_fan_%s_speed", Name: "speed"},
		},
	}
	environmentVoltageChartTmpl = collectorapi.Chart{
		ID:       "env_voltage_%s",
		Title:    "Environment Voltage",
		Units:    "Volts",
		Fam:      "environment voltage",
		Ctx:      "panos.environment.voltage",
		Type:     collectorapi.Line,
		Priority: prioEnvironmentVoltage,
		Dims: collectorapi.Dims{
			{ID: "env_voltage_%s", Name: "voltage", Div: 1000},
		},
	}
	environmentSensorAlarmChartTmpl = collectorapi.Chart{
		ID:       "env_sensor_%s_alarm",
		Title:    "Environment Sensor Alarm",
		Units:    "status",
		Fam:      "environment sensors",
		Ctx:      "panos.environment.sensor_alarm",
		Type:     collectorapi.Line,
		Priority: prioEnvironmentSensorAlarm,
		Dims: collectorapi.Dims{
			{ID: "env_sensor_%s_alarm", Name: "alarm"},
		},
	}
	environmentPowerSupplyChartTmpl = collectorapi.Chart{
		ID:       "env_psu_%s_status",
		Title:    "Power Supply Status",
		Units:    "status",
		Fam:      "environment power supplies",
		Ctx:      "panos.environment.power_supply_status",
		Type:     collectorapi.Line,
		Priority: prioEnvironmentPowerSupplyStatus,
		Dims: collectorapi.Dims{
			{ID: "env_psu_%s_inserted", Name: "inserted"},
			{ID: "env_psu_%s_alarm", Name: "alarm"},
		},
	}

	licenseCountChartTmpl = collectorapi.Chart{
		ID:       "license_count",
		Title:    "Licenses",
		Units:    "licenses",
		Fam:      "licenses",
		Ctx:      "panos.license.count",
		Type:     collectorapi.Line,
		Priority: prioLicenseCount,
		Dims: collectorapi.Dims{
			{ID: "license_count_total", Name: "total"},
			{ID: "license_count_expired", Name: "expired"},
		},
	}
	licenseStatusChartTmpl = collectorapi.Chart{
		ID:       "license_%s_status",
		Title:    "License Status",
		Units:    "status",
		Fam:      "licenses",
		Ctx:      "panos.license.status",
		Type:     collectorapi.Stacked,
		Priority: prioLicenseStatus,
		Dims: collectorapi.Dims{
			{ID: "license_%s_status_valid", Name: "valid"},
			{ID: "license_%s_status_expired", Name: "expired"},
		},
	}
	licenseExpirationChartTmpl = collectorapi.Chart{
		ID:       "license_%s_expiration",
		Title:    "License Time Until Expiration",
		Units:    "days",
		Fam:      "licenses",
		Ctx:      "panos.license.time_until_expiration",
		Type:     collectorapi.Line,
		Priority: prioLicenseExpiration,
		Dims: collectorapi.Dims{
			{ID: "license_%s_days_until_expiration", Name: "time_until_expiration"},
		},
	}

	ipsecTunnelsChartTmpl = collectorapi.Chart{
		ID:       "ipsec_tunnels",
		Title:    "IPsec Tunnels",
		Units:    "tunnels",
		Fam:      "ipsec",
		Ctx:      "panos.ipsec.tunnels",
		Type:     collectorapi.Line,
		Priority: prioIPSecTunnels,
		Dims: collectorapi.Dims{
			{ID: "ipsec_tunnels_active", Name: "active"},
		},
	}
	ipsecTunnelSALifetimeChartTmpl = collectorapi.Chart{
		ID:       "ipsec_tunnel_%s_sa_lifetime",
		Title:    "IPsec Tunnel SA Lifetime",
		Units:    "seconds",
		Fam:      "ipsec tunnels",
		Ctx:      "panos.ipsec.tunnel.sa_lifetime",
		Type:     collectorapi.Line,
		Priority: prioIPSecTunnelSALifetime,
		Dims: collectorapi.Dims{
			{ID: "ipsec_tunnel_%s_sa_lifetime", Name: "time_until_expiration"},
		},
	}

	bgpPeersCollectionChartTmpl = collectorapi.Chart{
		ID:       "bgp_peers_collection",
		Title:    "BGP Peer Collection",
		Units:    "peers",
		Fam:      "bgp peers",
		Ctx:      "panos.bgp.peers.collection",
		Type:     collectorapi.Line,
		Priority: prioBGPPeersCollection,
		Dims: collectorapi.Dims{
			{ID: "bgp_peers_collection_discovered", Name: "discovered"},
			{ID: "bgp_peers_collection_monitored", Name: "monitored"},
			{ID: "bgp_peers_collection_omitted_by_selector", Name: "omitted_by_selector"},
			{ID: "bgp_peers_collection_omitted_by_limit", Name: "omitted_by_limit"},
		},
	}
	bgpPrefixFamiliesCollectionChartTmpl = collectorapi.Chart{
		ID:       "bgp_prefix_families_collection",
		Title:    "BGP Prefix Family Collection",
		Units:    "families",
		Fam:      "bgp prefixes",
		Ctx:      "panos.bgp.prefix_families.collection",
		Type:     collectorapi.Line,
		Priority: prioBGPPrefixFamiliesCollection,
		Dims: collectorapi.Dims{
			{ID: "bgp_prefix_families_collection_discovered", Name: "discovered"},
			{ID: "bgp_prefix_families_collection_monitored", Name: "monitored"},
			{ID: "bgp_prefix_families_collection_omitted_by_selector", Name: "omitted_by_selector"},
			{ID: "bgp_prefix_families_collection_omitted_by_limit", Name: "omitted_by_limit"},
		},
	}
	bgpVirtualRoutersCollectionChartTmpl = collectorapi.Chart{
		ID:       "bgp_virtual_routers_collection",
		Title:    "BGP Virtual Router Collection",
		Units:    "virtual routers",
		Fam:      "bgp virtual routers",
		Ctx:      "panos.bgp.virtual_routers.collection",
		Type:     collectorapi.Line,
		Priority: prioBGPVirtualRoutersCollection,
		Dims: collectorapi.Dims{
			{ID: "bgp_virtual_routers_collection_discovered", Name: "discovered"},
			{ID: "bgp_virtual_routers_collection_monitored", Name: "monitored"},
			{ID: "bgp_virtual_routers_collection_omitted_by_selector", Name: "omitted_by_selector"},
			{ID: "bgp_virtual_routers_collection_omitted_by_limit", Name: "omitted_by_limit"},
		},
	}
	environmentSensorsCollectionChartTmpl = collectorapi.Chart{
		ID:       "env_sensors_collection",
		Title:    "Environment Sensor Collection",
		Units:    "sensors",
		Fam:      "environment sensors",
		Ctx:      "panos.environment.sensors.collection",
		Type:     collectorapi.Line,
		Priority: prioEnvironmentSensorsCollection,
		Dims: collectorapi.Dims{
			{ID: "env_sensors_collection_discovered", Name: "discovered"},
			{ID: "env_sensors_collection_monitored", Name: "monitored"},
			{ID: "env_sensors_collection_omitted_by_selector", Name: "omitted_by_selector"},
			{ID: "env_sensors_collection_omitted_by_limit", Name: "omitted_by_limit"},
		},
	}
	licensesCollectionChartTmpl = collectorapi.Chart{
		ID:       "license_collection",
		Title:    "License Collection",
		Units:    "licenses",
		Fam:      "licenses",
		Ctx:      "panos.license.collection",
		Type:     collectorapi.Line,
		Priority: prioLicensesCollection,
		Dims: collectorapi.Dims{
			{ID: "license_collection_discovered", Name: "discovered"},
			{ID: "license_collection_monitored", Name: "monitored"},
			{ID: "license_collection_omitted_by_selector", Name: "omitted_by_selector"},
			{ID: "license_collection_omitted_by_limit", Name: "omitted_by_limit"},
		},
	}
	ipsecTunnelsCollectionChartTmpl = collectorapi.Chart{
		ID:       "ipsec_tunnels_collection",
		Title:    "IPsec Tunnel Collection",
		Units:    "tunnels",
		Fam:      "ipsec tunnels",
		Ctx:      "panos.ipsec.tunnels.collection",
		Type:     collectorapi.Line,
		Priority: prioIPSecTunnelsCollection,
		Dims: collectorapi.Dims{
			{ID: "ipsec_tunnels_collection_discovered", Name: "discovered"},
			{ID: "ipsec_tunnels_collection_monitored", Name: "monitored"},
			{ID: "ipsec_tunnels_collection_omitted_by_selector", Name: "omitted_by_selector"},
			{ID: "ipsec_tunnels_collection_omitted_by_limit", Name: "omitted_by_limit"},
		},
	}
)

func (c *Collector) addPeerCharts(peer bgpPeer) {
	key := peerKey(peer)
	if c.activePeerCharts[key] {
		return
	}
	c.activePeerCharts[key] = true
	c.missingPeerCharts[key] = 0

	charts := peerChartTemplates.Copy()
	labels := peerLabels(peer)
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, key)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, key)
		}
	}

	if err := c.charts.Add(*charts...); err != nil {
		c.Warningf("error adding BGP peer charts for %s: %v", peer.PeerAddress, err)
	}
}

func (c *Collector) addPrefixCharts(peer bgpPeer, counter bgpPrefixCounter) {
	key := prefixKey(peer, counter)
	if c.activePrefixCharts[key] {
		return
	}
	c.activePrefixCharts[key] = true
	c.missingPrefixCharts[key] = 0

	charts := prefixChartTemplates.Copy()
	labels := append(peerLabels(peer), collectorapi.Label{Key: "afi", Value: counter.AFI, Source: collectorapi.LabelSourceAuto})
	labels = append(labels, collectorapi.Label{Key: "safi", Value: counter.SAFI, Source: collectorapi.LabelSourceAuto})

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, key)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, key)
		}
	}

	if err := c.charts.Add(*charts...); err != nil {
		c.Warningf("error adding BGP prefix charts for %s: %v", key, err)
	}
}

func (c *Collector) addVRCharts(vr string) {
	key := cleanID(vr)
	if c.activeVRCharts[key] {
		return
	}
	c.activeVRCharts[key] = true
	c.missingVRCharts[key] = 0

	charts := vrChartTemplates.Copy()
	labels := []collectorapi.Label{{Key: "vr", Value: vr, Source: collectorapi.LabelSourceAuto}}
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, key)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, key)
		}
	}

	if err := c.charts.Add(*charts...); err != nil {
		c.Warningf("error adding BGP VR charts for %s: %v", vr, err)
	}
}

func (c *Collector) addSystemCharts(info systemInfo) {
	charts := systemChartTemplates.Copy()
	labels := systemLabels(info)
	for _, chart := range *charts {
		chart.Labels = labels
		c.addDynamicChart(chart)
	}
}

func (c *Collector) addHACharts() {
	for _, chart := range *haChartTemplates.Copy() {
		c.addDynamicChart(chart)
	}
}

func (c *Collector) addHAEnabledChart() {
	c.addDynamicChart(haChartTemplates[0].Copy())
}

func (c *Collector) addEnvironmentTemperatureChart(key string, entry environmentEntry) {
	chart := environmentTemperatureChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = environmentLabels("temperature", entry)
	c.addDynamicChart(chart)
}

func (c *Collector) addEnvironmentFanChart(key string, entry environmentEntry) {
	chart := environmentFanChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = environmentLabels("fan", entry)
	c.addDynamicChart(chart)
}

func (c *Collector) addEnvironmentVoltageChart(key string, entry environmentEntry) {
	chart := environmentVoltageChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = environmentLabels("voltage", entry)
	c.addDynamicChart(chart)
}

func (c *Collector) addEnvironmentSensorAlarmChart(key, sensorType string, entry environmentEntry) {
	chart := environmentSensorAlarmChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = environmentLabels(sensorType, entry)
	c.addDynamicChart(chart)
}

func (c *Collector) addEnvironmentPowerSupplyChart(key string, entry environmentEntry) {
	chart := environmentPowerSupplyChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = environmentLabels("power_supply", entry)
	c.addDynamicChart(chart)
}

func (c *Collector) addLicenseCountChart() {
	c.addDynamicChart(licenseCountChartTmpl.Copy())
}

func (c *Collector) addLicenseStatusChart(key string, entry licenseEntry) {
	labels := licenseLabels(entry)
	chart := licenseStatusChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = labels
	c.addDynamicChart(chart)
}

func (c *Collector) addLicenseExpirationChart(key string, entry licenseEntry) {
	labels := licenseLabels(entry)
	chart := licenseExpirationChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = labels
	c.addDynamicChart(chart)
}

func (c *Collector) addIPSecTunnelsChart() {
	c.addDynamicChart(ipsecTunnelsChartTmpl.Copy())
}

func (c *Collector) addIPSecTunnelCharts(key string, tunnel ipsecTunnel) {
	chart := ipsecTunnelSALifetimeChartTmpl.Copy()
	formatChartIDs(chart, key)
	chart.Labels = ipsecTunnelLabels(tunnel)
	c.addDynamicChart(chart)
}

func (c *Collector) addBGPPeersCollectionChart() {
	c.addDynamicChart(bgpPeersCollectionChartTmpl.Copy())
}

func (c *Collector) addBGPPrefixFamiliesCollectionChart() {
	c.addDynamicChart(bgpPrefixFamiliesCollectionChartTmpl.Copy())
}

func (c *Collector) addBGPVirtualRoutersCollectionChart() {
	c.addDynamicChart(bgpVirtualRoutersCollectionChartTmpl.Copy())
}

func (c *Collector) addEnvironmentSensorsCollectionChart() {
	c.addDynamicChart(environmentSensorsCollectionChartTmpl.Copy())
}

func (c *Collector) addLicensesCollectionChart() {
	c.addDynamicChart(licensesCollectionChartTmpl.Copy())
}

func (c *Collector) addIPSecTunnelsCollectionChart() {
	c.addDynamicChart(ipsecTunnelsCollectionChartTmpl.Copy())
}

func (c *Collector) beginDynamicChartCollection() {
	if c.seenDynamicCharts == nil {
		c.seenDynamicCharts = make(map[string]bool)
		return
	}
	clear(c.seenDynamicCharts)
}

func (c *Collector) addDynamicChart(chart *collectorapi.Chart) {
	if chart == nil {
		return
	}
	if c.seenDynamicCharts != nil {
		c.seenDynamicCharts[chart.ID] = true
	}
	if c.activeDynamicCharts[chart.ID] {
		c.missingDynamicCharts[chart.ID] = 0
		return
	}

	c.activeDynamicCharts[chart.ID] = true
	c.missingDynamicCharts[chart.ID] = 0
	if err := c.charts.Add(chart); err != nil {
		c.Warningf("error adding PAN-OS chart %s: %v", chart.ID, err)
	}
}

func (c *Collector) removeStaleCharts(peers []bgpPeer, vrs []string) {
	seenPeers := make(map[string]bool)
	seenPrefixes := make(map[string]bool)
	seenVRs := make(map[string]bool)

	for _, peer := range peers {
		seenPeers[peerKey(peer)] = true
		for _, counter := range peer.PrefixCounters {
			seenPrefixes[prefixKey(peer, counter)] = true
		}
	}
	for _, vr := range vrs {
		seenVRs[vr] = true
	}

	for key := range c.activePeerCharts {
		if seenPeers[key] {
			c.missingPeerCharts[key] = 0
			continue
		}
		c.missingPeerCharts[key]++
		if c.missingPeerCharts[key] >= staleChartMaxMisses {
			delete(c.activePeerCharts, key)
			delete(c.missingPeerCharts, key)
			c.removePeerCharts(key)
		}
	}
	for key := range c.activePrefixCharts {
		if seenPrefixes[key] {
			c.missingPrefixCharts[key] = 0
			continue
		}
		c.missingPrefixCharts[key]++
		if c.missingPrefixCharts[key] >= staleChartMaxMisses {
			delete(c.activePrefixCharts, key)
			delete(c.missingPrefixCharts, key)
			c.removePrefixCharts(key)
		}
	}
	for key := range c.activeVRCharts {
		if seenVRs[key] {
			c.missingVRCharts[key] = 0
			continue
		}
		c.missingVRCharts[key]++
		if c.missingVRCharts[key] >= staleChartMaxMisses {
			delete(c.activeVRCharts, key)
			delete(c.missingVRCharts, key)
			c.removeVRCharts(key)
		}
	}
}

func (c *Collector) removeStaleDynamicCharts() {
	for id := range c.activeDynamicCharts {
		if c.seenDynamicCharts[id] {
			c.missingDynamicCharts[id] = 0
			continue
		}
		c.missingDynamicCharts[id]++
		if c.missingDynamicCharts[id] < staleChartMaxMisses {
			continue
		}

		delete(c.activeDynamicCharts, id)
		delete(c.missingDynamicCharts, id)
		if chart := c.charts.Get(id); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) removePeerCharts(key string) {
	for _, tmpl := range peerChartTemplates {
		if chart := c.charts.Get(fmt.Sprintf(tmpl.ID, key)); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) removePrefixCharts(key string) {
	for _, tmpl := range prefixChartTemplates {
		if chart := c.charts.Get(fmt.Sprintf(tmpl.ID, key)); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) removeVRCharts(key string) {
	for _, tmpl := range vrChartTemplates {
		if chart := c.charts.Get(fmt.Sprintf(tmpl.ID, key)); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func peerLabels(peer bgpPeer) []collectorapi.Label {
	return []collectorapi.Label{
		{Key: "vr", Value: peer.VR, Source: collectorapi.LabelSourceAuto},
		{Key: "peer_address", Value: peer.PeerAddress, Source: collectorapi.LabelSourceAuto},
		{Key: "local_address", Value: peer.LocalAddress, Source: collectorapi.LabelSourceAuto},
		{Key: "remote_as", Value: peer.RemoteAS, Source: collectorapi.LabelSourceAuto},
		{Key: "peer_group", Value: peer.PeerGroup, Source: collectorapi.LabelSourceAuto},
	}
}

func peerKey(peer bgpPeer) string {
	return cleanID(peer.VR + "_" + peer.PeerAddress)
}

func prefixKey(peer bgpPeer, counter bgpPrefixCounter) string {
	return cleanID(peer.VR + "_" + peer.PeerAddress + "_" + counter.AFI + "_" + counter.SAFI)
}

func formatChartIDs(chart *collectorapi.Chart, key string) {
	chart.ID = fmt.Sprintf(chart.ID, key)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf(dim.ID, key)
	}
}

func systemLabels(info systemInfo) []collectorapi.Label {
	values := map[string]string{
		"hostname":   firstNonEmpty(info.Hostname, info.DeviceName),
		"model":      info.Model,
		"serial":     info.Serial,
		"sw_version": info.SWVersion,
	}
	return labelsFromMap(values)
}

func environmentLabels(sensorType string, entry environmentEntry) []collectorapi.Label {
	return labelsFromMap(map[string]string{
		"slot":        firstNonEmpty(entry.Slot, "unknown"),
		"sensor":      environmentSensorName(entry),
		"sensor_type": sensorType,
	})
}

func licenseLabels(entry licenseEntry) []collectorapi.Label {
	return labelsFromMap(map[string]string{
		"feature":     entry.Feature,
		"description": entry.Description,
	})
}

func ipsecTunnelLabels(tunnel ipsecTunnel) []collectorapi.Label {
	return labelsFromMap(map[string]string{
		"tunnel":     firstNonEmpty(tunnel.Name, "unknown"),
		"gateway":    tunnel.Gateway,
		"remote":     tunnel.Remote,
		"tunnel_id":  firstNonEmpty(tunnel.TID, tunnel.ISPI, tunnel.OSPI),
		"protocol":   tunnel.Protocol,
		"encryption": tunnel.Encryption,
	})
}

func labelsFromMap(values map[string]string) []collectorapi.Label {
	labels := make([]collectorapi.Label, 0, len(values))
	for _, key := range sortedLabelKeys(values) {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		labels = append(labels, collectorapi.Label{Key: key, Value: value, Source: collectorapi.LabelSourceAuto})
	}
	return labels
}

func sortedLabelKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

var invalidIDChars = regexp.MustCompile(`[^a-zA-Z0-9_]+`)

func cleanID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, ".", "_")
	value = strings.ReplaceAll(value, ":", "_")
	value = invalidIDChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "unknown"
	}
	return value
}
