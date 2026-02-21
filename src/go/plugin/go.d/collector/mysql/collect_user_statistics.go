// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"strings"

	"github.com/blang/semver/v4"
)

const queryShowUserStatistics = "SHOW USER_STATISTICS;"

func mustDivideMariaDBUserStatsCPUTime(version *semver.Version, isMariaDB bool) bool {
	if !isMariaDB || version == nil {
		return false
	}
	return version.EQ(semver.Version{Major: 10, Minor: 11, Patch: 11}) ||
		version.EQ(semver.Version{Major: 11, Minor: 4, Patch: 5})
}

func (c *Collector) collectUserStatistics(ctx context.Context) error {
	// https://mariadb.com/kb/en/user-statistics/
	// https://mariadb.com/kb/en/information-schema-user_statistics-table/
	q := queryShowUserStatistics
	c.Debugf("executing query: '%s'", q)

	var user string
	_, err := c.collectQuery(ctx, q, func(column, value string, _ bool) {
		switch column {
		case "User":
			user = value
		case "Cpu_time":
			// https://jira.mariadb.org/browse/MDEV-36586
			needsDivision := mustDivideMariaDBUserStatsCPUTime(c.version, c.isMariaDB)

			if needsDivision {
				c.mx.setUser("userstats_cpu_time", user, int64(parseFloat(value)/1e7*1000))
			} else {
				c.mx.setUser("userstats_cpu_time", user, int64(parseFloat(value)*1000))
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
			c.mx.setUser("userstats_"+strings.ToLower(column), user, parseInt(value))
		}
	})
	return err
}
