// SPDX-License-Identifier: GPL-3.0-or-later

package pgbouncer

import (
	"bufio"
	"bytes"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataV170Version, _    = os.ReadFile("testdata/v1.7.0/version.txt")
	dataV1170Version, _   = os.ReadFile("testdata/v1.17.0/version.txt")
	dataV1170Config, _    = os.ReadFile("testdata/v1.17.0/config.txt")
	dataV1170Databases, _ = os.ReadFile("testdata/v1.17.0/databases.txt")
	dataV1170Pools, _     = os.ReadFile("testdata/v1.17.0/pools.txt")
	dataV1170Stats, _     = os.ReadFile("testdata/v1.17.0/stats.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataV170Version":    dataV170Version,
		"dataV1170Version":   dataV1170Version,
		"dataV1170Config":    dataV1170Config,
		"dataV1170Databases": dataV1170Databases,
		"dataV1170Pools":     dataV1170Pools,
		"dataV1170Stats":     dataV1170Stats,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestPgBouncer_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"Success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"Fail when DSN not set": {
			wantFail: true,
			config:   Config{DSN: ""},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p := New()
			p.Config = test.config

			if test.wantFail {
				assert.False(t, p.Init())
			} else {
				assert.True(t, p.Init())
			}
		})
	}
}

func TestPgBouncer_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestPgBouncer_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		wantFail    bool
	}{
		"Success when all queries are successful (v1.17.0)": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataV1170Version)
				mockExpect(t, m, queryShowConfig, dataV1170Config)
				mockExpect(t, m, queryShowDatabases, dataV1170Databases)
				mockExpect(t, m, queryShowStats, dataV1170Stats)
				mockExpect(t, m, queryShowPools, dataV1170Pools)
			},
		},
		"Fail when querying version returns an error": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpectErr(m, queryShowVersion)
			},
		},
		"Fail when querying version returns unsupported version": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataV170Version)
			},
		},
		"Fail when querying config returns an error": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, queryShowVersion, dataV1170Version)
				mockExpectErr(m, queryShowConfig)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			p := New()
			p.db = db
			defer func() { _ = db.Close() }()

			require.True(t, p.Init())

			test.prepareMock(t, mock)

			if test.wantFail {
				assert.False(t, p.Check())
			} else {
				assert.True(t, p.Check())
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPgBouncer_Collect(t *testing.T) {
	type testCaseStep struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		check       func(t *testing.T, p *PgBouncer)
	}
	tests := map[string][]testCaseStep{
		"Success on all queries (v1.17.0)": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataV1170Version)
					mockExpect(t, m, queryShowConfig, dataV1170Config)
					mockExpect(t, m, queryShowDatabases, dataV1170Databases)
					mockExpect(t, m, queryShowStats, dataV1170Stats)
					mockExpect(t, m, queryShowPools, dataV1170Pools)
				},
				check: func(t *testing.T, p *PgBouncer) {
					mx := p.Collect()

					expected := map[string]int64{
						"cl_conns_utilization":              47,
						"db_myprod1_avg_query_time":         575,
						"db_myprod1_avg_xact_time":          575,
						"db_myprod1_cl_active":              15,
						"db_myprod1_cl_cancel_req":          0,
						"db_myprod1_cl_waiting":             0,
						"db_myprod1_maxwait":                0,
						"db_myprod1_sv_active":              15,
						"db_myprod1_sv_conns_utilization":   0,
						"db_myprod1_sv_idle":                5,
						"db_myprod1_sv_login":               0,
						"db_myprod1_sv_tested":              0,
						"db_myprod1_sv_used":                0,
						"db_myprod1_total_query_count":      12683170,
						"db_myprod1_total_query_time":       7223566620,
						"db_myprod1_total_received":         809093651,
						"db_myprod1_total_sent":             1990971542,
						"db_myprod1_total_wait_time":        1029555,
						"db_myprod1_total_xact_count":       12683170,
						"db_myprod1_total_xact_time":        7223566620,
						"db_myprod2_avg_query_time":         581,
						"db_myprod2_avg_xact_time":          581,
						"db_myprod2_cl_active":              12,
						"db_myprod2_cl_cancel_req":          0,
						"db_myprod2_cl_waiting":             0,
						"db_myprod2_maxwait":                0,
						"db_myprod2_sv_active":              11,
						"db_myprod2_sv_conns_utilization":   0,
						"db_myprod2_sv_idle":                9,
						"db_myprod2_sv_login":               0,
						"db_myprod2_sv_tested":              0,
						"db_myprod2_sv_used":                0,
						"db_myprod2_total_query_count":      12538544,
						"db_myprod2_total_query_time":       7144226450,
						"db_myprod2_total_received":         799867464,
						"db_myprod2_total_sent":             1968267687,
						"db_myprod2_total_wait_time":        993313,
						"db_myprod2_total_xact_count":       12538544,
						"db_myprod2_total_xact_time":        7144226450,
						"db_pgbouncer_avg_query_time":       0,
						"db_pgbouncer_avg_xact_time":        0,
						"db_pgbouncer_cl_active":            2,
						"db_pgbouncer_cl_cancel_req":        0,
						"db_pgbouncer_cl_waiting":           0,
						"db_pgbouncer_maxwait":              0,
						"db_pgbouncer_sv_active":            0,
						"db_pgbouncer_sv_conns_utilization": 0,
						"db_pgbouncer_sv_idle":              0,
						"db_pgbouncer_sv_login":             0,
						"db_pgbouncer_sv_tested":            0,
						"db_pgbouncer_sv_used":              0,
						"db_pgbouncer_total_query_count":    45,
						"db_pgbouncer_total_query_time":     0,
						"db_pgbouncer_total_received":       0,
						"db_pgbouncer_total_sent":           0,
						"db_pgbouncer_total_wait_time":      0,
						"db_pgbouncer_total_xact_count":     45,
						"db_pgbouncer_total_xact_time":      0,
						"db_postgres_avg_query_time":        2790,
						"db_postgres_avg_xact_time":         2790,
						"db_postgres_cl_active":             18,
						"db_postgres_cl_cancel_req":         0,
						"db_postgres_cl_waiting":            0,
						"db_postgres_maxwait":               0,
						"db_postgres_sv_active":             18,
						"db_postgres_sv_conns_utilization":  0,
						"db_postgres_sv_idle":               2,
						"db_postgres_sv_login":              0,
						"db_postgres_sv_tested":             0,
						"db_postgres_sv_used":               0,
						"db_postgres_total_query_count":     25328823,
						"db_postgres_total_query_time":      72471882827,
						"db_postgres_total_received":        1615791619,
						"db_postgres_total_sent":            3976053858,
						"db_postgres_total_wait_time":       50439622253,
						"db_postgres_total_xact_count":      25328823,
						"db_postgres_total_xact_time":       72471882827,
					}

					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying version returns an error": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpectErr(m, queryShowVersion)
				},
				check: func(t *testing.T, p *PgBouncer) {
					mx := p.Collect()
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying version returns unsupported version": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataV170Version)
				},
				check: func(t *testing.T, p *PgBouncer) {
					mx := p.Collect()
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
		"Fail when querying config returns an error": {
			{
				prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
					mockExpect(t, m, queryShowVersion, dataV1170Version)
					mockExpectErr(m, queryShowConfig)
				},
				check: func(t *testing.T, p *PgBouncer) {
					mx := p.Collect()
					var expected map[string]int64
					assert.Equal(t, expected, mx)
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			p := New()
			p.db = db
			defer func() { _ = db.Close() }()

			require.True(t, p.Init())

			for i, step := range test {
				t.Run(fmt.Sprintf("step[%d]", i), func(t *testing.T) {
					step.prepareMock(t, mock)
					step.check(t, p)
				})
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func mockExpect(t *testing.T, mock sqlmock.Sqlmock, query string, rows []byte) {
	mock.ExpectQuery(query).WillReturnRows(mustMockRows(t, rows)).RowsWillBeClosed()
}

func mockExpectErr(mock sqlmock.Sqlmock, query string) {
	mock.ExpectQuery(query).WillReturnError(fmt.Errorf("mock error (%s)", query))
}

func mustMockRows(t *testing.T, data []byte) *sqlmock.Rows {
	rows, err := prepareMockRows(data)
	require.NoError(t, err)
	return rows
}

func prepareMockRows(data []byte) (*sqlmock.Rows, error) {
	r := bytes.NewReader(data)
	sc := bufio.NewScanner(r)

	var numColumns int
	var rows *sqlmock.Rows

	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "---") {
			continue
		}

		parts := strings.Split(s, "|")
		for i, v := range parts {
			parts[i] = strings.TrimSpace(v)
		}

		if rows == nil {
			numColumns = len(parts)
			rows = sqlmock.NewRows(parts)
			continue
		}

		if len(parts) != numColumns {
			return nil, fmt.Errorf("prepareMockRows(): columns != values (%d/%d)", numColumns, len(parts))
		}

		values := make([]driver.Value, len(parts))
		for i, v := range parts {
			values[i] = v
		}
		rows.AddRow(values...)
	}

	if rows == nil {
		return nil, errors.New("prepareMockRows(): nil rows result")
	}

	return rows, nil
}
