// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"context"
	"fmt"
	"strconv"
)

func (d *DB2) collectIndexInstances(ctx context.Context) error {
	if d.MaxIndexes <= 0 {
		return nil
	}

	var currentSchema, currentIndex, key string
	err := d.doQuery(ctx, fmt.Sprintf(queryIndexInstances, d.MaxIndexes), func(column, value string, lineEnd bool) {
		switch column {
		case "INDSCHEMA":
			currentSchema = value
		case "INDNAME":
			currentIndex = value
			key = fmt.Sprintf("%s.%s", currentSchema, currentIndex)

			if d.indexSelector != nil && !d.indexSelector.MatchString(key) {
				key = ""
				return
			}

			if _, exists := d.indexes[key]; !exists {
				d.indexes[key] = &indexMetrics{name: key}
				d.addIndexCharts(d.indexes[key])
			}
			d.mx.indexes[key] = indexInstanceMetrics{}
		case "NLEAF":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.indexes[key]
					metrics.LeafNodes = v
					d.mx.indexes[key] = metrics
				}
			}
		case "INDEX_SCANS":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.indexes[key]
					metrics.IndexScans = v
					d.mx.indexes[key] = metrics
				}
			}
		case "FULL_SCANS":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.indexes[key]
					metrics.FullScans = v
					d.mx.indexes[key] = metrics
				}
			}
		}
	})

	return err
}
