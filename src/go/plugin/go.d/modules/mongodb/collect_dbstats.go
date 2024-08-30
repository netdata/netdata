// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (m *Mongo) collectDbStats(mx map[string]int64) error {
	if m.dbSelector == nil {
		m.Debug("'database' selector not set, skip collecting database statistics")
		return nil
	}

	allDBs, err := m.conn.listDatabaseNames()
	if err != nil {
		return fmt.Errorf("cannot get database names: %v", err)
	}

	m.Debugf("all databases on the server: '%v'", allDBs)

	var dbs []string
	for _, db := range allDBs {
		if m.dbSelector.MatchString(db) {
			dbs = append(dbs, db)
		}
	}

	if len(allDBs) != len(dbs) {
		m.Debugf("databases remaining after filtering: %v", dbs)
	}

	seen := make(map[string]bool)
	for _, db := range dbs {
		s, err := m.conn.dbStats(db)
		if err != nil {
			return fmt.Errorf("dbStats command failed: %v", err)
		}

		seen[db] = true

		mx["database_"+db+"_collections"] = s.Collections
		mx["database_"+db+"_views"] = s.Views
		mx["database_"+db+"_indexes"] = s.Indexes
		mx["database_"+db+"_documents"] = s.Objects
		mx["database_"+db+"_data_size"] = s.DataSize
		mx["database_"+db+"_index_size"] = s.IndexSize
		mx["database_"+db+"_storage_size"] = s.StorageSize
	}

	for db := range seen {
		if !m.databases[db] {
			m.databases[db] = true
			m.Debugf("new database '%s': creating charts", db)
			m.addDatabaseCharts(db)
		}
	}

	for db := range m.databases {
		if !seen[db] {
			delete(m.databases, db)
			m.Debugf("stale database '%s': removing charts", db)
			m.removeDatabaseCharts(db)
		}
	}

	return nil
}

func (m *Mongo) addDatabaseCharts(name string) {
	charts := chartsTmplDatabase.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "database", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := m.Charts().Add(*charts...); err != nil {
		m.Warning(err)
	}
}

func (m *Mongo) removeDatabaseCharts(name string) {
	px := fmt.Sprintf("%s%s_", chartPxDatabase, name)

	for _, chart := range *m.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
