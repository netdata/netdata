// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func activityRows(values ...relationActivity) map[uint32]relationActivity {
	rows := make(map[uint32]relationActivity, len(values))
	for _, value := range values {
		rows[value.oid] = value
	}
	return rows
}

func activity(oid uint32, name string, counters ...int64) relationActivity {
	a := relationActivity{oid: oid, tableOID: oid, database: "db", schema: "public", name: name}
	copy(a.counters[:], counters)
	return a
}

func TestRelationSelection(t *testing.T) {
	tests := map[string]struct {
		before, after map[uint32]relationActivity
		retained      []uint32
		reset         string
		want          uint32
	}{
		"recent activity beats history":       {before: activityRows(activity(1, "historical", 100000), activity(2, "active", 0)), after: activityRows(activity(1, "historical", 100000), activity(2, "active", 1)), want: 2},
		"all five row counters contribute":    {before: activityRows(activity(1, "a", 0), activity(2, "b", 0)), after: activityRows(activity(1, "a", 4), activity(2, "b", 1, 1, 1, 1, 1)), want: 2},
		"reset invalidates whole vector":      {before: activityRows(activity(1, "a", 100, 100), activity(2, "b", 0)), after: activityRows(activity(1, "a", 1000, 99), activity(2, "b", 1)), want: 2},
		"new object has no lifetime activity": {before: activityRows(activity(1, "a", 0)), after: activityRows(activity(1, "a", 1), activity(2, "b", 100000)), want: 1},
		"rename starts new baseline":          {before: activityRows(activity(1, "a", 0), activity(2, "b", 0)), after: activityRows(activity(1, "renamed", 100000), activity(2, "b", 1)), want: 2},
		"database reset starts new baseline":  {before: activityRows(activity(1, "a", 0), activity(2, "b", 0)), after: activityRows(activity(1, "a", 1), activity(2, "b", 100000)), reset: "new", want: 1},
		"equal activity keeps membership":     {before: activityRows(activity(1, "a", 0), activity(2, "b", 0)), after: activityRows(activity(1, "a", 0), activity(2, "b", 0)), retained: []uint32{2}, want: 2},
		"large integer delta stays precise":   {before: activityRows(activity(1, "a", 1<<60), activity(2, "b", 1<<60)), after: activityRows(activity(1, "a", (1<<60)+1), activity(2, "b", (1<<60)+2)), want: 2},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			now := time.Unix(1000, 0)
			s := relationSelection{observed: now, previous: test.before, selected: make(map[uint32]relationActivity)}
			for _, oid := range test.retained {
				s.selected[oid] = test.before[oid]
			}
			s.choose(test.after, test.reset, now.Add(17*time.Second), 1)
			assert.Equal(t, []uint32{test.want}, s.oids())
		})
	}
}

func TestRelationSelectionWarmup(t *testing.T) {
	now := time.Unix(1000, 0)
	rows := activityRows(activity(1, "a", 1000), activity(2, "b", 0))
	var oversized relationSelection
	oversized.choose(rows, "", now, 1)
	assert.Empty(t, oversized.selected)
	assert.False(t, oversized.warmed)
	oversized.choose(activityRows(activity(1, "a", 1000), activity(2, "b", 5)), "", now.Add(time.Second), 1)
	assert.Equal(t, []uint32{2}, oversized.oids())
	assert.True(t, oversized.warmed)
	var small relationSelection
	small.choose(rows, "", now, 2)
	assert.Len(t, small.selected, 2)
	assert.True(t, small.warmed)
	var largeLimit relationSelection
	largeLimit.choose(rows, "", now, 1<<40)
	assert.Len(t, largeLimit.selected, 2)
	assert.True(t, largeLimit.warmed)
}

// pgx accepts oid arrays; sqlmock's default database/sql converter does not.
type oidArrayConverter struct{}

func (oidArrayConverter) ConvertValue(v any) (driver.Value, error) {
	if _, ok := v.([]uint32); ok {
		return v, nil
	}
	return driver.DefaultParameterConverter.ConvertValue(v)
}

func newRelationsMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual), sqlmock.ValueConverterOption(oidArrayConverter{}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func tableInventory(counters ...int64) *sqlmock.Rows {
	r := sqlmock.NewRows([]string{"datname", "stats_reset", "relid", "schemaname", "relname", "seq_tup_read", "idx_tup_fetch", "n_tup_ins", "n_tup_upd", "n_tup_del"})
	for i, n := range counters {
		r.AddRow("db", "", i+1, "public", fmt.Sprintf("t%d", i+1), n, 0, 0, 0, 0)
	}
	return r
}

func indexInventory() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"datname", "stats_reset", "relid", "indexrelid", "schemaname", "relname", "indexrelname", "idx_scan"}).AddRow("db", "", 3, 11, "public", "t3", "i1", 0)
}

func TestRefreshRelationsPrimaryAndSecondary(t *testing.T) {
	c := New()
	primary, pm := newRelationsMock(t)
	secondary, sm := newRelationsMock(t)
	c.db = primary
	c.dbConns["secondary"] = &dbConn{db: secondary}
	c.MaxDBTables, c.MaxDBIndexes = 1, 1
	now := time.Unix(1000, 0)
	for _, m := range []sqlmock.Sqlmock{pm, sm} {
		m.ExpectQuery(queryRelationActivity(false)).WillReturnRows(tableInventory(10000, 0, 0))
		m.ExpectQuery(queryRelationActivity(true)).WillReturnRows(indexInventory())
	}
	c.refreshRelations(now)
	for _, db := range []*sql.DB{primary, secondary} {
		assert.Empty(t, c.relationsFor(db).tables.selected)
		assert.Equal(t, []uint32{11}, c.relationsFor(db).indexes.oids())
	}
	pm.ExpectQuery(queryRelationActivity(false)).WillReturnRows(tableInventory(10000, 10, 0))
	sm.ExpectQuery(queryRelationActivity(false)).WillReturnRows(tableInventory(10000, 0, 10))
	c.refreshRelations(now.Add(time.Second))
	assert.Equal(t, []uint32{2}, c.relationsFor(primary).tables.oids())
	assert.Equal(t, []uint32{3}, c.relationsFor(secondary).tables.oids())
	// No refresh between the warmup and the next five-minute window.
	c.refreshRelations(now.Add(time.Minute))
	// A failed inventory, including an error after partial rows, cannot replace a baseline.
	pm.ExpectQuery(queryRelationActivity(false)).WillReturnRows(tableInventory(99999, 0).RowError(1, errors.New("interrupted")))
	pm.ExpectQuery(queryRelationActivity(true)).WillReturnError(errors.New("unavailable"))
	sm.ExpectQuery(queryRelationActivity(false)).WillReturnError(errors.New("unavailable"))
	sm.ExpectQuery(queryRelationActivity(true)).WillReturnRows(indexInventory())
	c.refreshRelations(now.Add(6 * time.Minute))
	assert.Equal(t, now.Add(time.Second), c.relationsFor(primary).tables.observed)
	assert.Equal(t, []uint32{2}, c.relationsFor(primary).tables.oids())
	assert.NoError(t, pm.ExpectationsWereMet())
	assert.NoError(t, sm.ExpectationsWereMet())
}

func tableDetails(oid uint32, name string, updates, hot int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"datname", "relid", "schemaname", "relname", "parent_relname", "n_tup_upd", "n_tup_hot_upd", "total_relation_size"}).AddRow("db", oid, "public", name, "", updates, hot, 10000)
}

func TestRelationDetailsFailureRotationAndReturn(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	db, mock := newRelationsMock(t)
	c.db = db
	c.MaxDBTables = 1
	s := &c.relationsFor(db).tables
	s.selected = activityRows(activity(1, "t1", 0))
	mock.ExpectQuery(queryStatUserTables(true)).WithArgs(sqlmock.AnyArg()).WillReturnRows(tableDetails(1, "t1", 100, 20))
	require.NoError(t, c.doDBQueryStatUserTables(db))
	mx := make(map[string]int64)
	c.collectMetrics(mx)
	_, hasRatio := mx["table_t1_db_db_schema_public_n_tup_hot_upd_perc"]
	assert.False(t, hasRatio)
	first := c.Charts().Get("table_t1_db_db_schema_public_ops_rows_rate")
	// Public metric and chart identities are preserved across selection changes.
	require.NotEmpty(t, mx["table_t1_db_db_schema_public_n_tup_upd"])
	c.resetMetrics()
	mock.ExpectQuery(queryStatUserTables(true)).WithArgs(sqlmock.AnyArg()).WillReturnRows(tableDetails(1, "t1", 120, 40))
	require.NoError(t, c.doDBQueryStatUserTables(db))
	mx = make(map[string]int64)
	c.collectMetrics(mx)
	assert.Equal(t, int64(100), mx["table_t1_db_db_schema_public_n_tup_hot_upd_perc"])
	c.resetMetrics()
	mock.ExpectQuery(queryStatUserTables(true)).WithArgs(sqlmock.AnyArg()).WillReturnError(errors.New("timeout"))
	require.Error(t, c.doDBQueryStatUserTables(db))
	mx = make(map[string]int64)
	c.collectMetrics(mx)
	assert.NotContains(t, mx, "table_t1_db_db_schema_public_n_tup_upd")
	assert.Len(t, c.mx.tables, 1)
	require.NotNil(t, first)
	assert.False(t, first.IsRemoved())
	s.selected = activityRows(activity(2, "t2", 0))
	c.retireUnselected(db, false)
	assert.Empty(t, c.mx.tables)
	assert.True(t, first.IsRemoved())
	s.selected = activityRows(activity(1, "t1", 0))
	mock.ExpectQuery(queryStatUserTables(true)).WithArgs(sqlmock.AnyArg()).WillReturnRows(tableDetails(1, "t1", 10000, 9000))
	require.NoError(t, c.doDBQueryStatUserTables(db))
	mx = make(map[string]int64)
	c.collectMetrics(mx)
	assert.NotContains(t, mx, "table_t1_db_db_schema_public_n_tup_hot_upd_perc")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLateBloatChartsAndExactRemoval(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	m := c.getTableMetrics("t", "db", "public")
	m.updated = true
	m.heapBlksRead.last, m.idxBlksRead.last, m.toastBlksRead.last, m.tidxBlksRead.last = -1, -1, -1, -1
	neighbor := c.getTableMetrics("t", "db", "public_extra")
	*neighbor = *m
	neighbor.schema = "public_extra"
	i := c.getIndexMetrics("i", "t", "db", "public")
	i.updated = true
	c.collectMetrics(make(map[string]int64))
	assert.Nil(t, c.Charts().Get("table_t_db_db_schema_public_bloat_size"))
	m.bloatSize, m.bloatSizePerc = new(int64(12)), new(int64(1))
	i.bloatSize, i.bloatSizePerc = new(int64(12)), new(int64(1))
	c.collectMetrics(make(map[string]int64))
	require.NotNil(t, c.Charts().Get("table_t_db_db_schema_public_bloat_size"))
	require.NotNil(t, c.Charts().Get("index_i_table_t_db_db_schema_public_bloat_size"))
	c.removeTableCharts(m)
	for _, chart := range *c.Charts() {
		for _, label := range chart.Labels {
			if label.Value == "public_extra" || label.Key == "index" {
				assert.False(t, chart.IsRemoved())
			}
		}
	}
}

func TestRelationDetailsCommitOnlyCompleteQueries(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	db, mock := newRelationsMock(t)
	c.db = db
	c.MaxDBTables, c.MaxDBIndexes = 0, 0
	mock.ExpectQuery(queryStatUserTables(false)).WillReturnRows(tableDetails(1, "t1", 100, 20))
	require.NoError(t, c.doDBQueryStatUserTables(db))
	c.collectMetrics(make(map[string]int64))
	c.resetMetrics()
	mock.ExpectQuery(queryStatUserTables(false)).WillReturnRows(tableDetails(1, "t1", 99999, 99999).AddRow("db", 2, "public", "t2", "", 1, 1, 100).RowError(1, errors.New("partial")))
	require.Error(t, c.doDBQueryStatUserTables(db))
	assert.Len(t, c.mx.tables, 1)
	assert.Equal(t, int64(100), c.mx.tables["t1_db_public"].nTupUpd.last)
	mock.ExpectQuery(queryStatUserTables(false)).WillReturnRows(tableDetails(1, "t1", 120, 30))
	require.NoError(t, c.doDBQueryStatUserTables(db))
	mx := make(map[string]int64)
	c.collectMetrics(mx)
	assert.NotContains(t, mx, "table_t1_db_db_schema_public_n_tup_hot_upd_perc")
	// Recreation under the same name cannot inherit derivative or slow-value state.
	c.resetMetrics()
	mock.ExpectQuery(queryStatUserTables(false)).WillReturnRows(tableDetails(2, "t1", 200, 100))
	require.NoError(t, c.doDBQueryStatUserTables(db))
	assert.Equal(t, uint32(2), c.mx.tables["t1_db_public"].oid)
	assert.False(t, c.mx.tables["t1_db_public"].sampled)
	// A successful empty index query retires indexes; failures leave charts available with a gap.
	idxRows := func() *sqlmock.Rows {
		return sqlmock.NewRows([]string{"datname", "indexrelid", "schemaname", "relname", "indexrelname", "idx_scan"}).AddRow("db", 11, "public", "t1", "i1", 10)
	}
	mock.ExpectQuery(queryStatUserIndexes(false)).WillReturnRows(idxRows())
	require.NoError(t, c.doDBQueryStatUserIndexes(db))
	c.collectMetrics(make(map[string]int64))
	c.resetMetrics()
	idx := c.mx.indexes["i1_t1_db_public"]
	mock.ExpectQuery(queryStatUserIndexes(false)).WillReturnError(errors.New("timeout"))
	require.Error(t, c.doDBQueryStatUserIndexes(db))
	mx = make(map[string]int64)
	c.collectMetrics(mx)
	assert.NotContains(t, mx, "index_i1_table_t1_db_db_schema_public_size")
	assert.False(t, idx.charts[0].IsRemoved())
	mock.ExpectQuery(queryStatUserIndexes(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "indexrelid", "schemaname", "relname", "indexrelname", "idx_scan"}))
	require.NoError(t, c.doDBQueryStatUserIndexes(db))
	assert.Empty(t, c.mx.indexes)
	assert.True(t, idx.charts[0].IsRemoved())
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSlowRelationMetricsDatabaseOwnership(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	one, mock := newRelationsMock(t)
	two, _ := newRelationsMock(t)
	c.MaxDBTables, c.MaxDBIndexes = 0, 0
	m := c.getTableMetrics("t", "one", "public")
	m.owner = one
	m.updated = true
	m.bloatSize, m.bloatSizePerc = new(int64(20)), new(int64(1))
	m.nullColumns = new(int64(2))
	other := c.getTableMetrics("t", "two", "public")
	*other = *m
	other.owner = two
	other.db = "two"
	mock.ExpectQuery(queryBloat(false)).WillReturnRows(sqlmock.NewRows([]string{"db", "schemaname", "tablename", "wastedbytes", "iname", "wastedibytes"}))
	require.NoError(t, c.doDBQueryBloat(one))
	assert.Equal(t, int64(0), *m.bloatSize)
	assert.Equal(t, int64(20), *other.bloatSize)
	mock.ExpectQuery(queryColumnsStats(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "schemaname", "relname", "null_percent"}))
	require.NoError(t, c.doDBQueryColumns(one))
	assert.Equal(t, int64(0), *m.nullColumns)
	assert.Equal(t, int64(2), *other.nullColumns)
	mock.ExpectQuery(queryBloat(false)).WillReturnError(errors.New("timeout"))
	require.Error(t, c.doDBQueryBloat(one))
	assert.False(t, m.bloatValid)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSlowRelationMetricsDetailGap(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	db, mock := newRelationsMock(t)
	c.db = db
	c.MaxDBTables, c.MaxDBIndexes = 0, 0
	stats := func() {
		mock.ExpectQuery(queryStatUserTables(false)).WillReturnRows(tableDetails(1, "t", 100, 20))
		require.NoError(t, c.doDBQueryStatUserTables(db))
		mock.ExpectQuery(queryStatUserIndexes(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "indexrelid", "schemaname", "relname", "indexrelname", "idx_scan", "size"}).AddRow("db", 11, "public", "t", "i", 10, 1000))
		require.NoError(t, c.doDBQueryStatUserIndexes(db))
	}
	slow := func() {
		mock.ExpectQuery(queryBloat(false)).WillReturnRows(sqlmock.NewRows([]string{"db", "schemaname", "tablename", "wastedbytes", "iname", "wastedibytes"}).AddRow("db", "public", "t", 100, "i", 50))
		require.NoError(t, c.doDBQueryBloat(db))
		mock.ExpectQuery(queryColumnsStats(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "schemaname", "relname", "null_percent"}).AddRow("db", "public", "t", 100))
		require.NoError(t, c.doDBQueryColumns(db))
	}
	stats()
	slow()
	c.collectMetrics(make(map[string]int64))
	c.resetMetrics()
	// A slow observation can succeed while the current detail/size sample is missing.
	mock.ExpectQuery(queryStatUserTables(false)).WillReturnError(errors.New("timeout"))
	require.Error(t, c.doDBQueryStatUserTables(db))
	mock.ExpectQuery(queryStatUserIndexes(false)).WillReturnError(errors.New("timeout"))
	require.Error(t, c.doDBQueryStatUserIndexes(db))
	slow()
	table := c.getTableMetrics("t", "db", "public")
	index := c.getIndexMetrics("i", "t", "db", "public")
	assert.Equal(t, int64(100), *table.bloatSize)
	assert.Equal(t, int64(50), *index.bloatSize)
	assert.False(t, table.bloatValid)
	assert.False(t, index.bloatValid)
	assert.False(t, table.nullValid)
	stats()
	mx := make(map[string]int64)
	c.collectMetrics(mx)
	assert.NotContains(t, mx, "table_t_db_db_schema_public_bloat_size")
	assert.NotContains(t, mx, "index_i_table_t_db_db_schema_public_bloat_size")
	assert.NotContains(t, mx, "table_t_db_db_schema_public_null_columns")
	// Only the next complete slow refresh restores these samples.
	slow()
	mx = make(map[string]int64)
	c.collectMetrics(mx)
	assert.Equal(t, int64(100), mx["table_t_db_db_schema_public_bloat_size"])
	assert.Equal(t, int64(50), mx["index_i_table_t_db_db_schema_public_bloat_size"])
	assert.Equal(t, int64(1), mx["table_t_db_db_schema_public_null_columns"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestRelationLimitsAndResetAllocations(t *testing.T) {
	c := New()
	assert.Equal(t, int64(50), c.MaxDBTables)
	assert.Equal(t, int64(250), c.MaxDBIndexes)
	c.MaxDBTables = -1
	require.Error(t, c.validateConfig())
	c.MaxDBTables, c.MaxDBIndexes = 0, -1
	require.Error(t, c.validateConfig())
	c.MaxDBIndexes = 0
	db, mock := newRelationsMock(t)
	c.db = db
	c.refreshRelations(time.Now()) // Unlimited collection needs no ranking snapshots.
	assert.NoError(t, mock.ExpectationsWereMet())
	for i := 0; i < 500; i++ {
		c.getTableMetrics(fmt.Sprint(i), "db", "public")
	}
	assert.Zero(t, testing.AllocsPerRun(10, c.resetMetrics))
}

func TestRelationIOUnavailablePairSeedsBaseline(t *testing.T) {
	c := New()
	require.NoError(t, c.Init(context.Background()))
	db, mock := newRelationsMock(t)
	c.db = db
	c.MaxDBTables = 0
	stats := func() {
		mock.ExpectQuery(queryStatUserTables(false)).WillReturnRows(tableDetails(1, "t1", 100, 20))
		require.NoError(t, c.doDBQueryStatUserTables(db))
	}
	io := func(read, hit any) {
		mock.ExpectQuery(queryStatIOUserTables(false)).WillReturnRows(sqlmock.NewRows([]string{"datname", "relid", "schemaname", "relname", "heap_blks_read_bytes", "heap_blks_hit_bytes", "idx_blks_read_bytes", "idx_blks_hit_bytes"}).AddRow("db", 1, "public", "t1", 0, 10, read, hit))
		require.NoError(t, c.doDBQueryStatIOUserTables(db, true))
	}
	stats()
	io(nil, nil)
	c.collectMetrics(make(map[string]int64))
	c.resetMetrics()
	stats()
	io(100, 200)
	mx := make(map[string]int64)
	c.collectMetrics(mx)
	assert.Contains(t, mx, "table_t1_db_db_schema_public_idx_blks_read")
	assert.NotContains(t, mx, "table_t1_db_db_schema_public_idx_blks_read_perc")
	c.resetMetrics()
	stats()
	io(110, 240)
	mx = make(map[string]int64)
	c.collectMetrics(mx)
	assert.Equal(t, int64(20), mx["table_t1_db_db_schema_public_idx_blks_read_perc"])
	assert.NoError(t, mock.ExpectationsWereMet())
}

// Reset/emission visits selected details only: O(N), with no retained-candidate work.
// Timings are a development-machine trend; allocations and scaling are the envelope.
func BenchmarkRelationMetrics(b *testing.B) {
	for _, n := range []int{50, 500} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c := New()
			if err := c.Init(context.Background()); err != nil {
				b.Fatal(err)
			}
			for i := 0; i < n; i++ {
				m := c.getTableMetrics(fmt.Sprint(i), "db", "public")
				m.updated, m.ioUpdated = true, true
			}
			c.collectMetrics(make(map[string]int64))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.resetMetrics()
				for _, m := range c.mx.tables {
					m.updated, m.ioUpdated = true, true
				}
				c.collectMetrics(make(map[string]int64))
			}
		})
	}
}

// Refresh retains O(M) candidate counters and sorts O(M log M); details stay O(N).
func BenchmarkRelationSelection(b *testing.B) {
	for _, n := range []int{50, 5000, 50000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			rows := make(map[uint32]relationActivity, n)
			for i := 0; i < n; i++ {
				rows[uint32(i)] = activity(uint32(i), fmt.Sprint(i), int64(i))
			}
			s := relationSelection{}
			now := time.Unix(1000, 0)
			s.choose(rows, "", now, 50)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				now = now.Add(time.Minute)
				s.choose(rows, "", now, 50)
			}
		})
	}
}
