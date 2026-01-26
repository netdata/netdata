// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	mssqlDriver "github.com/microsoft/go-mssqldb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_GetDeadlockInfoFunctionEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{
			name: "default enabled when unset",
			cfg:  Config{},
			want: true,
		},
		{
			name: "explicitly enabled",
			cfg: Config{
				DeadlockInfoFunctionEnabled: boolPtr(true),
			},
			want: true,
		},
		{
			name: "explicitly disabled",
			cfg: Config{
				DeadlockInfoFunctionEnabled: boolPtr(false),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.GetDeadlockInfoFunctionEnabled())
		})
	}
}

func TestParseDeadlockGraph_WithDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 123456000, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockGraph, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Equal(t, now.UTC(), res.deadlockTime)
	require.Equal(t, "process1", res.victimProcessID)
	require.Len(t, res.transactions, 2)

	txn1 := findTxn(res.transactions, "process1")
	txn2 := findTxn(res.transactions, "process2")

	require.NotNil(t, txn1)
	require.NotNil(t, txn2)

	assert.Equal(t, "WAITING", txn1.lockStatus)
	assert.Equal(t, "X", txn1.lockMode)
	assert.Contains(t, txn1.queryText, "deadlock_a")

	assert.Equal(t, "WAITING", txn2.lockStatus)
	assert.Equal(t, "X", txn2.lockMode)
	assert.Contains(t, txn2.queryText, "deadlock_b")
}

func TestParseDeadlockGraph_ThreeWayDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 222000000, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockGraphThreeWay, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)
	require.Equal(t, "process2", res.victimProcessID)
	require.Len(t, res.transactions, 3)

	deadlockID := generateDeadlockID(now)
	rows := buildDeadlockRows(res, deadlockID, map[int]string{5: "netdata"})
	require.Len(t, rows, 3)

	victimCount := 0
	for _, row := range rows {
		assert.Equal(t, deadlockID, row[deadlockIdxDeadlockID])
		assert.True(t, strings.HasPrefix(row[deadlockIdxRowID].(string), deadlockID+":"))
		if row[deadlockIdxIsVictim] == "true" {
			victimCount++
		}
	}
	assert.Equal(t, 1, victimCount)
}

func TestParseDeadlockGraph_WaitingWinsOverOwner(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockOwnerAfterWaiter, now)

	require.True(t, res.found)
	require.NoError(t, res.parseErr)

	txn := findTxn(res.transactions, "process1")
	require.NotNil(t, txn)
	assert.Equal(t, "WAITING", txn.lockStatus)
	assert.Equal(t, "X", txn.lockMode)
}

func TestParseDeadlockGraph_NoDeadlock(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseDeadlockGraph("", now)

	assert.False(t, res.found)
	assert.NoError(t, res.parseErr)
	assert.Len(t, res.transactions, 0)
}

func TestParseDeadlockGraph_Malformed(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)
	res := parseDeadlockGraph("<deadlock><broken>", now)

	assert.True(t, res.found)
	assert.Error(t, res.parseErr)
}

func TestCollectDeadlockInfo_ParseError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	deadlockTime := time.Date(2026, time.January, 25, 12, 34, 56, 0, time.UTC)
	deadlockRows := sqlmock.NewRows([]string{"deadlock_time", "deadlock_xml"}).
		AddRow(deadlockTime, "<deadlock><broken>")
	mock.ExpectQuery("WITH xevents").WillReturnRows(deadlockRows)

	dbNameRows := sqlmock.NewRows([]string{"database_id", "name"})
	mock.ExpectQuery("SELECT\\s+database_id").WillReturnRows(dbNameRows)

	c := New()
	c.db = db

	resp := c.collectDeadlockInfo(context.Background())
	require.Equal(t, deadlockParseErrorStatus, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "could not be parsed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("WITH xevents").
		WillReturnError(errors.New("boom"))

	c := New()
	c.db = db

	resp := c.collectDeadlockInfo(context.Background())
	require.Equal(t, 500, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "deadlock query failed")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBuildDeadlockRows(t *testing.T) {
	now := time.Date(2026, time.January, 25, 12, 0, 0, 654321000, time.UTC)
	res := parseDeadlockGraph(sampleDeadlockGraph, now)
	require.NoError(t, res.parseErr)

	deadlockID := generateDeadlockID(now)
	dbNames := map[int]string{5: "netdata"}
	rows := buildDeadlockRows(res, deadlockID, dbNames)

	require.Len(t, rows, 2)

	row := rows[0]
	assert.Equal(t, deadlockID, row[deadlockIdxDeadlockID])
	assert.Equal(t, deadlockID+":"+row[deadlockIdxProcessID].(string), row[deadlockIdxRowID])
	assert.Equal(t, "netdata", row[deadlockIdxDatabase])
}

func TestDeadlockPermissionErrorDetection(t *testing.T) {
	err := mssqlDriver.Error{Number: 297, Message: "VIEW SERVER STATE permission was denied"}
	assert.True(t, isDeadlockPermissionError(err))
}

func TestCollectDeadlockInfo_PermissionDenied(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("WITH xevents").
		WillReturnError(mssqlDriver.Error{Number: 297, Message: "VIEW SERVER STATE permission was denied"})

	c := New()
	c.db = db

	resp := c.collectDeadlockInfo(context.Background())
	require.Equal(t, 403, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "view server state")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_Disabled(t *testing.T) {
	c := New()
	c.Config.DeadlockInfoFunctionEnabled = boolPtr(false)

	resp := c.collectDeadlockInfo(context.Background())
	require.Equal(t, 503, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "disabled")
}

func TestCollectDeadlockInfo_Timeout(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("WITH xevents").
		WillReturnError(context.DeadlineExceeded)

	c := New()
	c.db = db

	resp := c.collectDeadlockInfo(context.Background())
	require.Equal(t, 504, resp.Status)
	assert.Contains(t, strings.ToLower(resp.Message), "timed out")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCollectDeadlockInfo_Success(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer db.Close()

	now := time.Date(2026, time.January, 25, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery("WITH xevents").
		WillReturnRows(
			sqlmock.NewRows([]string{"deadlock_time", "deadlock_xml"}).
				AddRow(now, sampleDeadlockGraph),
		)

	mock.ExpectQuery("SELECT database_id, name").
		WillReturnRows(
			sqlmock.NewRows([]string{"database_id", "name"}).
				AddRow(5, "netdata"),
		)

	c := New()
	c.db = db

	resp := c.collectDeadlockInfo(context.Background())
	require.Equal(t, 200, resp.Status)
	require.NotEmpty(t, resp.Data)
	require.NoError(t, mock.ExpectationsWereMet())
}

func findTxn(txns []*mssqlDeadlockTxn, id string) *mssqlDeadlockTxn {
	for _, txn := range txns {
		if txn.processID == id {
			return txn
		}
	}
	return nil
}

func boolPtr(v bool) *bool { return &v }

const sampleDeadlockGraph = `
<deadlock>
  <victim-list>
    <victimProcess id="process1" />
  </victim-list>
  <process-list>
    <process id="process1" spid="62" ecid="0" dbid="5" lockMode="S" waitresource="KEY: 5:111">
      <inputbuf>UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
    <process id="process2" spid="63" ecid="0" dbid="5" lockMode="X" waitresource="KEY: 5:222">
      <inputbuf>UPDATE dbo.deadlock_b SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
  </process-list>
  <resource-list>
    <keylock dbid="5">
      <owner-list>
        <owner id="process1" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process2" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process2" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process1" mode="X" />
      </waiter-list>
    </keylock>
  </resource-list>
</deadlock>
`

const sampleDeadlockOwnerAfterWaiter = `
<deadlock>
  <victim-list>
    <victimProcess id="process1" />
  </victim-list>
  <process-list>
    <process id="process1" spid="62" ecid="0" dbid="5" lockMode="S" waitresource="KEY: 5:111">
      <inputbuf>UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
  </process-list>
  <resource-list>
    <keylock dbid="5">
      <owner-list>
        <owner id="process2" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process1" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process1" mode="S" />
      </owner-list>
    </keylock>
  </resource-list>
</deadlock>
`

const sampleDeadlockGraphThreeWay = `
<deadlock>
  <victim-list>
    <victimProcess id="process2" />
  </victim-list>
  <process-list>
    <process id="process1" spid="62" ecid="0" dbid="5" lockMode="S" waitresource="KEY: 5:111">
      <inputbuf>UPDATE dbo.deadlock_a SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
    <process id="process2" spid="63" ecid="0" dbid="5" lockMode="X" waitresource="KEY: 5:222">
      <inputbuf>UPDATE dbo.deadlock_b SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
    <process id="process3" spid="64" ecid="0" dbid="5" lockMode="X" waitresource="KEY: 5:333">
      <inputbuf>UPDATE dbo.deadlock_c SET value = value + 1 WHERE id = 1</inputbuf>
    </process>
  </process-list>
  <resource-list>
    <keylock dbid="5">
      <owner-list>
        <owner id="process1" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process2" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process2" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process3" mode="X" />
      </waiter-list>
    </keylock>
    <keylock dbid="5">
      <owner-list>
        <owner id="process3" mode="S" />
      </owner-list>
      <waiter-list>
        <waiter id="process1" mode="X" />
      </waiter-list>
    </keylock>
  </resource-list>
</deadlock>
`
