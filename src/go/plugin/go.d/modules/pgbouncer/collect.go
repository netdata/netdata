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

func (p *PgBouncer) collect() (map[string]int64, error) {
	if p.db == nil {
		if err := p.openConnection(); err != nil {
			return nil, err
		}
	}
	if p.version == nil {
		ver, err := p.queryVersion()
		if err != nil {
			return nil, err
		}
		p.Debugf("connected to PgBouncer v%s", ver)
		if ver.LE(minSupportedVersion) {
			return nil, fmt.Errorf("unsupported version: v%s, required v%s+", ver, minSupportedVersion)
		}
		p.version = ver
	}

	now := time.Now()
	if now.Sub(p.recheckSettingsTime) > p.recheckSettingsEvery {
		v, err := p.queryMaxClientConn()
		if err != nil {
			return nil, err
		}
		p.maxClientConn = v
	}

	// http://www.pgbouncer.org/usage.html

	p.resetMetrics()

	if err := p.collectDatabases(); err != nil {
		return nil, err
	}
	if err := p.collectStats(); err != nil {
		return nil, err
	}
	if err := p.collectPools(); err != nil {
		return nil, err
	}

	mx := make(map[string]int64)
	p.collectMetrics(mx)

	return mx, nil
}

func (p *PgBouncer) collectMetrics(mx map[string]int64) {
	var clientConns int64
	for name, db := range p.metrics.dbs {
		if !db.updated {
			delete(p.metrics.dbs, name)
			p.removeDatabaseCharts(name)
			continue
		}
		if !db.hasCharts {
			db.hasCharts = true
			p.addNewDatabaseCharts(name, db.pgDBName)
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

	mx["cl_conns_utilization"] = calcPercentage(clientConns, p.maxClientConn)
}

func (p *PgBouncer) collectDatabases() error {
	q := queryShowDatabases
	p.Debugf("executing query: %v", q)

	var db string
	return p.collectQuery(q, func(column, value string) {
		switch column {
		case "name":
			db = value
			p.getDBMetrics(db).updated = true
		case "database":
			p.getDBMetrics(db).pgDBName = value
		case "max_connections":
			p.getDBMetrics(db).maxConnections = parseInt(value)
		case "current_connections":
			p.getDBMetrics(db).currentConnections = parseInt(value)
		case "paused":
			p.getDBMetrics(db).paused = parseInt(value)
		case "disabled":
			p.getDBMetrics(db).disabled = parseInt(value)
		}
	})
}

func (p *PgBouncer) collectStats() error {
	q := queryShowStats
	p.Debugf("executing query: %v", q)

	var db string
	return p.collectQuery(q, func(column, value string) {
		switch column {
		case "database":
			db = value
			p.getDBMetrics(db).updated = true
		case "total_xact_count":
			p.getDBMetrics(db).totalXactCount = parseInt(value)
		case "total_query_count":
			p.getDBMetrics(db).totalQueryCount = parseInt(value)
		case "total_received":
			p.getDBMetrics(db).totalReceived = parseInt(value)
		case "total_sent":
			p.getDBMetrics(db).totalSent = parseInt(value)
		case "total_xact_time":
			p.getDBMetrics(db).totalXactTime = parseInt(value)
		case "total_query_time":
			p.getDBMetrics(db).totalQueryTime = parseInt(value)
		case "total_wait_time":
			p.getDBMetrics(db).totalWaitTime = parseInt(value)
		case "avg_xact_time":
			p.getDBMetrics(db).avgXactTime = parseInt(value)
		case "avg_query_time":
			p.getDBMetrics(db).avgQueryTime = parseInt(value)
		}
	})
}

func (p *PgBouncer) collectPools() error {
	q := queryShowPools
	p.Debugf("executing query: %v", q)

	// an entry is made for each couple of (database, user).
	var db string
	return p.collectQuery(q, func(column, value string) {
		switch column {
		case "database":
			db = value
			p.getDBMetrics(db).updated = true
		case "cl_active":
			p.getDBMetrics(db).clActive += parseInt(value)
		case "cl_waiting":
			p.getDBMetrics(db).clWaiting += parseInt(value)
		case "cl_cancel_req":
			p.getDBMetrics(db).clCancelReq += parseInt(value)
		case "sv_active":
			p.getDBMetrics(db).svActive += parseInt(value)
		case "sv_idle":
			p.getDBMetrics(db).svIdle += parseInt(value)
		case "sv_used":
			p.getDBMetrics(db).svUsed += parseInt(value)
		case "sv_tested":
			p.getDBMetrics(db).svTested += parseInt(value)
		case "sv_login":
			p.getDBMetrics(db).svLogin += parseInt(value)
		case "maxwait":
			p.getDBMetrics(db).maxWait += parseInt(value)
		case "maxwait_us":
			p.getDBMetrics(db).maxWaitUS += parseInt(value)
		}
	})
}

func (p *PgBouncer) queryMaxClientConn() (int64, error) {
	q := queryShowConfig
	p.Debugf("executing query: %v", q)

	var v int64
	var key string
	err := p.collectQuery(q, func(column, value string) {
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

func (p *PgBouncer) queryVersion() (*semver.Version, error) {
	q := queryShowVersion
	p.Debugf("executing query: %v", q)

	var resp string
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Duration())
	defer cancel()
	if err := p.db.QueryRowContext(ctx, q).Scan(&resp); err != nil {
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

func (p *PgBouncer) openConnection() error {
	cfg, err := pgx.ParseConfig(p.DSN)
	if err != nil {
		return err
	}
	cfg.PreferSimpleProtocol = true

	db, err := sql.Open("pgx", stdlib.RegisterConnConfig(cfg))
	if err != nil {
		return fmt.Errorf("error on opening a connection with the PgBouncer database [%s]: %v", p.DSN, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	p.db = db

	return nil
}

func (p *PgBouncer) collectQuery(query string, assign func(column, value string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Duration())
	defer cancel()
	rows, err := p.db.QueryContext(ctx, query)
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

func (p *PgBouncer) getDBMetrics(dbname string) *dbMetrics {
	db, ok := p.metrics.dbs[dbname]
	if !ok {
		db = &dbMetrics{name: dbname}
		p.metrics.dbs[dbname] = db
	}
	return db
}

func (p *PgBouncer) resetMetrics() {
	for name, db := range p.metrics.dbs {
		p.metrics.dbs[name] = &dbMetrics{
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
