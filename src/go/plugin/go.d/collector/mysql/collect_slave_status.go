// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"

	"github.com/blang/semver/v4"
)

const (
	queryShowReplicaStatus   = "SHOW REPLICA STATUS;"
	queryShowSlaveStatus     = "SHOW SLAVE STATUS;"
	queryShowAllSlavesStatus = "SHOW ALL SLAVES STATUS;"
)

func (c *Collector) collectSlaveStatus(ctx context.Context) error {
	// https://mariadb.com/docs/reference/es/sql-statements/SHOW_ALL_SLAVES_STATUS/
	mariaDBMinVer := semver.Version{Major: 10, Minor: 2, Patch: 0}
	mysqlMinVer := semver.Version{Major: 8, Minor: 0, Patch: 22}
	var q string
	if c.isMariaDB && c.version.GTE(mariaDBMinVer) {
		q = queryShowAllSlavesStatus
	} else if !c.isMariaDB && c.version.GTE(mysqlMinVer) {
		q = queryShowReplicaStatus
	} else {
		q = queryShowSlaveStatus
	}
	c.Debugf("executing query: '%s'", q)

	type slaveStatusRow struct {
		name         string
		behindMaster int64
		sqlRunning   int64
		ioRunning    int64
	}
	row := slaveStatusRow{}

	_, err := c.collectQuery(ctx, q, func(column, value string, lineEnd bool) {
		switch column {
		case "Connection_name", "Channel_Name":
			row.name = value
		case "Seconds_Behind_Master", "Seconds_Behind_Source":
			row.behindMaster = parseInt(value)
		case "Slave_SQL_Running", "Replica_SQL_Running":
			row.sqlRunning = parseInt(convertSlaveSQLRunning(value))
		case "Slave_IO_Running", "Replica_IO_Running":
			row.ioRunning = parseInt(convertSlaveIORunning(value))
		}
		if lineEnd {
			c.mx.setReplication("seconds_behind_master", row.name, row.behindMaster)
			c.mx.setReplication("slave_sql_running", row.name, row.sqlRunning)
			c.mx.setReplication("slave_io_running", row.name, row.ioRunning)

			// Explicit row reset keeps per-row lifecycle obvious in callback flow.
			row = slaveStatusRow{}
		}
	})
	return err
}

func convertSlaveSQLRunning(value string) string {
	switch value {
	case "Yes":
		return "1"
	default:
		return "0"
	}
}

func convertSlaveIORunning(value string) string {
	// NOTE: There is 'Connecting' state and probably others
	switch value {
	case "Yes":
		return "1"
	default:
		return "0"
	}
}
