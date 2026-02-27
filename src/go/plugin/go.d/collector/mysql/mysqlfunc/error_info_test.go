// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestErrorInfoHandler(deps Deps) *funcErrorInfo {
	td, _ := deps.(*testDeps)
	router := &router{deps: deps}
	if td != nil {
		router.cfg = td.cfg
	}
	return newFuncErrorInfo(router)
}

func TestFuncErrorInfo_Handle_DBUnavailable(t *testing.T) {
	deps := newTestDeps()
	handler := newTestErrorInfoHandler(deps)

	resp := handler.Handle(context.Background(), errorInfoMethodID, nil)
	require.NotNil(t, resp)
	assert.Equal(t, 503, resp.Status)
	assert.Contains(t, resp.Message, "collector is still initializing")
}

func TestFuncErrorInfo_Handle_CleanupConcurrent(t *testing.T) {
	deps := newTestDeps()
	handler := newTestErrorInfoHandler(deps)

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			resp := handler.Handle(context.Background(), errorInfoMethodID, nil)
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

func TestFetchMySQLErrorRows_SyntheticDigestAndDedup(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT DIGEST, MYSQL_ERRNO, RETURNED_SQLSTATE, MESSAGE_TEXT, DIGEST_TEXT, SQL_TEXT FROM performance_schema.events_statements_history_long WHERE MYSQL_ERRNO <> 0 ORDER BY EVENT_ID DESC LIMIT 10",
	)).
		WillReturnRows(sqlmock.NewRows([]string{"DIGEST", "MYSQL_ERRNO", "RETURNED_SQLSTATE", "MESSAGE_TEXT", "DIGEST_TEXT", "SQL_TEXT"}).
			AddRow(nil, int64(1064), "42000", "syntax error", nil, "SELECT * FROM bad").
			AddRow(nil, int64(1064), "42000", "syntax error again", nil, "SELECT * FROM bad"). // duplicate synthetic digest
			AddRow("known-digest", int64(1213), "40001", "deadlock", "SELECT 2", nil).
			AddRow(nil, int64(1048), "23000", "null value", nil, nil)) // skipped: no digest/sql_text

	deps := newTestDeps()
	deps.setDB(db)

	source := mysqlErrorSource{
		table: "events_statements_history_long",
		columns: map[string]bool{
			"DIGEST":            true,
			"MYSQL_ERRNO":       true,
			"RETURNED_SQLSTATE": true,
			"MESSAGE_TEXT":      true,
			"DIGEST_TEXT":       true,
			"SQL_TEXT":          true,
			"EVENT_ID":          true,
		},
		status: mysqlErrorAttrEnabled,
	}

	rows, err := fetchMySQLErrorRows(context.Background(), deps, deps.cfg, source, nil, 10)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, generateSyntheticDigest("SELECT * FROM bad", 1064), rows[0].Digest)
	assert.Equal(t, "SELECT * FROM bad", rows[0].Query)
	require.NotNil(t, rows[0].ErrorNumber)
	assert.EqualValues(t, 1064, *rows[0].ErrorNumber)

	assert.Equal(t, "known-digest", rows[1].Digest)
	assert.Equal(t, "SELECT 2", rows[1].Query)
	require.NotNil(t, rows[1].ErrorNumber)
	assert.EqualValues(t, 1213, *rows[1].ErrorNumber)

	assert.NoError(t, mock.ExpectationsWereMet())
}
