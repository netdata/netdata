// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"database/sql"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mysql/mysqlfunc"
)

var errMySQLDBUnavailable = errors.New("collector database is not ready")

type funcDepsAdapter struct {
	collector *Collector
}

var _ mysqlfunc.Deps = (*funcDepsAdapter)(nil)

func (a funcDepsAdapter) DB() (mysqlfunc.Queryer, error) {
	db, err := a.collector.currentDB()
	if err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errMySQLDBUnavailable
	}
	return db, nil
}

func (c *Collector) dbReady() bool {
	c.dbMu.RLock()
	ready := c.db != nil
	c.dbMu.RUnlock()
	return ready
}

func (c *Collector) currentDB() (*sql.DB, error) {
	c.dbMu.RLock()
	db := c.db
	c.dbMu.RUnlock()
	if db == nil {
		return nil, errMySQLDBUnavailable
	}
	return db, nil
}

func (c *Collector) setDB(db *sql.DB) {
	c.dbMu.Lock()
	c.db = db
	c.dbMu.Unlock()
}

func (c *Collector) closeDB() error {
	c.dbMu.Lock()
	db := c.db
	c.db = nil
	c.dbMu.Unlock()
	if db == nil {
		return nil
	}
	return db.Close()
}
