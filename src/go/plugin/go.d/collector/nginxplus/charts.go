// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClientConnectionsRate = module.Priority + iota
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
	baseCharts = module.Charts{
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

	clientConnectionsRateChart = module.Chart{
		ID:       "client_connections_rate",
		Title:    "Client connections rate",
		Units:    "connections/s",
		Fam:      "connections",
		Ctx:      "nginxplus.client_connections_rate",
		Priority: prioClientConnectionsRate,
		Dims: module.Dims{
			{ID: "connections_accepted", Name: "accepted", Algo: module.Incremental},
			{ID: "connections_dropped", Name: "dropped", Algo: module.Incremental},
		},
	}
	clientConnectionsCountChart = module.Chart{
		ID:       "client_connections_count",
		Title:    "Client connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "nginxplus.client_connections_count",
		Priority: prioClientConnectionsCount,
		Dims: module.Dims{
			{ID: "connections_active", Name: "active"},
			{ID: "connections_idle", Name: "idle"},
		},
	}
	sslHandshakesRateChart = module.Chart{
		ID:       "ssl_handshakes_rate",
		Title:    "SSL handshakes rate",
		Units:    "handshakes/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_handshakes_rate",
		Priority: prioSSLHandshakesRate,
		Dims: module.Dims{
			{ID: "ssl_handshakes", Name: "successful", Algo: module.Incremental},
			{ID: "ssl_handshakes_failed", Name: "failed", Algo: module.Incremental},
		},
	}
	sslHandshakesFailuresRateChart = module.Chart{
		ID:       "ssl_handshakes_failures_rate",
		Title:    "SSL handshakes failures rate",
		Units:    "failures/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_handshakes_failures_rate",
		Priority: prioSSLHandshakesFailuresRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "ssl_no_common_protocol", Name: "no_common_protocol", Algo: module.Incremental},
			{ID: "ssl_no_common_cipher", Name: "no_common_cipher", Algo: module.Incremental},
			{ID: "ssl_handshake_timeout", Name: "timeout", Algo: module.Incremental},
			{ID: "ssl_peer_rejected_cert", Name: "peer_rejected_cert", Algo: module.Incremental},
		},
	}
	sslVerificationErrorsRateChart = module.Chart{
		ID:       "ssl_verification_errors_rate",
		Title:    "SSL verification errors rate",
		Units:    "errors/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_verification_errors_rate",
		Priority: prioSSLVerificationErrorsRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "ssl_verify_failures_no_cert", Name: "no_cert", Algo: module.Incremental},
			{ID: "ssl_verify_failures_expired_cert", Name: "expired_cert", Algo: module.Incremental},
			{ID: "ssl_verify_failures_revoked_cert", Name: "revoked_cert", Algo: module.Incremental},
			{ID: "ssl_verify_failures_hostname_mismatch", Name: "hostname_mismatch", Algo: module.Incremental},
			{ID: "ssl_verify_failures_other", Name: "other", Algo: module.Incremental},
		},
	}
	sslSessionReusesRateChart = module.Chart{
		ID:       "ssl_session_reuses_rate",
		Title:    "Session reuses during SSL handshake",
		Units:    "reuses/s",
		Fam:      "ssl",
		Ctx:      "nginxplus.ssl_session_reuses_rate",
		Priority: prioSSLSessionReusesRate,
		Dims: module.Dims{
			{ID: "ssl_session_reuses", Name: "ssl_session", Algo: module.Incremental},
		},
	}
	httpRequestsRateChart = module.Chart{
		ID:       "http_requests_rate",
		Title:    "HTTP requests rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_requests_rate",
		Priority: prioHTTPRequestsRate,
		Dims: module.Dims{
			{ID: "http_requests_total", Name: "requests", Algo: module.Incremental},
		},
	}
	httpRequestsCountChart = module.Chart{
		ID:       "http_requests_count",
		Title:    "HTTP requests",
		Units:    "requests",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_requests_count",
		Priority: prioHTTPRequestsCount,
		Dims: module.Dims{
			{ID: "http_requests_current", Name: "requests"},
		},
	}
	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "nginxplus.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime", Name: "uptime"},
		},
	}
)

var (
	httpServerZoneChartsTmpl = module.Charts{
		httpServerZoneRequestsRateChartTmpl.Copy(),
		httpServerZoneResponsesPerCodeClassRateChartTmpl.Copy(),
		httpServerZoneTrafficRateChartTmpl.Copy(),
		httpServerZoneRequestsProcessingCountChartTmpl.Copy(),
		httpServerZoneRequestsDiscardedRateChartTmpl.Copy(),
	}
	httpServerZoneRequestsRateChartTmpl = module.Chart{
		ID:       "http_server_zone_%s_requests_rate",
		Title:    "HTTP Server Zone requests rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_server_zone_requests_rate",
		Priority: prioHTTPServerZoneRequestsRate,
		Dims: module.Dims{
			{ID: "http_server_zone_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	}
	httpServerZoneResponsesPerCodeClassRateChartTmpl = module.Chart{
		ID:       "http_server_zone_%s_responses_per_code_class_rate",
		Title:    "HTTP Server Zone responses rate",
		Units:    "responses/s",
		Fam:      "http responses",
		Ctx:      "nginxplus.http_server_zone_responses_per_code_class_rate",
		Priority: prioHTTPServerZoneResponsesPerCodeClassRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "http_server_zone_%s_responses_1xx", Name: "1xx", Algo: module.Incremental},
			{ID: "http_server_zone_%s_responses_2xx", Name: "2xx", Algo: module.Incremental},
			{ID: "http_server_zone_%s_responses_3xx", Name: "3xx", Algo: module.Incremental},
			{ID: "http_server_zone_%s_responses_4xx", Name: "4xx", Algo: module.Incremental},
			{ID: "http_server_zone_%s_responses_5xx", Name: "5xx", Algo: module.Incremental},
		},
	}
	httpServerZoneTrafficRateChartTmpl = module.Chart{
		ID:       "http_server_zone_%s_traffic_rate",
		Title:    "HTTP Server Zone traffic",
		Units:    "bytes/s",
		Fam:      "http traffic",
		Ctx:      "nginxplus.http_server_zone_traffic_rate",
		Priority: prioHTTPServerZoneTrafficRate,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "http_server_zone_%s_bytes_received", Name: "received", Algo: module.Incremental},
			{ID: "http_server_zone_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	httpServerZoneRequestsProcessingCountChartTmpl = module.Chart{
		ID:       "http_server_zone_%s_requests_processing_count",
		Title:    "HTTP Server Zone currently processed requests",
		Units:    "requests",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_server_zone_requests_processing_count",
		Priority: prioHTTPServerZoneRequestsProcessingCount,
		Dims: module.Dims{
			{ID: "http_server_zone_%s_requests_processing", Name: "processing"},
		},
	}
	httpServerZoneRequestsDiscardedRateChartTmpl = module.Chart{
		ID:       "http_server_zone_%s_requests_discarded_rate",
		Title:    "HTTP Server Zone requests discarded rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_server_zone_requests_discarded_rate",
		Priority: prioHTTPServerZoneRequestsDiscardedRate,
		Dims: module.Dims{
			{ID: "http_server_zone_%s_requests_discarded", Name: "discarded", Algo: module.Incremental},
		},
	}
)

var (
	httpLocationZoneChartsTmpl = module.Charts{
		httpLocationZoneRequestsRateChartTmpl.Copy(),
		httpLocationZoneRequestsDiscardedRateChartTmpl.Copy(),
		httpLocationZoneTrafficRateChartTmpl.Copy(),
		httpLocationZoneResponsesPerCodeClassRateChartTmpl.Copy(),
	}
	httpLocationZoneRequestsRateChartTmpl = module.Chart{
		ID:       "http_location_zone_%s_requests_rate",
		Title:    "HTTP Location Zone requests rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_location_zone_requests_rate",
		Priority: prioHTTPLocationZoneRequestsRate,
		Dims: module.Dims{
			{ID: "http_location_zone_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	}
	httpLocationZoneResponsesPerCodeClassRateChartTmpl = module.Chart{
		ID:       "http_location_zone_%s_responses_per_code_class_rate",
		Title:    "HTTP Location Zone responses rate",
		Units:    "responses/s",
		Fam:      "http responses",
		Ctx:      "nginxplus.http_location_zone_responses_per_code_class_rate",
		Priority: prioHTTPLocationZoneResponsesPerCodeClassRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "http_location_zone_%s_responses_1xx", Name: "1xx", Algo: module.Incremental},
			{ID: "http_location_zone_%s_responses_2xx", Name: "2xx", Algo: module.Incremental},
			{ID: "http_location_zone_%s_responses_3xx", Name: "3xx", Algo: module.Incremental},
			{ID: "http_location_zone_%s_responses_4xx", Name: "4xx", Algo: module.Incremental},
			{ID: "http_location_zone_%s_responses_5xx", Name: "5xx", Algo: module.Incremental},
		},
	}
	httpLocationZoneTrafficRateChartTmpl = module.Chart{
		ID:       "http_location_zone_%s_traffic_rate",
		Title:    "HTTP Location Zone traffic rate",
		Units:    "bytes/s",
		Fam:      "http traffic",
		Ctx:      "nginxplus.http_location_zone_traffic_rate",
		Priority: prioHTTPLocationZoneTrafficRate,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "http_location_zone_%s_bytes_received", Name: "received", Algo: module.Incremental},
			{ID: "http_location_zone_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	httpLocationZoneRequestsDiscardedRateChartTmpl = module.Chart{
		ID:       "http_location_zone_%s_requests_discarded_rate",
		Title:    "HTTP Location Zone requests discarded rate",
		Units:    "requests/s",
		Fam:      "http requests",
		Ctx:      "nginxplus.http_location_zone_requests_discarded_rate",
		Priority: prioHTTPLocationZoneRequestsDiscardedRate,
		Dims: module.Dims{
			{ID: "http_location_zone_%s_requests_discarded", Name: "discarded", Algo: module.Incremental},
		},
	}
)

var (
	httpUpstreamChartsTmpl = module.Charts{
		httpUpstreamPeersCountChartTmpl.Copy(),
		httpUpstreamZombiesCountChartTmpl.Copy(),
		httpUpstreamKeepaliveCountChartTmpl.Copy(),
	}
	httpUpstreamPeersCountChartTmpl = module.Chart{
		ID:       "http_upstream_%s_zone_%s_peers_count",
		Title:    "HTTP Upstream peers",
		Units:    "peers",
		Fam:      "http upstream",
		Ctx:      "nginxplus.http_upstream_peers_count",
		Priority: prioHTTPUpstreamPeersCount,
		Dims: module.Dims{
			{ID: "http_upstream_%s_zone_%s_peers", Name: "peers"},
		},
	}
	httpUpstreamZombiesCountChartTmpl = module.Chart{
		ID:       "http_upstream_%s_zone_%s_zombies_count",
		Title:    "HTTP Upstream zombies",
		Units:    "servers",
		Fam:      "http upstream",
		Ctx:      "nginxplus.http_upstream_zombies_count",
		Priority: prioHTTPUpstreamZombiesCount,
		Dims: module.Dims{
			{ID: "http_upstream_%s_zone_%s_zombies", Name: "zombie"},
		},
	}
	httpUpstreamKeepaliveCountChartTmpl = module.Chart{
		ID:       "http_upstream_%s_zone_%s_keepalive_count",
		Title:    "HTTP Upstream keepalive",
		Units:    "connections",
		Fam:      "http upstream",
		Ctx:      "nginxplus.http_upstream_keepalive_count",
		Priority: prioHTTPUpstreamKeepaliveCount,
		Dims: module.Dims{
			{ID: "http_upstream_%s_zone_%s_keepalive", Name: "keepalive"},
		},
	}

	httpUpstreamServerChartsTmpl = module.Charts{
		httpUpstreamServerRequestsRateChartTmpl.Copy(),
		httpUpstreamServerResponsesPerCodeClassRateChartTmpl.Copy(),
		httpUpstreamServerResponseTimeChartTmpl.Copy(),
		httpUpstreamServerResponseHeaderTimeChartTmpl.Copy(),
		httpUpstreamServerTrafficRateChartTmpl.Copy(),
		httpUpstreamServerStateChartTmpl.Copy(),
		httpUpstreamServerDowntimeChartTmpl.Copy(),
		httpUpstreamServerConnectionsCountChartTmpl.Copy(),
	}
	httpUpstreamServerRequestsRateChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_requests_rate",
		Title:    "HTTP Upstream Server requests",
		Units:    "requests/s",
		Fam:      "http upstream requests",
		Ctx:      "nginxplus.http_upstream_server_requests_rate",
		Priority: prioHTTPUpstreamServerRequestsRate,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	}
	httpUpstreamServerResponsesPerCodeClassRateChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_responses_per_code_class_rate",
		Title:    "HTTP Upstream Server responses",
		Units:    "responses/s",
		Fam:      "http upstream responses",
		Ctx:      "nginxplus.http_upstream_server_responses_per_code_class_rate",
		Priority: prioHTTPUpstreamServerResponsesPerCodeClassRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_1xx", Name: "1xx", Algo: module.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_2xx", Name: "2xx", Algo: module.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_3xx", Name: "3xx", Algo: module.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_4xx", Name: "4xx", Algo: module.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_responses_5xx", Name: "5xx", Algo: module.Incremental},
		},
	}
	httpUpstreamServerResponseTimeChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_response_time",
		Title:    "HTTP Upstream Server average response time",
		Units:    "milliseconds",
		Fam:      "http upstream response time",
		Ctx:      "nginxplus.http_upstream_server_response_time",
		Priority: prioHTTPUpstreamServerResponseTime,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_response_time", Name: "response"},
		},
	}
	httpUpstreamServerResponseHeaderTimeChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_response_header_time",
		Title:    "HTTP Upstream Server average response header time",
		Units:    "milliseconds",
		Fam:      "http upstream response time",
		Ctx:      "nginxplus.http_upstream_server_response_header_time",
		Priority: prioHTTPUpstreamServerResponseHeaderTime,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_header_time", Name: "header"},
		},
	}
	httpUpstreamServerTrafficRateChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_traffic_rate",
		Title:    "HTTP Upstream Server traffic rate",
		Units:    "bytes/s",
		Fam:      "http upstream traffic",
		Ctx:      "nginxplus.http_upstream_server_traffic_rate",
		Priority: prioHTTPUpstreamServerTrafficRate,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_bytes_received", Name: "received", Algo: module.Incremental},
			{ID: "http_upstream_%s_server_%s_zone_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	httpUpstreamServerStateChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_state",
		Title:    "HTTP Upstream Server state",
		Units:    "state",
		Fam:      "http upstream state",
		Ctx:      "nginxplus.http_upstream_server_state",
		Priority: prioHTTPUpstreamServerState,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_state_up", Name: "up"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_down", Name: "down"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_draining", Name: "draining"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_unavail", Name: "unavail"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_checking", Name: "checking"},
			{ID: "http_upstream_%s_server_%s_zone_%s_state_unhealthy", Name: "unhealthy"},
		},
	}
	httpUpstreamServerConnectionsCountChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_connection_count",
		Title:    "HTTP Upstream Server connections",
		Units:    "connections",
		Fam:      "http upstream connections",
		Ctx:      "nginxplus.http_upstream_server_connections_count",
		Priority: prioHTTPUpstreamServerConnectionsCount,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_active", Name: "active"},
		},
	}
	httpUpstreamServerDowntimeChartTmpl = module.Chart{
		ID:       "http_upstream_%s_server_%s_zone_%s_downtime",
		Title:    "HTTP Upstream Server downtime",
		Units:    "seconds",
		Fam:      "http upstream state",
		Ctx:      "nginxplus.http_upstream_server_downtime",
		Priority: prioHTTPUpstreamServerDowntime,
		Dims: module.Dims{
			{ID: "http_upstream_%s_server_%s_zone_%s_downtime", Name: "downtime"},
		},
	}
)

var (
	httpCacheChartsTmpl = module.Charts{
		httpCacheStateChartTmpl.Copy(),
		httpCacheIOPSChartTmpl.Copy(),
		httpCacheIOChartTmpl.Copy(),
		httpCacheSizeChartTmpl.Copy(),
	}
	httpCacheStateChartTmpl = module.Chart{
		ID:       "http_cache_%s_state",
		Title:    "HTTP Cache state",
		Units:    "state",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_state",
		Priority: prioHTTPCacheState,
		Dims: module.Dims{
			{ID: "http_cache_%s_state_warm", Name: "warm"},
			{ID: "http_cache_%s_state_cold", Name: "cold"},
		},
	}
	httpCacheSizeChartTmpl = module.Chart{
		ID:       "http_cache_%s_size",
		Title:    "HTTP Cache size",
		Units:    "bytes",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_size",
		Priority: prioHTTPCacheSize,
		Dims: module.Dims{
			{ID: "http_cache_%s_size", Name: "size"},
		},
	}
	httpCacheIOPSChartTmpl = module.Chart{
		ID:       "http_cache_%s_iops",
		Title:    "HTTP Cache IOPS",
		Units:    "responses/s",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_iops",
		Priority: prioHTTPCacheIOPS,
		Dims: module.Dims{
			{ID: "http_cache_%s_served_responses", Name: "served", Algo: module.Incremental},
			{ID: "http_cache_%s_written_responses", Name: "written", Algo: module.Incremental},
			{ID: "http_cache_%s_bypassed_responses", Name: "bypassed", Algo: module.Incremental},
		},
	}
	httpCacheIOChartTmpl = module.Chart{
		ID:       "http_cache_%s_io",
		Title:    "HTTP Cache IO",
		Units:    "bytes/s",
		Fam:      "http cache",
		Ctx:      "nginxplus.http_cache_io",
		Priority: prioHTTPCacheIO,
		Dims: module.Dims{
			{ID: "http_cache_%s_served_bytes", Name: "served", Algo: module.Incremental},
			{ID: "http_cache_%s_written_bytes", Name: "written", Algo: module.Incremental},
			{ID: "http_cache_%s_bypassed_bytes", Name: "bypassed", Algo: module.Incremental},
		},
	}
)

var (
	streamServerZoneChartsTmpl = module.Charts{
		streamServerZoneConnectionsRateChartTmpl.Copy(),
		streamServerZoneTrafficRateChartTmpl.Copy(),
		streamServerZoneSessionsPerCodeClassRateChartTmpl.Copy(),
		streamServerZoneConnectionsProcessingCountRateChartTmpl.Copy(),
		streamServerZoneConnectionsDiscardedRateChartTmpl.Copy(),
	}
	streamServerZoneConnectionsRateChartTmpl = module.Chart{
		ID:       "stream_server_zone_%s_connections_rate",
		Title:    "Stream Server Zone connections rate",
		Units:    "connections/s",
		Fam:      "stream connections",
		Ctx:      "nginxplus.stream_server_zone_connections_rate",
		Priority: prioStreamServerZoneConnectionsRate,
		Dims: module.Dims{
			{ID: "stream_server_zone_%s_connections", Name: "accepted", Algo: module.Incremental},
		},
	}
	streamServerZoneSessionsPerCodeClassRateChartTmpl = module.Chart{
		ID:       "stream_server_zone_%s_sessions_per_code_class_rate",
		Title:    "Stream Server Zone sessions rate",
		Units:    "sessions/s",
		Fam:      "stream sessions",
		Ctx:      "nginxplus.stream_server_zone_sessions_per_code_class_rate",
		Priority: prioStreamServerZoneSessionsPerCodeClassRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "stream_server_zone_%s_sessions_2xx", Name: "2xx", Algo: module.Incremental},
			{ID: "stream_server_zone_%s_sessions_4xx", Name: "4xx", Algo: module.Incremental},
			{ID: "stream_server_zone_%s_sessions_5xx", Name: "5xx", Algo: module.Incremental},
		},
	}
	streamServerZoneTrafficRateChartTmpl = module.Chart{
		ID:       "stream_server_zone_%s_traffic_rate",
		Title:    "Stream Server Zone traffic rate",
		Units:    "bytes/s",
		Fam:      "stream traffic",
		Ctx:      "nginxplus.stream_server_zone_traffic_rate",
		Priority: prioStreamServerZoneTrafficRate,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "stream_server_zone_%s_bytes_received", Name: "received", Algo: module.Incremental},
			{ID: "stream_server_zone_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	streamServerZoneConnectionsProcessingCountRateChartTmpl = module.Chart{
		ID:       "stream_server_zone_%s_connections_processing_count",
		Title:    "Stream Server Zone connections processed",
		Units:    "connections",
		Fam:      "stream connections",
		Ctx:      "nginxplus.stream_server_zone_connections_processing_count",
		Priority: prioStreamServerZoneConnectionsProcessingCount,
		Dims: module.Dims{
			{ID: "stream_server_zone_%s_connections_processing", Name: "processing"},
		},
	}
	streamServerZoneConnectionsDiscardedRateChartTmpl = module.Chart{
		ID:       "stream_server_zone_%s_connections_discarded_rate",
		Title:    "Stream Server Zone connections discarded",
		Units:    "connections/s",
		Fam:      "stream connections",
		Ctx:      "nginxplus.stream_server_zone_connections_discarded_rate",
		Priority: prioStreamServerZoneConnectionsDiscardedRate,
		Dims: module.Dims{
			{ID: "stream_server_zone_%s_connections_discarded", Name: "discarded", Algo: module.Incremental},
		},
	}
)

var (
	streamUpstreamChartsTmpl = module.Charts{
		streamUpstreamPeersCountChartTmpl.Copy(),
		streamUpstreamZombiesCountChartTmpl.Copy(),
	}
	streamUpstreamPeersCountChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_zone_%s_peers_count",
		Title:    "Stream Upstream peers",
		Units:    "peers",
		Fam:      "stream upstream",
		Ctx:      "nginxplus.stream_upstream_peers_count",
		Priority: prioStreamUpstreamPeersCount,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_zone_%s_peers", Name: "peers"},
		},
	}
	streamUpstreamZombiesCountChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_zone_%s_zombies_count",
		Title:    "Stream Upstream zombies",
		Units:    "servers",
		Fam:      "stream upstream",
		Ctx:      "nginxplus.stream_upstream_zombies_count",
		Priority: prioStreamUpstreamZombiesCount,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_zone_%s_zombies", Name: "zombie"},
		},
	}

	streamUpstreamServerChartsTmpl = module.Charts{
		streamUpstreamServerConnectionsRateChartTmpl.Copy(),
		streamUpstreamServerTrafficRateChartTmpl.Copy(),
		streamUpstreamServerConnectionsCountChartTmpl.Copy(),
		streamUpstreamServerStateChartTmpl.Copy(),
		streamUpstreamServerDowntimeChartTmpl.Copy(),
	}
	streamUpstreamServerConnectionsRateChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_connection_rate",
		Title:    "Stream Upstream Server connections",
		Units:    "connections/s",
		Fam:      "stream upstream connections",
		Ctx:      "nginxplus.stream_upstream_server_connections_rate",
		Priority: prioStreamUpstreamServerConnectionsRate,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_connections", Name: "forwarded", Algo: module.Incremental},
		},
	}
	streamUpstreamServerTrafficRateChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_traffic_rate",
		Title:    "Stream Upstream Server traffic rate",
		Units:    "bytes/s",
		Fam:      "stream upstream traffic",
		Ctx:      "nginxplus.stream_upstream_server_traffic_rate",
		Priority: prioStreamUpstreamServerTrafficRate,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_bytes_received", Name: "received", Algo: module.Incremental},
			{ID: "stream_upstream_%s_server_%s_zone_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	streamUpstreamServerStateChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_state",
		Title:    "Stream Upstream Server state",
		Units:    "state",
		Fam:      "stream upstream state",
		Ctx:      "nginxplus.stream_upstream_server_state",
		Priority: prioStreamUpstreamServerState,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_up", Name: "up"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_down", Name: "down"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_unavail", Name: "unavail"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_checking", Name: "checking"},
			{ID: "stream_upstream_%s_server_%s_zone_%s_state_unhealthy", Name: "unhealthy"},
		},
	}
	streamUpstreamServerDowntimeChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_downtime",
		Title:    "Stream Upstream Server downtime",
		Units:    "seconds",
		Fam:      "stream upstream state",
		Ctx:      "nginxplus.stream_upstream_server_downtime",
		Priority: prioStreamUpstreamServerDowntime,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_downtime", Name: "downtime"},
		},
	}
	streamUpstreamServerConnectionsCountChartTmpl = module.Chart{
		ID:       "stream_upstream_%s_server_%s_zone_%s_connection_count",
		Title:    "Stream Upstream Server connections",
		Units:    "connections",
		Fam:      "stream upstream connections",
		Ctx:      "nginxplus.stream_upstream_server_connections_count",
		Priority: prioStreamUpstreamServerConnectionsCount,
		Dims: module.Dims{
			{ID: "stream_upstream_%s_server_%s_zone_%s_active", Name: "active"},
		},
	}
)

var (
	resolverZoneChartsTmpl = module.Charts{
		resolverZoneRequestsRateChartTmpl.Copy(),
		resolverZoneResponsesRateChartTmpl.Copy(),
	}
	resolverZoneRequestsRateChartTmpl = module.Chart{
		ID:       "resolver_zone_%s_requests_rate",
		Title:    "Resolver requests rate",
		Units:    "requests/s",
		Fam:      "resolver requests",
		Ctx:      "nginxplus.resolver_zone_requests_rate",
		Priority: prioResolverZoneRequestsRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "resolver_zone_%s_requests_name", Name: "name", Algo: module.Incremental},
			{ID: "resolver_zone_%s_requests_srv", Name: "srv", Algo: module.Incremental},
			{ID: "resolver_zone_%s_requests_addr", Name: "addr", Algo: module.Incremental},
		},
	}
	resolverZoneResponsesRateChartTmpl = module.Chart{
		ID:       "resolver_zone_%s_responses_rate",
		Title:    "Resolver responses rate",
		Units:    "responses/s",
		Fam:      "resolver responses",
		Ctx:      "nginxplus.resolver_zone_responses_rate",
		Priority: prioResolverZoneResponsesRate,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "resolver_zone_%s_responses_noerror", Name: "noerror", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_formerr", Name: "formerr", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_servfail", Name: "servfail", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_nxdomain", Name: "nxdomain", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_notimp", Name: "notimp", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_refused", Name: "refused", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_timedout", Name: "timedout", Algo: module.Incremental},
			{ID: "resolver_zone_%s_responses_unknown", Name: "unknown", Algo: module.Incremental},
		},
	}
)

func (c *Collector) addHTTPCacheCharts(name string) {
	charts := httpCacheChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
