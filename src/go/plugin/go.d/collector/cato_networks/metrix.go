// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	site  siteMetricInstruments
	iface interfaceMetricInstruments
	bgp   bgpMetricInstruments
}

type siteMetricInstruments struct {
	connectivityConnected    metrix.SnapshotGaugeVec
	connectivityDisconnected metrix.SnapshotGaugeVec
	connectivityDegraded     metrix.SnapshotGaugeVec
	connectivityUnknown      metrix.SnapshotGaugeVec
	operationalActive        metrix.SnapshotGaugeVec
	operationalDisabled      metrix.SnapshotGaugeVec
	operationalLocked        metrix.SnapshotGaugeVec
	operationalUnknown       metrix.SnapshotGaugeVec
	hosts                    metrix.SnapshotGaugeVec
	traffic                  trafficMetricWriters
}

type interfaceMetricInstruments struct {
	connected    metrix.SnapshotGaugeVec
	tunnelUptime metrix.SnapshotGaugeVec
	traffic      trafficMetricWriters
}

type bgpMetricInstruments struct {
	sessionUp           metrix.SnapshotGaugeVec
	routes              metrix.SnapshotGaugeVec
	routesLimit         metrix.SnapshotGaugeVec
	routesLimitExceeded metrix.SnapshotGaugeVec
	ribOutRoutes        metrix.SnapshotGaugeVec
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

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")

	siteVec := meter.Vec("site_id", "site_name", "pop_name")
	ifaceVec := meter.Vec("site_id", "site_name", "interface_id", "interface_name")
	bgpVec := meter.Vec("site_id", "site_name", "peer_ip", "peer_asn")

	return &collectorMetrics{
		site: siteMetricInstruments{
			connectivityConnected:    siteVec.Gauge("site_connectivity_connected"),
			connectivityDisconnected: siteVec.Gauge("site_connectivity_disconnected"),
			connectivityDegraded:     siteVec.Gauge("site_connectivity_degraded"),
			connectivityUnknown:      siteVec.Gauge("site_connectivity_unknown"),
			operationalActive:        siteVec.Gauge("site_operational_active"),
			operationalDisabled:      siteVec.Gauge("site_operational_disabled"),
			operationalLocked:        siteVec.Gauge("site_operational_locked"),
			operationalUnknown:       siteVec.Gauge("site_operational_unknown"),
			hosts:                    siteVec.Gauge("site_hosts"),
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
		iface: interfaceMetricInstruments{
			connected:    ifaceVec.Gauge("interface_connected"),
			tunnelUptime: ifaceVec.Gauge("interface_tunnel_uptime_seconds"),
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
			sessionUp:           bgpVec.Gauge("bgp_session_up"),
			routes:              bgpVec.Gauge("bgp_routes"),
			routesLimit:         bgpVec.Gauge("bgp_routes_limit"),
			routesLimitExceeded: bgpVec.Gauge("bgp_routes_limit_exceeded"),
			ribOutRoutes:        bgpVec.Gauge("bgp_rib_out_routes"),
		},
	}
}
