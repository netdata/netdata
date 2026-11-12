// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"sort"
	"time"
)

const (
	relationRefreshEvery = 5 * time.Minute
	// A failed activity scan is not repeated every update interval.
	relationRetryEvery = time.Minute
	// A selected object yields only to one scoring more than this many times higher, limiting rotation churn.
	relationRetainFactor = 2
	// A selected table also stays against tables below this many rows per second, so a trickle of rows does not
	// rotate the idle tail of a selection.
	relationIdleRowsPerSecond = 1.0
	// Repeated per-database query failures are logged at most once per window.
	queryWarningEvery = 5 * time.Minute
)

// OIDs belong to a connection's database. Public chart identities still use names.
// Tables rank by counters, indexes by size.
type relationActivity struct {
	oid, tableOID                 uint32
	database, schema, name, table string
	counters                      [5]int64
	size                          int64
}

type relationSelection struct {
	observed time.Time
	retryAt  time.Time
	reset    string
	previous map[uint32]relationActivity
	selected map[uint32]relationActivity
	warmed   bool
}

type databaseRelations struct {
	tables, indexes relationSelection
}

func (c *Collector) relationsFor(db *sql.DB) *databaseRelations {
	if c.relations == nil {
		c.relations = make(map[*sql.DB]*databaseRelations)
	}
	if c.relations[db] == nil {
		c.relations[db] = &databaseRelations{}
	}
	return c.relations[db]
}

// forEachDatabase passes an empty name for the DSN database.
func (c *Collector) forEachDatabase(fn func(name string, db *sql.DB)) {
	fn("", c.db)
	for name, conn := range c.dbConns {
		if conn.db != nil {
			fn(name, conn.db)
		}
	}
}

func (c *Collector) warnDatabaseQuery(operation, dbname string, err error) {
	target := "the DSN database"
	if dbname != "" {
		target = "database '" + dbname + "'"
	}
	c.Limit("postgres:"+operation+":"+dbname, 1, queryWarningEvery).
		Warningf("querying PostgreSQL %s in %s: %v", operation, target, err)
}

func (c *Collector) queryEachDatabase(operation string, query func(*sql.DB) error) error {
	c.forEachDatabase(func(name string, db *sql.DB) {
		if err := query(db); err != nil {
			c.warnDatabaseQuery(operation, name, err)
		}
	})
	return nil
}

func (c *Collector) refreshRelations(now time.Time) {
	c.forEachDatabase(func(name string, db *sql.DB) {
		r := c.relationsFor(db)
		for _, surface := range []struct {
			selection *relationSelection
			limit     int64
			indexes   bool
			operation string
		}{
			{&r.tables, c.MaxDBTables, false, "table activity"}, {&r.indexes, c.MaxDBIndexes, true, "index sizes"},
		} {
			if surface.limit == 0 {
				continue
			}
			s := surface.selection
			if now.Before(s.retryAt) || s.warmed && now.Sub(s.observed) < relationRefreshEvery {
				continue
			}
			rows, reset, err := c.queryRelationActivity(db, surface.indexes)
			if err != nil {
				s.retryAt = now.Add(relationRetryEvery)
				c.warnDatabaseQuery(surface.operation, name, err)
				continue
			}
			if !s.observed.IsZero() && s.reset != reset {
				for _, m := range c.mx.tables {
					if m.owner == db {
						m.sampled, m.ioSampled = false, false
					}
				}
			}
			s.choose(rows, reset, now, surface.limit, surface.indexes)
			c.retireUnselected(db, surface.indexes)
		}
	})
}

// choose ranks tables by recent row activity, a rate that needs two observations, and indexes by size.
// A selected object keeps its place unless another scores more than relationRetainFactor times higher;
// a selected table also keeps it against tables below relationIdleRowsPerSecond.
func (s *relationSelection) choose(rows map[uint32]relationActivity, reset string, now time.Time, limit int64, bySize bool) {
	type scored struct {
		relationActivity
		score    float64
		retained bool
	}
	ranked := make([]scored, 0, len(rows))
	seconds := now.Sub(s.observed).Seconds()
	for oid, row := range rows {
		item := scored{relationActivity: row}
		if bySize {
			item.score = float64(row.size)
		} else {
			item.score = s.activityRate(oid, row, reset, seconds)
		}
		if selected, ok := s.selected[oid]; ok && sameRelation(selected, row) {
			item.retained = true
			item.score *= relationRetainFactor
			if !bySize {
				item.score = max(item.score, relationIdleRowsPerSecond)
			}
		}
		ranked = append(ranked, item)
	}
	sort.Slice(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.retained != b.retained {
			return a.retained
		}
		if a.schema != b.schema {
			return a.schema < b.schema
		}
		if a.name != b.name {
			return a.name < b.name
		}
		return a.oid < b.oid
	})
	selected := make(map[uint32]relationActivity)
	// A first observation cannot establish recent activity in an oversized database.
	s.warmed = bySize || !s.observed.IsZero() || int64(len(rows)) <= limit
	if s.warmed {
		for i, row := range ranked {
			if int64(i) >= limit {
				break
			}
			selected[row.oid] = row.relationActivity
		}
	}
	s.previous = nil
	if !bySize {
		s.previous = rows
	}
	s.selected, s.reset, s.observed = selected, reset, now
}

// activityRate is the summed row-counter rate since the previous observation, or 0 without a valid baseline.
func (s *relationSelection) activityRate(oid uint32, row relationActivity, reset string, seconds float64) float64 {
	old, ok := s.previous[oid]
	if !ok || !sameRelation(old, row) || reset != s.reset || seconds <= 0 {
		return 0
	}
	var delta float64
	for i, value := range row.counters {
		if value < old.counters[i] {
			return 0
		}
		delta += float64(value - old.counters[i])
	}
	return delta / seconds
}

func sameRelation(a, b relationActivity) bool {
	return a.database == b.database && a.schema == b.schema && a.name == b.name && a.table == b.table && a.tableOID == b.tableOID
}

func (s *relationSelection) oids() []uint32 {
	ids := make([]uint32, 0, len(s.selected))
	for oid := range s.selected {
		ids = append(ids, oid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (c *Collector) queryRelationActivity(db *sql.DB, indexes bool) (map[uint32]relationActivity, string, error) {
	rows := make(map[uint32]relationActivity)
	var row relationActivity
	var reset string
	err := c.doDBQuery(db, queryRelationActivity(indexes), func(column, value string, end bool) {
		switch column {
		case "datname":
			row.database = value
		case "stats_reset":
			reset = value
		case "relid":
			row.tableOID = uint32(parseInt(value))
			if !indexes {
				row.oid = row.tableOID
			}
		case "indexrelid":
			row.oid = uint32(parseInt(value))
		case "schemaname":
			row.schema = value
		case "relname":
			if indexes {
				row.table = value
			} else {
				row.name = value
			}
		case "indexrelname":
			row.name = value
		case "size":
			row.size = parseInt(value)
		case "seq_tup_read":
			row.counters[0] = parseInt(value)
		case "idx_tup_fetch":
			row.counters[1] = parseInt(value)
		case "n_tup_ins":
			row.counters[2] = parseInt(value)
		case "n_tup_upd":
			row.counters[3] = parseInt(value)
		case "n_tup_del":
			row.counters[4] = parseInt(value)
		}
		if end {
			rows[row.oid] = row
			row = relationActivity{}
		}
	})
	return rows, reset, err
}

// queryRelationActivity returns the ranking inputs: row-activity counters for tables, sizes for indexes.
func queryRelationActivity(indexes bool) string {
	if indexes {
		return `SELECT current_database() AS datname, s.relid, s.indexrelid,
 s.schemaname, s.relname, s.indexrelname, pg_relation_size(s.indexrelid) AS size
 FROM pg_stat_user_indexes s
 WHERE has_schema_privilege(s.schemaname, 'USAGE');`
	}
	return `SELECT current_database() AS datname, d.stats_reset, s.relid,
 s.schemaname, s.relname, s.seq_tup_read, s.idx_tup_fetch, s.n_tup_ins, s.n_tup_upd, s.n_tup_del
 FROM pg_stat_user_tables s JOIN pg_stat_database d ON d.datname = current_database()
 WHERE has_schema_privilege(s.schemaname, 'USAGE');`
}

func (c *Collector) retireUnselected(db *sql.DB, indexes bool) {
	r := c.relationsFor(db)
	if indexes {
		for key, m := range c.mx.indexes {
			row, ok := r.indexes.selected[m.oid]
			if m.owner == db && (!ok || removeSpaces(row.name) != m.name || row.schema != m.schema || row.table != m.table) {
				c.removeIndexCharts(m)
				delete(c.mx.indexes, key)
			}
		}
	} else {
		for key, m := range c.mx.tables {
			row, ok := r.tables.selected[m.oid]
			if m.owner == db && (!ok || row.name != m.name || row.schema != m.schema) {
				c.removeTableCharts(m)
				delete(c.mx.tables, key)
			}
		}
	}
}

func (c *Collector) retireDatabaseRelations(db *sql.DB) {
	for key, m := range c.mx.tables {
		if m.owner == db {
			c.removeTableCharts(m)
			delete(c.mx.tables, key)
		}
	}
	for key, m := range c.mx.indexes {
		if m.owner == db {
			c.removeIndexCharts(m)
			delete(c.mx.indexes, key)
		}
	}
	delete(c.relations, db)
}
