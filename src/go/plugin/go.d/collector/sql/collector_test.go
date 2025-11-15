package sql

import (
	"context"
	"os"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
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
	const query = "SELECT 1 AS value"

	tests := map[string]struct {
		prepareMock func(t *testing.T, m sqlmock.Sqlmock)
		wantFail    bool
	}{
		"success when metrics collected": {
			wantFail: false,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{"value"}).AddRow("1")
				m.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()
			},
		},
		"error when query fails": {
			wantFail: true,
			prepareMock: func(t *testing.T, m sqlmock.Sqlmock) {
				m.ExpectQuery(query).WillReturnError(assert.AnError)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(
				sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual),
			)
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			collr := New()
			collr.db = db
			collr.Driver = "postgres"
			collr.DSN = "postgres://user:pass@localhost/db"
			collr.Metrics = []ConfigMetricBlock{
				{
					ID:    "m1",
					Mode:  "columns",
					Query: query,
					Charts: []ConfigChartConfig{
						{
							Title:   "test",
							Context: "pg.test",
							Units:   "events",
							Dims: []ConfigDimConfig{
								{Name: "value", Source: "value"},
							},
						},
					},
				},
			}

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
	t.Helper()

	type testCase struct {
		prepare func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector)
		check   func(t *testing.T, mx map[string]int64)
	}

	tests := map[string]testCase{
		"columns: metric columns without labels (bgwriter-like)": {
			prepare: func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector) {
				query := `
SELECT
  checkpoints_timed,
  checkpoints_req,
  checkpoint_write_time,
  checkpoint_sync_time,
  buffers_checkpoint_bytes,
  buffers_clean_bytes,
  maxwritten_clean,
  buffers_backend_bytes,
  buffers_backend_fsync,
  buffers_alloc_bytes
`
				rows := sqlmock.NewRows([]string{
					"checkpoints_timed",
					"checkpoints_req",
					"checkpoint_write_time",
					"checkpoint_sync_time",
					"buffers_checkpoint_bytes",
					"buffers_clean_bytes",
					"maxwritten_clean",
					"buffers_backend_bytes",
					"buffers_backend_fsync",
					"buffers_alloc_bytes",
				}).AddRow(
					"1814",
					"16",
					"167",
					"47",
					"32768",
					"0",
					"0",
					"0",
					"0",
					"27295744",
				)

				mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()

				coll.Driver = "postgres"
				coll.DSN = "postgres://user:pass@localhost/db"
				coll.Metrics = []ConfigMetricBlock{
					{
						ID:    "bgwriter",
						Mode:  "columns",
						Query: query,
						Charts: []ConfigChartConfig{
							{
								Title:   "bgwriter",
								Context: "pg.bgwriter",
								Units:   "bytes",
								Dims: []ConfigDimConfig{
									{Name: "checkpoints_timed", Source: "checkpoints_timed"},
									{Name: "checkpoints_req", Source: "checkpoints_req"},
									{Name: "checkpoint_write_time", Source: "checkpoint_write_time"},
									{Name: "checkpoint_sync_time", Source: "checkpoint_sync_time"},
									{Name: "buffers_checkpoint_bytes", Source: "buffers_checkpoint_bytes"},
									{Name: "buffers_clean_bytes", Source: "buffers_clean_bytes"},
									{Name: "maxwritten_clean", Source: "maxwritten_clean"},
									{Name: "buffers_backend_bytes", Source: "buffers_backend_bytes"},
									{Name: "buffers_backend_fsync", Source: "buffers_backend_fsync"},
									{Name: "buffers_alloc_bytes", Source: "buffers_alloc_bytes"},
								},
							},
						},
					},
				}
			},
			check: func(t *testing.T, mx map[string]int64) {
				chartID := "bgwriter_pg.bgwriter"

				expected := map[string]int64{
					buildDimID(chartID, "checkpoints_timed"):        1814,
					buildDimID(chartID, "checkpoints_req"):          16,
					buildDimID(chartID, "checkpoint_write_time"):    167,
					buildDimID(chartID, "checkpoint_sync_time"):     47,
					buildDimID(chartID, "buffers_checkpoint_bytes"): 32768,
					buildDimID(chartID, "buffers_clean_bytes"):      0,
					buildDimID(chartID, "maxwritten_clean"):         0,
					buildDimID(chartID, "buffers_backend_bytes"):    0,
					buildDimID(chartID, "buffers_backend_fsync"):    0,
					buildDimID(chartID, "buffers_alloc_bytes"):      27295744,
				}

				for k, want := range expected {
					got, ok := mx[k]
					require.True(t, ok, "expected metric %s", k)
					assert.EqualValues(t, want, got, "metric %s", k)
				}
			},
		},

		"columns: metrics as columns with label (datname)": {
			prepare: func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector) {
				query := `
SELECT
  datname,
  confl_tablespace,
  confl_lock,
  confl_snapshot,
  confl_bufferpin,
  confl_deadlock
`
				rows := sqlmock.NewRows([]string{
					"datname",
					"confl_tablespace",
					"confl_lock",
					"confl_snapshot",
					"confl_bufferpin",
					"confl_deadlock",
				}).
					AddRow("postgres", "0", "0", "0", "0", "0").
					AddRow("production", "0", "0", "0", "0", "0")

				mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()

				coll.Driver = "postgres"
				coll.DSN = "postgres://user:pass@localhost/db"
				coll.Metrics = []ConfigMetricBlock{
					{
						ID:    "conflicts",
						Mode:  "columns",
						Query: query,
						LabelsFromRow: []ConfigLabelFromRow{
							{Source: "datname", Name: "db"},
						},
						Charts: []ConfigChartConfig{
							{
								Title:   "conflicts",
								Context: "pg.conflicts",
								Units:   "conflicts",
								Dims: []ConfigDimConfig{
									{Name: "confl_tablespace", Source: "confl_tablespace"},
									{Name: "confl_lock", Source: "confl_lock"},
									{Name: "confl_snapshot", Source: "confl_snapshot"},
									{Name: "confl_bufferpin", Source: "confl_bufferpin"},
									{Name: "confl_deadlock", Source: "confl_deadlock"},
								},
							},
						},
					},
				}
			},
			check: func(t *testing.T, mx map[string]int64) {
				chartPostgres := "conflicts_pg.conflicts_postgres"
				chartProduction := "conflicts_pg.conflicts_production"

				keys := []string{
					"confl_tablespace",
					"confl_lock",
					"confl_snapshot",
					"confl_bufferpin",
					"confl_deadlock",
				}

				for _, dim := range keys {
					k1 := buildDimID(chartPostgres, dim)
					k2 := buildDimID(chartProduction, dim)

					v1, ok1 := mx[k1]
					v2, ok2 := mx[k2]

					require.True(t, ok1, "expected metric %s", k1)
					require.True(t, ok2, "expected metric %s", k2)

					assert.EqualValues(t, 0, v1, "metric %s", k1)
					assert.EqualValues(t, 0, v2, "metric %s", k2)
				}
			},
		},

		"columns: single column table": {
			prepare: func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector) {
				query := `SELECT extract`
				rows := sqlmock.NewRows([]string{"extract"}).
					AddRow("499906.075943")

				mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()

				coll.Driver = "postgres"
				coll.DSN = "postgres://user:pass@localhost/db"
				coll.Metrics = []ConfigMetricBlock{
					{
						ID:    "uptime",
						Mode:  "columns",
						Query: query,
						Charts: []ConfigChartConfig{
							{
								Title:   "uptime",
								Context: "pg.uptime",
								Units:   "seconds",
								Dims: []ConfigDimConfig{
									{Name: "extract", Source: "extract"},
								},
							},
						},
					},
				}
			},
			check: func(t *testing.T, mx map[string]int64) {
				chartID := "uptime_pg.uptime"
				key := buildDimID(chartID, "extract")

				got, ok := mx[key]
				require.True(t, ok, "expected metric %s", key)
				// 499906.075943 -> 499906 after truncation
				assert.EqualValues(t, 499906, got)
			},
		},

		"kv: metric names as row keys (state -> count)": {
			prepare: func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector) {
				query := `
SELECT
  state,
  count
`
				rows := sqlmock.NewRows([]string{"state", "count"}).
					AddRow("active", "1").
					AddRow("idle", "14").
					AddRow("idle in transaction", "7").
					AddRow("idle in transaction (aborted)", "1").
					AddRow("fastpath function call", "1").
					AddRow("disabled", "1")

				mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()

				coll.Driver = "postgres"
				coll.DSN = "postgres://user:pass@localhost/db"
				coll.Metrics = []ConfigMetricBlock{
					{
						ID:    "activity_states",
						Mode:  "kv",
						Query: query,
						KVMode: &ConfigKVMode{
							NameCol:  "state",
							ValueCol: "count",
						},
						Charts: []ConfigChartConfig{
							{
								Title:   "activity states",
								Context: "pg.activity_states",
								Units:   "events",
								Dims: []ConfigDimConfig{
									{Name: "active", Source: "active"},
									{Name: "idle", Source: "idle"},
									{Name: "idle_in_transaction", Source: "idle in transaction"},
									{Name: "idle_in_transaction_aborted", Source: "idle in transaction (aborted)"},
									{Name: "fastpath_function_call", Source: "fastpath function call"},
									{Name: "disabled", Source: "disabled"},
								},
							},
						},
					},
				}
			},
			check: func(t *testing.T, mx map[string]int64) {
				chartID := "activity_states_pg.activity_states"

				expected := map[string]int64{
					buildDimID(chartID, "active"):                      1,
					buildDimID(chartID, "idle"):                        14,
					buildDimID(chartID, "idle_in_transaction"):         7,
					buildDimID(chartID, "idle_in_transaction_aborted"): 1,
					buildDimID(chartID, "fastpath_function_call"):      1,
					buildDimID(chartID, "disabled"):                    1,
				}

				for k, want := range expected {
					got, ok := mx[k]
					require.True(t, ok, "expected metric %s", k)
					assert.EqualValues(t, want, got, "metric %s", k)
				}
			},
		},

		"columns: state-type metric with label from state": {
			prepare: func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector) {
				query := `
SELECT
  datname,
  state,
  xact_running_time,
  query_running_time
`
				rows := sqlmock.NewRows([]string{
					"datname",
					"state",
					"xact_running_time",
					"query_running_time",
				}).
					AddRow("some_db", "idle in transaction", "574.530219", "574.315061").
					AddRow("some_db", "idle in transaction", "574.867167", "574.330322").
					AddRow("postgres", "active", "0.000000", "0.000000").
					AddRow("some_db", "idle in transaction", "574.807256", "574.377105").
					AddRow("some_db", "idle in transaction", "574.680244", "574.357246").
					AddRow("some_db", "idle in transaction", "574.800283", "574.330328").
					AddRow("some_db", "idle in transaction", "574.396730", "574.290165").
					AddRow("some_db", "idle in transaction", "574.665428", "574.337164")

				mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()

				coll.Driver = "postgres"
				coll.DSN = "postgres://user:pass@localhost/db"
				coll.Metrics = []ConfigMetricBlock{
					{
						ID:    "xact_state",
						Mode:  "columns",
						Query: query,
						LabelsFromRow: []ConfigLabelFromRow{
							{Source: "state", Name: "state"},
						},
						Charts: []ConfigChartConfig{
							{
								Title:   "activity",
								Context: "pg.activity",
								Units:   "seconds",
								Dims: []ConfigDimConfig{
									{Name: "xact_running_time", Source: "xact_running_time"},
									{Name: "query_running_time", Source: "query_running_time"},
								},
							},
						},
					},
				}
			},
			check: func(t *testing.T, mx map[string]int64) {
				// Chart instances are split by state label.
				idleChartID := "xact_state_pg.activity_idle in transaction"
				activeChartID := "xact_state_pg.activity_active"

				xactIdleKey := buildDimID(idleChartID, "xact_running_time")
				queryIdleKey := buildDimID(idleChartID, "query_running_time")
				xactActiveKey := buildDimID(activeChartID, "xact_running_time")
				queryActiveKey := buildDimID(activeChartID, "query_running_time")

				// idle in transaction: aggregated sum over multiple rows -> > 0
				xIdle, ok := mx[xactIdleKey]
				require.True(t, ok, "expected metric %s", xactIdleKey)
				assert.Greater(t, xIdle, int64(0))

				qIdle, ok := mx[queryIdleKey]
				require.True(t, ok, "expected metric %s", queryIdleKey)
				assert.Greater(t, qIdle, int64(0))

				// active row has 0 durations.
				xAct, ok := mx[xactActiveKey]
				require.True(t, ok, "expected metric %s", xactActiveKey)
				assert.EqualValues(t, 0, xAct)

				qAct, ok := mx[queryActiveKey]
				require.True(t, ok, "expected metric %s", queryActiveKey)
				assert.EqualValues(t, 0, qAct)
			},
		},

		"columns: state-type metric with state mapped to metrics": {
			prepare: func(t *testing.T, mock sqlmock.Sqlmock, coll *Collector) {
				query := `
SELECT
  datname,
  state,
  xact_running_time,
  query_running_time
`
				rows := sqlmock.NewRows([]string{
					"datname",
					"state",
					"xact_running_time",
					"query_running_time",
				}).
					AddRow("some_db", "idle in transaction", "574.530219", "574.315061").
					AddRow("some_db", "idle in transaction", "574.867167", "574.330322").
					AddRow("postgres", "active", "0.000000", "0.000000").
					AddRow("some_db", "idle in transaction", "574.807256", "574.377105").
					AddRow("some_db", "idle in transaction", "574.680244", "574.357246").
					AddRow("some_db", "idle in transaction", "574.800283", "574.330328").
					AddRow("some_db", "idle in transaction", "574.396730", "574.290165").
					AddRow("some_db", "idle in transaction", "574.665428", "574.337164")

				mock.ExpectQuery(query).WillReturnRows(rows).RowsWillBeClosed()

				coll.Driver = "postgres"
				coll.DSN = "postgres://user:pass@localhost/db"
				coll.Metrics = []ConfigMetricBlock{
					{
						ID:    "xact_state_map",
						Mode:  "columns",
						Query: query,
						Charts: []ConfigChartConfig{
							{
								Title:   "activity",
								Context: "pg.activity",
								Units:   "status",
								Dims: []ConfigDimConfig{
									{
										Name:   "state_active",
										Source: "state",
										StatusWhen: &ConfigStatusWhen{
											Equals: "active",
										},
									},
									{
										Name:   "state_idle_in_transaction",
										Source: "state",
										StatusWhen: &ConfigStatusWhen{
											Equals: "idle in transaction",
										},
									},
								},
							},
						},
					},
				}
			},
			check: func(t *testing.T, mx map[string]int64) {
				chartID := "xact_state_map_pg.activity"

				activeKey := buildDimID(chartID, "state_active")
				idleKey := buildDimID(chartID, "state_idle_in_transaction")

				active, ok := mx[activeKey]
				require.True(t, ok, "expected metric %s", activeKey)
				assert.EqualValues(t, 1, active)

				idle, ok := mx[idleKey]
				require.True(t, ok, "expected metric %s", idleKey)
				assert.EqualValues(t, 1, idle)
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			coll := New()
			coll.db = db

			tt.prepare(t, mock, coll)

			require.NoError(t, coll.Init(context.Background()))

			mx := coll.Collect(context.Background())
			require.NotNil(t, mx)

			tt.check(t, mx)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestRedactDSN(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"simple user:pass@host": {
			input:    "user:password@localhost",
			expected: "user:****@localhost",
		},
		"simple user:pass@host:port": {
			input:    "user:password@localhost:5432",
			expected: "user:****@localhost:5432",
		},
		"no credentials just host": {
			input:    "localhost:5432",
			expected: "localhost:5432",
		},
		"URL without credentials": {
			input:    "postgresql://localhost/dbname",
			expected: "postgresql://localhost/dbname",
		},
		"colon in host but no password": {
			input:    "localhost:5432/db",
			expected: "localhost:5432/db",
		},
		"empty string": {
			input:    "",
			expected: "",
		},
		"just scheme": {
			input:    "postgresql://",
			expected: "postgresql://",
		},
		"simple user@host (no password)": {
			input:    "user@localhost",
			expected: "****@localhost",
		},
		"postgresql URL with password": {
			input:    "postgresql://user:password@localhost/dbname",
			expected: "postgresql://user:****@localhost/dbname",
		},
		"postgresql URL with password and port": {
			input:    "postgresql://user:password@localhost:5432/dbname",
			expected: "postgresql://user:****@localhost:5432/dbname",
		},
		"postgresql URL without password": {
			input:    "postgresql://user@localhost/dbname",
			expected: "postgresql://****@localhost/dbname",
		},
		"mysql URL with password": {
			input:    "mysql://root:secret@localhost:3306/mydb",
			expected: "mysql://root:****@localhost:3306/mydb",
		},
		"postgres URL with complex password": {
			input:    "postgres://admin:p@ss:w0rd@localhost/db",
			expected: "postgres://admin:****@localhost/db",
		},
		"URL with query params": {
			input:    "postgresql://user:pass@localhost/db?sslmode=disable",
			expected: "postgresql://user:****@localhost/db?sslmode=disable",
		},
		"user with special chars in password": {
			input:    "user:p@ssw0rd!@localhost",
			expected: "user:****@localhost",
		},
		"redis URL": {
			input:    "redis://user:password@localhost:6379/0",
			expected: "redis://user:****@localhost:6379/0",
		},
		"mongodb URL": {
			input:    "mongodb://admin:secret@localhost:27017/mydb",
			expected: "mongodb://admin:****@localhost:27017/mydb",
		},
		"URL with IP address": {
			input:    "postgresql://user:pass@192.168.1.1:5432/db",
			expected: "postgresql://user:****@192.168.1.1:5432/db",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := redactDSN(tc.input)

			assert.Equal(t, tc.expected, got)
		})
	}
}
