// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const (
	queryVersion                    = "select version();"
	queryStatsMySQLGlobal           = "SELECT * FROM stats_mysql_global;"
	queryStatsMySQLMemoryMetrics    = "SELECT * FROM stats_memory_metrics;"
	queryStatsMySQLCommandsCounters = "SELECT * FROM stats_mysql_commands_counters;"
	queryStatsMySQLUsers            = "SELECT * FROM stats_mysql_users;"
	queryStatsMySQLConnectionPool   = "SELECT * FROM stats_mysql_connection_pool;"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, err
		}
	}

	c.once.Do(func() {
		v, err := c.doQueryVersion()
		if err != nil {
			c.Warningf("error on querying version: %v", err)
		} else {
			c.Debugf("connected to ProxySQL version: %s", v)
		}
	})

	c.cache.reset()

	mx := make(map[string]int64)

	if err := c.collectStatsMySQLGlobal(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql global status: %v", err)
	}
	if err := c.collectStatsMySQLMemoryMetrics(mx); err != nil {
		return nil, fmt.Errorf("error on collecting memory metrics: %v", err)
	}
	if err := c.collectStatsMySQLCommandsCounters(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql command counters: %v", err)
	}
	if err := c.collectStatsMySQLUsers(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql users: %v", err)
	}
	if err := c.collectStatsMySQLConnectionPool(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql connection pool: %v", err)
	}

	c.updateCharts()

	return mx, nil
}

func (c *Collector) doQueryVersion() (string, error) {
	q := queryVersion
	c.Debugf("executing query: '%s'", q)

	var v string
	if err := c.doQueryRow(q, &v); err != nil {
		return "", err
	}

	return v, nil
}

func (c *Collector) collectStatsMySQLGlobal(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_global
	q := queryStatsMySQLGlobal
	c.Debugf("executing query: '%s'", q)

	var name string
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "Variable_Name":
			name = value
		case "Variable_Value":
			mx[name] = parseInt(value)
		}
	})
}

func (c *Collector) collectStatsMySQLMemoryMetrics(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_memory_metrics
	q := queryStatsMySQLMemoryMetrics
	c.Debugf("executing query: '%s'", q)

	var name string
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "Variable_Name":
			name = value
		case "Variable_Value":
			mx[name] = parseInt(value)
		}
	})
}

func (c *Collector) collectStatsMySQLCommandsCounters(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_commands_counters
	q := queryStatsMySQLCommandsCounters
	c.Debugf("executing query: '%s'", q)

	var command string
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "Command":
			command = value
			c.cache.getCommand(command).updated = true
		default:
			mx["mysql_command_"+command+"_"+column] = parseInt(value)
		}
	})
}

func (c *Collector) collectStatsMySQLUsers(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_users
	q := queryStatsMySQLUsers
	c.Debugf("executing query: '%s'", q)

	var user string
	var used int64
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "username":
			user = value
			c.cache.getUser(user).updated = true
		case "frontend_connections":
			used = parseInt(value)
			mx["mysql_user_"+user+"_"+column] = used
		case "frontend_max_connections":
			mx["mysql_user_"+user+"_frontend_connections_utilization"] = calcPercentage(used, parseInt(value))
		}
	})
}

func (c *Collector) collectStatsMySQLConnectionPool(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_connection_pool
	q := queryStatsMySQLConnectionPool
	c.Debugf("executing query: '%s'", q)

	var hg, host, port string
	var px string
	return c.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "hg", "hostgroup":
			hg = value
		case "srv_host":
			host = value
		case "srv_port":
			port = value
			c.cache.getBackend(hg, host, port).updated = true
			px = "backend_" + backendID(hg, host, port) + "_"
		case "status":
			mx[px+"status_ONLINE"] = metrix.Bool(value == "1")
			mx[px+"status_SHUNNED"] = metrix.Bool(value == "2")
			mx[px+"status_OFFLINE_SOFT"] = metrix.Bool(value == "3")
			mx[px+"status_OFFLINE_HARD"] = metrix.Bool(value == "4")
		default:
			mx[px+column] = parseInt(value)
		}
	})
}

func (c *Collector) updateCharts() {
	for k, m := range c.cache.commands {
		if !m.updated {
			delete(c.cache.commands, k)
			c.removeMySQLCommandCountersCharts(m.command)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addMySQLCommandCountersCharts(m.command)
		}
	}
	for k, m := range c.cache.users {
		if !m.updated {
			delete(c.cache.users, k)
			c.removeMySQLUserCharts(m.user)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addMySQLUsersCharts(m.user)
		}
	}
	for k, m := range c.cache.backends {
		if !m.updated {
			delete(c.cache.backends, k)
			c.removeBackendCharts(m.hg, m.host, m.port)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			c.addBackendCharts(m.hg, m.host, m.port)
		}
	}
}

func (c *Collector) openConnection() error {
	db, err := sql.Open("mysql", c.DSN)
	if err != nil {
		return fmt.Errorf("error on opening a connection with the proxysql instance [%s]: %v", c.DSN, err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging the proxysql instance [%s]: %v", c.DSN, err)
	}

	c.db = db
	return nil
}

func (c *Collector) doQueryRow(query string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	return c.db.QueryRowContext(ctx, query).Scan(v)
}

func (c *Collector) doQuery(query string, assign func(column, value string, rowEnd bool)) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	return readRows(rows, assign)
}

func readRows(rows *sql.Rows, assign func(column, value string, rowEnd bool)) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := makeValues(len(columns))

	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return err
		}
		for i, l := 0, len(values); i < l; i++ {
			assign(columns[i], valueToString(values[i]), i == l-1)
		}
	}
	return rows.Err()
}

func valueToString(value any) string {
	v, ok := value.(*sql.NullString)
	if !ok || !v.Valid {
		return ""
	}
	return v.String
}

func makeValues(size int) []any {
	vs := make([]any, size)
	for i := range vs {
		vs[i] = &sql.NullString{}
	}
	return vs
}

func parseInt(value string) int64 {
	v, _ := strconv.ParseInt(value, 10, 64)
	return v
}

func calcPercentage(value, total int64) (v int64) {
	if total == 0 {
		return 0
	}
	if v = value * 100 / total; v < 0 {
		v = -v
	}
	return v
}

func backendID(hg, host, port string) string {
	hg = strings.ReplaceAll(strings.ToLower(hg), " ", "_")
	host = strings.ReplaceAll(host, ".", "_")
	return hg + "_" + host + "_" + port
}
