// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package nsd

import (
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioQueries = collectorapi.Priority + iota
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

var charts = collectorapi.Charts{
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
	queriesChart = collectorapi.Chart{
		ID:       "queries",
		Title:    "Queries",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "nsd.queries",
		Priority: prioQueries,
		Dims: collectorapi.Dims{
			{ID: "num.queries", Name: "queries", Algo: collectorapi.Incremental},
		},
	}
	queriesByTypeChart = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "queries_by_type",
			Title:    "Queries Type",
			Units:    "queries/s",
			Fam:      "queries",
			Ctx:      "nsd.queries_by_type",
			Priority: prioQueriesByType,
			Type:     collectorapi.Stacked,
		}
		for _, v := range queryTypes {
			name := v
			if s, ok := queryTypeNumberMap[v]; ok {
				name = s
			}
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID:   "num.type." + v,
				Name: name,
				Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	queriesByOpcodeChart = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "queries_by_opcode",
			Title:    "Queries Opcode",
			Units:    "queries/s",
			Fam:      "queries",
			Ctx:      "nsd.queries_by_opcode",
			Priority: prioQueriesByOpcode,
			Type:     collectorapi.Stacked,
		}
		for _, v := range queryOpcodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID:   "num.opcode." + v,
				Name: v,
				Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	queriesByClassChart = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "queries_by_class",
			Title:    "Queries Class",
			Units:    "queries/s",
			Fam:      "queries",
			Ctx:      "nsd.queries_by_class",
			Priority: prioQueriesByClass,
			Type:     collectorapi.Stacked,
		}
		for _, v := range queryClasses {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID:   "num.class." + v,
				Name: v,
				Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()
	queriesByProtocolChart = collectorapi.Chart{
		ID:       "queries_by_protocol",
		Title:    "Queries Protocol",
		Units:    "queries/s",
		Fam:      "queries",
		Ctx:      "nsd.queries_by_protocol",
		Priority: prioQueriesByProtocol,
		Type:     collectorapi.Stacked,
		Dims: collectorapi.Dims{
			{ID: "num.udp", Name: "udp", Algo: collectorapi.Incremental},
			{ID: "num.udp6", Name: "udp6", Algo: collectorapi.Incremental},
			{ID: "num.tcp", Name: "tcp", Algo: collectorapi.Incremental},
			{ID: "num.tcp6", Name: "tcp6", Algo: collectorapi.Incremental},
			{ID: "num.tls", Name: "tls", Algo: collectorapi.Incremental},
			{ID: "num.tls6", Name: "tls6", Algo: collectorapi.Incremental},
		},
	}

	answersByRcodeChart = func() collectorapi.Chart {
		chart := collectorapi.Chart{
			ID:       "answers_by_rcode",
			Title:    "Answers Rcode",
			Units:    "answers/s",
			Fam:      "answers",
			Ctx:      "nsd.answers_by_rcode",
			Priority: prioAnswersByRcode,
			Type:     collectorapi.Stacked,
		}
		for _, v := range answerRcodes {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID:   "num.rcode." + v,
				Name: v,
				Algo: collectorapi.Incremental,
			})
		}
		return chart
	}()

	errorsChart = collectorapi.Chart{
		ID:       "errors",
		Title:    "Errors",
		Units:    "errors/s",
		Fam:      "errors",
		Ctx:      "nsd.errors",
		Priority: prioErrors,
		Dims: collectorapi.Dims{
			{ID: "num.rxerr", Name: "query", Algo: collectorapi.Incremental},
			{ID: "num.txerr", Name: "answer", Mul: -1, Algo: collectorapi.Incremental},
		},
	}

	dropsChart = collectorapi.Chart{
		ID:       "drops",
		Title:    "Drops",
		Units:    "drops/s",
		Fam:      "drops",
		Ctx:      "nsd.drops",
		Priority: prioDrops,
		Dims: collectorapi.Dims{
			{ID: "num.dropped", Name: "query", Algo: collectorapi.Incremental},
		},
	}

	zonesChart = collectorapi.Chart{
		ID:       "zones",
		Title:    "Zones",
		Units:    "zones",
		Fam:      "zones",
		Ctx:      "nsd.zones",
		Priority: prioZones,
		Dims: collectorapi.Dims{
			{ID: "zone.master", Name: "master"},
			{ID: "zone.slave", Name: "slave"},
		},
	}
	zoneTransfersRequestsChart = collectorapi.Chart{
		ID:       "zone_transfers_requests",
		Title:    "Zone Transfers",
		Units:    "requests/s",
		Fam:      "zones",
		Ctx:      "nsd.zone_transfers_requests",
		Priority: prioZoneTransfersRequests,
		Dims: collectorapi.Dims{
			{ID: "num.raxfr", Name: "AXFR", Algo: collectorapi.Incremental},
			{ID: "num.rixfr", Name: "IXFR", Algo: collectorapi.Incremental},
		},
	}
	zoneTransferMemoryChart = collectorapi.Chart{
		ID:       "zone_transfer_memory",
		Title:    "Zone Transfer Memory",
		Units:    "bytes",
		Fam:      "zones",
		Ctx:      "nsd.zone_transfer_memory",
		Priority: prioZoneTransferMemory,
		Dims: collectorapi.Dims{
			{ID: "size.xfrd.mem", Name: "used"},
		},
	}

	databaseSizeChart = collectorapi.Chart{
		ID:       "database_size",
		Title:    "Database Size",
		Units:    "bytes",
		Fam:      "database",
		Ctx:      "nsd.database_size",
		Priority: prioDatabaseSize,
		Dims: collectorapi.Dims{
			{ID: "size.db.disk", Name: "disk"},
			{ID: "size.db.mem", Name: "mem"},
		},
	}

	uptimeChart = collectorapi.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "nsd.uptime",
		Priority: prioUptime,
		Dims: collectorapi.Dims{
			{ID: "time.boot", Name: "uptime"},
		},
	}
)
