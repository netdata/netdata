// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	queryVersion                    = "select version();"
	queryStatsMySQLGlobal           = "SELECT * FROM stats_mysql_global;"
	queryStatsMySQLMemoryMetrics    = "SELECT * FROM stats_memory_metrics;"
	queryStatsMySQLCommandsCounters = "SELECT * FROM stats_mysql_commands_counters;"
	queryStatsMySQLUsers            = "SELECT * FROM stats_mysql_users;"
	queryStatsMySQLConnectionPool   = "SELECT * FROM stats_mysql_connection_pool;"
)

func (p *ProxySQL) collect() (map[string]int64, error) {
	if p.db == nil {
		if err := p.openConnection(); err != nil {
			return nil, err
		}
	}

	p.once.Do(func() {
		v, err := p.doQueryVersion()
		if err != nil {
			p.Warningf("error on querying version: %v", err)
		} else {
			p.Debugf("connected to ProxySQL version: %s", v)
		}
	})

	p.cache.reset()

	mx := make(map[string]int64)

	if err := p.collectStatsMySQLGlobal(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql global status: %v", err)
	}
	if err := p.collectStatsMySQLMemoryMetrics(mx); err != nil {
		return nil, fmt.Errorf("error on collecting memory metrics: %v", err)
	}
	if err := p.collectStatsMySQLCommandsCounters(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql command counters: %v", err)
	}
	if err := p.collectStatsMySQLUsers(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql users: %v", err)
	}
	if err := p.collectStatsMySQLConnectionPool(mx); err != nil {
		return nil, fmt.Errorf("error on collecting mysql connection pool: %v", err)
	}

	p.updateCharts()

	return mx, nil
}

func (p *ProxySQL) doQueryVersion() (string, error) {
	q := queryVersion
	p.Debugf("executing query: '%s'", q)

	var v string
	if err := p.doQueryRow(q, &v); err != nil {
		return "", err
	}

	return v, nil
}

func (p *ProxySQL) collectStatsMySQLGlobal(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_global
	q := queryStatsMySQLGlobal
	p.Debugf("executing query: '%s'", q)

	var name string
	return p.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "Variable_Name":
			name = value
		case "Variable_Value":
			mx[name] = parseInt(value)
		}
	})
}

func (p *ProxySQL) collectStatsMySQLMemoryMetrics(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_memory_metrics
	q := queryStatsMySQLMemoryMetrics
	p.Debugf("executing query: '%s'", q)

	var name string
	return p.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "Variable_Name":
			name = value
		case "Variable_Value":
			mx[name] = parseInt(value)
		}
	})
}

func (p *ProxySQL) collectStatsMySQLCommandsCounters(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_commands_counters
	q := queryStatsMySQLCommandsCounters
	p.Debugf("executing query: '%s'", q)

	var command string
	return p.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "Command":
			command = value
			p.cache.getCommand(command).updated = true
		default:
			mx["mysql_command_"+command+"_"+column] = parseInt(value)
		}
	})
}

func (p *ProxySQL) collectStatsMySQLUsers(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_users
	q := queryStatsMySQLUsers
	p.Debugf("executing query: '%s'", q)

	var user string
	var used int64
	return p.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "username":
			user = value
			p.cache.getUser(user).updated = true
		case "frontend_connections":
			used = parseInt(value)
			mx["mysql_user_"+user+"_"+column] = used
		case "frontend_max_connections":
			mx["mysql_user_"+user+"_frontend_connections_utilization"] = calcPercentage(used, parseInt(value))
		}
	})
}

func (p *ProxySQL) collectStatsMySQLConnectionPool(mx map[string]int64) error {
	// https://proxysql.com/documentation/stats-statistics/#stats_mysql_connection_pool
	q := queryStatsMySQLConnectionPool
	p.Debugf("executing query: '%s'", q)

	var hg, host, port string
	var px string
	return p.doQuery(q, func(column, value string, rowEnd bool) {
		switch column {
		case "hg", "hostgroup":
			hg = value
		case "srv_host":
			host = value
		case "srv_port":
			port = value
			p.cache.getBackend(hg, host, port).updated = true
			px = "backend_" + backendID(hg, host, port) + "_"
		case "status":
			mx[px+"status_ONLINE"] = boolToInt(value == "1")
			mx[px+"status_SHUNNED"] = boolToInt(value == "2")
			mx[px+"status_OFFLINE_SOFT"] = boolToInt(value == "3")
			mx[px+"status_OFFLINE_HARD"] = boolToInt(value == "4")
		default:
			mx[px+column] = parseInt(value)
		}
	})
}

func (p *ProxySQL) updateCharts() {
	for k, m := range p.cache.commands {
		if !m.updated {
			delete(p.cache.commands, k)
			p.removeMySQLCommandCountersCharts(m.command)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			p.addMySQLCommandCountersCharts(m.command)
		}
	}
	for k, m := range p.cache.users {
		if !m.updated {
			delete(p.cache.users, k)
			p.removeMySQLUserCharts(m.user)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			p.addMySQLUsersCharts(m.user)
		}
	}
	for k, m := range p.cache.backends {
		if !m.updated {
			delete(p.cache.backends, k)
			p.removeBackendCharts(m.hg, m.host, m.port)
			continue
		}
		if !m.hasCharts {
			m.hasCharts = true
			p.addBackendCharts(m.hg, m.host, m.port)
		}
	}
}

func (p *ProxySQL) openConnection() error {
	db, err := sql.Open("mysql", p.DSN)
	if err != nil {
		return fmt.Errorf("error on opening a connection with the proxysql instance [%s]: %v", p.DSN, err)
	}

	db.SetConnMaxLifetime(10 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("error on pinging the proxysql instance [%s]: %v", p.DSN, err)
	}

	p.db = db
	return nil
}

func (p *ProxySQL) doQueryRow(query string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Duration())
	defer cancel()

	return p.db.QueryRowContext(ctx, query).Scan(v)
}

func (p *ProxySQL) doQuery(query string, assign func(column, value string, rowEnd bool)) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Duration())
	defer cancel()

	rows, err := p.db.QueryContext(ctx, query)
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

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func backendID(hg, host, port string) string {
	hg = strings.ReplaceAll(strings.ToLower(hg), " ", "_")
	host = strings.ReplaceAll(host, ".", "_")
	return hg + "_" + host + "_" + port
}
