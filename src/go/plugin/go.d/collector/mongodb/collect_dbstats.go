// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) collectDbStats(mx map[string]int64) error {
	if c.dbSelector == nil {
		c.Debug("'database' selector not set, skip collecting database statistics")
		return nil
	}

	allDBs, err := c.conn.listDatabaseNames()
	if err != nil {
		return fmt.Errorf("cannot get database names: %v", err)
	}

	c.Debugf("all databases on the server: '%v'", allDBs)

	var dbs []string
	for _, db := range allDBs {
		if c.dbSelector.MatchString(db) {
			dbs = append(dbs, db)
		}
	}

	if len(allDBs) != len(dbs) {
		c.Debugf("databases remaining after filtering: %v", dbs)
	}

	seen := make(map[string]bool)
	for _, db := range dbs {
		s, err := c.conn.dbStats(db)
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
		if !c.databases[db] {
			c.databases[db] = true
			c.Debugf("new database '%s': creating charts", db)
			c.addDatabaseCharts(db)
		}
	}

	for db := range c.databases {
		if !seen[db] {
			delete(c.databases, db)
			c.Debugf("stale database '%s': removing charts", db)
			c.removeDatabaseCharts(db)
		}
	}

	return nil
}

func (c *Collector) addDatabaseCharts(name string) {
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

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDatabaseCharts(name string) {
	px := fmt.Sprintf("%s%s_", chartPxDatabase, name)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
