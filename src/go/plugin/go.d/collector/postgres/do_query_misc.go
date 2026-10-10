// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"strconv"

	"github.com/jackc/pgx/v5/stdlib"
)

func (c *Collector) doQueryServerVersion() (int, error) {
	q := queryServerVersion()

	var s string
	if err := c.doQueryRow(q, &s); err != nil {
		return 0, err
	}

	return strconv.Atoi(s)
}

func (c *Collector) doQueryIsSuperUser() (bool, error) {
	q := queryIsSuperUser()

	var v bool
	if err := c.doQueryRow(q, &v); err != nil {
		return false, err
	}

	return v, nil
}

func (c *Collector) doQueryCanExecutePgLsDir() (bool, error) {
	q := queryCanExecutePgLsDir()

	var v bool
	if err := c.doQueryRow(q, &v); err != nil {
		return false, err
	}

	return v, nil
}

func (c *Collector) doQueryPGIsInRecovery() (bool, error) {
	q := queryPGIsInRecovery()

	var v bool
	if err := c.doQueryRow(q, &v); err != nil {
		return false, err
	}

	return v, nil
}

func (c *Collector) doQuerySettingsMaxConnections() (int64, error) {
	q := querySettingsMaxConnections()

	var s string
	if err := c.doQueryRow(q, &s); err != nil {
		return 0, err
	}

	return strconv.ParseInt(s, 10, 64)
}

func (c *Collector) doQuerySettingsMaxLocksHeld() (int64, error) {
	q := querySettingsMaxLocksHeld()

	var s string
	if err := c.doQueryRow(q, &s); err != nil {
		return 0, err
	}

	return strconv.ParseInt(s, 10, 64)
}

const connErrMax = 3

var unregisterConnConfig = stdlib.UnregisterConnConfig

func closeDBAndUnregisterConnConfig(db *sql.DB, connStr string) {
	if db != nil {
		_ = db.Close()
	}
	if connStr != "" {
		unregisterConnConfig(connStr)
	}
}

func (c *Collector) doQueryQueryableDatabases() error {
	q := queryQueryableDatabaseList()

	var dbs []string
	err := c.doQuery(q, func(_, value string, _ bool) {
		if c.dbSr != nil && c.dbSr.MatchString(value) {
			dbs = append(dbs, value)
		}
	})
	if err != nil {
		return err
	}

	seen := make(map[string]bool, len(dbs))

	for _, dbname := range dbs {
		seen[dbname] = true

		conn, ok := c.dbConns[dbname]
		if !ok {
			conn = &dbConn{}
			c.dbConns[dbname] = conn
		}

		if conn.db != nil || conn.connErrors >= connErrMax {
			continue
		}

		db, connStr, err := c.openSecondaryConnection(dbname)
		if err != nil {
			c.Warning(err)
			conn.connErrors++
			continue
		}

		conn.db, conn.connStr = db, connStr
	}

	for dbname, conn := range c.dbConns {
		if seen[dbname] {
			continue
		}
		c.retireDatabaseRelations(conn.db)
		delete(c.dbConns, dbname)
		closeDBAndUnregisterConnConfig(conn.db, conn.connStr)
	}

	return nil
}
