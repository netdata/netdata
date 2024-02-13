// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"fmt"
)

func (p *Postgres) doQueryDatabasesMetrics() error {
	if err := p.doQueryDatabaseStats(); err != nil {
		return fmt.Errorf("querying database stats error: %v", err)
	}
	if err := p.doQueryDatabaseSize(); err != nil {
		return fmt.Errorf("querying database size error: %v", err)
	}
	if p.isPGInRecovery() {
		if err := p.doQueryDatabaseConflicts(); err != nil {
			return fmt.Errorf("querying database conflicts error: %v", err)
		}
	}
	if err := p.doQueryDatabaseLocks(); err != nil {
		return fmt.Errorf("querying database locks error: %v", err)
	}
	return nil
}

func (p *Postgres) doQueryDatabaseStats() error {
	q := queryDatabaseStats()

	var db string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
			p.getDBMetrics(db).updated = true
		case "numbackends":
			p.getDBMetrics(db).numBackends = parseInt(value)
		case "datconnlimit":
			p.getDBMetrics(db).datConnLimit = parseInt(value)
		case "xact_commit":
			p.getDBMetrics(db).xactCommit = parseInt(value)
		case "xact_rollback":
			p.getDBMetrics(db).xactRollback = parseInt(value)
		case "blks_read_bytes":
			p.getDBMetrics(db).blksRead.last = parseInt(value)
		case "blks_hit_bytes":
			p.getDBMetrics(db).blksHit.last = parseInt(value)
		case "tup_returned":
			p.getDBMetrics(db).tupReturned.last = parseInt(value)
		case "tup_fetched":
			p.getDBMetrics(db).tupFetched.last = parseInt(value)
		case "tup_inserted":
			p.getDBMetrics(db).tupInserted = parseInt(value)
		case "tup_updated":
			p.getDBMetrics(db).tupUpdated = parseInt(value)
		case "tup_deleted":
			p.getDBMetrics(db).tupDeleted = parseInt(value)
		case "conflicts":
			p.getDBMetrics(db).conflicts = parseInt(value)
		case "temp_files":
			p.getDBMetrics(db).tempFiles = parseInt(value)
		case "temp_bytes":
			p.getDBMetrics(db).tempBytes = parseInt(value)
		case "deadlocks":
			p.getDBMetrics(db).deadlocks = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryDatabaseSize() error {
	q := queryDatabaseSize(p.pgVersion)

	var db string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
		case "size":
			p.getDBMetrics(db).size = newInt(parseInt(value))
		}
	})
}

func (p *Postgres) doQueryDatabaseConflicts() error {
	q := queryDatabaseConflicts()

	var db string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
			p.getDBMetrics(db).updated = true
		case "confl_tablespace":
			p.getDBMetrics(db).conflTablespace = parseInt(value)
		case "confl_lock":
			p.getDBMetrics(db).conflLock = parseInt(value)
		case "confl_snapshot":
			p.getDBMetrics(db).conflSnapshot = parseInt(value)
		case "confl_bufferpin":
			p.getDBMetrics(db).conflBufferpin = parseInt(value)
		case "confl_deadlock":
			p.getDBMetrics(db).conflDeadlock = parseInt(value)
		}
	})
}

func (p *Postgres) doQueryDatabaseLocks() error {
	q := queryDatabaseLocks()

	var db, mode string
	var granted bool
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
			p.getDBMetrics(db).updated = true
		case "mode":
			mode = value
		case "granted":
			granted = value == "true" || value == "t"
		case "locks_count":
			// https://github.com/postgres/postgres/blob/7c34555f8c39eeefcc45b3c3f027d7a063d738fc/src/include/storage/lockdefs.h#L36-L45
			// https://www.postgresql.org/docs/7.2/locking-tables.html
			switch {
			case mode == "AccessShareLock" && granted:
				p.getDBMetrics(db).accessShareLockHeld = parseInt(value)
			case mode == "AccessShareLock":
				p.getDBMetrics(db).accessShareLockAwaited = parseInt(value)
			case mode == "RowShareLock" && granted:
				p.getDBMetrics(db).rowShareLockHeld = parseInt(value)
			case mode == "RowShareLock":
				p.getDBMetrics(db).rowShareLockAwaited = parseInt(value)
			case mode == "RowExclusiveLock" && granted:
				p.getDBMetrics(db).rowExclusiveLockHeld = parseInt(value)
			case mode == "RowExclusiveLock":
				p.getDBMetrics(db).rowExclusiveLockAwaited = parseInt(value)
			case mode == "ShareUpdateExclusiveLock" && granted:
				p.getDBMetrics(db).shareUpdateExclusiveLockHeld = parseInt(value)
			case mode == "ShareUpdateExclusiveLock":
				p.getDBMetrics(db).shareUpdateExclusiveLockAwaited = parseInt(value)
			case mode == "ShareLock" && granted:
				p.getDBMetrics(db).shareLockHeld = parseInt(value)
			case mode == "ShareLock":
				p.getDBMetrics(db).shareLockAwaited = parseInt(value)
			case mode == "ShareRowExclusiveLock" && granted:
				p.getDBMetrics(db).shareRowExclusiveLockHeld = parseInt(value)
			case mode == "ShareRowExclusiveLock":
				p.getDBMetrics(db).shareRowExclusiveLockAwaited = parseInt(value)
			case mode == "ExclusiveLock" && granted:
				p.getDBMetrics(db).exclusiveLockHeld = parseInt(value)
			case mode == "ExclusiveLock":
				p.getDBMetrics(db).exclusiveLockAwaited = parseInt(value)
			case mode == "AccessExclusiveLock" && granted:
				p.getDBMetrics(db).accessExclusiveLockHeld = parseInt(value)
			case mode == "AccessExclusiveLock":
				p.getDBMetrics(db).accessExclusiveLockAwaited = parseInt(value)
			}
		}
	})
}
