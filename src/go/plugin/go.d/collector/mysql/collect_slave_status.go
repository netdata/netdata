// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"strings"

	"github.com/blang/semver/v4"
)

const (
	queryShowReplicaStatus   = "SHOW REPLICA STATUS;"
	queryShowSlaveStatus     = "SHOW SLAVE STATUS;"
	queryShowAllSlavesStatus = "SHOW ALL SLAVES STATUS;"
)

func (c *Collector) collectSlaveStatus(mx map[string]int64) error {
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

	_, err := c.collectQuery(q, func(column, value string, lineEnd bool) {
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
			if !c.collectedReplConns[v.name] {
				c.collectedReplConns[v.name] = true
				c.addSlaveReplicationConnCharts(v.name)
			}
			s := strings.ToLower(slaveMetricSuffix(v.name))
			mx["seconds_behind_master"+s] = v.behindMaster
			mx["slave_sql_running"+s] = v.sqlRunning
			mx["slave_io_running"+s] = v.ioRunning
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

func slaveMetricSuffix(conn string) string {
	if conn == "" {
		return ""
	}
	return "_" + conn
}
