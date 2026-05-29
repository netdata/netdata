// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	site   siteMetricInstruments
	device deviceMetricInstruments
	iface  interfaceMetricInstruments
	bgp    bgpMetricInstruments
}

type siteMetricInstruments struct {
	connectivityStatus metrix.SnapshotStateSetVec
	operationalStatus  metrix.SnapshotStateSetVec
	hosts              metrix.SnapshotGaugeVec
	traffic            trafficMetricWriters
}

type deviceMetricInstruments struct {
	connectionStatus metrix.SnapshotStateSetVec
}

type interfaceMetricInstruments struct {
	connectionStatus metrix.SnapshotStateSetVec
	tunnelUptime     metrix.SnapshotGaugeVec
	traffic          trafficMetricWriters
}

type bgpMetricInstruments struct {
	sessionStatus    metrix.SnapshotStateSetVec
	routes           metrix.SnapshotGaugeVec
	routesLimit      metrix.SnapshotGaugeVec
	routesLimitState metrix.SnapshotStateSetVec
	ribOutRoutes     metrix.SnapshotGaugeVec
}

type trafficMetricWriters struct {
	bytesUp         metrix.SnapshotGaugeVec
	bytesDown       metrix.SnapshotGaugeVec
	lostUp          metrix.SnapshotGaugeVec
	lostDown        metrix.SnapshotGaugeVec
	jitterUp        metrix.SnapshotGaugeVec
	jitterDown      metrix.SnapshotGaugeVec
	discardUp       metrix.SnapshotGaugeVec
	discardDown     metrix.SnapshotGaugeVec
	rtt             metrix.SnapshotGaugeVec
	lastMileLatency metrix.SnapshotGaugeVec
	lastMileLoss    metrix.SnapshotGaugeVec
}

var siteConnectivityStates = []string{"connected", "disconnected", "degraded", "unknown"}
var siteOperationalStates = []string{"active", "disabled", "locked", "unknown"}
var deviceConnectionStates = []string{"connected", "disconnected"}
var interfaceConnectionStates = []string{"connected", "disconnected"}
var bgpSessionStates = []string{"up", "down", "unknown"}
var bgpRoutesLimitStates = []string{"ok", "exceeded"}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")

	siteVec := meter.Vec("site_id", "site_name", "pop_name")
	deviceVec := meter.Vec("site_id", "site_name", "device_id", "device_name")
	ifaceVec := meter.Vec("site_id", "site_name", "device_id", "device_name", "interface_id", "interface_name")
	bgpVec := meter.Vec("site_id", "site_name", "peer_ip", "peer_asn")

	return &collectorMetrics{
		site: siteMetricInstruments{
			connectivityStatus: newStateSetVec(siteVec, "site_connectivity_status", siteConnectivityStates),
			operationalStatus:  newStateSetVec(siteVec, "site_operational_status", siteOperationalStates),
			hosts:              siteVec.Gauge("site_hosts"),
			traffic: trafficMetricWriters{
				bytesUp:         siteVec.Gauge("site_bytes_upstream_max"),
				bytesDown:       siteVec.Gauge("site_bytes_downstream_max"),
				lostUp:          siteVec.Gauge("site_lost_upstream_percent"),
				lostDown:        siteVec.Gauge("site_lost_downstream_percent"),
				jitterUp:        siteVec.Gauge("site_jitter_upstream_ms"),
				jitterDown:      siteVec.Gauge("site_jitter_downstream_ms"),
				discardUp:       siteVec.Gauge("site_packets_discarded_upstream"),
				discardDown:     siteVec.Gauge("site_packets_discarded_downstream"),
				rtt:             siteVec.Gauge("site_rtt_ms"),
				lastMileLatency: siteVec.Gauge("site_last_mile_latency_ms"),
				lastMileLoss:    siteVec.Gauge("site_last_mile_packet_loss_percent"),
			},
		},
		device: deviceMetricInstruments{
			connectionStatus: newStateSetVec(deviceVec, "device_connection_status", deviceConnectionStates),
		},
		iface: interfaceMetricInstruments{
			connectionStatus: newStateSetVec(ifaceVec, "interface_connection_status", interfaceConnectionStates),
			tunnelUptime:     ifaceVec.Gauge("interface_tunnel_uptime_seconds"),
			traffic: trafficMetricWriters{
				bytesUp:     ifaceVec.Gauge("interface_bytes_upstream_max"),
				bytesDown:   ifaceVec.Gauge("interface_bytes_downstream_max"),
				lostUp:      ifaceVec.Gauge("interface_lost_upstream_percent"),
				lostDown:    ifaceVec.Gauge("interface_lost_downstream_percent"),
				jitterUp:    ifaceVec.Gauge("interface_jitter_upstream_ms"),
				jitterDown:  ifaceVec.Gauge("interface_jitter_downstream_ms"),
				discardUp:   ifaceVec.Gauge("interface_packets_discarded_upstream"),
				discardDown: ifaceVec.Gauge("interface_packets_discarded_downstream"),
				rtt:         ifaceVec.Gauge("interface_rtt_ms"),
			},
		},
		bgp: bgpMetricInstruments{
			sessionStatus:    newStateSetVec(bgpVec, "bgp_session_status", bgpSessionStates),
			routes:           bgpVec.Gauge("bgp_routes"),
			routesLimit:      bgpVec.Gauge("bgp_routes_limit"),
			routesLimitState: newStateSetVec(bgpVec, "bgp_routes_limit_status", bgpRoutesLimitStates),
			ribOutRoutes:     bgpVec.Gauge("bgp_rib_out_routes"),
		},
	}
}

func newStateSetVec(meter metrix.SnapshotVecMeter, name string, states []string) metrix.SnapshotStateSetVec {
	return meter.StateSet(name, metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(states...))
}
