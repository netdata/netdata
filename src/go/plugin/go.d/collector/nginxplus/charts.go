// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioClientConnectionsRate = collectorapi.Priority + iota
	prioClientConnectionsCount

	prioSSLHandshakesRate
	prioSSLHandshakesFailuresRate
	prioSSLVerificationErrorsRate
	prioSSLSessionReusesRate

	prioHTTPRequestsRate
	prioHTTPRequestsCount
	prioHTTPServerZoneRequestsRate
	prioHTTPLocationZoneRequestsRate
	prioHTTPServerZoneRequestsProcessingCount
	prioHTTPServerZoneRequestsDiscardedRate
	prioHTTPLocationZoneRequestsDiscardedRate

	prioHTTPServerZoneResponsesPerCodeClassRate
	prioHTTPLocationZoneResponsesPerCodeClassRate

	prioHTTPServerZoneTrafficRate
	prioHTTPLocationZoneTrafficRate

	prioHTTPUpstreamPeersCount
	prioHTTPUpstreamZombiesCount
	prioHTTPUpstreamKeepaliveCount

	prioHTTPUpstreamServerState
	prioHTTPUpstreamServerDowntime

	prioHTTPUpstreamServerConnectionsCount

	prioHTTPUpstreamServerRequestsRate

	prioHTTPUpstreamServerResponsesPerCodeClassRate

	prioHTTPUpstreamServerResponseTime
	prioHTTPUpstreamServerResponseHeaderTime

	prioHTTPUpstreamServerTrafficRate

	prioHTTPCacheState
	prioHTTPCacheIOPS
	prioHTTPCacheIO
	prioHTTPCacheSize

	prioStreamServerZoneConnectionsRate
	prioStreamServerZoneConnectionsProcessingCount
	prioStreamServerZoneConnectionsDiscardedRate

	prioStreamServerZoneSessionsPerCodeClassRate

	prioStreamServerZoneTrafficRate

	prioStreamUpstreamPeersCount
	prioStreamUpstreamZombiesCount

	prioStreamUpstreamServerState
	prioStreamUpstreamServerDowntime

	prioStreamUpstreamServerConnectionsRate
	prioStreamUpstreamServerConnectionsCount

	prioStreamUpstreamServerTrafficRate

	prioResolverZoneRequestsRate
	prioResolverZoneResponsesRate

	prioUptime
)

var (
	baseCharts = collectorapi.Charts{
		clientConnectionsRateChart.Copy(),
		clientConnectionsCountChart.Copy(),
		sslHandshakesRateChart.Copy(),
		sslHandshakesFailuresRateChart.Copy(),
		sslVerificationErrorsRateChart.Copy(),
		sslSessionReusesRateChart.Copy(),
		httpRequestsRateChart.Copy(),
		httpRequestsCountChart.Copy(),
		uptimeChart.Copy(),
	}

	clientConnectionsRateChart = collectorapi.Chart{
		ID:       "client_connections_rate",
		Title:    "Client connections rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "nginxplus.client_connections_rate",
		Priority: prioClientConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "connections_accepted", Name: "accepted", Algo: collectorapi.Incremental},
			{ID: "connections_dropped", Name: "dropped", Algo: collectorapi.Incremental},
		},
	}
	clientConnectionsCountChart = collectorapi.Chart{
		ID:       "client_connections_count",
		Title:    "Client connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "nginxplus.client_connections_count",
		Priority: prioClientConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "connections_active", Name: "active"},
			{ID: "connections_idle", Name: "idle"},
		},
	}
	sslHandshakesRateChart = collectorapi.Chart{
		ID:       "ssl_handshakes_rate",
		Title:    "SSL handshakes rate",
		Units:    "handshakes/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_handshakes_rate",
		Priority: prioSSLHandshakesRate,
		Dims: collectorapi.Dims{
			{ID: "ssl_handshakes", Name: "successful", Algo: collectorapi.Incremental},
			{ID: "ssl_handshakes_failed", Name: "failed", Algo: collectorapi.Incremental},
		},
	}
	sslHandshakesFailuresRateChart = collectorapi.Chart{
		ID:       "ssl_handshakes_failures_rate",
		Title:    "SSL handshakes failures rate",
		Units:    "failures/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_handshakes_failures_rate",
		Priority: prioSSLHandshakesFailuresRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "ssl_no_common_protocol", Name: "no_common_protocol", Algo: collectorapi.Incremental},
			{ID: "ssl_no_common_cipher", Name: "no_common_cipher", Algo: collectorapi.Incremental},
			{ID: "ssl_handshake_timeout", Name: "timeout", Algo: collectorapi.Incremental},
			{ID: "ssl_peer_rejected_cert", Name: "peer_rejected_cert", Algo: collectorapi.Incremental},
		},
	}
	sslVerificationErrorsRateChart = collectorapi.Chart{
		ID:       "ssl_verification_errors_rate",
		Title:    "SSL verification errors rate",
		Units:    "errors/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_verification_errors_rate",
		Priority: prioSSLVerificationErrorsRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "ssl_verify_failures_no_cert", Name: "no_cert", Algo: collectorapi.Incremental},
			{ID: "ssl_verify_failures_expired_cert", Name: "expired_cert", Algo: collectorapi.Incremental},
			{ID: "ssl_verify_failures_revoked_cert", Name: "revoked_cert", Algo: collectorapi.Incremental},
			{ID: "ssl_verify_failures_hostname_mismatch", Name: "hostname_mismatch", Algo: collectorapi.Incremental},
			{ID: "ssl_verify_failures_other", Name: "other", Algo: collectorapi.Incremental},
		},
	}
	sslSessionReusesRateChart = collectorapi.Chart{
		ID:       "ssl_session_reuses_rate",
		Title:    "Session reuses during SSL handshake",
		Units:    "reuses/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_session_reuses_rate",
		Priority: prioSSLSessionReusesRate,
		Dims: collectorapi.Dims{
			{ID: "ssl_session_reuses", Name: "ssl_session", Algo: collectorapi.Incremental},
		},
	}
	httpRequestsRateChart = collectorapi.Chart{
		ID:       "http_requests_rate",
		Title:    "HTTP requests rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_requests_rate",
		Priority: prioHTTPRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "http_requests_total", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	httpRequestsCountChart = collectorapi.Chart{
		ID:       "http_requests_count",
		Title:    "HTTP requests",
		Units:    "requests",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_requests_count",
		Priority: prioHTTPRequestsCount,
		Dims: collectorapi.Dims{
			{ID: "http_requests_current", Name: "requests"},
		},
	}
	uptimeChart = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "nginxplus.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "uptime", Name: "uptime"},
		},
	}
)

var (
	httpServerZoneChartsTmpl = collectorapi.Charts{
		httpServerZoneRequestsRateChartTmpl.Copy(),
		httpServerZoneResponsesPerCodeClassRateChartTmpl.Copy(),
		httpServerZoneTrafficRateChartTmpl.Copy(),
		httpServerZoneRequestsProcessingCountChartTmpl.Copy(),
		httpServerZoneRequestsDiscardedRateChartTmpl.Copy(),
	}
	httpServerZoneRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "http_server_zone_%s_requests_rate",
		Title:    "HTTP Server Zone requests rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_server_zone_requests_rate",
		Priority: prioHTTPServerZoneRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "http_server_zone_%s_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	httpServerZoneResponsesPerCodeClassRateChartTmpl = collectorapi.Chart{
		ID:       "http_server_zone_%s_responses_per_code_class_rate",
		Title:    "HTTP Server Zone responses rate",
		Units:    "responses/s",
		Fam:      "http responses",
		Ctx:      "nginxplus.http_server_zone_responses_per_code_class_rate",
		Priority: prioHTTPServerZoneResponsesPerCodeClassRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "http_server_zone_%s_responses_1xx", Name: "1xx", Algo: collectorapi.Incremental},
			{ID: "http_server_zone_%s_responses_2xx", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "http_server_zone_%s_responses_3xx", Name: "3xx", Algo: collectorapi.Incremental},
			{ID: "http_server_zone_%s_responses_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "http_server_zone_%s_responses_5xx", Name: "5xx", Algo: collectorapi.Incremental},
		},
	}
	httpServerZoneTrafficRateChartTmpl = collectorapi.Chart{
		ID:       "http_server_zone_%s_traffic_rate",
		Title:    "HTTP Server Zone traffic",
		Units:    "bytes/s",
		Fam:      "http traffic",
		Ctx:      "nginxplus.http_server_zone_traffic_rate",
		Priority: prioHTTPServerZoneTrafficRate,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "http_server_zone_%s_bytes_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "http_server_zone_%s_bytes_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	httpServerZoneRequestsProcessingCountChartTmpl = collectorapi.Chart{
		ID:       "http_server_zone_%s_requests_processing_count",
		Title:    "HTTP Server Zone currently processed requests",
		Units:    "requests",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_server_zone_requests_processing_count",
		Priority: prioHTTPServerZoneRequestsProcessingCount,
		Dims: collectorapi.Dims{
			{ID: "http_server_zone_%s_requests_processing", Name: "processing"},
		},
	}
	httpServerZoneRequestsDiscardedRateChartTmpl = collectorapi.Chart{
		ID:       "http_server_zone_%s_requests_discarded_rate",
		Title:    "HTTP Server Zone requests discarded rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_server_zone_requests_discarded_rate",
		Priority: prioHTTPServerZoneRequestsDiscardedRate,
		Dims: collectorapi.Dims{
			{ID: "http_server_zone_%s_requests_discarded", Name: "discarded", Algo: collectorapi.Incremental},
		},
	}
)

var (
	httpLocationZoneChartsTmpl = collectorapi.Charts{
		httpLocationZoneRequestsRateChartTmpl.Copy(),
		httpLocationZoneRequestsDiscardedRateChartTmpl.Copy(),
		httpLocationZoneTrafficRateChartTmpl.Copy(),
		httpLocationZoneResponsesPerCodeClassRateChartTmpl.Copy(),
	}
	httpLocationZoneRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "http_location_zone_%s_requests_rate",
		Title:    "HTTP Location Zone requests rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_location_zone_requests_rate",
		Priority: prioHTTPLocationZoneRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "http_location_zone_%s_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	httpLocationZoneResponsesPerCodeClassRateChartTmpl = collectorapi.Chart{
		ID:       "http_location_zone_%s_responses_per_code_class_rate",
		Title:    "HTTP Location Zone responses rate",
		Units:    "responses/s",
		Fam:      "http responses",
		Ctx:      "nginxplus.http_location_zone_responses_per_code_class_rate",
		Priority: prioHTTPLocationZoneResponsesPerCodeClassRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "http_location_zone_%s_responses_1xx", Name: "1xx", Algo: collectorapi.Incremental},
			{ID: "http_location_zone_%s_responses_2xx", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "http_location_zone_%s_responses_3xx", Name: "3xx", Algo: collectorapi.Incremental},
			{ID: "http_location_zone_%s_responses_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "http_location_zone_%s_responses_5xx", Name: "5xx", Algo: collectorapi.Incremental},
		},
	}
	httpLocationZoneTrafficRateChartTmpl = collectorapi.Chart{
		ID:       "http_location_zone_%s_traffic_rate",
		Title:    "HTTP Location Zone traffic rate",
		Units:    "bytes/s",
		Fam:      "http traffic",
		Ctx:      "nginxplus.http_location_zone_traffic_rate",
		Priority: prioHTTPLocationZoneTrafficRate,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "http_location_zone_%s_bytes_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "http_location_zone_%s_bytes_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	httpLocationZoneRequestsDiscardedRateChartTmpl = collectorapi.Chart{
		ID:       "http_location_zone_%s_requests_discarded_rate",
		Title:    "HTTP Location Zone requests discarded rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_location_zone_requests_discarded_rate",
		Priority: prioHTTPLocationZoneRequestsDiscardedRate,
		Dims: collectorapi.Dims{
			{ID: "http_location_zone_%s_requests_discarded", Name: "discarded", Algo: collectorapi.Incremental},
		},
	}
)

var (
	httpUpstreamChartsTmpl = collectorapi.Charts{
		httpUpstreamPeersCountChartTmpl.Copy(),
		httpUpstreamZombiesCountChartTmpl.Copy(),
		httpUpstreamKeepaliveCountChartTmpl.Copy(),
	}
	httpUpstreamPeersCountChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_zone_%s_peers_count",
		Title:    "HTTP Upstream peers",
		Units:    "peers",
		Fam:      "http upstream",
		Ctx:      "nginxplus.http_upstream_peers_count",
		Priority: prioHTTPUpstreamPeersCount,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_zone_%s_peers", Name: "peers"},
		},
	}
	httpUpstreamZombiesCountChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_zone_%s_zombies_count",
		Title:    "HTTP Upstream zombies",
		Units:    "servers",
		Fam:      "http upstream",
		Ctx:      "nginxplus.http_upstream_zombies_count",
		Priority: prioHTTPUpstreamZombiesCount,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_zone_%s_zombies", Name: "zombie"},
		},
	}
	httpUpstreamKeepaliveCountChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_zone_%s_keepalive_count",
		Title:    "HTTP Upstream keepalive",
		Units:    "connections",
		Fam:      "http upstream",
		Ctx:      "nginxplus.http_upstream_keepalive_count",
		Priority: prioHTTPUpstreamKeepaliveCount,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_zone_%s_keepalive", Name: "keepalive"},
		},
	}

	httpUpstreamServerChartsTmpl = collectorapi.Charts{
		httpUpstreamServerRequestsRateChartTmpl.Copy(),
		httpUpstreamServerResponsesPerCodeClassRateChartTmpl.Copy(),
		httpUpstreamServerResponseTimeChartTmpl.Copy(),
		httpUpstreamServerResponseHeaderTimeChartTmpl.Copy(),
		httpUpstreamServerTrafficRateChartTmpl.Copy(),
		httpUpstreamServerStateChartTmpl.Copy(),
		httpUpstreamServerDowntimeChartTmpl.Copy(),
		httpUpstreamServerConnectionsCountChartTmpl.Copy(),
	}
	httpUpstreamServerRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_requests_rate",
		Title:    "HTTP Upstream Server requests",
		Units:    "requests/s",
		Fam:      "http upstream requests",
		Ctx:      "nginxplus.http_upstream_server_requests_rate",
		Priority: prioHTTPUpstreamServerRequestsRate,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_requests", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	httpUpstreamServerResponsesPerCodeClassRateChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_responses_per_code_class_rate",
		Title:    "HTTP Upstream Server responses",
		Units:    "responses/s",
		Fam:      "http upstream responses",
		Ctx:      "nginxplus.http_upstream_server_responses_per_code_class_rate",
		Priority: prioHTTPUpstreamServerResponsesPerCodeClassRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_1xx", Name: "1xx", Algo: collectorapi.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_2xx", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_3xx", Name: "3xx", Algo: collectorapi.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_5xx", Name: "5xx", Algo: collectorapi.Incremental},
		},
	}
	httpUpstreamServerResponseTimeChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_response_time",
		Title:    "HTTP Upstream Server average response time",
		Units:    "milliseconds",
		Fam:      "http upstream response time",
		Ctx:      "nginxplus.http_upstream_server_response_time",
		Priority: prioHTTPUpstreamServerResponseTime,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_response_time", Name: "response"},
		},
	}
	httpUpstreamServerResponseHeaderTimeChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_response_header_time",
		Title:    "HTTP Upstream Server average response header time",
		Units:    "milliseconds",
		Fam:      "http upstream response time",
		Ctx:      "nginxplus.http_upstream_server_response_header_time",
		Priority: prioHTTPUpstreamServerResponseHeaderTime,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_header_time", Name: "header"},
		},
	}
	httpUpstreamServerTrafficRateChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_traffic_rate",
		Title:    "HTTP Upstream Server traffic rate",
		Units:    "bytes/s",
		Fam:      "http upstream traffic",
		Ctx:      "nginxplus.http_upstream_server_traffic_rate",
		Priority: prioHTTPUpstreamServerTrafficRate,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_bytes_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_bytes_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	httpUpstreamServerStateChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_state",
		Title:    "HTTP Upstream Server state",
		Units:    "state",
		Fam:      "http upstream state",
		Ctx:      "nginxplus.http_upstream_server_state",
		Priority: prioHTTPUpstreamServerState,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_state_up", Name: "up"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_down", Name: "down"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_draining", Name: "draining"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_unavail", Name: "unavail"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_checking", Name: "checking"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_unhealthy", Name: "unhealthy"},
		},
	}
	httpUpstreamServerConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_connection_count",
		Title:    "HTTP Upstream Server connections",
		Units:    "connections",
		Fam:      "http upstream connections",
		Ctx:      "nginxplus.http_upstream_server_connections_count",
		Priority: prioHTTPUpstreamServerConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_active", Name: "active"},
		},
	}
	httpUpstreamServerDowntimeChartTmpl = collectorapi.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_downtime",
		Title:    "HTTP Upstream Server downtime",
		Units:    "seconds",
		Fam:      "http upstream state",
		Ctx:      "nginxplus.http_upstream_server_downtime",
		Priority: prioHTTPUpstreamServerDowntime,
		Dims: collectorapi.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_downtime", Name: "downtime"},
		},
	}
)

var (
	httpCacheChartsTmpl = collectorapi.Charts{
		httpCacheStateChartTmpl.Copy(),
		httpCacheIOPSChartTmpl.Copy(),
		httpCacheIOChartTmpl.Copy(),
		httpCacheSizeChartTmpl.Copy(),
	}
	httpCacheStateChartTmpl = collectorapi.Chart{
		ID:       "http_cache_%s_state",
		Title:    "HTTP Cache state",
		Units:    "state",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_state",
		Priority: prioHTTPCacheState,
		Dims: collectorapi.Dims{
			{ID: "http_cache_%s_state_warm", Name: "warm"},
			{ID: "http_cache_%s_state_cold", Name: "cold"},
		},
	}
	httpCacheSizeChartTmpl = collectorapi.Chart{
		ID:       "http_cache_%s_size",
		Title:    "HTTP Cache size",
		Units:    "bytes",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_size",
		Priority: prioHTTPCacheSize,
		Dims: collectorapi.Dims{
			{ID: "http_cache_%s_size", Name: "size"},
		},
	}
	httpCacheIOPSChartTmpl = collectorapi.Chart{
		ID:       "http_cache_%s_iops",
		Title:    "HTTP Cache IOPS",
		Units:    "responses/s",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_iops",
		Priority: prioHTTPCacheIOPS,
		Dims: collectorapi.Dims{
			{ID: "http_cache_%s_served_responses", Name: "served", Algo: collectorapi.Incremental},
			{ID: "http_cache_%s_written_responses", Name: "written", Algo: collectorapi.Incremental},
			{ID: "http_cache_%s_bypassed_responses", Name: "bypassed", Algo: collectorapi.Incremental},
		},
	}
	httpCacheIOChartTmpl = collectorapi.Chart{
		ID:       "http_cache_%s_io",
		Title:    "HTTP Cache IO",
		Units:    "bytes/s",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_io",
		Priority: prioHTTPCacheIO,
		Dims: collectorapi.Dims{
			{ID: "http_cache_%s_served_bytes", Name: "served", Algo: collectorapi.Incremental},
			{ID: "http_cache_%s_written_bytes", Name: "written", Algo: collectorapi.Incremental},
			{ID: "http_cache_%s_bypassed_bytes", Name: "bypassed", Algo: collectorapi.Incremental},
		},
	}
)

var (
	streamServerZoneChartsTmpl = collectorapi.Charts{
		streamServerZoneConnectionsRateChartTmpl.Copy(),
		streamServerZoneTrafficRateChartTmpl.Copy(),
		streamServerZoneSessionsPerCodeClassRateChartTmpl.Copy(),
		streamServerZoneConnectionsProcessingCountRateChartTmpl.Copy(),
		streamServerZoneConnectionsDiscardedRateChartTmpl.Copy(),
	}
	streamServerZoneConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "stream_server_zone_%s_connections_rate",
		Title:    "Stream Server Zone connections rate",
		Units:    "connections/s",
		Fam:      "stream connections",
		Ctx:      "nginxplus.stream_server_zone_connections_rate",
		Priority: prioStreamServerZoneConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "stream_server_zone_%s_connections", Name: "accepted", Algo: collectorapi.Incremental},
		},
	}
	streamServerZoneSessionsPerCodeClassRateChartTmpl = collectorapi.Chart{
		ID:       "stream_server_zone_%s_sessions_per_code_class_rate",
		Title:    "Stream Server Zone sessions rate",
		Units:    "sessions/s",
		Fam:      "stream sessions",
		Ctx:      "nginxplus.stream_server_zone_sessions_per_code_class_rate",
		Priority: prioStreamServerZoneSessionsPerCodeClassRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "stream_server_zone_%s_sessions_2xx", Name: "2xx", Algo: collectorapi.Incremental},
			{ID: "stream_server_zone_%s_sessions_4xx", Name: "4xx", Algo: collectorapi.Incremental},
			{ID: "stream_server_zone_%s_sessions_5xx", Name: "5xx", Algo: collectorapi.Incremental},
		},
	}
	streamServerZoneTrafficRateChartTmpl = collectorapi.Chart{
		ID:       "stream_server_zone_%s_traffic_rate",
		Title:    "Stream Server Zone traffic rate",
		Units:    "bytes/s",
		Fam:      "stream traffic",
		Ctx:      "nginxplus.stream_server_zone_traffic_rate",
		Priority: prioStreamServerZoneTrafficRate,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "stream_server_zone_%s_bytes_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "stream_server_zone_%s_bytes_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	streamServerZoneConnectionsProcessingCountRateChartTmpl = collectorapi.Chart{
		ID:       "stream_server_zone_%s_connections_processing_count",
		Title:    "Stream Server Zone connections processed",
		Units:    "connections",
		Fam:      "stream connections",
		Ctx:      "nginxplus.stream_server_zone_connections_processing_count",
		Priority: prioStreamServerZoneConnectionsProcessingCount,
		Dims: collectorapi.Dims{
			{ID: "stream_server_zone_%s_connections_processing", Name: "processing"},
		},
	}
	streamServerZoneConnectionsDiscardedRateChartTmpl = collectorapi.Chart{
		ID:       "stream_server_zone_%s_connections_discarded_rate",
		Title:    "Stream Server Zone connections discarded",
		Units:    "connections/s",
		Fam:      "stream connections",
		Ctx:      "nginxplus.stream_server_zone_connections_discarded_rate",
		Priority: prioStreamServerZoneConnectionsDiscardedRate,
		Dims: collectorapi.Dims{
			{ID: "stream_server_zone_%s_connections_discarded", Name: "discarded", Algo: collectorapi.Incremental},
		},
	}
)

var (
	streamUpstreamChartsTmpl = collectorapi.Charts{
		streamUpstreamPeersCountChartTmpl.Copy(),
		streamUpstreamZombiesCountChartTmpl.Copy(),
	}
	streamUpstreamPeersCountChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_zone_%s_peers_count",
		Title:    "Stream Upstream peers",
		Units:    "peers",
		Fam:      "stream upstream",
		Ctx:      "nginxplus.stream_upstream_peers_count",
		Priority: prioStreamUpstreamPeersCount,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_zone_%s_peers", Name: "peers"},
		},
	}
	streamUpstreamZombiesCountChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_zone_%s_zombies_count",
		Title:    "Stream Upstream zombies",
		Units:    "servers",
		Fam:      "stream upstream",
		Ctx:      "nginxplus.stream_upstream_zombies_count",
		Priority: prioStreamUpstreamZombiesCount,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_zone_%s_zombies", Name: "zombie"},
		},
	}

	streamUpstreamServerChartsTmpl = collectorapi.Charts{
		streamUpstreamServerConnectionsRateChartTmpl.Copy(),
		streamUpstreamServerTrafficRateChartTmpl.Copy(),
		streamUpstreamServerConnectionsCountChartTmpl.Copy(),
		streamUpstreamServerStateChartTmpl.Copy(),
		streamUpstreamServerDowntimeChartTmpl.Copy(),
	}
	streamUpstreamServerConnectionsRateChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_connection_rate",
		Title:    "Stream Upstream Server connections",
		Units:    "connections/s",
		Fam:      "stream upstream connections",
		Ctx:      "nginxplus.stream_upstream_server_connections_rate",
		Priority: prioStreamUpstreamServerConnectionsRate,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_connections", Name: "forwarded", Algo: collectorapi.Incremental},
		},
	}
	streamUpstreamServerTrafficRateChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_traffic_rate",
		Title:    "Stream Upstream Server traffic rate",
		Units:    "bytes/s",
		Fam:      "stream upstream traffic",
		Ctx:      "nginxplus.stream_upstream_server_traffic_rate",
		Priority: prioStreamUpstreamServerTrafficRate,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_bytes_received", Name: "received", Algo: collectorapi.Incremental},
			{ID: "stream_upstream_%s_server_%s_zone_%s_bytes_sent", Name: "sent", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	streamUpstreamServerStateChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_state",
		Title:    "Stream Upstream Server state",
		Units:    "state",
		Fam:      "stream upstream state",
		Ctx:      "nginxplus.stream_upstream_server_state",
		Priority: prioStreamUpstreamServerState,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_up", Name: "up"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_down", Name: "down"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_unavail", Name: "unavail"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_checking", Name: "checking"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_unhealthy", Name: "unhealthy"},
		},
	}
	streamUpstreamServerDowntimeChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_downtime",
		Title:    "Stream Upstream Server downtime",
		Units:    "seconds",
		Fam:      "stream upstream state",
		Ctx:      "nginxplus.stream_upstream_server_downtime",
		Priority: prioStreamUpstreamServerDowntime,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_downtime", Name: "downtime"},
		},
	}
	streamUpstreamServerConnectionsCountChartTmpl = collectorapi.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_connection_count",
		Title:    "Stream Upstream Server connections",
		Units:    "connections",
		Fam:      "stream upstream connections",
		Ctx:      "nginxplus.stream_upstream_server_connections_count",
		Priority: prioStreamUpstreamServerConnectionsCount,
		Dims: collectorapi.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_active", Name: "active"},
		},
	}
)

var (
	resolverZoneChartsTmpl = collectorapi.Charts{
		resolverZoneRequestsRateChartTmpl.Copy(),
		resolverZoneResponsesRateChartTmpl.Copy(),
	}
	resolverZoneRequestsRateChartTmpl = collectorapi.Chart{
		ID:       "resolver_zone_%s_requests_rate",
		Title:    "Resolver requests rate",
		Units:    "requests/s",
		Fam:      "resolver requests",
		Ctx:      "nginxplus.resolver_zone_requests_rate",
		Priority: prioResolverZoneRequestsRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "resolver_zone_%s_requests_name", Name: "name", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_requests_srv", Name: "srv", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_requests_addr", Name: "addr", Algo: collectorapi.Incremental},
		},
	}
	resolverZoneResponsesRateChartTmpl = collectorapi.Chart{
		ID:       "resolver_zone_%s_responses_rate",
		Title:    "Resolver responses rate",
		Units:    "responses/s",
		Fam:      "resolver responses",
		Ctx:      "nginxplus.resolver_zone_responses_rate",
		Priority: prioResolverZoneResponsesRate,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "resolver_zone_%s_responses_noerror", Name: "noerror", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_formerr", Name: "formerr", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_servfail", Name: "servfail", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_nxdomain", Name: "nxdomain", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_notimp", Name: "notimp", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_refused", Name: "refused", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_timedout", Name: "timedout", Algo: collectorapi.Incremental},
			{ID: "resolver_zone_%s_responses_unknown", Name: "unknown", Algo: collectorapi.Incremental},
		},
	}
)

func (c *Collector) addHTTPCacheCharts(name string) {
	charts := httpCacheChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []collectorapi.Label{
			{Key: "http_cache", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHTTPCacheCharts(name string) {
	px := fmt.Sprintf("http_cache_%s_", name)
	c.removeCharts(px)
}

func (c *Collector) addHTTPServerZoneCharts(zone string) {
	charts := httpServerZoneChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "http_server_zone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHTTPServerZoneCharts(zone string) {
	px := fmt.Sprintf("http_server_zone_%s_", zone)
	c.removeCharts(px)
}

func (c *Collector) addHTTPLocationZoneCharts(zone string) {
	charts := httpLocationZoneChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "http_location_zone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHTTPLocationZoneCharts(zone string) {
	px := fmt.Sprintf("http_location_zone_%s_", zone)
	c.removeCharts(px)
}

func (c *Collector) addHTTPUpstreamCharts(name, zone string) {
	charts := httpUpstreamChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "http_upstream_name", Value: name},
			{Key: "http_upstream_zone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHTTPUpstreamCharts(name, zone string) {
	px := fmt.Sprintf("http_upstream_%s_zone_%s", name, zone)
	c.removeCharts(px)
}

func (c *Collector) addHTTPUpstreamServerCharts(name, serverAddr, serverName, zone string) {
	charts := httpUpstreamServerChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name, serverAddr, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "http_upstream_name", Value: name},
			{Key: "http_upstream_zone", Value: zone},
			{Key: "http_upstream_server_address", Value: serverAddr},
			{Key: "http_upstream_server_name", Value: serverName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name, serverAddr, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHTTPUpstreamServerCharts(name, serverAddr, zone string) {
	px := fmt.Sprintf("http_upstream_%s_server_%s_zone_%s_", name, zone, serverAddr)
	c.removeCharts(px)
}

func (c *Collector) addStreamServerZoneCharts(zone string) {
	charts := streamServerZoneChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "stream_server_zone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeStreamServerZoneCharts(zone string) {
	px := fmt.Sprintf("stream_server_zone_%s_", zone)
	c.removeCharts(px)
}

func (c *Collector) addStreamUpstreamCharts(zone, name string) {
	charts := streamUpstreamChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, zone, name)
		chart.Labels = []collectorapi.Label{
			{Key: "stream_upstream_zone", Value: name},
			{Key: "stream_upstream_zone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, zone, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeStreamUpstreamCharts(name, zone string) {
	px := fmt.Sprintf("stream_upstream_%s_zone_%s_", name, zone)
	c.removeCharts(px)
}

func (c *Collector) addStreamUpstreamServerCharts(name, serverAddr, serverName, zone string) {
	charts := streamUpstreamServerChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name, serverAddr, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "stream_upstream_name", Value: name},
			{Key: "stream_upstream_zone", Value: zone},
			{Key: "stream_upstream_server_address", Value: serverAddr},
			{Key: "stream_upstream_server_name", Value: serverName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name, serverAddr, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeStreamUpstreamServerCharts(name, serverAddr, zone string) {
	px := fmt.Sprintf("stream_upstream_%s_server_%s_zone_%s", name, serverAddr, zone)
	c.removeCharts(px)
}

func (c *Collector) addResolverZoneCharts(zone string) {
	charts := resolverZoneChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, zone)
		chart.Labels = []collectorapi.Label{
			{Key: "resolver_zone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeResolverZoneCharts(zone string) {
	px := fmt.Sprintf("resolver_zone_%s_", zone)
	c.removeCharts(px)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
