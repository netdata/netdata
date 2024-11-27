// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"fmt"
)

func (c *Collector) doQueryDatabasesMetrics() error {
	if err := c.doQueryDatabaseStats(); err != nil {
		return fmt.Errorf("querying database stats error: %v", err)
	}
	if err := c.doQueryDatabaseSize(); err != nil {
		return fmt.Errorf("querying database size error: %v", err)
	}
	if c.isPGInRecovery() {
		if err := c.doQueryDatabaseConflicts(); err != nil {
			return fmt.Errorf("querying database conflicts error: %v", err)
		}
	}
	if err := c.doQueryDatabaseLocks(); err != nil {
		return fmt.Errorf("querying database locks error: %v", err)
	}
	return nil
}

func (c *Collector) doQueryDatabaseStats() error {
	q := queryDatabaseStats()

	var db string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
			c.getDBMetrics(db).updated = true
		case "numbackends":
			c.getDBMetrics(db).numBackends = parseInt(value)
		case "datconnlimit":
			c.getDBMetrics(db).datConnLimit = parseInt(value)
		case "xact_commit":
			c.getDBMetrics(db).xactCommit = parseInt(value)
		case "xact_rollback":
			c.getDBMetrics(db).xactRollback = parseInt(value)
		case "blks_read_bytes":
			c.getDBMetrics(db).blksRead.last = parseInt(value)
		case "blks_hit_bytes":
			c.getDBMetrics(db).blksHit.last = parseInt(value)
		case "tup_returned":
			c.getDBMetrics(db).tupReturned.last = parseInt(value)
		case "tup_fetched":
			c.getDBMetrics(db).tupFetched.last = parseInt(value)
		case "tup_inserted":
			c.getDBMetrics(db).tupInserted = parseInt(value)
		case "tup_updated":
			c.getDBMetrics(db).tupUpdated = parseInt(value)
		case "tup_deleted":
			c.getDBMetrics(db).tupDeleted = parseInt(value)
		case "conflicts":
			c.getDBMetrics(db).conflicts = parseInt(value)
		case "temp_files":
			c.getDBMetrics(db).tempFiles = parseInt(value)
		case "temp_bytes":
			c.getDBMetrics(db).tempBytes = parseInt(value)
		case "deadlocks":
			c.getDBMetrics(db).deadlocks = parseInt(value)
		}
	})
}

func (c *Collector) doQueryDatabaseSize() error {
	q := queryDatabaseSize(c.pgVersion)

	var db string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
		case "size":
			c.getDBMetrics(db).size = newInt(parseInt(value))
		}
	})
}

func (c *Collector) doQueryDatabaseConflicts() error {
	q := queryDatabaseConflicts()

	var db string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
			c.getDBMetrics(db).updated = true
		case "confl_tablespace":
			c.getDBMetrics(db).conflTablespace = parseInt(value)
		case "confl_lock":
			c.getDBMetrics(db).conflLock = parseInt(value)
		case "confl_snapshot":
			c.getDBMetrics(db).conflSnapshot = parseInt(value)
		case "confl_bufferpin":
			c.getDBMetrics(db).conflBufferpin = parseInt(value)
		case "confl_deadlock":
			c.getDBMetrics(db).conflDeadlock = parseInt(value)
		}
	})
}

func (c *Collector) doQueryDatabaseLocks() error {
	q := queryDatabaseLocks()

	var db, mode string
	var granted bool
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			db = value
			c.getDBMetrics(db).updated = true
		case "mode":
			mode = value
		case "granted":
			granted = value == "true" || value == "t"
		case "locks_count":
			// https://github.com/postgres/postgres/blob/7c34555f8c39eeefcc45b3c3f027d7a063d738fc/src/include/storage/lockdefs.h#L36-L45
			// https://www.postgresql.org/docs/7.2/locking-tables.html
			switch {
			case mode == "AccessShareLock" && granted:
				c.getDBMetrics(db).accessShareLockHeld = parseInt(value)
			case mode == "AccessShareLock":
				c.getDBMetrics(db).accessShareLockAwaited = parseInt(value)
			case mode == "RowShareLock" && granted:
				c.getDBMetrics(db).rowShareLockHeld = parseInt(value)
			case mode == "RowShareLock":
				c.getDBMetrics(db).rowShareLockAwaited = parseInt(value)
			case mode == "RowExclusiveLock" && granted:
				c.getDBMetrics(db).rowExclusiveLockHeld = parseInt(value)
			case mode == "RowExclusiveLock":
				c.getDBMetrics(db).rowExclusiveLockAwaited = parseInt(value)
			case mode == "ShareUpdateExclusiveLock" && granted:
				c.getDBMetrics(db).shareUpdateExclusiveLockHeld = parseInt(value)
			case mode == "ShareUpdateExclusiveLock":
				c.getDBMetrics(db).shareUpdateExclusiveLockAwaited = parseInt(value)
			case mode == "ShareLock" && granted:
				c.getDBMetrics(db).shareLockHeld = parseInt(value)
			case mode == "ShareLock":
				c.getDBMetrics(db).shareLockAwaited = parseInt(value)
			case mode == "ShareRowExclusiveLock" && granted:
				c.getDBMetrics(db).shareRowExclusiveLockHeld = parseInt(value)
			case mode == "ShareRowExclusiveLock":
				c.getDBMetrics(db).shareRowExclusiveLockAwaited = parseInt(value)
			case mode == "ExclusiveLock" && granted:
				c.getDBMetrics(db).exclusiveLockHeld = parseInt(value)
			case mode == "ExclusiveLock":
				c.getDBMetrics(db).exclusiveLockAwaited = parseInt(value)
			case mode == "AccessExclusiveLock" && granted:
				c.getDBMetrics(db).accessExclusiveLockHeld = parseInt(value)
			case mode == "AccessExclusiveLock":
				c.getDBMetrics(db).accessExclusiveLockAwaited = parseInt(value)
			}
		}
	})
}
