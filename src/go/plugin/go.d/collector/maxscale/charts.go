// SPDX-License-Identifier: GPL-3.0-or-later

package maxscale

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioPollEvents = module.Priority + iota

	prioSessions
	prioZombies

	prioServerState
	prioServerConnections

	prioThreadsByState

	prioCurrentFDs

	prioQCCacheEfficiency
	prioQCCacheOperations

	prioUptime
)

var charts = module.Charts{
	pollEventsChart.Copy(),
	currentSessionsChart.Copy(),
	currentZombieConnectionsChart.Copy(),
	threadsByStateChart.Copy(),
	currentFDsChart.Copy(),
	qcCacheEfficiencyChart.Copy(),
	qcCacheOperationsChart.Copy(),
	uptimeChart.Copy(),
}

var (
	pollEventsChart = module.Chart{
		ID:       "poll_events",
		Title:    "Poll Events",
		Units:    "events/s",
		Fam:      "poll events",
		Ctx:      "maxscale.poll_events",
		Priority: prioPollEvents,
		Dims: module.Dims{
			{ID: "threads_reads", Name: "reads", Algo: module.Incremental},
			{ID: "threads_writes", Name: "writes", Algo: module.Incremental},
			{ID: "threads_accepts", Name: "accepts", Algo: module.Incremental},
			{ID: "threads_errors", Name: "errors", Algo: module.Incremental},
			{ID: "threads_hangups", Name: "hangups", Algo: module.Incremental},
		},
	}

	currentSessionsChart = module.Chart{
		ID:       "current_sessions",
		Title:    "Curren Sessions",
		Units:    "sessions",
		Fam:      "sessions",
		Ctx:      "maxscale.current_sessions",
		Priority: prioSessions,
		Dims: module.Dims{
			{ID: "threads_sessions", Name: "sessions"},
		},
	}
	currentZombieConnectionsChart = module.Chart{
		ID:       "current_zombie_connections",
		Title:    "Current Zombie Connections",
		Units:    "connections",
		Fam:      "sessions",
		Ctx:      "maxscale.current_zombie_connections",
		Priority: prioZombies,
		Dims: module.Dims{
			{ID: "threads_zombies", Name: "zombie"},
		},
	}

	threadsByStateChart = func() module.Chart {
		chart := module.Chart{
			ID:       "threads_by_state",
			Title:    "Threads Count by State",
			Units:    "threads",
			Fam:      "threads",
			Ctx:      "maxscale.threads_by_state",
			Priority: prioThreadsByState,
			Type:     module.Stacked,
		}
		for _, v := range threadStates {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "threads_state_" + v,
				Name: strings.ToLower(v),
			})
		}
		return chart
	}()

	currentFDsChart = module.Chart{
		ID:       "current_file_descriptors",
		Title:    "Current Managed File Descriptors",
		Units:    "fds",
		Fam:      "fds",
		Ctx:      "maxscale.current_fds",
		Priority: prioCurrentFDs,
		Dims: module.Dims{
			{ID: "threads_current_fds", Name: "managed"},
		},
	}

	qcCacheEfficiencyChart = module.Chart{
		ID:       "qc_cache_efficiency",
		Title:    "QC Cache Efficiency",
		Units:    "requests/s",
		Fam:      "qc cache",
		Ctx:      "maxscale.qc_cache_efficiency",
		Priority: prioQCCacheEfficiency,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "threads_qc_cache_hits", Name: "hits", Algo: module.Incremental},
			{ID: "threads_qc_cache_misses", Name: "misses", Algo: module.Incremental},
		},
	}
	qcCacheOperationsChart = module.Chart{
		ID:       "qc_cache_operations",
		Title:    "QC Cache Operations",
		Units:    "operations/s",
		Fam:      "qc cache",
		Ctx:      "maxscale.qc_cache_operations",
		Priority: prioQCCacheOperations,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "threads_qc_cache_inserts", Name: "inserts", Algo: module.Incremental},
			{ID: "threads_qc_cache_evictions", Name: "evictions", Algo: module.Incremental},
		},
	}

	uptimeChart = module.Chart{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "maxscale.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "uptime"},
		},
	}
)

var serverChartsTmpl = module.Charts{
	serverStateChartTmpl.Copy(),
	serverCurrentConnectionsChartTmpl.Copy(),
}

var (
	serverStateChartTmpl = func() module.Chart {
		chart := module.Chart{
			ID:       "server_%s_state",
			Title:    "Server State",
			Units:    "state",
			Fam:      "servers",
			Ctx:      "maxscale.server_state",
			Priority: prioServerState,
		}
		for _, v := range serverStates {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   "server_%s_state_" + v,
				Name: strings.ToLower(cleanChartID(v)),
			})
		}
		return chart
	}()
	serverCurrentConnectionsChartTmpl = module.Chart{
		ID:       "server_%s_current_connections",
		Title:    "Server Current Connections",
		Units:    "connections",
		Fam:      "servers",
		Ctx:      "maxscale.server_current_connections",
		Priority: prioServerConnections,
		Dims: module.Dims{
			{ID: "server_%s_connections", Name: "connections"},
		},
	}
)

func (c *Collector) addServerCharts(id, addr string) {
	srvCharts := serverChartsTmpl.Copy()

	for _, chart := range *srvCharts {
		chart.ID = fmt.Sprintf(chart.ID, id)
		chart.ID = cleanChartID(chart.ID)
		chart.Labels = []module.Label{
			{Key: "server", Value: id},
			{Key: "address", Value: addr},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, id)
		}
	}

	if err := c.Charts().Add(*srvCharts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeServerCharts(id string) {
	px := fmt.Sprintf("server_%s_", id)
	px = cleanChartID(px)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanChartID(id string) string {
	r := strings.NewReplacer(".", "_", " ", "_")
	return r.Replace(id)
}
