// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

const (
	prioRequests = module.Priority + iota
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

var baseCharts = module.Charts{
	chartConnections.Copy(),
	chartConnsAsync.Copy(),
	chartWorkers.Copy(),
	chartScoreboard.Copy(),
}

var extendedCharts = module.Charts{
	chartRequests.Copy(),
	chartBandwidth.Copy(),
	chartReqPerSec.Copy(),
	chartBytesPerSec.Copy(),
	chartBytesPerReq.Copy(),
	chartUptime.Copy(),
}

func newCharts(s *serverStatus) *module.Charts {
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
	chartConnections = module.Chart{
		ID:       "connections",
		Title:    "Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "apache.connections",
		Priority: prioConnection,
		Dims: module.Dims{
			{ID: "conns_total", Name: "connections"},
		},
	}
	chartConnsAsync = module.Chart{
		ID:       "conns_async",
		Title:    "Async Connections",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "apache.conns_async",
		Type:     module.Stacked,
		Priority: prioConnsAsync,
		Dims: module.Dims{
			{ID: "conns_async_keep_alive", Name: "keepalive"},
			{ID: "conns_async_closing", Name: "closing"},
			{ID: "conns_async_writing", Name: "writing"},
		},
	}
	chartWorkers = module.Chart{
		ID:       "workers",
		Title:    "Workers Threads",
		Units:    "workers",
		Fam:      "workers",
		Ctx:      "apache.workers",
		Type:     module.Stacked,
		Priority: prioWorkers,
		Dims: module.Dims{
			{ID: "idle_workers", Name: "idle"},
			{ID: "busy_workers", Name: "busy"},
		},
	}
	chartScoreboard = module.Chart{
		ID:       "scoreboard",
		Title:    "Scoreboard",
		Units:    "connections",
		Fam:      "connections",
		Ctx:      "apache.scoreboard",
		Priority: prioScoreboard,
		Dims: module.Dims{
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
	chartRequests = module.Chart{
		ID:       "requests",
		Title:    "Requests",
		Units:    "requests/s",
		Fam:      "requests",
		Ctx:      "apache.requests",
		Priority: prioRequests,
		Dims: module.Dims{
			{ID: "total_accesses", Name: "requests", Algo: module.Incremental},
		},
	}
	chartBandwidth = module.Chart{
		ID:       "net",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "bandwidth",
		Ctx:      "apache.net",
		Type:     module.Area,
		Priority: prioNet,
		Dims: module.Dims{
			{ID: "total_kBytes", Name: "sent", Algo: module.Incremental, Mul: 8},
		},
	}
	chartReqPerSec = module.Chart{
		ID:       "reqpersec",
		Title:    "Lifetime Average Number Of Requests Per Second",
		Units:    "requests/s",
		Fam:      "statistics",
		Ctx:      "apache.reqpersec",
		Type:     module.Area,
		Priority: prioReqPerSec,
		Dims: module.Dims{
			{ID: "req_per_sec", Name: "requests", Div: 100000},
		},
	}
	chartBytesPerSec = module.Chart{
		ID:       "bytespersec",
		Title:    "Lifetime Average Number Of Bytes Served Per Second",
		Units:    "KiB/s",
		Fam:      "statistics",
		Ctx:      "apache.bytespersec",
		Type:     module.Area,
		Priority: prioBytesPerSec,
		Dims: module.Dims{
			{ID: "bytes_per_sec", Name: "served", Mul: 8, Div: 1024 * 100000},
		},
	}
	chartBytesPerReq = module.Chart{
		ID:       "bytesperreq",
		Title:    "Lifetime Average Response Size",
		Units:    "KiB",
		Fam:      "statistics",
		Ctx:      "apache.bytesperreq",
		Type:     module.Area,
		Priority: prioBytesPerReq,
		Dims: module.Dims{
			{ID: "bytes_per_req", Name: "size", Div: 1024 * 100000},
		},
	}
	chartUptime = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "availability",
		Ctx:      "apache.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}
)
