// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Chart templates for per-instance metrics
var (
	// Database charts
	databaseStatusChartTmpl = module.Chart{
		ID:       "database_%s_status",
		Title:    "Database %s Status",
		Units:    "status",
		Fam:      "database",
		Ctx:      "db2.database_status",
		Priority: module.Priority + 100,
		Dims: module.Dims{
			{ID: "database_%s_status", Name: "status"},
		},
	}

	databaseApplicationsChartTmpl = module.Chart{
		ID:       "database_%s_applications",
		Title:    "Database %s Applications",
		Units:    "applications",
		Fam:      "database",
		Ctx:      "db2.database_applications",
		Priority: module.Priority + 101,
		Dims: module.Dims{
			{ID: "database_%s_applications", Name: "applications"},
		},
	}

	// Bufferpool charts
	bufferpoolHitRatioChartTmpl = module.Chart{
		ID:       "bufferpool_%s_hit_ratio",
		Title:    "Buffer Pool %s Hit Ratio",
		Units:    "percentage",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_hit_ratio",
		Priority: module.Priority + 200,
		Dims: module.Dims{
			{ID: "bufferpool_%s_hit_ratio", Name: "hit_ratio", Div: precision},
		},
	}

	bufferpoolIOChartTmpl = module.Chart{
		ID:       "bufferpool_%s_io",
		Title:    "Buffer Pool %s I/O",
		Units:    "operations/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_io",
		Priority: module.Priority + 201,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "bufferpool_%s_reads", Name: "reads", Algo: module.Incremental},
			{ID: "bufferpool_%s_writes", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}

	bufferpoolPagesChartTmpl = module.Chart{
		ID:       "bufferpool_%s_pages",
		Title:    "Buffer Pool %s Pages",
		Units:    "pages",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_pages",
		Priority: module.Priority + 202,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_used_pages", Name: "used"},
			{ID: "bufferpool_%s_total_pages", Name: "total"},
		},
	}

	// Tablespace charts
	tablespaceUsageChartTmpl = module.Chart{
		ID:       "tablespace_%s_usage",
		Title:    "Tablespace %s Usage",
		Units:    "percentage",
		Fam:      "tablespace",
		Ctx:      "db2.tablespace_usage",
		Priority: module.Priority + 300,
		Dims: module.Dims{
			{ID: "tablespace_%s_used_percent", Name: "used", Div: precision},
		},
	}

	tablespaceSizeChartTmpl = module.Chart{
		ID:       "tablespace_%s_size",
		Title:    "Tablespace %s Size",
		Units:    "bytes",
		Fam:      "tablespace",
		Ctx:      "db2.tablespace_size",
		Priority: module.Priority + 301,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "tablespace_%s_used_size_kb", Name: "used", Mul: 1024},
			{ID: "tablespace_%s_free_size_kb", Name: "free", Mul: 1024},
		},
	}

	// Connection charts
	connectionStateChartTmpl = module.Chart{
		ID:       "connection_%s_state",
		Title:    "Connection %s State",
		Units:    "state",
		Fam:      "connection",
		Ctx:      "db2.connection_state",
		Priority: module.Priority + 400,
		Dims: module.Dims{
			{ID: "connection_%s_state", Name: "state"},
		},
	}

	connectionActivityChartTmpl = module.Chart{
		ID:       "connection_%s_activity",
		Title:    "Connection %s Row Activity",
		Units:    "rows/s",
		Fam:      "connection",
		Ctx:      "db2.connection_activity",
		Priority: module.Priority + 401,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "connection_%s_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "connection_%s_rows_written", Name: "written", Algo: module.Incremental, Mul: -1},
		},
	}
)

// Chart creation functions
func newDatabaseCharts(db *databaseMetrics) *module.Charts {
	charts := module.Charts{
		databaseStatusChartTmpl.Copy(),
		databaseApplicationsChartTmpl.Copy(),
	}

	cleanName := cleanName(db.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "database", Value: db.name},
			{Key: "status", Value: db.status},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func newBufferpoolCharts(bp *bufferpoolMetrics) *module.Charts {
	charts := module.Charts{
		bufferpoolHitRatioChartTmpl.Copy(),
		bufferpoolIOChartTmpl.Copy(),
		bufferpoolPagesChartTmpl.Copy(),
	}

	cleanName := cleanName(bp.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "bufferpool", Value: bp.name},
			{Key: "page_size", Value: fmt.Sprintf("%d", bp.pageSize)},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func newTablespaceCharts(ts *tablespaceMetrics) *module.Charts {
	charts := module.Charts{
		tablespaceUsageChartTmpl.Copy(),
		tablespaceSizeChartTmpl.Copy(),
	}

	cleanName := cleanName(ts.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "tablespace", Value: ts.name},
			{Key: "type", Value: ts.tbspType},
			{Key: "content_type", Value: ts.contentType},
			{Key: "state", Value: ts.state},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func newConnectionCharts(conn *connectionMetrics) *module.Charts {
	charts := module.Charts{
		connectionStateChartTmpl.Copy(),
		connectionActivityChartTmpl.Copy(),
	}

	cleanID := cleanName(conn.applicationID)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanID)
		chart.Labels = []module.Label{
			{Key: "application_id", Value: conn.applicationID},
			{Key: "application_name", Value: conn.applicationName},
			{Key: "client_hostname", Value: conn.clientHostname},
			{Key: "client_user", Value: conn.clientUser},
			{Key: "state", Value: conn.connectionState},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanID)
		}
	}

	return &charts
}
