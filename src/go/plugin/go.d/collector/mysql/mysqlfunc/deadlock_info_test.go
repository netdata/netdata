// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInnoDBDeadlock_WithDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 123456000, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatus, now)

	assert.True(t, res.found)
	assert.NoError(t, res.parseErr)
	assert.Len(t, res.transactions, 2)
	assert.Equal(t, 2, res.victimTxnNum)
	assert.Equal(t, now.UTC(), res.deadlockTime)

	assert.Equal(t, "10", res.transactions[0].threadID)
	assert.Equal(t, "11", res.transactions[1].threadID)
	assert.Equal(t, "WAITING", res.transactions[0].lockStatus)
	assert.Equal(t, "GRANTED", res.transactions[1].lockStatus)
}

func TestParseInnoDBDeadlock_MariaDBThreadID(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusMariaDB, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Len(t, res.transactions, 2)

	txnByNum := make(map[int]*mysqlDeadlockTxn, len(res.transactions))
	for _, txn := range res.transactions {
		txnByNum[txn.txnNum] = txn
	}

	require.Contains(t, txnByNum, 1)
	require.Contains(t, txnByNum, 2)

	assert.Equal(t, "55", txnByNum[1].threadID)
	assert.Contains(t, txnByNum[1].queryText, "deadlock_a")
	assert.NotEmpty(t, txnByNum[1].waitResource)
}

func TestParseInnoDBDeadlock_SkipsLockWaitLine(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusLockWaitLine, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Len(t, res.transactions, 1)

	txn := res.transactions[0]
	require.NotNil(t, txn)
	assert.Equal(t, "90", txn.threadID)
	assert.Equal(t, "UPDATE deadlock_a SET value = value + 1 WHERE id = 1", txn.queryText)
}

func TestParseInnoDBDeadlock_WaitingHeaderWithoutTxn(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusWaitNoTxn, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Len(t, res.transactions, 1)

	txn := res.transactions[0]
	require.NotNil(t, txn)
	assert.Equal(t, "99", txn.threadID)
	assert.Equal(t, "WAITING", txn.lockStatus)
	assert.Equal(t, "X", txn.lockMode)
	assert.NotEmpty(t, txn.waitResource)
}

func TestParseInnoDBDeadlock_HoldsBeforeWaiting(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusHoldsFirst, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Len(t, res.transactions, 2)

	var txn1 *mysqlDeadlockTxn
	for _, txn := range res.transactions {
		if txn.txnNum == 1 {
			txn1 = txn
			break
		}
	}

	require.NotNil(t, txn1, "transaction (1) should be present")
	assert.Equal(t, "WAITING", txn1.lockStatus)
	assert.Equal(t, "AUTO-INC", txn1.lockMode)
}

func TestParseDeadlockLockMode_HyphenAndUnderscore(t *testing.T) {
	mode, ok := parseDeadlockLockMode("lock mode AUTO-INC waiting")
	require.True(t, ok)
	assert.Equal(t, "AUTO-INC", mode)

	mode, ok = parseDeadlockLockMode("lock_mode AUTO_INC")
	require.True(t, ok)
	assert.Equal(t, "AUTO_INC", mode)
}

func TestParseInnoDBDeadlock_WithTimestamp(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusWithTimestamp, now)

	assert.True(t, res.found)
	assert.NoError(t, res.parseErr)
	assert.Equal(t, "2026-01-25 12:34:56", res.deadlockTime.In(time.Local).Format("2006-01-02 15:04:05"))
}

func TestParseInnoDBDeadlock_ThreeWay(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusThreeWay, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Len(t, res.transactions, 3)
	assert.Equal(t, 3, res.victimTxnNum)

	txnByNum := make(map[int]*mysqlDeadlockTxn, len(res.transactions))
	for _, txn := range res.transactions {
		txnByNum[txn.txnNum] = txn
	}

	require.Contains(t, txnByNum, 1)
	require.Contains(t, txnByNum, 2)
	require.Contains(t, txnByNum, 3)

	assert.Equal(t, "30", txnByNum[1].threadID)
	assert.Equal(t, "WAITING", txnByNum[1].lockStatus)

	assert.Equal(t, "31", txnByNum[2].threadID)
	assert.Equal(t, "GRANTED", txnByNum[2].lockStatus)

	assert.Equal(t, "32", txnByNum[3].threadID)
	// Victim fallback should mark transaction (3) as WAITING even without WAITING/HOLDS sections.
	assert.Equal(t, "WAITING", txnByNum[3].lockStatus)
}

func TestParseInnoDBDeadlock_WaitingWithoutLockMode(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusWaitingNoLockMode, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Len(t, res.transactions, 2)

	var txn1 *mysqlDeadlockTxn
	for _, txn := range res.transactions {
		if txn.txnNum == 1 {
			txn1 = txn
			break
		}
	}

	require.NotNil(t, txn1, "transaction (1) should be present")
	assert.Equal(t, "WAITING", txn1.lockStatus)
	assert.Empty(t, txn1.lockMode)
	assert.NotEmpty(t, txn1.waitResource)
}

func TestParseInnoDBDeadlock_NoDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock("no deadlock here", now)

	assert.False(t, res.found)
	assert.NoError(t, res.parseErr)
	assert.Len(t, res.transactions, 0)
}

func TestParseInnoDBDeadlock_MalformedSection(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatusMalformed, now)

	assert.True(t, res.found)
	assert.Error(t, res.parseErr)
	assert.Len(t, res.transactions, 0)
}

// newTestDeadlockHandler creates a funcDeadlockInfo handler for testing.
func newTestDeadlockHandler(deps Deps) *funcDeadlockInfo {
	td, _ := deps.(*testDeps)
	router := &router{deps: deps}
	if td != nil {
		router.cfg = td.cfg
	}
	return newFuncDeadlockInfo(router)
}

func TestFuncDeadlockInfo_collectData_ParseError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows := sqlmock.NewRows([]string{"Type", "Name", "Status"}).
		AddRow("InnoDB", "Status", sampleDeadlockStatusMalformed)
	mock.ExpectQuery(queryShowEngineInnoDBStatus).WillReturnRows(rows)

	deps := newTestDeps()
	deps.setDB(db)
	handler := newTestDeadlockHandler(deps)

	resp := handler.collectData(context.Background())
	require.NotNil(t, resp)
	assert.Equal(t, deadlockParseErrorStatus, resp.Status)
	assert.Contains(t, resp.Message, "could not be parsed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFuncDeadlockInfo_collectData_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(queryShowEngineInnoDBStatus).
		WillReturnError(errors.New("boom"))

	deps := newTestDeps()
	deps.setDB(db)
	handler := newTestDeadlockHandler(deps)

	resp := handler.collectData(context.Background())
	require.NotNil(t, resp)
	assert.Equal(t, 500, resp.Status)
	assert.Contains(t, resp.Message, "deadlock query failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFuncDeadlockInfo_collectData_Timeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(queryShowEngineInnoDBStatus).
		WillReturnError(context.DeadlineExceeded)

	deps := newTestDeps()
	deps.setDB(db)
	handler := newTestDeadlockHandler(deps)

	resp := handler.collectData(context.Background())
	require.NotNil(t, resp)
	assert.Equal(t, 504, resp.Status)
	assert.Contains(t, resp.Message, "timed out")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFuncDeadlockInfo_collectData_PermissionDenied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	deps := newTestDeps()
	deps.setDB(db)
	handler := newTestDeadlockHandler(deps)

	permErr := &mysqlDriver.MySQLError{
		Number:  1227,
		Message: "Access denied; you need (at least one of) the PROCESS privilege(s) for this operation",
	}
	mock.ExpectQuery(queryShowEngineInnoDBStatus).WillReturnError(permErr)

	resp := handler.collectData(context.Background())
	require.NotNil(t, resp)
	assert.Equal(t, 403, resp.Status)
	assert.Contains(t, resp.Message, "PROCESS")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFuncDeadlockInfo_collectData_Disabled(t *testing.T) {
	deps := newTestDeps()
	deps.cfg.DeadlockInfo.Disabled = true
	handler := newTestDeadlockHandler(deps)

	resp := handler.collectData(context.Background())
	require.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "disabled")
}

func TestFuncDeadlockInfo_Handle_DBUnavailable(t *testing.T) {
	deps := newTestDeps()
	handler := newTestDeadlockHandler(deps)

	resp := handler.Handle(context.Background(), deadlockInfoMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "collector is still initializing")
}

func TestFuncDeadlockInfo_Handle_CleanupConcurrent(t *testing.T) {
	deps := newTestDeps()
	handler := newTestDeadlockHandler(deps)

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			resp := handler.Handle(context.Background(), deadlockInfoMethodID, nil)
			require.NotNil(t, resp)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			deps.cleanup()
		}
	}()

	wg.Wait()
}

func TestBuildDeadlockRows(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 123456000, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatus, now)
	deadlockID := generateDeadlockID(now)
	rows := buildDeadlockRows(res, deadlockID)

	assert.Len(t, rows, 2)
	assert.Equal(t, deadlockID, rows[0].deadlockID)
	assert.Equal(t, deadlockID, rows[1].deadlockID)
	assert.Equal(t, deadlockID+":10", rows[0].rowID)
	assert.Equal(t, deadlockID+":11", rows[1].rowID)

	hasDatabase := false
	for _, row := range rows {
		if row.database == "netdata" {
			hasDatabase = true
			break
		}
	}
	assert.True(t, hasDatabase, "expected at least one row with database populated")
}

const sampleDeadlockStatus = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 100, ACTIVE 0 sec
MySQL thread id 10, OS thread handle 1, query id 100 localhost root updating
UPDATE Animals SET value = value + 1 WHERE name='Aardvark'
*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 1 page no 2 n bits 72 index PRIMARY of table netdata.Birds trx id 100 lock mode X waiting
*** (2) TRANSACTION:
TRANSACTION 101, ACTIVE 0 sec
MySQL thread id 11, OS thread handle 2, query id 101 localhost root updating
UPDATE Birds SET value = value + 1 WHERE name='Buzzard'
*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 1 page no 3 n bits 72 index PRIMARY of table netdata.Animals trx id 101 lock mode X
*** WE ROLL BACK TRANSACTION (2)
`

const sampleDeadlockStatusMariaDB = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 500, ACTIVE 1 sec
MariaDB thread id 55, OS thread handle 1, query id 500 localhost root updating
mysql tables in use 1, locked 1
UPDATE deadlock_a SET value = value + 1 WHERE id = 1
*** (1) waiting for this lock to be granted:
RECORD LOCKS space id 5 page no 6 n bits 72 index PRIMARY of table netdata.deadlock_a trx id 500 lock_mode X locks rec but not gap waiting
*** (2) TRANSACTION:
TRANSACTION 501, ACTIVE 1 sec
MariaDB thread id 56, OS thread handle 1, query id 501 localhost root updating
UPDATE deadlock_b SET value = value + 1 WHERE id = 1
*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 5 page no 6 n bits 72 index PRIMARY of table netdata.deadlock_a trx id 501 lock_mode X locks rec but not gap
*** WE ROLL BACK TRANSACTION (2)
`

const sampleDeadlockStatusLockWaitLine = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 900, ACTIVE 1 sec
MySQL thread id 90, OS thread handle 1, query id 900 localhost root updating
LOCK WAIT 4 lock struct(s), heap size 1128, 2 row lock(s), undo log entries 1
UPDATE deadlock_a SET value = value + 1 WHERE id = 1
*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 3 page no 4 n bits 72 index PRIMARY of table netdata.deadlock_a trx id 900 lock_mode X locks rec but not gap waiting
*** WE ROLL BACK TRANSACTION (1)
`

const sampleDeadlockStatusWaitNoTxn = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 990, ACTIVE 1 sec
MariaDB thread id 99, OS thread handle 1, query id 990 localhost root Updating
UPDATE deadlock_b SET value = value + 1 WHERE id = 1
*** WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 7 page no 3 n bits 320 index PRIMARY of table netdata.deadlock_b trx id 42 lock_mode X locks rec but not gap waiting
*** WE ROLL BACK TRANSACTION (1)
`

const sampleDeadlockStatusHoldsFirst = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 200, ACTIVE 0 sec
MySQL thread id 20, OS thread handle 1, query id 200 localhost root updating
UPDATE deadlock_b SET value = value + 1 WHERE id = 1
*** (1) HOLDS THE LOCK(S):
RECORD LOCKS space id 3 page no 4 n bits 72 index PRIMARY of table netdata.deadlock_a trx id 200 lock mode S
*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 4 page no 4 n bits 72 index PRIMARY of table netdata.deadlock_b trx id 200 lock mode AUTO-INC waiting
*** (2) TRANSACTION:
TRANSACTION 201, ACTIVE 0 sec
MySQL thread id 21, OS thread handle 2, query id 201 localhost root updating
UPDATE deadlock_a SET value = value + 1 WHERE id = 1
*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 4 page no 4 n bits 72 index PRIMARY of table netdata.deadlock_b trx id 201 lock mode X
*** WE ROLL BACK TRANSACTION (2)
`

const sampleDeadlockStatusWithTimestamp = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
2026-01-25 12:34:56
*** (1) TRANSACTION:
TRANSACTION 100, ACTIVE 0 sec
MySQL thread id 10, OS thread handle 1, query id 100 localhost root updating
UPDATE Animals SET value = value + 1 WHERE name='Aardvark'
*** (2) TRANSACTION:
TRANSACTION 101, ACTIVE 0 sec
MySQL thread id 11, OS thread handle 2, query id 101 localhost root updating
UPDATE Birds SET value = value + 1 WHERE name='Buzzard'
*** WE ROLL BACK TRANSACTION (2)
`

const sampleDeadlockStatusThreeWay = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 300, ACTIVE 0 sec
MySQL thread id 30, OS thread handle 1, query id 300 localhost root updating
UPDATE alpha SET value = value + 1 WHERE id = 1
*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 7 page no 8 n bits 72 index PRIMARY of table netdata.beta trx id 300 lock mode X waiting
*** (2) TRANSACTION:
TRANSACTION 301, ACTIVE 0 sec
MySQL thread id 31, OS thread handle 2, query id 301 localhost root updating
UPDATE beta SET value = value + 1 WHERE id = 1
*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 7 page no 9 n bits 72 index PRIMARY of table netdata.gamma trx id 301 lock mode S
*** (3) TRANSACTION:
TRANSACTION 302, ACTIVE 0 sec
MySQL thread id 32, OS thread handle 3, query id 302 localhost root updating
UPDATE gamma SET value = value + 1 WHERE id = 1
*** WE ROLL BACK TRANSACTION (3)
`

const sampleDeadlockStatusWaitingNoLockMode = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
*** (1) TRANSACTION:
TRANSACTION 400, ACTIVE 0 sec
MySQL thread id 40, OS thread handle 1, query id 400 localhost root updating
UPDATE delta SET value = value + 1 WHERE id = 1
*** (1) WAITING FOR THIS LOCK TO BE GRANTED:
RECORD LOCKS space id 10 page no 11 n bits 72 index PRIMARY of table netdata.epsilon trx id 400 waiting
*** (2) TRANSACTION:
TRANSACTION 401, ACTIVE 0 sec
MySQL thread id 41, OS thread handle 2, query id 401 localhost root updating
UPDATE epsilon SET value = value + 1 WHERE id = 1
*** (2) HOLDS THE LOCK(S):
RECORD LOCKS space id 10 page no 12 n bits 72 index PRIMARY of table netdata.delta trx id 401 lock mode X
*** WE ROLL BACK TRANSACTION (1)
`

const sampleDeadlockStatusMalformed = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
THIS IS NOT A VALID DEADLOCK SECTION
`
