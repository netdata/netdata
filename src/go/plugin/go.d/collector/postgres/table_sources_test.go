// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tableIO(oid uint32, name string, read, hit int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"datname", "relid", "schemaname", "relname", "heap_blks_read_bytes", "heap_blks_hit_bytes"}).AddRow("db", oid, "public", name, read, hit)
}

func TestTableSourcesIndependent(t *testing.T) {
	for _, limit := range []int64{0, 1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			c := New()
			require.NoError(t, c.Init(context.Background()))
			db, mock := newRelationsMock(t)
			c.db, c.MaxDBTables = db, limit
			c.relationsFor(db).tables.selected = activityRows(activity(1, "t1", 0))
			collect := func(stats, io *sqlmock.Rows) map[string]int64 {
				c.resetMetrics()
				s := mock.ExpectQuery(queryStatUserTables(limit > 0))
				i := mock.ExpectQuery(queryStatIOUserTables(limit > 0))
				if limit > 0 {
					s.WithArgs(sqlmock.AnyArg())
					i.WithArgs(sqlmock.AnyArg())
				}
				if stats == nil {
					s.WillReturnError(errors.New("statistics timeout"))
				} else {
					s.WillReturnRows(stats)
				}
				if io == nil {
					i.WillReturnError(errors.New("I/O timeout"))
				} else {
					i.WillReturnRows(io)
				}
				require.NoError(t, c.doQueryTablesMetrics())
				mx := make(map[string]int64)
				c.collectMetrics(mx)
				return mx
			}
			px := "table_t1_db_db_schema_public_"
			mx := collect(tableDetails(1, "t1", 100, 20), tableIO(1, "t1", 100, 200))
			assert.Equal(t, int64(100), mx[px+"heap_blks_read"])
			m := c.mx.tables["t1_db_public"]
			chart := c.Charts().Get("table_t1_db_db_schema_public_io_rate")
			require.NotNil(t, chart)
			mx = collect(nil, tableIO(1, "t1", 110, 240))
			assert.Equal(t, int64(110), mx[px+"heap_blks_read"])
			assert.Equal(t, int64(20), mx[px+"heap_blks_read_perc"])
			assert.NotContains(t, mx, px+"n_tup_upd")
			assert.NotContains(t, mx, px+"total_size")
			assert.False(t, chart.IsRemoved())
			assert.True(t, c.mx.tables["t1_db_public"].hasCharts)
			assert.Equal(t, len(m.charts), len(c.mx.tables["t1_db_public"].charts))
			mx = collect(tableDetails(1, "t1", 120, 30), tableIO(1, "t1", 120, 280))
			assert.Equal(t, int64(20), mx[px+"heap_blks_read_perc"])
			assert.NotContains(t, mx, px+"n_tup_hot_upd_perc")
			partial := tableIO(1, "t1", 9999, 9999).AddRow("db", 2, "public", "t2", 1, 1).RowError(1, errors.New("partial I/O"))
			mx = collect(tableDetails(1, "t1", 130, 35), partial)
			assert.Equal(t, int64(130), mx[px+"n_tup_upd"])
			assert.NotContains(t, mx, px+"heap_blks_read")
			assert.Equal(t, int64(120), c.mx.tables["t1_db_public"].heapBlksRead.last)
			mx = collect(nil, tableIO(1, "t1", 140, 360))
			assert.Equal(t, int64(140), mx[px+"heap_blks_read"])
			assert.NotContains(t, mx, px+"heap_blks_read_perc")
			mx = collect(nil, tableIO(1, "t1", 150, 400))
			assert.Equal(t, int64(20), mx[px+"heap_blks_read_perc"])
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestTableIOInitialObservationAndIdentity(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	db, mock := newRelationsMock(t)
	c.db, c.MaxDBTables = db, 0
	collect := func(stats *sqlmock.Rows, oid uint32, name string) map[string]int64 {
		c.resetMetrics()
		s := mock.ExpectQuery(queryStatUserTables(false))
		if stats == nil {
			s.WillReturnError(errors.New("statistics timeout"))
		} else {
			s.WillReturnRows(stats)
		}
		mock.ExpectQuery(queryStatIOUserTables(false)).WillReturnRows(tableIO(oid, name, 100, 200))
		require.NoError(t, c.doQueryTablesMetrics())
		mx := make(map[string]int64)
		c.collectMetrics(mx)
		return mx
	}
	px := "table_t1_db_db_schema_public_"
	mx := collect(nil, 1, "t1")
	assert.Equal(t, int64(100), mx[px+"heap_blks_read"])
	assert.NotContains(t, mx, px+"n_tup_upd")
	assert.NotContains(t, mx, px+"idx_blks_read") // No default zeros for unknown pairs.
	assert.Nil(t, c.Charts().Get("table_t1_db_db_schema_public_ops_rows_rate"))
	ioChart := c.Charts().Get("table_t1_db_db_schema_public_io_rate")
	require.NotNil(t, ioChart)
	mx = collect(tableDetails(1, "t1", 100, 20), 1, "t1")
	assert.Equal(t, int64(100), mx[px+"n_tup_upd"])
	assert.NotNil(t, c.Charts().Get("table_t1_db_db_schema_public_ops_rows_rate"))
	assert.Same(t, ioChart, c.Charts().Get("table_t1_db_db_schema_public_io_rate"))
	// A relation recreated between the successful stats and I/O queries must not replace that sample.
	mx = collect(tableDetails(1, "t1", 110, 25), 2, "t1")
	assert.Equal(t, int64(110), mx[px+"n_tup_upd"])
	assert.NotContains(t, mx, px+"heap_blks_read")
	assert.Equal(t, uint32(1), c.mx.tables["t1_db_public"].oid)
	// The same applies to rename between the queries, even though the OID stays the same.
	mx = collect(tableDetails(1, "t1", 120, 30), 1, "renamed")
	assert.NotContains(t, mx, "table_renamed_db_db_schema_public_heap_blks_read")
	assert.Len(t, c.mx.tables, 1)
	// A complete independent I/O observation can establish the new identity after stats fails.
	mx = collect(nil, 1, "renamed")
	assert.Equal(t, int64(100), mx["table_renamed_db_db_schema_public_heap_blks_read"])
	assert.NotContains(t, c.mx.tables, "t1_db_public")
	assert.True(t, ioChart.IsRemoved())
	old := c.mx.tables["renamed_db_public"]
	mx = collect(nil, 2, "renamed")
	assert.Equal(t, int64(100), mx["table_renamed_db_db_schema_public_heap_blks_read"])
	assert.NotContains(t, mx, "table_renamed_db_db_schema_public_heap_blks_read_perc")
	assert.Equal(t, uint32(2), c.mx.tables["renamed_db_public"].oid)
	assert.True(t, old.charts[0].IsRemoved())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTableSourcesDatabaseIsolation(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	primary, pm := newRelationsMock(t)
	secondary, sm := newRelationsMock(t)
	c.db, c.MaxDBTables = primary, 0
	c.dbConns["secondary"] = &dbConn{db: secondary}
	pm.ExpectQuery(queryStatUserTables(false)).WillReturnError(errors.New("primary stats timeout"))
	pm.ExpectQuery(queryStatIOUserTables(false)).WillReturnRows(tableIO(1, "t1", 100, 200))
	sm.ExpectQuery(queryStatUserTables(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "relid", "schemaname", "relname", "n_tup_upd"}).AddRow("secondary", 1, "public", "t1", 50))
	sm.ExpectQuery(queryStatIOUserTables(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "relid", "schemaname", "relname", "heap_blks_read_bytes", "heap_blks_hit_bytes"}).AddRow("secondary", 1, "public", "t1", 300, 400))
	require.NoError(t, c.doQueryTablesMetrics())
	mx := make(map[string]int64)
	c.collectMetrics(mx)
	assert.Equal(t, int64(100), mx["table_t1_db_db_schema_public_heap_blks_read"])
	assert.NotContains(t, mx, "table_t1_db_db_schema_public_n_tup_upd")
	assert.Equal(t, int64(50), mx["table_t1_db_secondary_schema_public_n_tup_upd"])
	assert.Equal(t, int64(300), mx["table_t1_db_secondary_schema_public_heap_blks_read"])
	assert.NoError(t, pm.ExpectationsWereMet())
	assert.NoError(t, sm.ExpectationsWereMet())
}
