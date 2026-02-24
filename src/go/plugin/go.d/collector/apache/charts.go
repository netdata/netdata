// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

const (
	prioRequests = collectorapi.Priority + iota
	prioConnection
	prioConnsAsync
	prioScoreboard
	prioNet
	prioWorkers
	prioReqPerSec
	prioBytesPerSec
	prioBytesPerReq
	prioUptime
)

var baseCharts = collectorapi.Charts{
	chartConnections.Copy(),
	chartConnsAsync.Copy(),
	chartWorkers.Copy(),
	chartScoreboard.Copy(),
}

var extendedCharts = collectorapi.Charts{
	chartRequests.Copy(),
	chartBandwidth.Copy(),
	chartReqPerSec.Copy(),
	chartBytesPerSec.Copy(),
	chartBytesPerReq.Copy(),
	chartUptime.Copy(),
}

func newCharts(s *serverStatus) *collectorapi.Charts {
	charts := baseCharts.Copy()

	// ServerMPM: prefork
	if s.Connections.Total == nil {
		_ = charts.Remove(chartConnections.ID)
	}
	if s.Connections.Async.KeepAlive == nil {
		_ = charts.Remove(chartConnsAsync.ID)
	}

	if s.Total.Accesses != nil {
		_ = charts.Add(*extendedCharts.Copy()...)
	}

	return charts
}

// simple status
var (
	chartConnections = collectorapi.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "apache.connections",
		Priority: prioConnection,
		Dims: collectorapi.Dims{
			{ID: "conns_total", Name: "connections"},
		},
	}
	chartConnsAsync = collectorapi.Chart{
		ID:       "conns_async",
		Title:    "Async Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "apache.conns_async",
		Type:     collectorapi.Stacked,
		Priority: prioConnsAsync,
		Dims: collectorapi.Dims{
			{ID: "conns_async_keep_alive", Name: "keepalive"},
			{ID: "conns_async_closing", Name: "closing"},
			{ID: "conns_async_writing", Name: "writing"},
		},
	}
	chartWorkers = collectorapi.Chart{
		ID:       "workers",
		Title:    "Workers Threads",
		Units:    "workers",
		Fam:      "workers",
		Ctx:      "apache.workers",
		Type:     collectorapi.Stacked,
		Priority: prioWorkers,
		Dims: collectorapi.Dims{
			{ID: "idle_workers", Name: "idle"},
			{ID: "busy_workers", Name: "busy"},
		},
	}
	chartScoreboard = collectorapi.Chart{
		ID:       "scoreboard",
		Title:    "Scoreboard",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "apache.scoreboard",
		Priority: prioScoreboard,
		Dims: collectorapi.Dims{
			{ID: "scoreboard_waiting", Name: "waiting"},
			{ID: "scoreboard_starting", Name: "starting"},
			{ID: "scoreboard_reading", Name: "reading"},
			{ID: "scoreboard_sending", Name: "sending"},
			{ID: "scoreboard_keepalive", Name: "keepalive"},
			{ID: "scoreboard_dns_lookup", Name: "dns_lookup"},
			{ID: "scoreboard_closing", Name: "closing"},
			{ID: "scoreboard_logging", Name: "logging"},
			{ID: "scoreboard_finishing", Name: "finishing"},
			{ID: "scoreboard_idle_cleanup", Name: "idle_cleanup"},
			{ID: "scoreboard_open", Name: "open"},
		},
	}
)

// extended status
var (
	chartRequests = collectorapi.Chart{
		ID:       "requests",
		Title:    "Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "apache.requests",
		Priority: prioRequests,
		Dims: collectorapi.Dims{
			{ID: "total_accesses", Name: "requests", Algo: collectorapi.Incremental},
		},
	}
	chartBandwidth = collectorapi.Chart{
		ID:       "net",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "bandwidth",
		Ctx:      "apache.net",
		Type:     collectorapi.Area,
		Priority: prioNet,
		Dims: collectorapi.Dims{
			{ID: "total_kBytes", Name: "sent", Algo: collectorapi.Incremental, Mul: 8},
		},
	}
	chartReqPerSec = collectorapi.Chart{
		ID:       "reqpersec",
		Title:    "Lifetime Average Number Of Requests Per Second",
		Units:    "requests/s",
		Fam:      "statistics",
		Ctx:      "apache.reqpersec",
		Type:     collectorapi.Area,
		Priority: prioReqPerSec,
		Dims: collectorapi.Dims{
			{ID: "req_per_sec", Name: "requests", Div: 100000},
		},
	}
	chartBytesPerSec = collectorapi.Chart{
		ID:       "bytespersec",
		Title:    "Lifetime Average Number Of Bytes Served Per Second",
		Units:    "KiB/s",
		Fam:      "statistics",
		Ctx:      "apache.bytespersec",
		Type:     collectorapi.Area,
		Priority: prioBytesPerSec,
		Dims: collectorapi.Dims{
			{ID: "bytes_per_sec", Name: "served", Mul: 8, Div: 1024 * 100000},
		},
	}
	chartBytesPerReq = collectorapi.Chart{
		ID:       "bytesperreq",
		Title:    "Lifetime Average Response Size",
		Units:    "KiB",
		Fam:      "statistics",
		Ctx:      "apache.bytesperreq",
		Type:     collectorapi.Area,
		Priority: prioBytesPerReq,
		Dims: collectorapi.Dims{
			{ID: "bytes_per_req", Name: "size", Div: 1024 * 100000},
		},
	}
	chartUptime = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "availability",
		Ctx:      "apache.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "uptime"},
		},
	}
)
