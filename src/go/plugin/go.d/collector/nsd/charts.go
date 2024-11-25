// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioQueries = module.Priority + iota
	prioQueriesByType
	prioQueriesByOpcode
	prioQueriesByClass
	prioQueriesByProtocol

	prioAnswersByRcode

	prioErrors

	prioDrops

	prioZones
	prioZoneTransfersRequests
	prioZoneTransferMemory

	prioDatabaseSize

	prioUptime
)

var charts = module.Charts{
	queriesChart.Copy(),
	queriesByTypeChart.Copy(),
	queriesByOpcodeChart.Copy(),
	queriesByClassChart.Copy(),
	queriesByProtocolChart.Copy(),

	answersByRcodeChart.Copy(),

	zonesChart.Copy(),
	zoneTransfersRequestsChart.Copy(),
	zoneTransferMemoryChart.Copy(),

	databaseSizeChart.Copy(),

	errorsChart.Copy(),

	dropsChart.Copy(),

	uptimeChart.Copy(),
}

var (
	queriesChart = module.Chart{
		ID:       "queries",
		Title:    "Queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "nsd.queries",
		Priority: prioQueries,
		Dims: module.Dims{
			{ID: "num.queries", Name: "queries", Algo: module.Incremental},
		},
	}
	queriesByTypeChart = func() module.Chart {
		chart := module.Chart{
			ID:       "queries_by_type",
			Title:    "Queries Type",
			Units:    "queries/s",
			Fam:      "queries",
			Ctx:      "nsd.queries_by_type",
			Priority: prioQueriesByType,
			Type:     module.Stacked,
		}
		for _, v := range queryTypes {
			name := v
			if s, ok := queryTypeNumberMap[v]; ok {
				name = s
			}
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "num.type." + v,
				Name: name,
				Algo: module.Incremental,
			})
		}
		return chart
	}()
	queriesByOpcodeChart = func() module.Chart {
		chart := module.Chart{
			ID:       "queries_by_opcode",
			Title:    "Queries Opcode",
			Units:    "queries/s",
			Fam:      "queries",
			Ctx:      "nsd.queries_by_opcode",
			Priority: prioQueriesByOpcode,
			Type:     module.Stacked,
		}
		for _, v := range queryOpcodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "num.opcode." + v,
				Name: v,
				Algo: module.Incremental,
			})
		}
		return chart
	}()
	queriesByClassChart = func() module.Chart {
		chart := module.Chart{
			ID:       "queries_by_class",
			Title:    "Queries Class",
			Units:    "queries/s",
			Fam:      "queries",
			Ctx:      "nsd.queries_by_class",
			Priority: prioQueriesByClass,
			Type:     module.Stacked,
		}
		for _, v := range queryClasses {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "num.class." + v,
				Name: v,
				Algo: module.Incremental,
			})
		}
		return chart
	}()
	queriesByProtocolChart = module.Chart{
		ID:       "queries_by_protocol",
		Title:    "Queries Protocol",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "nsd.queries_by_protocol",
		Priority: prioQueriesByProtocol,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "num.udp", Name: "udp", Algo: module.Incremental},
			{ID: "num.udp6", Name: "udp6", Algo: module.Incremental},
			{ID: "num.tcp", Name: "tcp", Algo: module.Incremental},
			{ID: "num.tcp6", Name: "tcp6", Algo: module.Incremental},
			{ID: "num.tls", Name: "tls", Algo: module.Incremental},
			{ID: "num.tls6", Name: "tls6", Algo: module.Incremental},
		},
	}

	answersByRcodeChart = func() module.Chart {
		chart := module.Chart{
			ID:       "answers_by_rcode",
			Title:    "Answers Rcode",
			Units:    "answers/s",
			Fam:      "answers",
			Ctx:      "nsd.answers_by_rcode",
			Priority: prioAnswersByRcode,
			Type:     module.Stacked,
		}
		for _, v := range answerRcodes {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "num.rcode." + v,
				Name: v,
				Algo: module.Incremental,
			})
		}
		return chart
	}()

	errorsChart = module.Chart{
		ID:       "errors",
		Title:    "Errors",
		Units:    "errors/s",
		Fam:      "errors",
		Ctx:      "nsd.errors",
		Priority: prioErrors,
		Dims: module.Dims{
			{ID: "num.rxerr", Name: "query", Algo: module.Incremental},
			{ID: "num.txerr", Name: "answer", Mul: -1, Algo: module.Incremental},
		},
	}

	dropsChart = module.Chart{
		ID:       "drops",
		Title:    "Drops",
		Units:    "drops/s",
		Fam:      "drops",
		Ctx:      "nsd.drops",
		Priority: prioDrops,
		Dims: module.Dims{
			{ID: "num.dropped", Name: "query", Algo: module.Incremental},
		},
	}

	zonesChart = module.Chart{
		ID:       "zones",
		Title:    "Zones",
		Units:    "zones",
		Fam:      "zones",
		Ctx:      "nsd.zones",
		Priority: prioZones,
		Dims: module.Dims{
			{ID: "zone.master", Name: "master"},
			{ID: "zone.slave", Name: "slave"},
		},
	}
	zoneTransfersRequestsChart = module.Chart{
		ID:       "zone_transfers_requests",
		Title:    "Zone Transfers",
		Units:    "requests/s",
		Fam:      "zones",
		Ctx:      "nsd.zone_transfers_requests",
		Priority: prioZoneTransfersRequests,
		Dims: module.Dims{
			{ID: "num.raxfr", Name: "AXFR", Algo: module.Incremental},
			{ID: "num.rixfr", Name: "IXFR", Algo: module.Incremental},
		},
	}
	zoneTransferMemoryChart = module.Chart{
		ID:       "zone_transfer_memory",
		Title:    "Zone Transfer Memory",
		Units:    "bytes",
		Fam:      "zones",
		Ctx:      "nsd.zone_transfer_memory",
		Priority: prioZoneTransferMemory,
		Dims: module.Dims{
			{ID: "size.xfrd.mem", Name: "used"},
		},
	}

	databaseSizeChart = module.Chart{
		ID:       "database_size",
		Title:    "Database Size",
		Units:    "bytes",
		Fam:      "database",
		Ctx:      "nsd.database_size",
		Priority: prioDatabaseSize,
		Dims: module.Dims{
			{ID: "size.db.disk", Name: "disk"},
			{ID: "size.db.mem", Name: "mem"},
		},
	}

	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "nsd.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "time.boot", Name: "uptime"},
		},
	}
)
