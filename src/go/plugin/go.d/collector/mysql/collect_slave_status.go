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

	v := struct {
		name         string
		behindMaster int64
		sqlRunning   int64
		ioRunning    int64
	}{}

	_, err := c.collectQuery(ctx, q, func(column, value string, lineEnd bool) {
		switch column {
		case "Connection_name", "Channel_Name":
			v.name = value
		case "Seconds_Behind_Master", "Seconds_Behind_Source":
			v.behindMaster = parseInt(value)
		case "Slave_SQL_Running", "Replica_SQL_Running":
			v.sqlRunning = parseInt(convertSlaveSQLRunning(value))
		case "Slave_IO_Running", "Replica_IO_Running":
			v.ioRunning = parseInt(convertSlaveIORunning(value))
		}
		if lineEnd {
			c.mx.setReplication("seconds_behind_master", v.name, v.behindMaster)
			c.mx.setReplication("slave_sql_running", v.name, v.sqlRunning)
			c.mx.setReplication("slave_io_running", v.name, v.ioRunning)
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
