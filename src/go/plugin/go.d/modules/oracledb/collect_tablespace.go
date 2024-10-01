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

func (o *OracleDB) collectTablespace(mx map[string]int64) error {
	q := queryTablespace
	o.Debugf("executing query: %s", q)

	var ts struct {
		name       string
		autoExtent bool
		allocBytes int64
		maxBytes   int64
		usedBytes  int64
	}

	seen := make(map[string]bool)

	err := o.doQuery(q, func(column, value string, lineEnd bool) error {
		var err error

		switch column {
		case "TABLESPACE_NAME":
			ts.name = value
		case "AUTOEXTENSIBLE":
			ts.autoExtent = value == "YES"
		case "ALLOCATED_BYTES":
			ts.allocBytes, err = strconv.ParseInt(value, 10, 64)
		case "MAX_BYTES":
			ts.maxBytes, err = strconv.ParseInt(value, 10, 64)
		case "USED_BYTES":
			ts.usedBytes, err = strconv.ParseInt(value, 10, 64)
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

			mx[px+"max_size_bytes"] = limit
			mx[px+"used_bytes"] = ts.usedBytes
			mx[px+"avail_bytes"] = limit - ts.usedBytes
			mx[px+"utilization"] = 0
			if limit > 0 {
				mx[px+"utilization"] = int64(float64(ts.usedBytes) / float64(limit) * 100 * precision)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for name := range seen {
		if !o.seenTablespaces[name] {
			o.seenTablespaces[name] = true
			o.addTablespaceCharts(name)
		}
	}
	for name := range o.seenTablespaces {
		if !seen[name] {
			delete(o.seenTablespaces, name)
			o.removeTablespaceChart(name)
		}
	}

	return nil
}
