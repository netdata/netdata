// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"fmt"
	"strconv"
)

const queryTablespace = `
SELECT 
    ts.tablespace_name,
    ts.contents AS tablespace_type,
    CASE 
        WHEN df_all.files_with_autoextend = 0 THEN 'NO'
        WHEN df_all.files_with_autoextend = df_all.total_files THEN 'YES'
        ELSE 'MIXED (' || df_all.files_with_autoextend || '/' || df_all.total_files || ')'
    END AS autoextend_status,
    CASE 
        -- For PERMANENT tablespaces: allocated - free
        WHEN ts.contents = 'PERMANENT' THEN 
            COALESCE(df_all.allocated_bytes, 0) - COALESCE(fs.free_bytes, 0)
        -- For UNDO tablespaces: sum of UNDO segments
        WHEN ts.contents = 'UNDO' THEN 
            COALESCE(us.used_bytes, 0)
        -- For TEMPORARY tablespaces: use v$temp_space_header
        WHEN ts.contents = 'TEMPORARY' THEN 
            COALESCE(tu.used_bytes, 0)
        ELSE 0
    END AS used_bytes,
    COALESCE(df_all.allocated_bytes, 0) AS allocated_bytes,
    -- For max_bytes, only count maxbytes from files with autoextend=YES
    COALESCE(df_all.max_bytes, 0) AS max_bytes
FROM 
    dba_tablespaces ts
    -- Combined datafiles and tempfiles info with autoextend counts
    LEFT JOIN (
        SELECT 
            tablespace_name,
            COUNT(*) AS total_files,
            SUM(CASE WHEN autoextensible = 'YES' THEN 1 ELSE 0 END) AS files_with_autoextend,
            SUM(bytes) AS allocated_bytes,
            -- Only count maxbytes for files with autoextend enabled
            SUM(CASE 
                WHEN autoextensible = 'YES' AND maxbytes > 0 THEN maxbytes
                WHEN autoextensible = 'YES' AND maxbytes = 0 THEN bytes
                ELSE bytes  -- For non-autoextend files, current size is the max
            END) AS max_bytes
        FROM dba_data_files
        GROUP BY tablespace_name
        UNION ALL
        SELECT 
            tablespace_name,
            COUNT(*) AS total_files,
            SUM(CASE WHEN autoextensible = 'YES' THEN 1 ELSE 0 END) AS files_with_autoextend,
            SUM(bytes) AS allocated_bytes,
            SUM(CASE 
                WHEN autoextensible = 'YES' AND maxbytes > 0 THEN maxbytes
                WHEN autoextensible = 'YES' AND maxbytes = 0 THEN bytes
                ELSE bytes
            END) AS max_bytes
        FROM dba_temp_files
        GROUP BY tablespace_name
    ) df_all ON ts.tablespace_name = df_all.tablespace_name
    -- Free space for permanent tablespaces
    LEFT JOIN (
        SELECT 
            tablespace_name,
            SUM(bytes) AS free_bytes
        FROM dba_free_space
        GROUP BY tablespace_name
    ) fs ON ts.tablespace_name = fs.tablespace_name
    -- Used space for UNDO tablespaces
    LEFT JOIN (
        SELECT 
            tablespace_name,
            SUM(bytes) AS used_bytes
        FROM dba_segments
        WHERE segment_type LIKE 'UNDO%'
        GROUP BY tablespace_name
    ) us ON ts.tablespace_name = us.tablespace_name
    -- Used space for TEMPORARY tablespaces
    LEFT JOIN (
        SELECT 
            tablespace_name,
            SUM(bytes_used) AS used_bytes
        FROM v$temp_space_header
        GROUP BY tablespace_name
    ) tu ON ts.tablespace_name = tu.tablespace_name
WHERE 
    df_all.tablespace_name IS NOT NULL  -- Only show tablespaces with datafiles/tempfiles
ORDER BY 
    ts.tablespace_name
`

type tablespaceInfo struct {
	name       string
	typ        string
	autoExtent string
	allocBytes float64
	maxBytes   float64
	usedBytes  float64
}

func (c *Collector) collectTablespace(mx map[string]int64) error {
	q := queryTablespace
	c.Debugf("executing query: %s", q)

	var ts tablespaceInfo

	seen := make(map[string]tablespaceInfo)

	err := c.doQuery(q, func(column, value string, lineEnd bool) error {
		var err error

		switch column {
		case "TABLESPACE_NAME":
			ts.name = value
		case "TABLESPACE_TYPE":
			ts.typ = value
		case "AUTOEXTEND_STATUS":
			switch value {
			case "YES":
				value = "enabled"
			case "NO":
				value = "disabled"
			case "MIXED":
				value = "mixed"
			}
			ts.autoExtent = value
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
			if ts.typ == "TEMPORARY" {
				return nil
			}
			seen[ts.name] = ts

			limit := ts.maxBytes
			if ts.autoExtent == "disabled" {
				limit = ts.allocBytes
			}

			used := ts.usedBytes
			if used > limit {
				used = limit
			}

			avail := limit - used

			var util float64
			if limit > 0 {
				if util = used / limit * 100; util > 100 {
					util = 100
				}
			}

			px := fmt.Sprintf("tablespace_%s_", ts.name)

			mx[px+"max_size_bytes"] = int64(limit)
			mx[px+"used_bytes"] = int64(used)
			mx[px+"avail_bytes"] = int64(avail)
			mx[px+"utilization"] = 0
			if limit > 0 {
				mx[px+"utilization"] = int64(util * precision)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	for _, ts := range seen {
		if !c.seenTablespaces[ts.name] {
			c.seenTablespaces[ts.name] = true
			c.addTablespaceCharts(ts)
		}
	}
	for name := range c.seenTablespaces {
		if _, ok := seen[name]; !ok {
			delete(c.seenTablespaces, name)
			c.removeTablespaceChart(name)
		}
	}

	return nil
}
