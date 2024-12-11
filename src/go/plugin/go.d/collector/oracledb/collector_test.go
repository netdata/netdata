// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"bufio"
	"bytes"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer2130XESysMetric, _  = os.ReadFile("testdata/v21.3.0-xe/sysmetric.txt")
	dataVer2130XESysStat, _    = os.ReadFile("testdata/v21.3.0-xe/sysstat.txt")
	dataVer2130XETablespace, _ = os.ReadFile("testdata/v21.3.0-xe/tablespace.txt")
	dataVer2130XEWaitClass, _  = os.ReadFile("testdata/v21.3.0-xe/wait_class.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":          dataConfigJSON,
		"dataConfigYAML":          dataConfigYAML,
		"dataVer2130XESysMetric":  dataVer2130XESysMetric,
		"dataVer2130XESysStat":    dataVer2130XESysStat,
		"dataVer2130XETablespace": dataVer2130XETablespace,
		"dataVer2130XEWaitClass":  dataVer2130XEWaitClass,
	} {
		require.NotNil(t, data, name)
		if !strings.HasPrefix(name, "dataConfig") {
			_, err := prepareMockRows(data)
			require.NoError(t, err, fmt.Sprintf("prepare mock rows: %s", name))
		}
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"empty DSN": {
			config:   Config{DSN: ""},
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]func(t *testing.T) (collr *Collector, cleanup func()){
		"db connection not initialized": func(t *testing.T) (collr *Collector, cleanup func()) {
			return New(), func() {}
		},
		"db connection initialized": func(t *testing.T) (collr *Collector, cleanup func()) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)

			mock.ExpectClose()
			collr = New()
			collr.db = db
			cleanup = func() { _ = db.Close() }

			return collr, cleanup
		},
	}

	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := prepare(t)
			defer cleanup()

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			assert.Nil(t, collr.db)
		})
	}

}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		wantFail    bool
	}{
		"success on all queries": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, querySysMetrics, dataVer2130XESysMetric)
				mockExpect(t, m, querySysStat, dataVer2130XESysStat)
				mockExpect(t, m, queryWaitClass, dataVer2130XEWaitClass)
				mockExpect(t, m, queryTablespace, dataVer2130XETablespace)
			},
		},
		"fail if any query fails": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpectErr(m, querySysMetrics)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			collr := New()
			collr.DSN = "oracle://user:pass@127.0.0.1:32001/XE"
			collr.db = db
			defer func() { _ = db.Close() }()

			require.NoError(t, collr.Init(context.Background()))

			test.prepareMock(t, mock)

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		wantCharts  int
		wantMetrics map[string]int64
	}{
		"success on all queries": {
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpect(t, m, querySysMetrics, dataVer2130XESysMetric)
				mockExpect(t, m, querySysStat, dataVer2130XESysStat)
				mockExpect(t, m, queryWaitClass, dataVer2130XEWaitClass)
				mockExpect(t, m, queryTablespace, dataVer2130XETablespace)
			},
			wantCharts: len(globalCharts) + len(tablespaceChartsTmpl)*4 + len(waitClassChartsTmpl)*10,
			wantMetrics: map[string]int64{
				"Average Active Sessions":             93,
				"Buffer Cache Hit Ratio":              100000,
				"Cursor Cache Hit Ratio":              377385,
				"Database Wait Time Ratio":            0,
				"Global Cache Blocks Corrupted":       0,
				"Global Cache Blocks Lost":            0,
				"Library Cache Hit Ratio":             98779,
				"Row Cache Hit Ratio":                 99640,
				"SQL Service Response Time":           247,
				"Session Count":                       142000,
				"Session Limit %":                     7274,
				"enqueue timeouts":                    229,
				"execute count":                       4066130,
				"logons cumulative":                   8717,
				"logons current":                      93,
				"parse count (total)":                 1251128,
				"physical read bytes":                 538132480,
				"physical reads":                      65690,
				"physical write bytes":                785661952,
				"physical writes":                     95906,
				"sorts (disk)":                        0,
				"sorts (memory)":                      220071,
				"table scans (long tables)":           998,
				"table scans (short tables)":          798515,
				"tablespace_SYSAUX_avail_bytes":       215023616,
				"tablespace_SYSAUX_max_size_bytes":    912261120,
				"tablespace_SYSAUX_used_bytes":        697237504,
				"tablespace_SYSAUX_utilization":       76429,
				"tablespace_SYSTEM_avail_bytes":       5898240,
				"tablespace_SYSTEM_max_size_bytes":    1415577600,
				"tablespace_SYSTEM_used_bytes":        1409679360,
				"tablespace_SYSTEM_utilization":       99583,
				"tablespace_UNDOTBS1_avail_bytes":     114032640,
				"tablespace_UNDOTBS1_max_size_bytes":  125829120,
				"tablespace_UNDOTBS1_used_bytes":      11796480,
				"tablespace_UNDOTBS1_utilization":     9375,
				"tablespace_USERS_avail_bytes":        2424832,
				"tablespace_USERS_max_size_bytes":     5242880,
				"tablespace_USERS_used_bytes":         2818048,
				"tablespace_USERS_utilization":        53750,
				"user commits":                        16056,
				"user rollbacks":                      2,
				"wait_class_Administrative_wait_time": 0,
				"wait_class_Application_wait_time":    0,
				"wait_class_Commit_wait_time":         0,
				"wait_class_Concurrency_wait_time":    0,
				"wait_class_Configuration_wait_time":  0,
				"wait_class_Network_wait_time":        0,
				"wait_class_Other_wait_time":          0,
				"wait_class_Scheduler_wait_time":      0,
				"wait_class_System I/O_wait_time":     4,
				"wait_class_User I/O_wait_time":       0,
			},
		},
		"fail if any query fails": {
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				mockExpectErr(m, querySysMetrics)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			collr := New()
			collr.DSN = "oracle://user:pass@127.0.0.1:32001/XE"
			collr.db = db
			defer func() { _ = db.Close() }()

			require.NoError(t, collr.Init(context.Background()))

			test.prepareMock(t, mock)

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*collr.Charts()), "wantCharts")
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func mockExpect(t *testing.T, mock sqlmock.Sqlmock, query string, rows []byte) {
	mockRows, err := prepareMockRows(rows)
	require.NoError(t, err)
	mock.ExpectQuery(query).WillReturnRows(mockRows).RowsWillBeClosed()
}

func mockExpectErr(mock sqlmock.Sqlmock, query string) {
	mock.ExpectQuery(query).WillReturnError(fmt.Errorf("mock error (%s)", query))
}

func prepareMockRows(data []byte) (*sqlmock.Rows, error) {
	if len(data) == 0 {
		return sqlmock.NewRows(nil), nil
	}

	r := bytes.NewReader(data)
	sc := bufio.NewScanner(r)

	var numColumns int
	var rows *sqlmock.Rows

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}

		parts := strings.Split(line, "|")
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

	return rows, sc.Err()
}
