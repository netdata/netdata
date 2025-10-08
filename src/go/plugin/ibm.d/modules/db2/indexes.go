// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package db2

import (
	"context"
	"fmt"
	"strconv"
)

func (c *Collector) collectIndexInstances(ctx context.Context) error {
	if c.MaxIndexes <= 0 {
		return nil
	}

	var currentSchema, currentIndex, key string
	err := c.doQuery(ctx, queryIndexInstances, func(column, value string, lineEnd bool) {
		switch column {
		case "INDSCHEMA":
			currentSchema = value
		case "INDNAME":
			currentIndex = value
			key = fmt.Sprintf("%s.%s", currentSchema, currentIndex)

			if c.indexSelector != nil && !c.indexSelector.MatchString(key) {
				key = ""
				return
			}

			if _, exists := c.indexes[key]; !exists {
				c.indexes[key] = &indexMetrics{name: key}
			}
			c.mx.indexes[key] = indexInstanceMetrics{}
		case "NLEAF":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.indexes[key]
					metrics.LeafNodes = v
					c.mx.indexes[key] = metrics
				}
			}
		case "INDEX_SCANS":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.indexes[key]
					metrics.IndexScans = v
					c.mx.indexes[key] = metrics
				}
			}
		case "FULL_SCANS":
			if key != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.indexes[key]
					metrics.FullScans = v
					c.mx.indexes[key] = metrics
				}
			}
		}
	})

	return err
}
