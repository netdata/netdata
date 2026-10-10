// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"sort"
	"time"
)

const relationRefreshEvery = 5 * time.Minute

// OIDs belong to a connection's database. Public chart identities still use names.
type relationActivity struct {
	oid, tableOID                 uint32
	database, schema, name, table string
	counters                      [5]int64
}

type relationSelection struct {
	observed time.Time
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

func (c *Collector) forEachDatabase(fn func(*sql.DB)) {
	fn(c.db)
	for _, conn := range c.dbConns {
		if conn.db != nil {
			fn(conn.db)
		}
	}
}

func (c *Collector) queryEachDatabase(operation string, query func(*sql.DB) error) error {
	c.forEachDatabase(func(db *sql.DB) {
		if err := query(db); err != nil {
			c.Warningf("querying PostgreSQL %s: %v", operation, err)
		}
	})
	return nil
}

func (c *Collector) refreshRelations(now time.Time) {
	c.forEachDatabase(func(db *sql.DB) {
		r := c.relationsFor(db)
		for _, surface := range []struct {
			selection *relationSelection
			limit     int64
			indexes   bool
		}{
			{&r.tables, c.MaxDBTables, false}, {&r.indexes, c.MaxDBIndexes, true},
		} {
			if surface.limit == 0 {
				continue
			}
			s := surface.selection
			if s.warmed && now.Sub(s.observed) < relationRefreshEvery {
				continue
			}
			rows, reset, err := c.queryRelationActivity(db, surface.indexes)
			if err != nil {
				c.Warningf("querying PostgreSQL activity (indexes=%t): %v", surface.indexes, err)
				continue
			}
			if !s.observed.IsZero() && s.reset != reset {
				for _, m := range c.mx.tables {
					if m.owner == db {
						m.sampled, m.ioSampled = false, false
					}
				}
			}
			s.choose(rows, reset, now, surface.limit)
			c.retireUnselected(db, surface.indexes)
		}
	})
}

func (s *relationSelection) choose(rows map[uint32]relationActivity, reset string, now time.Time, limit int64) {
	type scored struct {
		relationActivity
		score    float64
		retained bool
	}
	ranked := make([]scored, 0, len(rows))
	seconds := now.Sub(s.observed).Seconds()
	for oid, row := range rows {
		item := scored{relationActivity: row}
		old, exists := s.previous[oid]
		valid := exists && sameRelation(old, row) && reset == s.reset && seconds > 0
		for i, value := range row.counters {
			if value < old.counters[i] {
				valid = false
			}
			if value >= old.counters[i] {
				item.score += float64(value - old.counters[i])
			}
		}
		if valid {
			item.score /= seconds
		} else {
			item.score = 0
		}
		if selected, ok := s.selected[oid]; ok {
			item.retained = sameRelation(selected, row)
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
	if len(rows) <= int(limit) || !s.observed.IsZero() {
		for i, row := range ranked {
			if int64(i) >= limit {
				break
			}
			selected[row.oid] = row.relationActivity
		}
	}
	s.warmed = !s.observed.IsZero() || len(rows) <= int(limit)
	s.previous, s.selected, s.reset, s.observed = rows, selected, reset, now
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
		case "seq_tup_read", "idx_scan":
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

func queryRelationActivity(indexes bool) string {
	if indexes {
		return `SELECT current_database() AS datname, d.stats_reset, s.relid, s.indexrelid,
 s.schemaname, s.relname, s.indexrelname, s.idx_scan
 FROM pg_stat_user_indexes s JOIN pg_stat_database d ON d.datname = current_database()
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
