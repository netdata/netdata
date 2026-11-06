// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run against an isolated PostgreSQL cluster; this test creates databases and relations.
func TestRelationSelectionPostgreSQL(t *testing.T) {
	dsn := os.Getenv("NETDATA_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("NETDATA_POSTGRES_TEST_DSN must name an isolated test cluster")
	}
	ctx := context.Background()
	c := New()
	c.DSN = dsn
	c.DBSelector = "*"
	c.MaxDBTables, c.MaxDBIndexes = 1, 1
	require.NoError(t, c.Init(ctx))
	t.Cleanup(func() { c.Cleanup(ctx) })
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	var primaryName string
	require.NoError(t, db.QueryRow(`SELECT current_database()`).Scan(&primaryName))
	_, err = db.Exec(`DROP DATABASE IF EXISTS netdata_top_secondary WITH (FORCE)`)
	require.NoError(t, err)
	_, err = db.Exec(`DROP TABLE IF EXISTS a,b,c`)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE DATABASE netdata_top_secondary`)
	require.NoError(t, err)
	seed, connStr, err := c.openSecondaryConnection("netdata_top_secondary")
	require.NoError(t, err)
	_, err = seed.Exec(`CREATE TABLE a(id int PRIMARY KEY); CREATE TABLE b(id int PRIMARY KEY); CREATE TABLE c(id int PRIMARY KEY)`)
	closeDBAndUnregisterConnConfig(seed, connStr)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE a(id int PRIMARY KEY, value text); CREATE TABLE b(id int PRIMARY KEY, value text); CREATE TABLE c(id int PRIMARY KEY, value text); INSERT INTO a SELECT n, repeat('x',100) FROM generate_series(1,1000) n; ANALYZE a`)
	require.NoError(t, err)
	if os.Getenv("NETDATA_POSTGRES_QUERY_PLAN_DIR") != "" {
		_, err = db.Exec(`DROP SCHEMA IF EXISTS cardinality CASCADE; CREATE SCHEMA cardinality; DO $$ BEGIN FOR n IN 1..200 LOOP EXECUTE format('CREATE TABLE cardinality.t%s(id int PRIMARY KEY)',n); END LOOP; END $$`)
		require.NoError(t, err)
	}
	_, err = c.collect()
	require.NoError(t, err)
	assert.Empty(t, c.relationsFor(c.db).tables.selected)
	assert.Empty(t, c.relationsFor(c.db).indexes.selected)
	secondary := c.dbConns["netdata_top_secondary"]
	require.NotNil(t, secondary)
	require.NotNil(t, secondary.db)
	assert.Empty(t, c.relationsFor(secondary.db).tables.selected)
	assert.Empty(t, c.relationsFor(secondary.db).indexes.selected)
	secondaryWork, secondaryConnStr, err := c.openSecondaryConnection("netdata_top_secondary")
	require.NoError(t, err)
	_, err = secondaryWork.Exec(`INSERT INTO c SELECT n FROM generate_series(1,3000) n`)
	closeDBAndUnregisterConnConfig(secondaryWork, secondaryConnStr)
	require.NoError(t, err)
	// A separate workload connection publishes its statistics when it closes (also on PostgreSQL 14).
	work, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	_, err = work.Exec(`INSERT INTO b SELECT n, repeat('x',100) FROM generate_series(1,2000) n; SET enable_seqscan = off; DO $$ BEGIN FOR n IN 1..100 LOOP PERFORM * FROM a WHERE id=1; END LOOP; END $$`)
	require.NoError(t, err)
	require.NoError(t, work.Close())
	require.Eventually(t, func() bool {
		var inserts, scans int64
		if db.QueryRow(`SELECT n_tup_ins FROM pg_stat_user_tables WHERE relname='b'`).Scan(&inserts) != nil {
			return false
		}
		if db.QueryRow(`SELECT idx_scan FROM pg_stat_user_indexes WHERE indexrelname='a_pkey'`).Scan(&scans) != nil {
			return false
		}
		var secondaryInserts int64
		if secondary.db.QueryRow(`SELECT n_tup_ins FROM pg_stat_user_tables WHERE relname='c'`).Scan(&secondaryInserts) != nil {
			return false
		}
		return inserts >= 2000 && scans >= 100 && secondaryInserts >= 3000
	}, 10*time.Second, 100*time.Millisecond)
	mx, err := c.collect()
	require.NoError(t, err)
	r := c.relationsFor(c.db)
	require.Len(t, r.tables.selected, 1)
	require.Len(t, r.indexes.selected, 1)
	for _, row := range r.tables.selected {
		assert.Equal(t, "b", row.name)
	}
	for _, row := range r.indexes.selected {
		assert.Equal(t, "a_pkey", row.name)
		assert.Equal(t, "a", row.table)
	}
	for _, row := range c.relationsFor(secondary.db).tables.selected {
		assert.Equal(t, "c", row.name)
	}
	assert.Contains(t, mx, "table_c_db_netdata_top_secondary_schema_public_n_tup_ins")
	assert.NotContains(t, mx, "table_a_db_netdata_top_secondary_schema_public_n_tup_ins")
	assert.Contains(t, mx, "table_b_db_"+primaryName+"_schema_public_n_tup_ins")
	assert.NotContains(t, mx, "table_a_db_"+primaryName+"_schema_public_n_tup_ins")
	assert.Contains(t, mx, "index_a_pkey_table_a_db_"+primaryName+"_schema_public_size")
	// Exercise real bound SQL, including bloat for selected indexes on unselected tables.
	require.NoError(t, c.doDBQueryBloat(c.db))
	require.NoError(t, c.doDBQueryColumns(c.db))
	if dir := os.Getenv("NETDATA_POSTGRES_QUERY_PLAN_DIR"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0700))
		for name, test := range map[string]struct {
			query string
			args  []any
		}{
			"table_activity":    {query: queryRelationActivity(false)},
			"index_activity":    {query: queryRelationActivity(true)},
			"tables_unlimited":  {query: queryStatUserTables(false)},
			"tables_top":        {query: queryStatUserTables(true), args: []any{r.tables.oids()}},
			"io_unlimited":      {query: queryStatIOUserTables(false)},
			"io_top":            {query: queryStatIOUserTables(true), args: []any{r.tables.oids()}},
			"indexes_unlimited": {query: queryStatUserIndexes(false)},
			"indexes_top":       {query: queryStatUserIndexes(true), args: []any{r.indexes.oids()}},
			"bloat_unlimited":   {query: queryBloat(false)},
			"bloat_top":         {query: queryBloat(true), args: []any{r.tables.oids(), r.indexes.oids()}},
			"columns_unlimited": {query: queryColumnsStats(false)},
			"columns_top":       {query: queryColumnsStats(true), args: []any{r.tables.oids()}},
		} {
			rows, err := db.Query("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+test.query, test.args...)
			require.NoError(t, err)
			var plan strings.Builder
			for rows.Next() {
				var line string
				require.NoError(t, rows.Scan(&line))
				plan.WriteString(line)
			}
			require.NoError(t, rows.Err())
			require.NoError(t, rows.Close())
			require.NoError(t, os.WriteFile(filepath.Join(dir, name+".json"), []byte(plan.String()), 0600))
		}
	}
	// Mixed unlimited/bounded parameters must encode NULL versus an empty oid array correctly.
	c.MaxDBTables = 0
	require.NoError(t, c.doDBQueryBloat(c.db))
	require.NoError(t, c.doDBQueryStatUserTables(c.db))
	c.MaxDBTables, c.MaxDBIndexes = 1, 0
	require.NoError(t, c.doDBQueryBloat(c.db))
	require.NoError(t, c.doDBQueryStatUserIndexes(c.db))
	// Selection state and charts disappear after authoritative database discovery exclusion.
	_, err = db.Exec(`ALTER DATABASE netdata_top_secondary ALLOW_CONNECTIONS false`)
	require.NoError(t, err)
	require.NoError(t, c.doQueryQueryableDatabases())
	assert.NotContains(t, c.dbConns, "netdata_top_secondary")
	assert.NotContains(t, c.relations, secondary.db)
}
