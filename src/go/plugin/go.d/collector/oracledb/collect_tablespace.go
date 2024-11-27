// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strconv"
)

const queryTablespace = `
SELECT
    f.tablespace_name,
    f.autoextensible,
    SUM(f.bytes) AS allocated_bytes,
    SUM(f.maxbytes) AS max_bytes,
    (SUM(f.bytes) - COALESCE(SUM(fs.free_bytes), 0)) AS used_bytes
FROM
    dba_data_files f
LEFT JOIN
    (
        SELECT
            tablespace_name,
            SUM(bytes) AS free_bytes
        FROM
            dba_free_space
        GROUP BY
            tablespace_name
    ) fs
    ON f.tablespace_name = fs.tablespace_name
GROUP BY
    f.tablespace_name, f.autoextensible
`

func (c *Collector) collectTablespace(mx map[string]int64) error {
	q := queryTablespace
	c.Debugf("executing query: %s", q)

	var ts struct {
		name       string
		autoExtent bool
		allocBytes float64
		maxBytes   float64
		usedBytes  float64
	}

	seen := make(map[string]bool)

	err := c.doQuery(q, func(column, value string, lineEnd bool) error {
		var err error

		switch column {
		case "TABLESPACE_NAME":
			ts.name = value
		case "AUTOEXTENSIBLE":
			ts.autoExtent = value == "YES"
		case "ALLOCATED_BYTES":
			ts.allocBytes, err = strconv.ParseFloat(value, 64)
		case "MAX_BYTES":
			ts.maxBytes, err = strconv.ParseFloat(value, 64)
		case "USED_BYTES":
			ts.usedBytes, err = strconv.ParseFloat(value, 64)
		}
		if err != nil {
			return fmt.Errorf("could not parse column '%s' value '%s': %w", column, value, err)
		}

		if lineEnd {
			seen[ts.name] = true

			limit := ts.allocBytes
			if ts.autoExtent {
				limit = ts.maxBytes
			}

			px := fmt.Sprintf("tablespace_%s_", ts.name)

			mx[px+"max_size_bytes"] = int64(limit)
			mx[px+"used_bytes"] = int64(ts.usedBytes)
			mx[px+"avail_bytes"] = int64(limit - ts.usedBytes)
			mx[px+"utilization"] = 0
			if limit > 0 {
				mx[px+"utilization"] = int64(ts.usedBytes / limit * 100 * precision)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for name := range seen {
		if !c.seenTablespaces[name] {
			c.seenTablespaces[name] = true
			c.addTablespaceCharts(name)
		}
	}
	for name := range c.seenTablespaces {
		if !seen[name] {
			delete(c.seenTablespaces, name)
			c.removeTablespaceChart(name)
		}
	}

	return nil
}
