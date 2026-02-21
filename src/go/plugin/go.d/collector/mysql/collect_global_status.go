// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"errors"
	"strings"
)

const queryShowGlobalStatus = "SHOW GLOBAL STATUS;"
const queryShowGlobalStatusProbe = "SHOW GLOBAL STATUS LIKE 'Threads_connected';"

func (c *Collector) probeGlobalStatus(ctx context.Context) error {
	q := queryShowGlobalStatusProbe
	c.Debugf("executing query: '%s'", q)

	var hasRows bool
	_, err := c.collectQuery(ctx, q, func(_, _ string, lineEnd bool) {
		if lineEnd {
			hasRows = true
		}
	})
	if err != nil {
		return err
	}
	if !hasRows {
		return errors.New("global status probe returned no rows")
	}
	return nil
}

func (c *Collector) collectGlobalStatus(ctx context.Context, state *collectRunState, writeMetrics bool) error {
	// MariaDB: https://mariadb.com/kb/en/server-status-variables/
	// MySQL: https://dev.mysql.com/doc/refman/8.0/en/server-status-variable-reference.html
	q := queryShowGlobalStatus
	c.Debugf("executing query: '%s'", q)

	var name string
	_, err := c.collectQuery(ctx, q, func(column, value string, _ bool) {
		switch column {
		case "Variable_name":
			name = value
		case "Value":
			switch name {
			case "wsrep_connected":
				parsedValue := parseInt(convertWsrepConnected(value))
				if writeMetrics {
					c.mx.set("wsrep_connected", parsedValue)
				}
			case "wsrep_ready":
				parsedValue := parseInt(convertWsrepReady(value))
				if writeMetrics {
					c.mx.set("wsrep_ready", parsedValue)
				}
			case "wsrep_local_state":
				if writeMetrics {
					c.mx.setWsrepLocalState(value)
				}
			case "wsrep_cluster_status":
				if writeMetrics {
					c.mx.setWsrepClusterStatus(value)
				}
			default:
				metricName := strings.ToLower(name)
				parsedValue := parseInt(value)
				if writeMetrics {
					c.mx.set(metricName, parsedValue)
				}

				switch metricName {
				case "connections":
					state.connections = parsedValue
				case "threads_created":
					state.threadsCreated = parsedValue
				}
			}
		}
	})
	return err
}

func convertWsrepConnected(val string) string {
	// https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_connected
	switch val {
	case "OFF":
		return "0"
	case "ON":
		return "1"
	default:
		return "-1"
	}
}

func convertWsrepReady(val string) string {
	// https://www.percona.com/doc/percona-xtradb-cluster/LATEST/wsrep-status-index.html#wsrep_ready
	switch val {
	case "OFF":
		return "0"
	case "ON":
		return "1"
	default:
		return "-1"
	}
}
