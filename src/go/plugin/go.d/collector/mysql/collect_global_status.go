// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"errors"
	"strings"
)

const (
	queryShowGlobalStatus      = "SHOW GLOBAL STATUS;"
	queryShowGlobalStatusProbe = `SHOW GLOBAL STATUS 
WHERE VARIABLE_NAME IN ('Threads_connected', 'wsrep_received');`
)

func (c *Collector) collectGlobalStatus(ctx context.Context, state *collectRunState) error {
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
			metricName := strings.ToLower(name)
			if isGaleraMetric(metricName) && !c.galeraDetected {
				return
			}

			switch metricName {
			case "wsrep_connected":
				parsedValue := parseInt(convertWsrepConnected(value))
				c.mx.set("wsrep_connected", parsedValue)
			case "wsrep_ready":
				parsedValue := parseInt(convertWsrepReady(value))
				c.mx.set("wsrep_ready", parsedValue)
			case "wsrep_local_state":
				c.mx.setWsrepLocalState(value)
			case "wsrep_cluster_status":
				c.mx.setWsrepClusterStatus(value)
			default:
				parsedValue := parseInt(value)
				c.mx.set(metricName, parsedValue)

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

func (c *Collector) probeGlobalStatus(ctx context.Context) error {
	q := queryShowGlobalStatusProbe
	c.Debugf("executing query: '%s'", q)

	var hasRows bool
	_, err := c.collectQuery(ctx, q, func(column, value string, lineEnd bool) {
		if column == "Variable_name" {
			switch value {
			case "wsrep_received":
				c.galeraDetected = true
			}
		}
		hasRows = hasRows || lineEnd
	})
	if err != nil {
		return err
	}
	if !hasRows {
		return errors.New("global status probe returned no rows")
	}
	return nil
}

func isGaleraMetric(name string) bool {
	return strings.HasPrefix(name, "wsrep_")
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
