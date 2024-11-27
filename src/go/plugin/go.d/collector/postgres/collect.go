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

func (c *Collector) collect() (map[string]int64, error) {
	if c.db == nil {
		db, err := c.openPrimaryConnection()
		if err != nil {
			return nil, err
		}
		c.db = db
	}

	if c.pgVersion == 0 {
		ver, err := c.doQueryServerVersion()
		if err != nil {
			return nil, fmt.Errorf("querying server version error: %v", err)
		}
		c.pgVersion = ver
		c.Debugf("connected to PostgreSQL v%d", c.pgVersion)
	}

	if c.superUser == nil {
		v, err := c.doQueryIsSuperUser()
		if err != nil {
			return nil, fmt.Errorf("querying is super user error: %v", err)
		}
		c.superUser = &v
		c.Debugf("connected as super user: %v", *c.superUser)
	}

	if c.pgIsInRecovery == nil {
		v, err := c.doQueryPGIsInRecovery()
		if err != nil {
			return nil, fmt.Errorf("querying recovery status error: %v", err)
		}
		c.pgIsInRecovery = &v
		c.Debugf("the instance is in recovery mode: %v", *c.pgIsInRecovery)
	}

	now := time.Now()

	if now.Sub(c.recheckSettingsTime) > c.recheckSettingsEvery {
		c.recheckSettingsTime = now
		maxConn, err := c.doQuerySettingsMaxConnections()
		if err != nil {
			return nil, fmt.Errorf("querying settings max connections error: %v", err)
		}
		c.mx.maxConnections = maxConn

		maxLocks, err := c.doQuerySettingsMaxLocksHeld()
		if err != nil {
			return nil, fmt.Errorf("querying settings max locks held error: %v", err)
		}
		c.mx.maxLocksHeld = maxLocks
	}

	c.resetMetrics()

	if c.pgVersion >= pgVersion10 {
		// need 'backend_type' in pg_stat_activity
		c.addXactQueryRunningTimeChartsOnce.Do(func() {
			c.addTransactionsRunTimeHistogramChart()
			c.addQueriesRunTimeHistogramChart()
		})
	}
	if c.isSuperUser() {
		c.addWALFilesChartsOnce.Do(c.addWALFilesCharts)
	}

	if err := c.doQueryGlobalMetrics(); err != nil {
		return nil, err
	}
	if err := c.doQueryReplicationMetrics(); err != nil {
		return nil, err
	}
	if err := c.doQueryDatabasesMetrics(); err != nil {
		return nil, err
	}
	if c.dbSr != nil {
		if err := c.doQueryQueryableDatabases(); err != nil {
			return nil, err
		}
	}
	if err := c.doQueryTablesMetrics(); err != nil {
		return nil, err
	}
	if err := c.doQueryIndexesMetrics(); err != nil {
		return nil, err
	}

	if now.Sub(c.doSlowTime) > c.doSlowEvery {
		c.doSlowTime = now
		if err := c.doQueryBloat(); err != nil {
			return nil, err
		}
		if err := c.doQueryColumns(); err != nil {
			return nil, err
		}
	}

	mx := make(map[string]int64)
	c.collectMetrics(mx)

	return mx, nil
}

func (c *Collector) openPrimaryConnection() (*sql.DB, error) {
	db, err := sql.Open("pgx", c.DSN)
	if err != nil {
		return nil, fmt.Errorf("error on opening a connection with the Postgres database [%s]: %v", c.DSN, err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(10 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("error on pinging the Postgres database [%s]: %v", c.DSN, err)
	}

	return db, nil
}

func (c *Collector) openSecondaryConnection(dbname string) (*sql.DB, string, error) {
	cfg, err := pgx.ParseConfig(c.DSN)
	if err != nil {
		return nil, "", fmt.Errorf("error on parsing DSN [%s]: %v", c.DSN, err)
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

	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		stdlib.UnregisterConnConfig(connStr)
		_ = db.Close()
		return nil, "", fmt.Errorf("error on pinging the secondary Postgres database [%s]: %v", dbname, err)
	}

	return db, connStr, nil
}

func (c *Collector) isSuperUser() bool { return c.superUser != nil && *c.superUser }

func (c *Collector) isPGInRecovery() bool { return c.pgIsInRecovery != nil && *c.pgIsInRecovery }

func (c *Collector) getDBMetrics(name string) *dbMetrics {
	db, ok := c.mx.dbs[name]
	if !ok {
		db = &dbMetrics{name: name}
		c.mx.dbs[name] = db
	}
	return db
}

func (c *Collector) getTableMetrics(name, db, schema string) *tableMetrics {
	key := name + "_" + db + "_" + schema
	m, ok := c.mx.tables[key]
	if !ok {
		m = &tableMetrics{db: db, schema: schema, name: name}
		c.mx.tables[key] = m
	}
	return m
}

func (c *Collector) hasTableMetrics(name, db, schema string) bool {
	key := name + "_" + db + "_" + schema
	_, ok := c.mx.tables[key]
	return ok
}

func (c *Collector) getIndexMetrics(name, table, db, schema string) *indexMetrics {
	key := name + "_" + table + "_" + db + "_" + schema
	m, ok := c.mx.indexes[key]
	if !ok {
		m = &indexMetrics{name: name, db: db, schema: schema, table: table}
		c.mx.indexes[key] = m
	}
	return m
}

func (c *Collector) hasIndexMetrics(name, table, db, schema string) bool {
	key := name + "_" + table + "_" + db + "_" + schema
	_, ok := c.mx.indexes[key]
	return ok
}

func (c *Collector) getReplAppMetrics(name string) *replStandbyAppMetrics {
	app, ok := c.mx.replApps[name]
	if !ok {
		app = &replStandbyAppMetrics{name: name}
		c.mx.replApps[name] = app
	}
	return app
}

func (c *Collector) getReplSlotMetrics(name string) *replSlotMetrics {
	slot, ok := c.mx.replSlots[name]
	if !ok {
		slot = &replSlotMetrics{name: name}
		c.mx.replSlots[name] = slot
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
