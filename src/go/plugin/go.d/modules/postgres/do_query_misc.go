// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"strconv"

	"github.com/jackc/pgx/v5/stdlib"
)

func (p *Postgres) doQueryServerVersion() (int, error) {
	q := queryServerVersion()

	var s string
	if err := p.doQueryRow(q, &s); err != nil {
		return 0, err
	}

	return strconv.Atoi(s)
}

func (p *Postgres) doQueryIsSuperUser() (bool, error) {
	q := queryIsSuperUser()

	var v bool
	if err := p.doQueryRow(q, &v); err != nil {
		return false, err
	}

	return v, nil
}

func (p *Postgres) doQueryPGIsInRecovery() (bool, error) {
	q := queryPGIsInRecovery()

	var v bool
	if err := p.doQueryRow(q, &v); err != nil {
		return false, err
	}

	return v, nil
}

func (p *Postgres) doQuerySettingsMaxConnections() (int64, error) {
	q := querySettingsMaxConnections()

	var s string
	if err := p.doQueryRow(q, &s); err != nil {
		return 0, err
	}

	return strconv.ParseInt(s, 10, 64)
}

func (p *Postgres) doQuerySettingsMaxLocksHeld() (int64, error) {
	q := querySettingsMaxLocksHeld()

	var s string
	if err := p.doQueryRow(q, &s); err != nil {
		return 0, err
	}

	return strconv.ParseInt(s, 10, 64)
}

const connErrMax = 3

func (p *Postgres) doQueryQueryableDatabases() error {
	q := queryQueryableDatabaseList()

	var dbs []string
	err := p.doQuery(q, func(_, value string, _ bool) {
		if p.dbSr != nil && p.dbSr.MatchString(value) {
			dbs = append(dbs, value)
		}
	})
	if err != nil {
		return err
	}

	seen := make(map[string]bool, len(dbs))

	for _, dbname := range dbs {
		seen[dbname] = true

		conn, ok := p.dbConns[dbname]
		if !ok {
			conn = &dbConn{}
			p.dbConns[dbname] = conn
		}

		if conn.db != nil || conn.connErrors >= connErrMax {
			continue
		}

		db, connStr, err := p.openSecondaryConnection(dbname)
		if err != nil {
			p.Warning(err)
			conn.connErrors++
			continue
		}

		tables, err := p.doDBQueryUserTablesCount(db)
		if err != nil {
			p.Warning(err)
			conn.connErrors++
			_ = db.Close()
			stdlib.UnregisterConnConfig(connStr)
			continue
		}

		indexes, err := p.doDBQueryUserIndexesCount(db)
		if err != nil {
			p.Warning(err)
			conn.connErrors++
			_ = db.Close()
			stdlib.UnregisterConnConfig(connStr)
			continue
		}

		if (p.MaxDBTables != 0 && tables > p.MaxDBTables) || (p.MaxDBIndexes != 0 && indexes > p.MaxDBIndexes) {
			p.Warningf("database '%s' has too many user tables(%d/%d)/indexes(%d/%d), skipping it",
				dbname, tables, p.MaxDBTables, indexes, p.MaxDBIndexes)
			conn.connErrors = connErrMax
			_ = db.Close()
			stdlib.UnregisterConnConfig(connStr)
			continue
		}

		conn.db, conn.connStr = db, connStr
	}

	for dbname, conn := range p.dbConns {
		if seen[dbname] {
			continue
		}
		delete(p.dbConns, dbname)
		if conn.connStr != "" {
			stdlib.UnregisterConnConfig(conn.connStr)
		}
		if conn.db != nil {
			_ = conn.db.Close()
		}
	}

	return nil
}

func (p *Postgres) doDBQueryUserTablesCount(db *sql.DB) (int64, error) {
	q := queryUserTablesCount()

	var v string
	if err := p.doDBQueryRow(db, q, &v); err != nil {
		return 0, err
	}

	return parseInt(v), nil
}

func (p *Postgres) doDBQueryUserIndexesCount(db *sql.DB) (int64, error) {
	q := queryUserIndexesCount()

	var v string
	if err := p.doDBQueryRow(db, q, &v); err != nil {
		return 0, err
	}

	return parseInt(v), nil
}
