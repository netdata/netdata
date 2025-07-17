// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"fmt"
	"time"
)

// Chart management functions
func (d *DB2) addDatabaseCharts(db *databaseMetrics) {
	charts := d.newDatabaseCharts(db)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add database chart for %s: %v", db.name, err)
		}
	}
}

func (d *DB2) removeDatabaseCharts(name string) {
	cleanName := cleanName(name)
	chartPrefixes := []string{
		fmt.Sprintf("database_%s_status", cleanName),
		fmt.Sprintf("database_%s_applications", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addBufferpoolCharts(bp *bufferpoolMetrics) {
	charts := d.newBufferpoolCharts(bp)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add bufferpool chart for %s: %v", bp.name, err)
		}
	}
}

func (d *DB2) removeBufferpoolCharts(name string) {
	cleanName := cleanName(name)
	chartPrefixes := []string{
		fmt.Sprintf("bufferpool_%s_hit_ratio", cleanName),
		fmt.Sprintf("bufferpool_%s_io", cleanName),
		fmt.Sprintf("bufferpool_%s_pages", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addTablespaceCharts(ts *tablespaceMetrics) {
	charts := d.newTablespaceCharts(ts)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add tablespace chart for %s: %v", ts.name, err)
		}
	}
}

func (d *DB2) removeTablespaceCharts(name string) {
	cleanName := cleanName(name)
	chartPrefixes := []string{
		fmt.Sprintf("tablespace_%s_usage", cleanName),
		fmt.Sprintf("tablespace_%s_size", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addConnectionCharts(conn *connectionMetrics) {
	charts := d.newConnectionCharts(conn)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add connection chart for %s: %v", conn.applicationID, err)
		}
	}
}

func (d *DB2) removeConnectionCharts(id string) {
	cleanID := cleanName(id)
	chartPrefixes := []string{
		fmt.Sprintf("connection_%s_state", cleanID),
		fmt.Sprintf("connection_%s_activity", cleanID),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addTableCharts(t *tableMetrics) {
	charts := d.newTableCharts(t)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add table chart for %s: %v", t.name, err)
		}
	}
}

func (d *DB2) removeTableCharts(name string) {
	cleanName := cleanName(name)
	chartPrefixes := []string{
		fmt.Sprintf("table_%s_size", cleanName),
		fmt.Sprintf("table_%s_activity", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addIndexCharts(i *indexMetrics) {
	charts := d.newIndexCharts(i)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add index chart for %s: %v", i.name, err)
		}
	}
}

func (d *DB2) removeIndexCharts(name string) {
	cleanName := cleanName(name)
	chartPrefixes := []string{
		fmt.Sprintf("index_%s_usage", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addMemoryPoolCharts(pool *memoryPoolMetrics) {
	charts := d.newMemoryPoolCharts(pool)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add memory pool chart for %s: %v", pool.poolType, err)
		}
	}
}

func (d *DB2) removeMemoryPoolCharts(poolType string) {
	cleanName := cleanName(poolType)
	chartPrefixes := []string{
		fmt.Sprintf("memory_pool_%s_usage", cleanName),
		fmt.Sprintf("memory_pool_%s_hwm", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addTableIOCharts(table *tableMetrics) {
	charts := d.newTableIOCharts(table)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add table I/O chart for %s: %v", table.name, err)
		}
	}
}

func (d *DB2) removeTableIOCharts(name string) {
	cleanName := cleanName(name)
	chartPrefixes := []string{
		fmt.Sprintf("table_io_%s_scans", cleanName),
		fmt.Sprintf("table_io_%s_rows", cleanName),
		fmt.Sprintf("table_io_%s_activity", cleanName),
	}

	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

// Track instance updates for cleanup
type instanceUpdate struct {
	lastSeen time.Time
}

var (
	databaseUpdates   = make(map[string]*instanceUpdate)
	bufferpoolUpdates = make(map[string]*instanceUpdate)
	tablespaceUpdates = make(map[string]*instanceUpdate)
	connectionUpdates = make(map[string]*instanceUpdate)
	tableUpdates      = make(map[string]*instanceUpdate)
	indexUpdates      = make(map[string]*instanceUpdate)
)

func (d *DB2) cleanupStaleInstances() {
	now := time.Now()
	staleTimeout := time.Minute * 5

	// Mark current instances as seen
	for name := range d.mx.databases {
		if _, exists := databaseUpdates[name]; !exists {
			databaseUpdates[name] = &instanceUpdate{}
		}
		databaseUpdates[name].lastSeen = now
	}

	for name := range d.mx.bufferpools {
		if _, exists := bufferpoolUpdates[name]; !exists {
			bufferpoolUpdates[name] = &instanceUpdate{}
		}
		bufferpoolUpdates[name].lastSeen = now
	}

	for name := range d.mx.tablespaces {
		if _, exists := tablespaceUpdates[name]; !exists {
			tablespaceUpdates[name] = &instanceUpdate{}
		}
		tablespaceUpdates[name].lastSeen = now
	}

	for id := range d.mx.connections {
		if _, exists := connectionUpdates[id]; !exists {
			connectionUpdates[id] = &instanceUpdate{}
		}
		connectionUpdates[id].lastSeen = now
	}

	for name := range d.mx.tables {
		if _, exists := tableUpdates[name]; !exists {
			tableUpdates[name] = &instanceUpdate{}
		}
		tableUpdates[name].lastSeen = now
	}

	for name := range d.mx.indexes {
		if _, exists := indexUpdates[name]; !exists {
			indexUpdates[name] = &instanceUpdate{}
		}
		indexUpdates[name].lastSeen = now
	}

	// Remove stale instances
	for name, update := range databaseUpdates {
		if now.Sub(update.lastSeen) > staleTimeout {
			delete(d.databases, name)
			d.removeDatabaseCharts(name)
			delete(databaseUpdates, name)
		}
	}

	for name, update := range bufferpoolUpdates {
		if now.Sub(update.lastSeen) > staleTimeout {
			delete(d.bufferpools, name)
			d.removeBufferpoolCharts(name)
			delete(bufferpoolUpdates, name)
		}
	}

	for name, update := range tablespaceUpdates {
		if now.Sub(update.lastSeen) > staleTimeout {
			delete(d.tablespaces, name)
			d.removeTablespaceCharts(name)
			delete(tablespaceUpdates, name)
		}
	}

	for id, update := range connectionUpdates {
		if now.Sub(update.lastSeen) > staleTimeout {
			delete(d.connections, id)
			d.removeConnectionCharts(id)
			delete(connectionUpdates, id)
		}
	}

	for name, update := range tableUpdates {
		if now.Sub(update.lastSeen) > staleTimeout {
			delete(d.tables, name)
			d.removeTableCharts(name)
			delete(tableUpdates, name)
		}
	}

	for name, update := range indexUpdates {
		if now.Sub(update.lastSeen) > staleTimeout {
			delete(d.indexes, name)
			d.removeIndexCharts(name)
			delete(indexUpdates, name)
		}
	}
}

// Generic chart removal method
func (d *DB2) removeCharts(prefix string) {
	for _, chart := range *d.charts {
		if len(chart.ID) >= len(prefix) && chart.ID[:len(prefix)] == prefix {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
