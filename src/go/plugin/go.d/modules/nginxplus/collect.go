// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"errors"
	"fmt"
	"time"
)

func (n *NginxPlus) collect() (map[string]int64, error) {
	if n.apiVersion == 0 {
		v, err := n.queryAPIVersion()
		if err != nil {
			return nil, err
		}
		n.apiVersion = v
	}

	now := time.Now()
	if now.Sub(n.queryEndpointsTime) > n.queryEndpointsEvery {
		n.queryEndpointsTime = now
		if err := n.queryAvailableEndpoints(); err != nil {
			return nil, err
		}
	}

	ms := n.queryMetrics()
	if ms.empty() {
		return nil, errors.New("no metrics collected")
	}

	mx := make(map[string]int64)
	n.cache.resetUpdated()
	n.collectInfo(mx, ms)
	n.collectConnections(mx, ms)
	n.collectSSL(mx, ms)
	n.collectHTTPRequests(mx, ms)
	n.collectHTTPCache(mx, ms)
	n.collectHTTPServerZones(mx, ms)
	n.collectHTTPLocationZones(mx, ms)
	n.collectHTTPUpstreams(mx, ms)
	n.collectStreamServerZones(mx, ms)
	n.collectStreamUpstreams(mx, ms)
	n.collectResolvers(mx, ms)
	n.updateCharts()

	return mx, nil
}

func (n *NginxPlus) collectInfo(mx map[string]int64, ms *nginxMetrics) {
	if ms.info == nil {
		return
	}
	mx["uptime"] = int64(ms.info.Timestamp.Sub(ms.info.LoadTimestamp).Seconds())
}

func (n *NginxPlus) collectConnections(mx map[string]int64, ms *nginxMetrics) {
	if ms.connections == nil {
		return
	}
	mx["connections_accepted"] = ms.connections.Accepted
	mx["connections_dropped"] = ms.connections.Dropped
	mx["connections_active"] = ms.connections.Active
	mx["connections_idle"] = ms.connections.Idle
}

func (n *NginxPlus) collectSSL(mx map[string]int64, ms *nginxMetrics) {
	if ms.ssl == nil {
		return
	}
	mx["ssl_handshakes"] = ms.ssl.Handshakes
	mx["ssl_handshakes_failed"] = ms.ssl.HandshakesFailed
	mx["ssl_session_reuses"] = ms.ssl.SessionReuses
	mx["ssl_no_common_protocol"] = ms.ssl.NoCommonProtocol
	mx["ssl_no_common_cipher"] = ms.ssl.NoCommonCipher
	mx["ssl_handshake_timeout"] = ms.ssl.HandshakeTimeout
	mx["ssl_peer_rejected_cert"] = ms.ssl.PeerRejectedCert
	mx["ssl_verify_failures_no_cert"] = ms.ssl.VerifyFailures.NoCert
	mx["ssl_verify_failures_expired_cert"] = ms.ssl.VerifyFailures.ExpiredCert
	mx["ssl_verify_failures_revoked_cert"] = ms.ssl.VerifyFailures.RevokedCert
	mx["ssl_verify_failures_hostname_mismatch"] = ms.ssl.VerifyFailures.HostnameMismatch
	mx["ssl_verify_failures_other"] = ms.ssl.VerifyFailures.Other
}

func (n *NginxPlus) collectHTTPRequests(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpRequests == nil {
		return
	}
	mx["http_requests_total"] = ms.httpRequests.Total
	mx["http_requests_current"] = ms.httpRequests.Current
}

func (n *NginxPlus) collectHTTPCache(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpCaches == nil {
		return
	}
	for name, cache := range *ms.httpCaches {
		n.cache.putHTTPCache(name)
		px := fmt.Sprintf("http_cache_%s_", name)
		mx[px+"state_cold"] = boolToInt(cache.Cold)
		mx[px+"state_warm"] = boolToInt(!cache.Cold)
		mx[px+"size"] = cache.Size
		mx[px+"served_responses"] = cache.Hit.Responses + cache.Stale.Responses + cache.Updating.Responses + cache.Revalidated.Responses
		mx[px+"written_responses"] = cache.Miss.ResponsesWritten + cache.Expired.ResponsesWritten + cache.Bypass.ResponsesWritten
		mx[px+"bypassed_responses"] = cache.Miss.Responses + cache.Expired.Responses + cache.Bypass.Responses
		mx[px+"served_bytes"] = cache.Hit.Bytes + cache.Stale.Bytes + cache.Updating.Bytes + cache.Revalidated.Bytes
		mx[px+"written_bytes"] = cache.Miss.BytesWritten + cache.Expired.BytesWritten + cache.Bypass.BytesWritten
		mx[px+"bypassed_bytes"] = cache.Miss.Bytes + cache.Expired.Bytes + cache.Bypass.Bytes
	}
}

func (n *NginxPlus) collectHTTPServerZones(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpServerZones == nil {
		return
	}
	for name, zone := range *ms.httpServerZones {
		n.cache.putHTTPServerZone(name)

		px := fmt.Sprintf("http_server_zone_%s_", name)
		mx[px+"requests_processing"] = zone.Processing
		mx[px+"requests"] = zone.Requests
		mx[px+"requests_discarded"] = zone.Discarded
		mx[px+"bytes_received"] = zone.Received
		mx[px+"bytes_sent"] = zone.Sent
		mx[px+"responses"] = zone.Responses.Total
		mx[px+"responses_1xx"] = zone.Responses.Class1xx
		mx[px+"responses_2xx"] = zone.Responses.Class2xx
		mx[px+"responses_3xx"] = zone.Responses.Class3xx
		mx[px+"responses_4xx"] = zone.Responses.Class4xx
		mx[px+"responses_5xx"] = zone.Responses.Class5xx
	}
}

func (n *NginxPlus) collectHTTPLocationZones(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpLocationZones == nil {
		return
	}
	for name, zone := range *ms.httpLocationZones {
		n.cache.putHTTPLocationZone(name)

		px := fmt.Sprintf("http_location_zone_%s_", name)
		mx[px+"requests"] = zone.Requests
		mx[px+"requests_discarded"] = zone.Discarded
		mx[px+"bytes_received"] = zone.Received
		mx[px+"bytes_sent"] = zone.Sent
		mx[px+"responses"] = zone.Responses.Total
		mx[px+"responses_1xx"] = zone.Responses.Class1xx
		mx[px+"responses_2xx"] = zone.Responses.Class2xx
		mx[px+"responses_3xx"] = zone.Responses.Class3xx
		mx[px+"responses_4xx"] = zone.Responses.Class4xx
		mx[px+"responses_5xx"] = zone.Responses.Class5xx
	}
}

func (n *NginxPlus) collectHTTPUpstreams(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpUpstreams == nil {
		return
	}
	for name, upstream := range *ms.httpUpstreams {
		n.cache.putHTTPUpstream(name, upstream.Zone)

		px := fmt.Sprintf("http_upstream_%s_zone_%s_", name, upstream.Zone)
		mx[px+"zombies"] = upstream.Zombies
		mx[px+"keepalive"] = upstream.Keepalive
		mx[px+"peers"] = int64(len(upstream.Peers))

		for _, peer := range upstream.Peers {
			n.cache.putHTTPUpstreamServer(name, peer.Server, peer.Name, upstream.Zone)

			px = fmt.Sprintf("http_upstream_%s_server_%s_zone_%s_", name, peer.Server, upstream.Zone)
			mx[px+"active"] = peer.Active
			mx[px+"state_up"] = boolToInt(peer.State == "up")
			mx[px+"state_down"] = boolToInt(peer.State == "down")
			mx[px+"state_draining"] = boolToInt(peer.State == "draining")
			mx[px+"state_unavail"] = boolToInt(peer.State == "unavail")
			mx[px+"state_checking"] = boolToInt(peer.State == "checking")
			mx[px+"state_unhealthy"] = boolToInt(peer.State == "unhealthy")
			mx[px+"bytes_received"] = peer.Received
			mx[px+"bytes_sent"] = peer.Sent
			mx[px+"requests"] = peer.Requests
			mx[px+"responses"] = peer.Responses.Total
			mx[px+"responses_1xx"] = peer.Responses.Class1xx
			mx[px+"responses_2xx"] = peer.Responses.Class2xx
			mx[px+"responses_3xx"] = peer.Responses.Class3xx
			mx[px+"responses_4xx"] = peer.Responses.Class4xx
			mx[px+"responses_5xx"] = peer.Responses.Class5xx
			mx[px+"response_time"] = peer.ResponseTime
			mx[px+"header_time"] = peer.HeaderTime
			mx[px+"downtime"] = peer.Downtime / 1000
		}
	}
}

func (n *NginxPlus) collectStreamServerZones(mx map[string]int64, ms *nginxMetrics) {
	if ms.streamServerZones == nil {
		return
	}
	for name, zone := range *ms.streamServerZones {
		n.cache.putStreamServerZone(name)

		px := fmt.Sprintf("stream_server_zone_%s_", name)
		mx[px+"connections"] = zone.Connections
		mx[px+"connections_processing"] = zone.Processing
		mx[px+"connections_discarded"] = zone.Discarded
		mx[px+"bytes_received"] = zone.Received
		mx[px+"bytes_sent"] = zone.Sent
		mx[px+"sessions"] = zone.Sessions.Total
		mx[px+"sessions_2xx"] = zone.Sessions.Class2xx
		mx[px+"sessions_4xx"] = zone.Sessions.Class4xx
		mx[px+"sessions_5xx"] = zone.Sessions.Class5xx
	}
}

func (n *NginxPlus) collectStreamUpstreams(mx map[string]int64, ms *nginxMetrics) {
	if ms.streamUpstreams == nil {
		return
	}
	for name, upstream := range *ms.streamUpstreams {
		n.cache.putStreamUpstream(name, upstream.Zone)

		px := fmt.Sprintf("stream_upstream_%s_zone_%s_", name, upstream.Zone)
		mx[px+"zombies"] = upstream.Zombies
		mx[px+"peers"] = int64(len(upstream.Peers))

		for _, peer := range upstream.Peers {
			n.cache.putStreamUpstreamServer(name, peer.Server, peer.Name, upstream.Zone)

			px = fmt.Sprintf("stream_upstream_%s_server_%s_zone_%s_", name, peer.Server, upstream.Zone)
			mx[px+"active"] = peer.Active
			mx[px+"connections"] = peer.Connections
			mx[px+"state_up"] = boolToInt(peer.State == "up")
			mx[px+"state_down"] = boolToInt(peer.State == "down")
			mx[px+"state_unavail"] = boolToInt(peer.State == "unavail")
			mx[px+"state_checking"] = boolToInt(peer.State == "checking")
			mx[px+"state_unhealthy"] = boolToInt(peer.State == "unhealthy")
			mx[px+"bytes_received"] = peer.Received
			mx[px+"bytes_sent"] = peer.Sent
			mx[px+"downtime"] = peer.Downtime / 1000
		}
	}
}

func (n *NginxPlus) collectResolvers(mx map[string]int64, ms *nginxMetrics) {
	if ms.resolvers == nil {
		return
	}
	for name, zone := range *ms.resolvers {
		n.cache.putResolver(name)

		px := fmt.Sprintf("resolver_zone_%s_", name)
		mx[px+"requests_name"] = zone.Requests.Name
		mx[px+"requests_srv"] = zone.Requests.Srv
		mx[px+"requests_addr"] = zone.Requests.Addr
		mx[px+"responses_noerror"] = zone.Responses.NoError
		mx[px+"responses_formerr"] = zone.Responses.Formerr
		mx[px+"responses_servfail"] = zone.Responses.Servfail
		mx[px+"responses_nxdomain"] = zone.Responses.Nxdomain
		mx[px+"responses_notimp"] = zone.Responses.Notimp
		mx[px+"responses_refused"] = zone.Responses.Refused
		mx[px+"responses_timedout"] = zone.Responses.TimedOut
		mx[px+"responses_unknown"] = zone.Responses.Unknown
	}
}

func (n *NginxPlus) updateCharts() {
	const notSeenLimit = 3

	for key, v := range n.cache.httpCaches {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addHTTPCacheCharts(v.name)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.httpCaches, key)
				n.removeHTTPCacheCharts(v.name)
			}
		}
	}
	for key, v := range n.cache.httpServerZones {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addHTTPServerZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.httpServerZones, key)
				n.removeHTTPServerZoneCharts(v.zone)
			}
		}
	}
	for key, v := range n.cache.httpLocationZones {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addHTTPLocationZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.httpLocationZones, key)
				n.removeHTTPLocationZoneCharts(v.zone)
			}
		}
	}
	for key, v := range n.cache.httpUpstreams {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addHTTPUpstreamCharts(v.name, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.httpUpstreams, key)
				n.removeHTTPUpstreamCharts(v.name, v.zone)
			}
		}
	}
	for key, v := range n.cache.httpUpstreamServers {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addHTTPUpstreamServerCharts(v.name, v.serverAddr, v.serverName, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.httpUpstreamServers, key)
				n.removeHTTPUpstreamServerCharts(v.name, v.serverAddr, v.zone)
			}
		}
	}
	for key, v := range n.cache.streamServerZones {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addStreamServerZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.streamServerZones, key)
				n.removeStreamServerZoneCharts(v.zone)
			}
		}
	}
	for key, v := range n.cache.streamUpstreams {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addStreamUpstreamCharts(v.name, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.streamUpstreams, key)
				n.removeStreamUpstreamCharts(v.name, v.zone)
			}
		}
	}
	for key, v := range n.cache.streamUpstreamServers {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addStreamUpstreamServerCharts(v.name, v.serverAddr, v.serverName, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.streamUpstreamServers, key)
				n.removeStreamUpstreamServerCharts(v.name, v.serverAddr, v.zone)
			}
		}
	}
	for key, v := range n.cache.resolvers {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			n.addResolverZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(n.cache.resolvers, key)
				n.removeResolverZoneCharts(v.zone)
			}
		}
	}
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
