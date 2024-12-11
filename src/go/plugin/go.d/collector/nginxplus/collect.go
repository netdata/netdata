// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.apiVersion == 0 {
		v, err := c.queryAPIVersion()
		if err != nil {
			return nil, err
		}
		c.apiVersion = v
	}

	now := time.Now()
	if now.Sub(c.queryEndpointsTime) > c.queryEndpointsEvery {
		c.queryEndpointsTime = now
		if err := c.queryAvailableEndpoints(); err != nil {
			return nil, err
		}
	}

	ms := c.queryMetrics()
	if ms.empty() {
		return nil, errors.New("no metrics collected")
	}

	mx := make(map[string]int64)
	c.cache.resetUpdated()
	c.collectInfo(mx, ms)
	c.collectConnections(mx, ms)
	c.collectSSL(mx, ms)
	c.collectHTTPRequests(mx, ms)
	c.collectHTTPCache(mx, ms)
	c.collectHTTPServerZones(mx, ms)
	c.collectHTTPLocationZones(mx, ms)
	c.collectHTTPUpstreams(mx, ms)
	c.collectStreamServerZones(mx, ms)
	c.collectStreamUpstreams(mx, ms)
	c.collectResolvers(mx, ms)
	c.updateCharts()

	return mx, nil
}

func (c *Collector) collectInfo(mx map[string]int64, ms *nginxMetrics) {
	if ms.info == nil {
		return
	}
	mx["uptime"] = int64(ms.info.Timestamp.Sub(ms.info.LoadTimestamp).Seconds())
}

func (c *Collector) collectConnections(mx map[string]int64, ms *nginxMetrics) {
	if ms.connections == nil {
		return
	}
	mx["connections_accepted"] = ms.connections.Accepted
	mx["connections_dropped"] = ms.connections.Dropped
	mx["connections_active"] = ms.connections.Active
	mx["connections_idle"] = ms.connections.Idle
}

func (c *Collector) collectSSL(mx map[string]int64, ms *nginxMetrics) {
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

func (c *Collector) collectHTTPRequests(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpRequests == nil {
		return
	}
	mx["http_requests_total"] = ms.httpRequests.Total
	mx["http_requests_current"] = ms.httpRequests.Current
}

func (c *Collector) collectHTTPCache(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpCaches == nil {
		return
	}
	for name, cache := range *ms.httpCaches {
		c.cache.putHTTPCache(name)
		px := fmt.Sprintf("http_cache_%s_", name)
		mx[px+"state_cold"] = metrix.Bool(cache.Cold)
		mx[px+"state_warm"] = metrix.Bool(!cache.Cold)
		mx[px+"size"] = cache.Size
		mx[px+"served_responses"] = cache.Hit.Responses + cache.Stale.Responses + cache.Updating.Responses + cache.Revalidated.Responses
		mx[px+"written_responses"] = cache.Miss.ResponsesWritten + cache.Expired.ResponsesWritten + cache.Bypass.ResponsesWritten
		mx[px+"bypassed_responses"] = cache.Miss.Responses + cache.Expired.Responses + cache.Bypass.Responses
		mx[px+"served_bytes"] = cache.Hit.Bytes + cache.Stale.Bytes + cache.Updating.Bytes + cache.Revalidated.Bytes
		mx[px+"written_bytes"] = cache.Miss.BytesWritten + cache.Expired.BytesWritten + cache.Bypass.BytesWritten
		mx[px+"bypassed_bytes"] = cache.Miss.Bytes + cache.Expired.Bytes + cache.Bypass.Bytes
	}
}

func (c *Collector) collectHTTPServerZones(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpServerZones == nil {
		return
	}
	for name, zone := range *ms.httpServerZones {
		c.cache.putHTTPServerZone(name)

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

func (c *Collector) collectHTTPLocationZones(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpLocationZones == nil {
		return
	}
	for name, zone := range *ms.httpLocationZones {
		c.cache.putHTTPLocationZone(name)

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

func (c *Collector) collectHTTPUpstreams(mx map[string]int64, ms *nginxMetrics) {
	if ms.httpUpstreams == nil {
		return
	}
	for name, upstream := range *ms.httpUpstreams {
		c.cache.putHTTPUpstream(name, upstream.Zone)

		px := fmt.Sprintf("http_upstream_%s_zone_%s_", name, upstream.Zone)
		mx[px+"zombies"] = upstream.Zombies
		mx[px+"keepalive"] = upstream.Keepalive
		mx[px+"peers"] = int64(len(upstream.Peers))

		for _, peer := range upstream.Peers {
			c.cache.putHTTPUpstreamServer(name, peer.Server, peer.Name, upstream.Zone)

			px = fmt.Sprintf("http_upstream_%s_server_%s_zone_%s_", name, peer.Server, upstream.Zone)
			mx[px+"active"] = peer.Active
			for _, v := range []string{"up", "down", "draining", "unavail", "checking", "unhealthy"} {
				mx[px+"state_"+v] = metrix.Bool(peer.State == v)
			}
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

func (c *Collector) collectStreamServerZones(mx map[string]int64, ms *nginxMetrics) {
	if ms.streamServerZones == nil {
		return
	}
	for name, zone := range *ms.streamServerZones {
		c.cache.putStreamServerZone(name)

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

func (c *Collector) collectStreamUpstreams(mx map[string]int64, ms *nginxMetrics) {
	if ms.streamUpstreams == nil {
		return
	}
	for name, upstream := range *ms.streamUpstreams {
		c.cache.putStreamUpstream(name, upstream.Zone)

		px := fmt.Sprintf("stream_upstream_%s_zone_%s_", name, upstream.Zone)
		mx[px+"zombies"] = upstream.Zombies
		mx[px+"peers"] = int64(len(upstream.Peers))

		for _, peer := range upstream.Peers {
			c.cache.putStreamUpstreamServer(name, peer.Server, peer.Name, upstream.Zone)

			px = fmt.Sprintf("stream_upstream_%s_server_%s_zone_%s_", name, peer.Server, upstream.Zone)

			mx[px+"active"] = peer.Active
			mx[px+"connections"] = peer.Connections
			for _, v := range []string{"up", "down", "unavail", "checking", "unhealthy"} {
				mx[px+"state_"+v] = metrix.Bool(peer.State == v)
			}
			mx[px+"bytes_received"] = peer.Received
			mx[px+"bytes_sent"] = peer.Sent
			mx[px+"downtime"] = peer.Downtime / 1000
		}
	}
}

func (c *Collector) collectResolvers(mx map[string]int64, ms *nginxMetrics) {
	if ms.resolvers == nil {
		return
	}
	for name, zone := range *ms.resolvers {
		c.cache.putResolver(name)

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

func (c *Collector) updateCharts() {
	const notSeenLimit = 3

	for key, v := range c.cache.httpCaches {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addHTTPCacheCharts(v.name)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.httpCaches, key)
				c.removeHTTPCacheCharts(v.name)
			}
		}
	}
	for key, v := range c.cache.httpServerZones {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addHTTPServerZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.httpServerZones, key)
				c.removeHTTPServerZoneCharts(v.zone)
			}
		}
	}
	for key, v := range c.cache.httpLocationZones {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addHTTPLocationZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.httpLocationZones, key)
				c.removeHTTPLocationZoneCharts(v.zone)
			}
		}
	}
	for key, v := range c.cache.httpUpstreams {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addHTTPUpstreamCharts(v.name, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.httpUpstreams, key)
				c.removeHTTPUpstreamCharts(v.name, v.zone)
			}
		}
	}
	for key, v := range c.cache.httpUpstreamServers {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addHTTPUpstreamServerCharts(v.name, v.serverAddr, v.serverName, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.httpUpstreamServers, key)
				c.removeHTTPUpstreamServerCharts(v.name, v.serverAddr, v.zone)
			}
		}
	}
	for key, v := range c.cache.streamServerZones {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addStreamServerZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.streamServerZones, key)
				c.removeStreamServerZoneCharts(v.zone)
			}
		}
	}
	for key, v := range c.cache.streamUpstreams {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addStreamUpstreamCharts(v.name, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.streamUpstreams, key)
				c.removeStreamUpstreamCharts(v.name, v.zone)
			}
		}
	}
	for key, v := range c.cache.streamUpstreamServers {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addStreamUpstreamServerCharts(v.name, v.serverAddr, v.serverName, v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.streamUpstreamServers, key)
				c.removeStreamUpstreamServerCharts(v.name, v.serverAddr, v.zone)
			}
		}
	}
	for key, v := range c.cache.resolvers {
		if v.updated && !v.hasCharts {
			v.hasCharts = true
			c.addResolverZoneCharts(v.zone)
			continue
		}
		if !v.updated {
			if v.notSeenTimes++; v.notSeenTimes >= notSeenLimit {
				delete(c.cache.resolvers, key)
				c.removeResolverZoneCharts(v.zone)
			}
		}
	}
}
