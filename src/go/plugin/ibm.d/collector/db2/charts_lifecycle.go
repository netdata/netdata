// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"fmt"
	"time"
)

// Chart management functions
func (d *DB2) addDatabaseCharts(db *databaseMetrics) {
	charts := newDatabaseCharts(db)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add database chart for %s: %v", db.name, err)
		}
	}
}

func (d *DB2) removeDatabaseCharts(name string) {
	chartPrefixes := []string{
		fmt.Sprintf("database_%s_status", name),
		fmt.Sprintf("database_%s_applications", name),
	}
	
	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addBufferpoolCharts(bp *bufferpoolMetrics) {
	charts := newBufferpoolCharts(bp)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add bufferpool chart for %s: %v", bp.name, err)
		}
	}
}

func (d *DB2) removeBufferpoolCharts(name string) {
	chartPrefixes := []string{
		fmt.Sprintf("bufferpool_%s_hit_ratio", name),
		fmt.Sprintf("bufferpool_%s_io", name),
		fmt.Sprintf("bufferpool_%s_pages", name),
	}
	
	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addTablespaceCharts(ts *tablespaceMetrics) {
	charts := newTablespaceCharts(ts)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add tablespace chart for %s: %v", ts.name, err)
		}
	}
}

func (d *DB2) removeTablespaceCharts(name string) {
	chartPrefixes := []string{
		fmt.Sprintf("tablespace_%s_usage", name),
		fmt.Sprintf("tablespace_%s_size", name),
	}
	
	for _, prefix := range chartPrefixes {
		if chart := d.charts.Get(prefix); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (d *DB2) addConnectionCharts(conn *connectionMetrics) {
	charts := newConnectionCharts(conn)
	for _, chart := range *charts {
		if err := d.charts.Add(chart.Copy()); err != nil {
			d.Warningf("failed to add connection chart for %s: %v", conn.applicationID, err)
		}
	}
}

func (d *DB2) removeConnectionCharts(id string) {
	chartPrefixes := []string{
		fmt.Sprintf("connection_%s_state", id),
		fmt.Sprintf("connection_%s_activity", id),
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
}