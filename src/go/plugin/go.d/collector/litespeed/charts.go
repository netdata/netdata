// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package litespeed

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioRequests = collectorapi.Priority + iota
	prioRequestsProcessing
	prioNetThroughputHttp
	prioNetThroughputHttps
	prioConnectionsHttp
	prioConnectionsHttps
	prioPublicCacheHits
	prioPrivateCacheHits
	prioStaticHits
)

var charts = collectorapi.Charts{
	requestsChart.Copy(),
	requestsProcessingChart.Copy(),

	netThroughputHttpChart.Copy(),
	netThroughputHttpsChart.Copy(),

	connectionsHttpChart.Copy(),
	connectionsHttpsChart.Copy(),

	publicCacheHitsChart.Copy(),
	privateCacheHitsChart.Copy(),
	staticCacheHitsChart.Copy(),
}

var (
	requestsChart = collectorapi.Chart{
		ID:       "requests",
		Title:    "Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "litespeed.requests",
		Priority: prioRequests,
		Dims: collectorapi.Dims{
			{ID: "req_per_sec", Name: "requests", Div: precision},
		},
	}
	requestsProcessingChart = collectorapi.Chart{
		ID:       "requests_processing",
		Title:    "Processing requests",
		Units:    "requests",
		Fam:      "requests",
		Ctx:      "litespeed.requests_processing",
		Priority: prioRequestsProcessing,
		Dims: collectorapi.Dims{
			{ID: "req_processing", Name: "processing"},
		},
	}
)

var (
	netThroughputHttpChart = collectorapi.Chart{
		ID:       "net_throughput_http",
		Title:    "HTTP throughput",
		Units:    "kilobits/s",
		Fam:      "throughput",
		Ctx:      "litespeed.net_throughput",
		Type:     collectorapi.Area,
		Priority: prioNetThroughputHttp,
		Dims: collectorapi.Dims{
			{ID: "bps_in", Name: "in"},
			{ID: "bps_out", Name: "out", Div: -1},
		},
	}
	netThroughputHttpsChart = collectorapi.Chart{
		ID:       "net_throughput_https",
		Title:    "HTTPs throughput",
		Units:    "kilobits/s",
		Fam:      "throughput",
		Ctx:      "litespeed.net_ssl_throughput",
		Type:     collectorapi.Area,
		Priority: prioNetThroughputHttps,
		Dims: collectorapi.Dims{
			{ID: "ssl_bps_in", Name: "in"},
			{ID: "ssl_bps_out", Name: "out", Div: -1},
		},
	}
)

var (
	connectionsHttpChart = collectorapi.Chart{
		ID:       "connections_http",
		Title:    "HTTP connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "litespeed.connections",
		Type:     collectorapi.Stacked,
		Priority: prioConnectionsHttp,
		Dims: collectorapi.Dims{
			{ID: "availconn", Name: "free"},
			{ID: "plainconn", Name: "used"},
		},
	}
	connectionsHttpsChart = collectorapi.Chart{
		ID:       "connections_https",
		Title:    "HTTPs connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "litespeed.ssl_connections",
		Type:     collectorapi.Stacked,
		Priority: prioConnectionsHttps,
		Dims: collectorapi.Dims{
			{ID: "availssl", Name: "free"},
			{ID: "sslconn", Name: "used"},
		},
	}
)

var (
	publicCacheHitsChart = collectorapi.Chart{
		ID:       "pub_cache_hits",
		Title:    "Public cache hits",
		Units:    "hits/s",
		Fam:      "cache",
		Ctx:      "litespeed.public_cache",
		Priority: prioPublicCacheHits,
		Dims: collectorapi.Dims{
			{ID: "pub_cache_hits_per_sec", Name: "hits", Div: precision},
		},
	}
	privateCacheHitsChart = collectorapi.Chart{
		ID:       "private_cache_hits",
		Title:    "Private cache hits",
		Units:    "hits/s",
		Fam:      "cache",
		Ctx:      "litespeed.private_cache",
		Priority: prioPrivateCacheHits,
		Dims: collectorapi.Dims{
			{ID: "private_cache_hits_per_sec", Name: "hits", Div: precision},
		},
	}

	staticCacheHitsChart = collectorapi.Chart{
		ID:       "static_hits",
		Title:    "Static hits",
		Units:    "hits/s",
		Fam:      "static",
		Ctx:      "litespeed.static",
		Priority: prioStaticHits,
		Dims: collectorapi.Dims{
			{ID: "static_hits_per_sec", Name: "hits", Div: precision},
		},
	}
)
