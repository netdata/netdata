// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_GetDeadlockInfoFunctionEnabled(t *testing.T) {
	tests := []struct {
		name     string
		cfg      Config
		expected bool
	}{
		{
			name:     "default nil pointer enables function",
			cfg:      Config{},
			expected: true,
		},
		{
			name: "explicit true enables function",
			cfg: Config{
				DeadlockInfoFunctionEnabled: boolPtr(true),
			},
			expected: true,
		},
		{
			name: "explicit false disables function",
			cfg: Config{
				DeadlockInfoFunctionEnabled: boolPtr(false),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.cfg.GetDeadlockInfoFunctionEnabled())
		})
	}
}

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

func TestCollector_collectDeadlockInfo_PermissionDenied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	collr := New()
	collr.db = db

	permErr := &mysqlDriver.MySQLError{
		Number:  1227,
		Message: "Access denied; you need (at least one of) the PROCESS privilege(s) for this operation",
	}
	mock.ExpectQuery(queryShowEngineInnoDBStatus).WillReturnError(permErr)

	resp := collr.collectDeadlockInfo(context.Background())
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.Status)
	assert.Contains(t, resp.Message, "PROCESS")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildDeadlockRows(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 123456000, time.UTC)
	res := parseInnoDBDeadlock(sampleDeadlockStatus, now)
	deadlockID := generateDeadlockID(now)
	rows := buildDeadlockRows(res, deadlockID)

	assert.Len(t, rows, 2)
	assert.Equal(t, deadlockID, rows[0][deadlockIdxDeadlockID])
	assert.Equal(t, deadlockID, rows[1][deadlockIdxDeadlockID])
	assert.Equal(t, deadlockID+":10", rows[0][deadlockIdxRowID])
	assert.Equal(t, deadlockID+":11", rows[1][deadlockIdxRowID])
}

func boolPtr(v bool) *bool {
	return &v
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

const sampleDeadlockStatusMalformed = `
------------------------
LATEST DETECTED DEADLOCK
------------------------
THIS IS NOT A VALID DEADLOCK SECTION
`
