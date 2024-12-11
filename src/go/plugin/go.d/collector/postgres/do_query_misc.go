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

		tables, err := c.doDBQueryUserTablesCount(db)
		if err != nil {
			c.Warning(err)
			conn.connErrors++
			_ = db.Close()
			stdlib.UnregisterConnConfig(connStr)
			continue
		}

		indexes, err := c.doDBQueryUserIndexesCount(db)
		if err != nil {
			c.Warning(err)
			conn.connErrors++
			_ = db.Close()
			stdlib.UnregisterConnConfig(connStr)
			continue
		}

		if (c.MaxDBTables != 0 && tables > c.MaxDBTables) || (c.MaxDBIndexes != 0 && indexes > c.MaxDBIndexes) {
			c.Warningf("database '%s' has too many user tables(%d/%d)/indexes(%d/%d), skipping it",
				dbname, tables, c.MaxDBTables, indexes, c.MaxDBIndexes)
			conn.connErrors = connErrMax
			_ = db.Close()
			stdlib.UnregisterConnConfig(connStr)
			continue
		}

		conn.db, conn.connStr = db, connStr
	}

	for dbname, conn := range c.dbConns {
		if seen[dbname] {
			continue
		}
		delete(c.dbConns, dbname)
		if conn.connStr != "" {
			stdlib.UnregisterConnConfig(conn.connStr)
		}
		if conn.db != nil {
			_ = conn.db.Close()
		}
	}

	return nil
}

func (c *Collector) doDBQueryUserTablesCount(db *sql.DB) (int64, error) {
	q := queryUserTablesCount()

	var v string
	if err := c.doDBQueryRow(db, q, &v); err != nil {
		return 0, err
	}

	return parseInt(v), nil
}

func (c *Collector) doDBQueryUserIndexesCount(db *sql.DB) (int64, error) {
	q := queryUserIndexesCount()

	var v string
	if err := c.doDBQueryRow(db, q, &v); err != nil {
		return 0, err
	}

	return parseInt(v), nil
}
