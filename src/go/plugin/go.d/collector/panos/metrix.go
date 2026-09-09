// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	system systemMetrics
	ha     haMetrics
	env    environmentMetricInstruments
	lic    licenseMetricInstruments
	ipsec  ipsecMetricInstruments
	bgp    bgpMetricInstruments
}

type systemMetrics struct {
	uptime          metrix.SnapshotGaugeVec
	certStatus      metrix.SnapshotStateSetVec
	operationalMode metrix.SnapshotStateSetVec
}

type haMetrics struct {
	status               metrix.StateSetInstrument
	localState           metrix.StateSetInstrument
	peerState            metrix.StateSetInstrument
	peerConnectionStatus metrix.StateSetInstrument
	stateSync            metrix.StateSetInstrument
	linkStatus           metrix.SnapshotStateSetVec
}

type environmentMetricInstruments struct {
	temperature         metrix.SnapshotGaugeVec
	fanSpeed            metrix.SnapshotGaugeVec
	voltage             metrix.SnapshotGaugeVec
	sensorAlarm         metrix.SnapshotStateSetVec
	powerSupplyPresence metrix.SnapshotStateSetVec
	powerSupplyAlarm    metrix.SnapshotStateSetVec
}

type licenseMetricInstruments struct {
	countTotal          metrix.SnapshotGauge
	countExpired        metrix.SnapshotGauge
	status              metrix.SnapshotStateSetVec
	timeUntilExpiration metrix.SnapshotGaugeVec
}

type ipsecMetricInstruments struct {
	tunnelsActive metrix.SnapshotGauge
	saLifetime    metrix.SnapshotGaugeVec
}

type bgpMetricInstruments struct {
	peerState                  metrix.SnapshotStateSetVec
	peerUptime                 metrix.SnapshotGaugeVec
	peerMessagesIn             metrix.SnapshotCounterVec
	peerMessagesOut            metrix.SnapshotCounterVec
	peerUpdatesIn              metrix.SnapshotCounterVec
	peerUpdatesOut             metrix.SnapshotCounterVec
	peerFlaps                  metrix.SnapshotCounterVec
	peerEstablishedTransitions metrix.SnapshotCounterVec

	peerPrefixesReceivedTotal    metrix.SnapshotGaugeVec
	peerPrefixesReceivedAccepted metrix.SnapshotGaugeVec
	peerPrefixesReceivedRejected metrix.SnapshotGaugeVec
	peerPrefixesAdvertised       metrix.SnapshotGaugeVec

	vrPeersByState     map[string]metrix.SnapshotGaugeVec
	vrPeersConfigured  metrix.SnapshotGaugeVec
	vrPeersEstablished metrix.SnapshotGaugeVec
}

var bgpStates = []string{"idle", "connect", "active", "opensent", "openconfirm", "established", "unknown"}
var certStatusStates = []string{"valid", "invalid"}
var haStates = []string{"active", "passive", "non_functional", "suspended", "unknown"}
var haStatusStates = []string{"enabled", "disabled"}
var licenseStatusStates = []string{"valid", "expired"}
var operationalModeStates = []string{"normal", "other"}
var upDownStates = []string{"up", "down", "unknown"}
var haSyncStates = []string{"synchronized", "not_synchronized", "unknown"}
var alarmStates = []string{"clear", "alarm"}
var presenceStates = []string{"present", "absent"}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	system := meter.Vec("hostname", "model", "serial", "sw_version")
	haLink := meter.Vec("link")
	environment := meter.Vec("sensor_type", "slot", "sensor")
	licenses := meter.Vec("feature", "description")
	ipsecTunnels := meter.Vec("tunnel", "gateway", "remote", "tunnel_id", "protocol", "encryption")
	bgpPeer := meter.Vec("vr", "peer_address", "local_address", "remote_as", "peer_group")
	bgpPrefix := meter.Vec("vr", "peer_address", "local_address", "remote_as", "peer_group", "afi", "safi")
	bgpVR := meter.Vec("vr")

	return &collectorMetrics{
		system: systemMetrics{
			uptime:          system.Gauge("system_uptime"),
			certStatus:      newStateSetVec(system, "system_device_certificate_status", certStatusStates),
			operationalMode: newStateSetVec(system, "system_operational_mode", operationalModeStates),
		},
		ha: haMetrics{
			status:               newStateSet(meter, "ha_status", haStatusStates),
			localState:           newStateSet(meter, "ha_local_state", haStates),
			peerState:            newStateSet(meter, "ha_peer_state", haStates),
			peerConnectionStatus: newStateSet(meter, "ha_peer_connection_status", upDownStates),
			stateSync:            newStateSet(meter, "ha_state_sync_status", haSyncStates),
			linkStatus:           newStateSetVec(haLink, "ha_link_status", upDownStates),
		},
		env: environmentMetricInstruments{
			temperature:         environment.Gauge("environment_temperature"),
			fanSpeed:            environment.Gauge("environment_fan_speed"),
			voltage:             environment.Gauge("environment_voltage"),
			sensorAlarm:         newStateSetVec(environment, "environment_sensor_alarm_status", alarmStates),
			powerSupplyPresence: newStateSetVec(environment, "environment_power_supply_presence_status", presenceStates),
			powerSupplyAlarm:    newStateSetVec(environment, "environment_power_supply_alarm_status", alarmStates),
		},
		lic: licenseMetricInstruments{
			countTotal:          meter.Gauge("license_count_total"),
			countExpired:        meter.Gauge("license_count_expired"),
			status:              newStateSetVec(licenses, "license_status", licenseStatusStates),
			timeUntilExpiration: licenses.Gauge("license_time_until_expiration"),
		},
		ipsec: ipsecMetricInstruments{
			tunnelsActive: meter.Gauge("ipsec_tunnels_active"),
			saLifetime:    ipsecTunnels.Gauge("ipsec_tunnel_sa_lifetime"),
		},
		bgp: bgpMetricInstruments{
			peerState:                  newStateSetVec(bgpPeer, "bgp_peer_state", bgpStates),
			peerUptime:                 bgpPeer.Gauge("bgp_peer_uptime"),
			peerMessagesIn:             bgpPeer.Counter("bgp_peer_messages_in"),
			peerMessagesOut:            bgpPeer.Counter("bgp_peer_messages_out"),
			peerUpdatesIn:              bgpPeer.Counter("bgp_peer_updates_in"),
			peerUpdatesOut:             bgpPeer.Counter("bgp_peer_updates_out"),
			peerFlaps:                  bgpPeer.Counter("bgp_peer_flaps"),
			peerEstablishedTransitions: bgpPeer.Counter("bgp_peer_established_transitions"),

			peerPrefixesReceivedTotal:    bgpPrefix.Gauge("bgp_peer_prefixes_received_total"),
			peerPrefixesReceivedAccepted: bgpPrefix.Gauge("bgp_peer_prefixes_received_accepted"),
			peerPrefixesReceivedRejected: bgpPrefix.Gauge("bgp_peer_prefixes_received_rejected"),
			peerPrefixesAdvertised:       bgpPrefix.Gauge("bgp_peer_prefixes_advertised"),

			vrPeersByState:     newBGPStateGauges(bgpVR, "bgp_vr_peers_by_state"),
			vrPeersConfigured:  bgpVR.Gauge("bgp_vr_peers_total_configured"),
			vrPeersEstablished: bgpVR.Gauge("bgp_vr_peers_total_established"),
		},
	}
}

func newStateSet(meter metrix.SnapshotMeter, name string, states []string) metrix.StateSetInstrument {
	return meter.StateSet(name, metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(states...))
}

func newBGPStateGauges(meter metrix.SnapshotVecMeter, prefix string) map[string]metrix.SnapshotGaugeVec {
	return newStateGaugeVecs(meter, prefix, bgpStates)
}

func newStateSetVec(meter metrix.SnapshotVecMeter, name string, states []string) metrix.SnapshotStateSetVec {
	return meter.StateSet(name, metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(states...))
}

func newStateGaugeVecs(meter metrix.SnapshotVecMeter, prefix string, states []string) map[string]metrix.SnapshotGaugeVec {
	gauges := make(map[string]metrix.SnapshotGaugeVec, len(states))
	for _, state := range states {
		gauges[state] = meter.Gauge(prefix + "_" + state)
	}
	return gauges
}
