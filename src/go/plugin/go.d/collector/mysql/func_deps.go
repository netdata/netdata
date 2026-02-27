// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"database/sql"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/mysql/mysqlfunc"
)

type funcDepsAdapter struct {
	collector *Collector
}

func (a funcDepsAdapter) DB() (mysqlfunc.Queryer, error) {
	return a.collector.currentDB()
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
		return nil, errors.New("collector database is not ready")
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
