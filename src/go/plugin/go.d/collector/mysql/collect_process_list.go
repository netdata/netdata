// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"github.com/blang/semver/v4"
)

// Table Schema:
// (MariaDB) https://mariadb.com/kb/en/information-schema-processlist-table/
// (MySql) https://dev.mysql.com/doc/refman/5.7/en/information-schema-processlist-table.html
const (
	queryShowProcessList = `
SELECT 
  time, 
  user 
FROM 
  information_schema.processlist 
WHERE 
  info IS NOT NULL 
  AND info NOT LIKE '%PROCESSLIST%' 
ORDER BY 
  time;`
)

// Performance Schema
// (MySQL) https://dev.mysql.com/doc/refman/8.0/en/performance-schema-processlist-table.html
const (
	queryShowProcessListPS = `
SELECT 
  time, 
  user 
FROM 
  performance_schema.processlist 
WHERE 
  info IS NOT NULL 
  AND info NOT LIKE '%PROCESSLIST%' 
ORDER BY 
  time;`
)

func (c *Collector) collectProcessListStatistics(mx map[string]int64) error {
	var q string
	mysqlMinVer := semver.Version{Major: 8, Minor: 0, Patch: 22}
	if !c.isMariaDB && c.version.GTE(mysqlMinVer) && c.varPerformanceSchema == "ON" {
		q = queryShowProcessListPS
	} else {
		q = queryShowProcessList
	}
	c.Debugf("executing query: '%s'", q)

	var maxTime int64 // slowest query milliseconds in process list

	duration, err := c.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "time":
			maxTime = parseInt(value)
		case "user":
			// system user refers to non-client threads
			// event_scheduler is the thread used to monitor scheduled events
			// system user and event_scheduler threads are grouped as system/database threads
			// authenticated and unauthenticated user are grouped as users
			// please see USER section in
			// https://dev.mysql.com/doc/refman/8.0/en/information-schema-processlist-table.html
			switch value {
			case "system user", "event_scheduler":
				mx["process_list_queries_count_system"] += 1
			default:
				mx["process_list_queries_count_user"] += 1
			}
		}
	})
	if err != nil {
		return err
	}

	if _, ok := mx["process_list_queries_count_system"]; !ok {
		mx["process_list_queries_count_system"] = 0
	}
	if _, ok := mx["process_list_queries_count_user"]; !ok {
		mx["process_list_queries_count_user"] = 0
	}
	mx["process_list_fetch_query_duration"] = duration
	mx["process_list_longest_query_duration"] = maxTime

	return nil
}
