// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver/v4"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/stdlib"
)

// 'SHOW STATS;' response was changed significantly in v1.8.0
// v1.8.0 was released in 2015 - no need to complicate the code to support the old version.
var minSupportedVersion = semver.Version{Major: 1, Minor: 8, Patch: 0}

const (
	queryShowVersion   = "SHOW VERSION;"
	queryShowConfig    = "SHOW CONFIG;"
	queryShowDatabases = "SHOW DATABASES;"
	queryShowStats     = "SHOW STATS;"
	queryShowPools     = "SHOW POOLS;"
)

func (c *Collector) collect() (map[string]int64, error) {
	if c.db == nil {
		if err := c.openConnection(); err != nil {
			return nil, err
		}
	}
	if c.version == nil {
		ver, err := c.queryVersion()
		if err != nil {
			return nil, err
		}
		c.Debugf("connected to PgBouncer v%s", ver)
		if ver.LE(minSupportedVersion) {
			return nil, fmt.Errorf("unsupported version: v%s, required v%s+", ver, minSupportedVersion)
		}
		c.version = ver
	}

	now := time.Now()
	if now.Sub(c.recheckSettingsTime) > c.recheckSettingsEvery {
		v, err := c.queryMaxClientConn()
		if err != nil {
			return nil, err
		}
		c.maxClientConn = v
	}

	// http://www.pgbouncer.org/usage.html

	c.resetMetrics()

	if err := c.collectDatabases(); err != nil {
		return nil, err
	}
	if err := c.collectStats(); err != nil {
		return nil, err
	}
	if err := c.collectPools(); err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	c.collectMetrics(mx)

	return mx, nil
}

func (c *Collector) collectMetrics(mx map[string]int64) {
	var clientConns int64
	for name, db := range c.metrics.dbs {
		if !db.updated {
			delete(c.metrics.dbs, name)
			c.removeDatabaseCharts(name)
			continue
		}
		if !db.hasCharts {
			db.hasCharts = true
			c.addNewDatabaseCharts(name, db.pgDBName)
		}

		mx["db_"+name+"_total_xact_count"] = db.totalXactCount
		mx["db_"+name+"_total_xact_time"] = db.totalXactTime
		mx["db_"+name+"_avg_xact_time"] = db.avgXactTime

		mx["db_"+name+"_total_query_count"] = db.totalQueryCount
		mx["db_"+name+"_total_query_time"] = db.totalQueryTime
		mx["db_"+name+"_avg_query_time"] = db.avgQueryTime

		mx["db_"+name+"_total_wait_time"] = db.totalWaitTime
		mx["db_"+name+"_maxwait"] = db.maxWait*1e6 + db.maxWaitUS

		mx["db_"+name+"_cl_active"] = db.clActive
		mx["db_"+name+"_cl_waiting"] = db.clWaiting
		mx["db_"+name+"_cl_cancel_req"] = db.clCancelReq
		clientConns += db.clActive + db.clWaiting + db.clCancelReq

		mx["db_"+name+"_sv_active"] = db.svActive
		mx["db_"+name+"_sv_idle"] = db.svIdle
		mx["db_"+name+"_sv_used"] = db.svUsed
		mx["db_"+name+"_sv_tested"] = db.svTested
		mx["db_"+name+"_sv_login"] = db.svLogin

		mx["db_"+name+"_total_received"] = db.totalReceived
		mx["db_"+name+"_total_sent"] = db.totalSent

		mx["db_"+name+"_sv_conns_utilization"] = calcPercentage(db.currentConnections, db.maxConnections)
	}

	mx["cl_conns_utilization"] = calcPercentage(clientConns, c.maxClientConn)
}

func (c *Collector) collectDatabases() error {
	q := queryShowDatabases
	c.Debugf("executing query: %v", q)

	var db string
	return c.collectQuery(q, func(column, value string) {
		switch column {
		case "name":
			db = value
			c.getDBMetrics(db).updated = true
		case "database":
			c.getDBMetrics(db).pgDBName = value
		case "max_connections":
			c.getDBMetrics(db).maxConnections = parseInt(value)
		case "current_connections":
			c.getDBMetrics(db).currentConnections = parseInt(value)
		case "paused":
			c.getDBMetrics(db).paused = parseInt(value)
		case "disabled":
			c.getDBMetrics(db).disabled = parseInt(value)
		}
	})
}

func (c *Collector) collectStats() error {
	q := queryShowStats
	c.Debugf("executing query: %v", q)

	var db string
	return c.collectQuery(q, func(column, value string) {
		switch column {
		case "database":
			db = value
			c.getDBMetrics(db).updated = true
		case "total_xact_count":
			c.getDBMetrics(db).totalXactCount = parseInt(value)
		case "total_query_count":
			c.getDBMetrics(db).totalQueryCount = parseInt(value)
		case "total_received":
			c.getDBMetrics(db).totalReceived = parseInt(value)
		case "total_sent":
			c.getDBMetrics(db).totalSent = parseInt(value)
		case "total_xact_time":
			c.getDBMetrics(db).totalXactTime = parseInt(value)
		case "total_query_time":
			c.getDBMetrics(db).totalQueryTime = parseInt(value)
		case "total_wait_time":
			c.getDBMetrics(db).totalWaitTime = parseInt(value)
		case "avg_xact_time":
			c.getDBMetrics(db).avgXactTime = parseInt(value)
		case "avg_query_time":
			c.getDBMetrics(db).avgQueryTime = parseInt(value)
		}
	})
}

func (c *Collector) collectPools() error {
	q := queryShowPools
	c.Debugf("executing query: %v", q)

	// an entry is made for each couple of (database, user).
	var db string
	return c.collectQuery(q, func(column, value string) {
		switch column {
		case "database":
			db = value
			c.getDBMetrics(db).updated = true
		case "cl_active":
			c.getDBMetrics(db).clActive += parseInt(value)
		case "cl_waiting":
			c.getDBMetrics(db).clWaiting += parseInt(value)
		case "cl_cancel_req":
			c.getDBMetrics(db).clCancelReq += parseInt(value)
		case "sv_active":
			c.getDBMetrics(db).svActive += parseInt(value)
		case "sv_idle":
			c.getDBMetrics(db).svIdle += parseInt(value)
		case "sv_used":
			c.getDBMetrics(db).svUsed += parseInt(value)
		case "sv_tested":
			c.getDBMetrics(db).svTested += parseInt(value)
		case "sv_login":
			c.getDBMetrics(db).svLogin += parseInt(value)
		case "maxwait":
			c.getDBMetrics(db).maxWait += parseInt(value)
		case "maxwait_us":
			c.getDBMetrics(db).maxWaitUS += parseInt(value)
		}
	})
}

func (c *Collector) queryMaxClientConn() (int64, error) {
	q := queryShowConfig
	c.Debugf("executing query: %v", q)

	var v int64
	var key string
	err := c.collectQuery(q, func(column, value string) {
		switch column {
		case "key":
			key = value
		case "value":
			if key == "max_client_conn" {
				v = parseInt(value)
			}
		}
	})
	return v, err
}

var reVersion = regexp.MustCompile(`\d+\.\d+\.\d+`)

func (c *Collector) queryVersion() (*semver.Version, error) {
	q := queryShowVersion
	c.Debugf("executing query: %v", q)

	var resp string
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()
	if err := c.db.QueryRowContext(ctx, q).Scan(&resp); err != nil {
		return nil, err
	}

	if !strings.Contains(resp, "PgBouncer") {
		return nil, fmt.Errorf("not PgBouncer instance: version response: %s", resp)
	}

	ver := reVersion.FindString(resp)
	if ver == "" {
		return nil, fmt.Errorf("couldn't parse version string '%s' (expected pattern '%s')", resp, reVersion)
	}

	v, err := semver.New(ver)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse version string '%s': %v", ver, err)
	}

	return v, nil
}

func (c *Collector) openConnection() error {
	cfg, err := pgx.ParseConfig(c.DSN)
	if err != nil {
		return err
	}
	cfg.PreferSimpleProtocol = true

	db, err := sql.Open("pgx", stdlib.RegisterConnConfig(cfg))
	if err != nil {
		return fmt.Errorf("error on opening a connection with the PgBouncer database [%s]: %v", c.DSN, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	c.db = db

	return nil
}

func (c *Collector) collectQuery(query string, assign func(column, value string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()
	rows, err := c.db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := makeNullStrings(len(columns))
	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return err
		}
		for i, v := range values {
			assign(columns[i], valueToString(v))
		}
	}
	return rows.Err()
}

func (c *Collector) getDBMetrics(dbname string) *dbMetrics {
	db, ok := c.metrics.dbs[dbname]
	if !ok {
		db = &dbMetrics{name: dbname}
		c.metrics.dbs[dbname] = db
	}
	return db
}

func (c *Collector) resetMetrics() {
	for name, db := range c.metrics.dbs {
		c.metrics.dbs[name] = &dbMetrics{
			name:      db.name,
			pgDBName:  db.pgDBName,
			hasCharts: db.hasCharts,
		}
	}
}

func valueToString(value any) string {
	v, ok := value.(*sql.NullString)
	if !ok || !v.Valid {
		return ""
	}
	return v.String
}

func makeNullStrings(size int) []any {
	vs := make([]any, size)
	for i := range vs {
		vs[i] = &sql.NullString{}
	}
	return vs
}

func parseInt(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func calcPercentage(value, total int64) int64 {
	if total == 0 {
		return 0
	}
	return value * 100 / total
}
