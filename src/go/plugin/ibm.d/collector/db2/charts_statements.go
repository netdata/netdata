// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioStatementExecutions = module.Priority + 1800
	prioStatementTime       = module.Priority + 1801
	prioStatementRows       = module.Priority + 1802
	prioStatementIO         = module.Priority + 1803
	prioStatementWaits      = module.Priority + 1804
	
	prioMemoryPoolUsage     = module.Priority + 1900
	prioMemoryPoolHWM       = module.Priority + 1901
	
	prioTableIOScans        = module.Priority + 2000
	prioTableIORows         = module.Priority + 2001
	prioTableIOActivity     = module.Priority + 2002
)

// Statement chart templates
var (
	statementExecutionsChartTmpl = module.Chart{
		ID:       "statement_%s_executions",
		Title:    "Statement Executions",
		Units:    "executions/s",
		Fam:      "statements",
		Ctx:      "db2.statement_executions",
		Priority: prioStatementExecutions,
		Dims: module.Dims{
			{ID: "statement_%s_num_executions", Name: "executions", Algo: module.Incremental},
		},
	}
	
	statementAvgTimeChartTmpl = module.Chart{
		ID:       "statement_%s_avg_time",
		Title:    "Statement Average Execution Time",
		Units:    "milliseconds",
		Fam:      "statements",
		Ctx:      "db2.statement_avg_time",
		Priority: prioStatementTime,
		Dims: module.Dims{
			{ID: "statement_%s_avg_exec_time", Name: "avg_time"},
		},
	}
	
	statementCPUTimeChartTmpl = module.Chart{
		ID:       "statement_%s_cpu_time",
		Title:    "Statement CPU Time",
		Units:    "milliseconds/s",
		Fam:      "statements",
		Ctx:      "db2.statement_cpu_time",
		Priority: prioStatementTime + 1,
		Dims: module.Dims{
			{ID: "statement_%s_total_cpu_time", Name: "cpu_time", Algo: module.Incremental},
		},
	}
	
	statementRowsChartTmpl = module.Chart{
		ID:       "statement_%s_rows",
		Title:    "Statement Row Activity",
		Units:    "rows/s",
		Fam:      "statements",
		Ctx:      "db2.statement_rows",
		Priority: prioStatementRows,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "statement_%s_rows_read", Name: "read", Algo: module.Incremental},
			{ID: "statement_%s_rows_modified", Name: "modified", Algo: module.Incremental, Mul: -1},
		},
	}
	
	statementIOChartTmpl = module.Chart{
		ID:       "statement_%s_io",
		Title:    "Statement I/O Operations",
		Units:    "operations/s",
		Fam:      "statements",
		Ctx:      "db2.statement_io",
		Priority: prioStatementIO,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "statement_%s_logical_reads", Name: "logical", Algo: module.Incremental},
			{ID: "statement_%s_physical_reads", Name: "physical", Algo: module.Incremental, Mul: -1},
		},
	}
	
	statementWaitsChartTmpl = module.Chart{
		ID:       "statement_%s_waits",
		Title:    "Statement Waits and Sorts",
		Units:    "milliseconds",
		Fam:      "statements",
		Ctx:      "db2.statement_waits",
		Priority: prioStatementWaits,
		Dims: module.Dims{
			{ID: "statement_%s_lock_wait_time", Name: "lock_wait", Algo: module.Incremental},
			{ID: "statement_%s_total_sorts", Name: "sorts", Algo: module.Incremental},
		},
	}
)

// Memory pool chart templates
var (
	memoryPoolUsageChartTmpl = module.Chart{
		ID:       "memory_pool_%s_usage",
		Title:    "Memory Pool Usage",
		Units:    "bytes",
		Fam:      "memory/pools",
		Ctx:      "db2.memory_pool_usage",
		Priority: prioMemoryPoolUsage,
		Dims: module.Dims{
			{ID: "memory_pool_%s_pool_used", Name: "used"},
		},
	}
	
	memoryPoolHWMChartTmpl = module.Chart{
		ID:       "memory_pool_%s_hwm",
		Title:    "Memory Pool High Water Mark",
		Units:    "bytes",
		Fam:      "memory/pools",
		Ctx:      "db2.memory_pool_hwm",
		Priority: prioMemoryPoolHWM,
		Dims: module.Dims{
			{ID: "memory_pool_%s_pool_used_hwm", Name: "hwm"},
		},
	}
)

// Table I/O chart templates
var (
	tableIOScansChartTmpl = module.Chart{
		ID:       "table_io_%s_scans",
		Title:    "Table Scans",
		Units:    "scans/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_scans",
		Priority: prioTableIOScans,
		Dims: module.Dims{
			{ID: "table_io_%s_table_scans", Name: "scans", Algo: module.Incremental},
		},
	}
	
	tableIORowsChartTmpl = module.Chart{
		ID:       "table_io_%s_rows",
		Title:    "Table Row Operations",
		Units:    "rows/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_rows",
		Priority: prioTableIORows,
		Dims: module.Dims{
			{ID: "table_io_%s_rows_read", Name: "read", Algo: module.Incremental},
		},
	}
	
	tableIOActivityChartTmpl = module.Chart{
		ID:       "table_io_%s_activity",
		Title:    "Table DML Activity",
		Units:    "operations/s",
		Fam:      "table_io",
		Ctx:      "db2.table_io_activity",
		Priority: prioTableIOActivity,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "table_io_%s_rows_inserted", Name: "inserts", Algo: module.Incremental},
			{ID: "table_io_%s_rows_updated", Name: "updates", Algo: module.Incremental},
			{ID: "table_io_%s_rows_deleted", Name: "deletes", Algo: module.Incremental},
			{ID: "table_io_%s_overflow_accesses", Name: "overflows", Algo: module.Incremental},
		},
	}
)

// Chart creation functions
func (d *DB2) newStatementCharts(stmt *statementMetrics) *module.Charts {
	charts := module.Charts{
		statementExecutionsChartTmpl.Copy(),
		statementAvgTimeChartTmpl.Copy(),
		statementCPUTimeChartTmpl.Copy(),
		statementRowsChartTmpl.Copy(),
		statementIOChartTmpl.Copy(),
		statementWaitsChartTmpl.Copy(),
	}
	
	cleanID := stmt.id
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanID)
		chart.Labels = []module.Label{
			{Key: "statement_id", Value: stmt.id},
			{Key: "statement_preview", Value: truncateString(stmt.stmtPreview, 50)},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanID)
		}
	}
	
	return &charts
}

func (d *DB2) newMemoryPoolCharts(pool *memoryPoolMetrics) *module.Charts {
	charts := module.Charts{
		memoryPoolUsageChartTmpl.Copy(),
		memoryPoolHWMChartTmpl.Copy(),
	}
	
	cleanName := cleanName(pool.poolType)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "pool_type", Value: pool.poolType},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}
	
	return &charts
}

func (d *DB2) newTableIOCharts(table *tableMetrics) *module.Charts {
	charts := module.Charts{
		tableIOScansChartTmpl.Copy(),
		tableIORowsChartTmpl.Copy(),
		tableIOActivityChartTmpl.Copy(),
	}
	
	cleanName := cleanName(table.name)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName)
		chart.Labels = []module.Label{
			{Key: "table", Value: table.name},
			{Key: "db2_edition", Value: d.edition},
			{Key: "db2_version", Value: d.version},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName)
		}
	}
	
	return &charts
}

// Chart management functions
func (d *DB2) addStatementCharts(stmt *statementMetrics) error {
	charts := d.newStatementCharts(stmt)
	return d.charts.Add(*charts...)
}

func (d *DB2) removeStatementCharts(stmtID string) {
	px := fmt.Sprintf("statement_%s_", stmtID)
	d.removeCharts(px)
}

func (d *DB2) addMemoryPoolCharts(pool *memoryPoolMetrics) error {
	charts := d.newMemoryPoolCharts(pool)
	return d.charts.Add(*charts...)
}

func (d *DB2) removeMemoryPoolCharts(poolType string) {
	px := fmt.Sprintf("memory_pool_%s_", cleanName(poolType))
	d.removeCharts(px)
}

func (d *DB2) addTableIOCharts(table *tableMetrics) error {
	charts := d.newTableIOCharts(table)
	return d.charts.Add(*charts...)
}

func (d *DB2) removeTableIOCharts(tableName string) {
	px := fmt.Sprintf("table_io_%s_", tableName)
	d.removeCharts(px)
}

// Helper function
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}