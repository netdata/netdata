// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package db2

import (
	"context"
	"fmt"
	"strconv"
)

func (c *Collector) collectTableInstances(ctx context.Context) error {
	if c.MaxTables <= 0 {
		return nil
	}

	var currentTable, currentSchema, key string
	err := c.doQuery(ctx, queryTableInstances, func(column, value string, lineEnd bool) {
		switch column {
		case "TABSCHEMA":
			currentSchema = value
		case "TABNAME":
			currentTable = value
			key = fmt.Sprintf("%s.%s", currentSchema, currentTable)

			if !c.allowTable(key) {
				key = ""
				return
			}

			if _, exists := c.tables[key]; !exists {
				c.tables[key] = &tableMetrics{name: key}
			}
			c.mx.tables[key] = tableInstanceMetrics{}
		case "DATA_OBJECT_P_SIZE":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tables[key]
					metrics.DataSize = v
					c.mx.tables[key] = metrics
				}
			}
		case "INDEX_OBJECT_P_SIZE":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tables[key]
					metrics.IndexSize = v
					c.mx.tables[key] = metrics
				}
			}
		case "LONG_OBJECT_P_SIZE":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tables[key]
					metrics.LongObjSize = v
					c.mx.tables[key] = metrics
				}
			}
		case "ROWS_READ":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tables[key]
					metrics.RowsRead = v
					c.mx.tables[key] = metrics
				}
			}
		case "ROWS_WRITTEN":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tables[key]
					metrics.RowsWritten = v
					c.mx.tables[key] = metrics
				}
			}
		}

		if lineEnd {
			key = ""
			currentTable = ""
			currentSchema = ""
		}
	})

	return err
}
