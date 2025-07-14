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
		Ctx:      "db2.bufferpool_instance_hit_ratio",
		Priority: module.Priority + 200,
		Dims: module.Dims{
			{ID: "bufferpool_%s_hit_ratio", Name: "overall", Div: precision},
		},
	}

	bufferpoolDetailedHitRatioChartTmpl = module.Chart{
		ID:       "bufferpool_%s_detailed_hit_ratio",
		Title:    "Buffer Pool %s Detailed Hit Ratios",
		Units:    "percentage",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_detailed_hit_ratio",
		Priority: module.Priority + 201,
		Dims: module.Dims{
			{ID: "bufferpool_%s_data_hit_ratio", Name: "data", Div: precision},
			{ID: "bufferpool_%s_index_hit_ratio", Name: "index", Div: precision},
			{ID: "bufferpool_%s_xda_hit_ratio", Name: "xda", Div: precision},
			{ID: "bufferpool_%s_column_hit_ratio", Name: "column", Div: precision},
		},
	}

	bufferpoolReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_reads",
		Title:    "Buffer Pool %s Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_reads",
		Priority: module.Priority + 202,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolDataReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_data_reads",
		Title:    "Buffer Pool %s Data Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_data_reads",
		Priority: module.Priority + 203,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_data_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_data_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolIndexReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_index_reads",
		Title:    "Buffer Pool %s Index Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_index_reads",
		Priority: module.Priority + 204,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_index_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_index_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolPagesChartTmpl = module.Chart{
		ID:       "bufferpool_%s_pages",
		Title:    "Buffer Pool %s Pages",
		Units:    "pages",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_pages",
		Priority: module.Priority + 210,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_used_pages", Name: "used"},
			{ID: "bufferpool_%s_total_pages", Name: "total"},
		},
	}

	bufferpoolWritesChartTmpl = module.Chart{
		ID:       "bufferpool_%s_writes",
		Title:    "Buffer Pool %s Writes",
		Units:    "writes/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_writes",
		Priority: module.Priority + 211,
		Dims: module.Dims{
			{ID: "bufferpool_%s_writes", Name: "writes", Algo: module.Incremental},
		},
	}

	bufferpoolXDAReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_xda_reads",
		Title:    "Buffer Pool %s XDA Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_xda_reads",
		Priority: module.Priority + 205,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_xda_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_xda_physical_reads", Name: "physical", Algo: module.Incremental},
		},
	}

	bufferpoolColumnReadsChartTmpl = module.Chart{
		ID:       "bufferpool_%s_column_reads",
		Title:    "Buffer Pool %s Column Page Reads",
		Units:    "reads/s",
		Fam:      "bufferpool",
		Ctx:      "db2.bufferpool_instance_column_reads",
		Priority: module.Priority + 206,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "bufferpool_%s_column_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "bufferpool_%s_column_physical_reads", Name: "physical", Algo: module.Incremental},
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
			{ID: "tablespace_%s_used_size", Name: "used"},
			{ID: "tablespace_%s_free_size", Name: "free"},
		},
	}

	tablespaceUsableSizeChartTmpl = module.Chart{
		ID:       "tablespace_%s_usable_size",
		Title:    "Tablespace %s Usable Size",
		Units:    "bytes",
		Fam:      "tablespace",
		Ctx:      "db2.tablespace_usable_size",
		Priority: module.Priority + 302,
		Dims: module.Dims{
			{ID: "tablespace_%s_total_size", Name: "total"},
			{ID: "tablespace_%s_usable_size", Name: "usable"},
		},
	}

	tablespaceStateChartTmpl = module.Chart{
		ID:       "tablespace_%s_state",
		Title:    "Tablespace %s State",
		Units:    "state",
		Fam:      "tablespace",
		Ctx:      "db2.tablespace_state",
		Priority: module.Priority + 303,
		Dims: module.Dims{
			{ID: "tablespace_%s_state", Name: "state"},
		},
	}

	// Connection charts
	connectionStateChartTmpl = module.Chart{
		ID:       "connection_%s_state",
		Title:    "Connection State",
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
		Title:    "Connection Row Activity",
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

	connectionCPUTimeChartTmpl = module.Chart{
		ID:       "connection_%s_cpu_time",
		Title:    "Connection CPU Time",
		Units:    "milliseconds/s",
		Fam:      "connection",
		Ctx:      "db2.connection_cpu_time",
		Priority: module.Priority + 402,
		Dims: module.Dims{
			{ID: "connection_%s_total_cpu_time", Name: "cpu_time", Algo: module.Incremental},
		},
	}

	connectionExecutingChartTmpl = module.Chart{
		ID:       "connection_%s_executing",
		Title:    "Connection Query Execution",
		Units:    "queries",
		Fam:      "connection",
		Ctx:      "db2.connection_executing",
		Priority: module.Priority + 403,
		Dims: module.Dims{
			{ID: "connection_%s_executing_queries", Name: "executing"},
		},
	}

	// Connection wait time charts (DB2 9.7+ LUW)
	connectionWaitTimeChartTmpl = module.Chart{
		ID:       "connection_%s_wait_time",
		Title:    "Connection Wait Time",
		Units:    "milliseconds/s",
		Fam:      "connection",
		Ctx:      "db2.connection_wait_time",
		Priority: module.Priority + 404,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "connection_%s_lock_wait_time", Name: "lock", Algo: module.Incremental},
			{ID: "connection_%s_log_disk_wait_time", Name: "log_disk", Algo: module.Incremental},
			{ID: "connection_%s_log_buffer_wait_time", Name: "log_buffer", Algo: module.Incremental},
			{ID: "connection_%s_pool_read_time", Name: "pool_read", Algo: module.Incremental},
			{ID: "connection_%s_pool_write_time", Name: "pool_write", Algo: module.Incremental},
			{ID: "connection_%s_direct_read_time", Name: "direct_read", Algo: module.Incremental},
			{ID: "connection_%s_direct_write_time", Name: "direct_write", Algo: module.Incremental},
		},
	}

	connectionNetworkWaitTimeChartTmpl = module.Chart{
		ID:       "connection_%s_network_wait_time",
		Title:    "Connection Network Wait Time",
		Units:    "milliseconds/s",
		Fam:      "connection",
		Ctx:      "db2.connection_network_wait_time",
		Priority: module.Priority + 405,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "connection_%s_fcm_recv_wait_time", Name: "recv", Algo: module.Incremental},
			{ID: "connection_%s_fcm_send_wait_time", Name: "send", Algo: module.Incremental, Mul: -1},
		},
	}

	connectionProcessingTimeChartTmpl = module.Chart{
		ID:       "connection_%s_processing_time",
		Title:    "Connection Processing Time",
		Units:    "milliseconds/s",
		Fam:      "connection",
		Ctx:      "db2.connection_processing_time",
		Priority: module.Priority + 406,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "connection_%s_total_routine_time", Name: "routine", Algo: module.Incremental},
			{ID: "connection_%s_total_compile_time", Name: "compile", Algo: module.Incremental},
			{ID: "connection_%s_total_section_time", Name: "section", Algo: module.Incremental},
		},
	}

	connectionTransactionTimeChartTmpl = module.Chart{
		ID:       "connection_%s_transaction_time",
		Title:    "Connection Transaction Time",
		Units:    "milliseconds/s",
		Fam:      "connection",
		Ctx:      "db2.connection_transaction_time",
		Priority: module.Priority + 407,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "connection_%s_total_commit_time", Name: "commit", Algo: module.Incremental},
			{ID: "connection_%s_total_rollback_time", Name: "rollback", Algo: module.Incremental, Mul: -1},
		},
	}

	// Table charts
	tableSizeChartTmpl = module.Chart{
		ID:       "table_%s_size",
		Title:    "Table %s Size",
		Units:    "bytes",
		Fam:      "table",
		Ctx:      "db2.table_size",
		Priority: module.Priority + 500,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "table_%s_data_size", Name: "data", Mul: 1024},
			{ID: "table_%s_index_size", Name: "index", Mul: 1024},
			{ID: "table_%s_long_obj_size", Name: "long_obj", Mul: 1024},
		},
	}

	tableActivityChartTmpl = module.Chart{
		ID:       "table_%s_activity",
		Title:    "Table %s Activity",
		Units:    "rows/s",
		Fam:      "table",
		Ctx:      "db2.table_activity",
		Priority: module.Priority + 501,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "table_%s_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "table_%s_rows_written", Name: "written", Algo: module.Incremental, Mul: -1},
		},
	}

	// Index charts
	indexUsageChartTmpl = module.Chart{
		ID:       "index_%s_usage",
		Title:    "Index %s Usage",
		Units:    "scans/s",
		Fam:      "index",
		Ctx:      "db2.index_usage",
		Priority: module.Priority + 600,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "index_%s_index_scans", Name: "index", Algo: module.Incremental},
			{ID: "index_%s_full_scans", Name: "full", Algo: module.Incremental, Mul: -1},
		},
	}
)

// Chart creation functions
func (d *DB2) newDatabaseCharts(db *databaseMetrics) *module.Charts {
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
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newBufferpoolCharts(bp *bufferpoolMetrics) *module.Charts {
	charts := module.Charts{
		bufferpoolHitRatioChartTmpl.Copy(),
		bufferpoolDetailedHitRatioChartTmpl.Copy(),
		bufferpoolReadsChartTmpl.Copy(),
		bufferpoolDataReadsChartTmpl.Copy(),
		bufferpoolIndexReadsChartTmpl.Copy(),
		bufferpoolXDAReadsChartTmpl.Copy(),
		bufferpoolColumnReadsChartTmpl.Copy(),
		bufferpoolPagesChartTmpl.Copy(),
		bufferpoolWritesChartTmpl.Copy(),
	}

	cleanName := cleanName(bp.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "bufferpool", Value: bp.name},
			{Key: "page_size", Value: fmt.Sprintf("%d", bp.pageSize)},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newTablespaceCharts(ts *tablespaceMetrics) *module.Charts {
	charts := module.Charts{
		tablespaceUsageChartTmpl.Copy(),
		tablespaceSizeChartTmpl.Copy(),
		tablespaceUsableSizeChartTmpl.Copy(),
		tablespaceStateChartTmpl.Copy(),
	}

	cleanName := cleanName(ts.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "tablespace", Value: ts.name},
			{Key: "type", Value: ts.tbspType},
			{Key: "content_type", Value: ts.contentType},
			{Key: "state", Value: ts.state},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newConnectionCharts(conn *connectionMetrics) *module.Charts {
	charts := module.Charts{
		connectionStateChartTmpl.Copy(),
		connectionActivityChartTmpl.Copy(),
		connectionCPUTimeChartTmpl.Copy(),
		connectionExecutingChartTmpl.Copy(),
	}

	// Add wait time charts if enabled and supported (DB2 9.7+ LUW)
	if d.CollectWaitMetrics && d.useMonGetFunctions {
		charts = append(charts,
			connectionWaitTimeChartTmpl.Copy(),
			connectionNetworkWaitTimeChartTmpl.Copy(),
			connectionProcessingTimeChartTmpl.Copy(),
			connectionTransactionTimeChartTmpl.Copy(),
		)
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
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanID)
		}
	}

	return &charts
}

func (d *DB2) newTableCharts(t *tableMetrics) *module.Charts {
	charts := module.Charts{
		tableSizeChartTmpl.Copy(),
		tableActivityChartTmpl.Copy(),
	}

	cleanName := cleanName(t.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "table", Value: t.name},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}

func (d *DB2) newIndexCharts(i *indexMetrics) *module.Charts {
	charts := module.Charts{
		indexUsageChartTmpl.Copy(),
	}

	cleanName := cleanName(i.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "index", Value: i.name},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}

	return &charts
}
