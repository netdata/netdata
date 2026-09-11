// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package mssql

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests execute the generated SQL, replacing only its external data sources with
// controlled rows. SQLmock cannot validate aggregation, window ordering or unit conversion.
// Run with MSSQL_DSN and go test -tags=integration -run TestIntegration_TopQueriesSQL.
func TestIntegration_TopQueriesSQL(t *testing.T) {
	db, err := sql.Open("sqlserver", getDSN(t))
	require.NoError(t, err)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))

	for name, source := range map[string]topQueriesSource{
		"plan cache":  topQueriesSourcePlanCache,
		"query store": topQueriesSourceQueryStore,
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			// This selects the database-local Query Store SQL without changing its aggregation.
			c.setServerProperties("12.0.2000.8", engineEditionAzureSQLDatabase)
			h := newFuncTopQueries(&funcRouter{collector: c})
			wanted := map[string]float64{
				"calls": 4, "totalTime": 930, "avgTime": 232.5,
				"lastTime": 10, "minTime": 5, "maxTime": 900,
				"avgCpu": 103, "lastCpu": 4,
				"avgRows": 4.75, "lastRows": 3,
				"avgDop": 2.75, "lastDop": 3,
			}
			var cols []topQueriesColumn
			for _, col := range topQueriesColumns {
				if _, ok := wanted[col.Name]; ok || col.Name == "query" || col.Name == "queryHash" {
					cols = append(cols, col)
				}
			}
			query := h.buildTopQueriesSQL(source, cols, "totalTime", 0, 10)
			fixtures := topQueriesPlanCacheSQLFixture
			if source == topQueriesSourcePlanCache {
				query = strings.NewReplacer(
					"sys.dm_exec_query_stats", "fixture_stats",
					"sys.dm_exec_sql_text(qs.sql_handle)", "(SELECT text, dbid FROM fixture_text WHERE sql_handle = qs.sql_handle)",
				).Replace(query)
			} else {
				fixtures = topQueriesQueryStoreSQLFixture
				query = strings.NewReplacer(
					"sys.query_store_runtime_stats_interval", "fixture_intervals",
					"sys.query_store_runtime_stats", "fixture_stats",
					"sys.query_store_query_text", "fixture_text",
					"sys.query_store_query", "fixture_queries",
					"sys.query_store_plan", "fixture_plans",
				).Replace(query)
			}
			rows, err := db.QueryContext(ctx, fixtures+query)
			require.NoError(t, err)
			defer rows.Close()
			data, err := h.scanDynamicRows(rows, cols)
			require.NoError(t, err)
			require.Len(t, data, 1)
			for i, col := range cols {
				switch col.Name {
				case "query":
					assert.Equal(t, "SELECT value FROM sample", data[0][i])
				case "queryHash":
					assert.Equal(t, "0x1122334455667788", data[0][i])
				default:
					assert.InDelta(t, wanted[col.Name], data[0][i], 0.00001, col.Name)
				}
			}
		})
	}
}

// The latest row has smaller last values but three executions. Totals must include both
// rows; averages must be weighted by execution count; last values must select only the latest.
const topQueriesPlanCacheSQLFixture = `
WITH fixture_stats AS (
  SELECT CAST(0x1122334455667788 AS binary(8)) AS query_hash,
         CAST(0x01 AS varbinary(64)) AS sql_handle, CAST(0x01 AS varbinary(64)) AS plan_handle,
         0 AS statement_start_offset, -1 AS statement_end_offset,
         CAST('2026-01-01T10:00:00' AS datetime) AS last_execution_time,
         CAST(1 AS bigint) AS execution_count,
         CAST(900000 AS bigint) AS total_elapsed_time, CAST(900000 AS bigint) AS last_elapsed_time,
         CAST(900000 AS bigint) AS min_elapsed_time, CAST(900000 AS bigint) AS max_elapsed_time,
         CAST(400000 AS bigint) AS total_worker_time, CAST(400000 AS bigint) AS last_worker_time,
         CAST(10 AS bigint) AS total_rows, CAST(10 AS bigint) AS last_rows,
         CAST(2 AS bigint) AS total_dop, CAST(2 AS bigint) AS last_dop
  UNION ALL
  SELECT 0x1122334455667788, 0x01, 0x02, 0, -1, '2026-01-01T11:00:00',
         3, 30000, 10000, 5000, 15000, 12000, 4000, 9, 3, 9, 3
  UNION ALL
  -- A newer system-database row must be excluded before ranking or aggregation.
  SELECT 0x1122334455667788, 0x02, 0x03, 0, -1, '2026-01-01T12:00:00',
         1, 800000, 800000, 800000, 800000, 300000, 300000, 8, 8, 8, 8
), fixture_text AS (
  SELECT CAST(0x01 AS varbinary(64)) AS sql_handle,
         CAST('SELECT value FROM sample' AS nvarchar(max)) AS text, CAST(NULL AS smallint) AS dbid
  UNION ALL
  SELECT 0x02, 'SELECT value FROM sample', 1
)
`

const topQueriesQueryStoreSQLFixture = `
WITH fixture_queries AS (
  SELECT 1 AS query_id, CAST(0x1122334455667788 AS binary(8)) AS query_hash, 1 AS query_text_id
  UNION ALL SELECT 2, 0x1122334455667788, 2
), fixture_text AS (
  SELECT 1 AS query_text_id, CAST('SELECT value FROM sample' AS nvarchar(max)) AS query_sql_text
  -- Equal text under different IDs must still aggregate as one query pattern.
  UNION ALL SELECT 2, 'SELECT value FROM sample'
), fixture_plans AS (
  SELECT 1 AS query_id, 1 AS plan_id UNION ALL SELECT 2, 2
), fixture_intervals AS (
  SELECT 1 AS runtime_stats_interval_id, CAST('2026-01-01T00:00:00+00:00' AS datetimeoffset) AS start_time
), fixture_stats AS (
  SELECT 1 AS plan_id, 1 AS runtime_stats_id, 1 AS runtime_stats_interval_id, 0 AS execution_type,
         CAST('2026-01-01T10:00:00+00:00' AS datetimeoffset) AS last_execution_time,
         CAST(1 AS bigint) AS count_executions,
         CAST(900000 AS float) AS avg_duration, CAST(900000 AS bigint) AS last_duration,
         CAST(900000 AS bigint) AS min_duration, CAST(900000 AS bigint) AS max_duration,
         CAST(400000 AS float) AS avg_cpu_time, CAST(400000 AS bigint) AS last_cpu_time,
         CAST(10 AS float) AS avg_rowcount, CAST(10 AS bigint) AS last_rowcount,
         CAST(2 AS float) AS avg_dop, CAST(2 AS bigint) AS last_dop
  UNION ALL
  SELECT 2, 2, 1, 0, '2026-01-01T11:00:00+00:00',
         3, 10000, 10000, 5000, 15000, 4000, 4000, 3, 3, 3, 3
)
`
