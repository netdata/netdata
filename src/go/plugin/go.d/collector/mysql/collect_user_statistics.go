// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"strings"

	"github.com/blang/semver/v4"
)

const queryShowUserStatistics = "SHOW USER_STATISTICS;"

func (c *Collector) collectUserStatistics(mx map[string]int64) error {
	// https://mariadb.com/kb/en/user-statistics/
	// https://mariadb.com/kb/en/information-schema-user_statistics-table/
	q := queryShowUserStatistics
	c.Debugf("executing query: '%s'", q)

	var user, prefix string
	_, err := c.collectQuery(q, func(column, value string, _ bool) {
		switch column {
		case "User":
			user = value
			prefix = "userstats_" + user + "_"
			if !c.collectedUsers[user] {
				c.collectedUsers[user] = true
				c.addUserStatisticsCharts(user)
			}
		case "Cpu_time":
			if c.isMariaDB && c.version.GTE(semver.Version{Major: 11, Minor: 4, Patch: 5}) {
				// TODO: theoretically should divide by 1e6 to convert to seconds,
				// but empirically need 1e7 to match pre-11.4.5 values.
				// Needs investigation - possible unit reporting inconsistency in MariaDB
				mx[strings.ToLower(prefix+column)] = int64(parseFloat(value) / 1e7 * 1000)
			} else {
				mx[strings.ToLower(prefix+column)] = int64(parseFloat(value) * 1000)
			}
		case
			"Total_connections",
			"Lost_connections",
			"Denied_connections",
			"Empty_queries",
			"Binlog_bytes_written",
			"Rows_read",
			"Rows_sent",
			"Rows_deleted",
			"Rows_inserted",
			"Rows_updated",
			"Rows_fetched", // Percona
			"Select_commands",
			"Update_commands",
			"Other_commands",
			"Access_denied",
			"Commit_transactions",
			"Rollback_transactions":
			mx[strings.ToLower(prefix+column)] = parseInt(value)
		}
	})
	return err
}
