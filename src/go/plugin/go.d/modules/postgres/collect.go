// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

const (
	pgVersion94 = 9_04_00
	pgVersion10 = 10_00_00
	pgVersion11 = 11_00_00
	pgVersion17 = 17_00_00
)

func (p *Postgres) collect() (map[string]int64, error) {
	if p.db == nil {
		db, err := p.openPrimaryConnection()
		if err != nil {
			return nil, err
		}
		p.db = db
	}

	if p.pgVersion == 0 {
		ver, err := p.doQueryServerVersion()
		if err != nil {
			return nil, fmt.Errorf("querying server version error: %v", err)
		}
		p.pgVersion = ver
		p.Debugf("connected to PostgreSQL v%d", p.pgVersion)
	}

	if p.superUser == nil {
		v, err := p.doQueryIsSuperUser()
		if err != nil {
			return nil, fmt.Errorf("querying is super user error: %v", err)
		}
		p.superUser = &v
		p.Debugf("connected as super user: %v", *p.superUser)
	}

	if p.pgIsInRecovery == nil {
		v, err := p.doQueryPGIsInRecovery()
		if err != nil {
			return nil, fmt.Errorf("querying recovery status error: %v", err)
		}
		p.pgIsInRecovery = &v
		p.Debugf("the instance is in recovery mode: %v", *p.pgIsInRecovery)
	}

	now := time.Now()

	if now.Sub(p.recheckSettingsTime) > p.recheckSettingsEvery {
		p.recheckSettingsTime = now
		maxConn, err := p.doQuerySettingsMaxConnections()
		if err != nil {
			return nil, fmt.Errorf("querying settings max connections error: %v", err)
		}
		p.mx.maxConnections = maxConn

		maxLocks, err := p.doQuerySettingsMaxLocksHeld()
		if err != nil {
			return nil, fmt.Errorf("querying settings max locks held error: %v", err)
		}
		p.mx.maxLocksHeld = maxLocks
	}

	p.resetMetrics()

	if p.pgVersion >= pgVersion10 {
		// need 'backend_type' in pg_stat_activity
		p.addXactQueryRunningTimeChartsOnce.Do(func() {
			p.addTransactionsRunTimeHistogramChart()
			p.addQueriesRunTimeHistogramChart()
		})
	}
	if p.isSuperUser() {
		p.addWALFilesChartsOnce.Do(p.addWALFilesCharts)
	}

	if err := p.doQueryGlobalMetrics(); err != nil {
		return nil, err
	}
	if err := p.doQueryReplicationMetrics(); err != nil {
		return nil, err
	}
	if err := p.doQueryDatabasesMetrics(); err != nil {
		return nil, err
	}
	if p.dbSr != nil {
		if err := p.doQueryQueryableDatabases(); err != nil {
			return nil, err
		}
	}
	if err := p.doQueryTablesMetrics(); err != nil {
		return nil, err
	}
	if err := p.doQueryIndexesMetrics(); err != nil {
		return nil, err
	}

	if now.Sub(p.doSlowTime) > p.doSlowEvery {
		p.doSlowTime = now
		if err := p.doQueryBloat(); err != nil {
			return nil, err
		}
		if err := p.doQueryColumns(); err != nil {
			return nil, err
		}
	}

	mx := make(map[string]int64)
	p.collectMetrics(mx)

	return mx, nil
}

func (p *Postgres) openPrimaryConnection() (*sql.DB, error) {
	db, err := sql.Open("pgx", p.DSN)
	if err != nil {
		return nil, fmt.Errorf("error on opening a connection with the Postgres database [%s]: %v", p.DSN, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error on pinging the Postgres database [%s]: %v", p.DSN, err)
	}

	return db, nil
}

func (p *Postgres) openSecondaryConnection(dbname string) (*sql.DB, string, error) {
	cfg, err := pgx.ParseConfig(p.DSN)
	if err != nil {
		return nil, "", fmt.Errorf("error on parsing DSN [%s]: %v", p.DSN, err)
	}

	cfg.Database = dbname
	connStr := stdlib.RegisterConnConfig(cfg)

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		stdlib.UnregisterConnConfig(connStr)
		return nil, "", fmt.Errorf("error on opening a secondary connection with the Postgres database [%s]: %v", dbname, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), p.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		stdlib.UnregisterConnConfig(connStr)
		_ = db.Close()
		return nil, "", fmt.Errorf("error on pinging the secondary Postgres database [%s]: %v", dbname, err)
	}

	return db, connStr, nil
}

func (p *Postgres) isSuperUser() bool { return p.superUser != nil && *p.superUser }

func (p *Postgres) isPGInRecovery() bool { return p.pgIsInRecovery != nil && *p.pgIsInRecovery }

func (p *Postgres) getDBMetrics(name string) *dbMetrics {
	db, ok := p.mx.dbs[name]
	if !ok {
		db = &dbMetrics{name: name}
		p.mx.dbs[name] = db
	}
	return db
}

func (p *Postgres) getTableMetrics(name, db, schema string) *tableMetrics {
	key := name + "_" + db + "_" + schema
	m, ok := p.mx.tables[key]
	if !ok {
		m = &tableMetrics{db: db, schema: schema, name: name}
		p.mx.tables[key] = m
	}
	return m
}

func (p *Postgres) hasTableMetrics(name, db, schema string) bool {
	key := name + "_" + db + "_" + schema
	_, ok := p.mx.tables[key]
	return ok
}

func (p *Postgres) getIndexMetrics(name, table, db, schema string) *indexMetrics {
	key := name + "_" + table + "_" + db + "_" + schema
	m, ok := p.mx.indexes[key]
	if !ok {
		m = &indexMetrics{name: name, db: db, schema: schema, table: table}
		p.mx.indexes[key] = m
	}
	return m
}

func (p *Postgres) hasIndexMetrics(name, table, db, schema string) bool {
	key := name + "_" + table + "_" + db + "_" + schema
	_, ok := p.mx.indexes[key]
	return ok
}

func (p *Postgres) getReplAppMetrics(name string) *replStandbyAppMetrics {
	app, ok := p.mx.replApps[name]
	if !ok {
		app = &replStandbyAppMetrics{name: name}
		p.mx.replApps[name] = app
	}
	return app
}

func (p *Postgres) getReplSlotMetrics(name string) *replSlotMetrics {
	slot, ok := p.mx.replSlots[name]
	if !ok {
		slot = &replSlotMetrics{name: name}
		p.mx.replSlots[name] = slot
	}
	return slot
}

func parseInt(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func parseFloat(s string) int64 {
	v, _ := strconv.ParseFloat(s, 64)
	return int64(v)
}

func newInt(v int64) *int64 {
	return &v
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

func calcDeltaPercentage(a, b incDelta) int64 {
	return calcPercentage(a.delta(), a.delta()+b.delta())
}

func removeSpaces(s string) string {
	return reSpace.ReplaceAllString(s, "_")
}

var reSpace = regexp.MustCompile(`\s+`)
