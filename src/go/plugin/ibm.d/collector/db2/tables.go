// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"context"
	"fmt"
	"strconv"
)

func (d *DB2) collectTableInstances(ctx context.Context) error {
	if d.MaxTables <= 0 {
		return nil
	}

	var currentTable, currentSchema, key string
	err := d.doQuery(ctx, fmt.Sprintf(queryTableInstances, d.MaxTables), func(column, value string, lineEnd bool) {
		switch column {
		case "TABSCHEMA":
			currentSchema = value
		case "TABNAME":
			currentTable = value
			key = fmt.Sprintf("%s.%s", currentSchema, currentTable)

			if d.tableSelector != nil && !d.tableSelector.MatchString(key) {
				key = ""
				return
			}

			if _, exists := d.tables[key]; !exists {
				d.tables[key] = &tableMetrics{name: key}
				d.addTableCharts(d.tables[key])
			}
			d.mx.tables[key] = tableInstanceMetrics{}
		case "DATA_OBJECT_P_SIZE":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tables[key]
					metrics.DataSize = v
					d.mx.tables[key] = metrics
				}
			}
		case "INDEX_OBJECT_P_SIZE":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tables[key]
					metrics.IndexSize = v
					d.mx.tables[key] = metrics
				}
			}
		case "LONG_OBJECT_P_SIZE":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tables[key]
					metrics.LongObjSize = v
					d.mx.tables[key] = metrics
				}
			}
		case "ROWS_READ":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tables[key]
					metrics.RowsRead = v
					d.mx.tables[key] = metrics
				}
			}
		case "ROWS_WRITTEN":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tables[key]
					metrics.RowsWritten = v
					d.mx.tables[key] = metrics
				}
			}
		}
	})

	return err
}
