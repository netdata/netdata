// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)


// collectMemoryPoolInstances collects memory pool metrics
func (d *DB2) collectMemoryPoolInstances(ctx context.Context) error {
	// Initialize maps if needed
	if d.mx.memoryPools == nil {
		d.mx.memoryPools = make(map[string]memoryPoolInstanceMetrics)
	}

	// Mark all pools as not updated
	for _, pool := range d.memoryPools {
		pool.updated = false
	}

	var currentPoolType string
	var currentMetrics memoryPoolInstanceMetrics
	
	err := d.doQuery(ctx, queryMonGetMemoryPool, func(column, value string, lineEnd bool) {
		switch column {
		case "MEMORY_POOL_TYPE":
			currentPoolType = cleanName(value)
			
			// Create pool metadata if new
			if _, exists := d.memoryPools[currentPoolType]; !exists {
				d.memoryPools[currentPoolType] = &memoryPoolMetrics{
					poolType: value,
				}
				// Add charts for new pool
				d.addMemoryPoolCharts(d.memoryPools[currentPoolType])
			}
			d.memoryPools[currentPoolType].updated = true
			
		case "MEMORY_POOL_USED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.PoolUsed = v
			}
			
		case "MEMORY_POOL_USED_HWM":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.PoolUsedHWM = v
			}
		}
		
		// At end of row, save metrics
		if lineEnd && currentPoolType != "" {
			d.mx.memoryPools[currentPoolType] = currentMetrics
			currentPoolType = ""
			currentMetrics = memoryPoolInstanceMetrics{}
		}
	})
	
	if err != nil {
		return err
	}
	
	// Remove stale pools
	for poolType, pool := range d.memoryPools {
		if !pool.updated {
			delete(d.memoryPools, poolType)
			d.removeMemoryPoolCharts(poolType)
			delete(d.mx.memoryPools, poolType)
		}
	}
	
	return nil
}

// collectConnectionWaits collects enhanced wait metrics for connections
func (d *DB2) collectConnectionWaits(ctx context.Context) error {
	// This enhances existing connection metrics with wait time details
	// We add wait metrics to the existing connection instance metrics
	
	if !d.CollectWaitMetrics || d.MaxConnections <= 0 {
		return nil
	}
	
	// Build query to get wait metrics from MON_GET_CONNECTION
	query := `
		SELECT 
			APPLICATION_ID,
			TOTAL_WAIT_TIME,
			LOCK_WAIT_TIME,
			LOG_DISK_WAIT_TIME,
			LOG_BUFFER_WAIT_TIME,
			POOL_READ_TIME,
			POOL_WRITE_TIME,
			DIRECT_READ_TIME,
			DIRECT_WRITE_TIME,
			FCM_RECV_WAIT_TIME,
			FCM_SEND_WAIT_TIME,
			TOTAL_ROUTINE_TIME,
			TOTAL_COMPILE_TIME,
			TOTAL_SECTION_TIME,
			TOTAL_COMMIT_TIME,
			TOTAL_ROLLBACK_TIME
		FROM TABLE(MON_GET_CONNECTION(NULL,-2)) AS T
		-- WHERE APPLICATION_HANDLE IS NOT NULL
		ORDER BY TOTAL_WAIT_TIME DESC
	`
	
	var currentAppID string
	var waitMetrics struct {
		TotalWaitTime       int64
		LockWaitTime        int64
		LogDiskWaitTime     int64
		LogBufferWaitTime   int64
		PoolReadTime        int64
		PoolWriteTime       int64
		DirectReadTime      int64
		DirectWriteTime     int64
		FCMRecvWaitTime     int64
		FCMSendWaitTime     int64
		TotalRoutineTime    int64
		TotalCompileTime    int64
		TotalSectionTime    int64
		TotalCommitTime     int64
		TotalRollbackTime   int64
	}
	
	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "APPLICATION_ID":
			currentAppID = strings.TrimSpace(value)
			
		case "TOTAL_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.TotalWaitTime = v
			}
			
		case "LOCK_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.LockWaitTime = v
			}
			
		case "LOG_DISK_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.LogDiskWaitTime = v
			}
			
		case "LOG_BUFFER_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.LogBufferWaitTime = v
			}
			
		case "POOL_READ_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.PoolReadTime = v
			}
			
		case "POOL_WRITE_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.PoolWriteTime = v
			}
			
		case "DIRECT_READ_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.DirectReadTime = v
			}
			
		case "DIRECT_WRITE_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.DirectWriteTime = v
			}
			
		case "FCM_RECV_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.FCMRecvWaitTime = v
			}
			
		case "FCM_SEND_WAIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.FCMSendWaitTime = v
			}
			
		case "TOTAL_ROUTINE_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.TotalRoutineTime = v
			}
			
		case "TOTAL_COMPILE_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.TotalCompileTime = v
			}
			
		case "TOTAL_SECTION_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.TotalSectionTime = v
			}
			
		case "TOTAL_COMMIT_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.TotalCommitTime = v
			}
			
		case "TOTAL_ROLLBACK_TIME":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				waitMetrics.TotalRollbackTime = v
			}
		}
		
		// At end of row, add wait metrics to existing connection metrics
		if lineEnd && currentAppID != "" {
			// Only update if we already track this connection
			if metrics, exists := d.mx.connections[currentAppID]; exists {
				// Add wait time metrics to existing connection metrics
				metrics.TotalWaitTime = waitMetrics.TotalWaitTime
				metrics.LockWaitTime = waitMetrics.LockWaitTime
				metrics.LogDiskWaitTime = waitMetrics.LogDiskWaitTime
				metrics.LogBufferWaitTime = waitMetrics.LogBufferWaitTime
				metrics.PoolReadTime = waitMetrics.PoolReadTime
				metrics.PoolWriteTime = waitMetrics.PoolWriteTime
				metrics.DirectReadTime = waitMetrics.DirectReadTime
				metrics.DirectWriteTime = waitMetrics.DirectWriteTime
				metrics.FCMRecvWaitTime = waitMetrics.FCMRecvWaitTime
				metrics.FCMSendWaitTime = waitMetrics.FCMSendWaitTime
				metrics.TotalRoutineTime = waitMetrics.TotalRoutineTime
				metrics.TotalCompileTime = waitMetrics.TotalCompileTime
				metrics.TotalSectionTime = waitMetrics.TotalSectionTime
				metrics.TotalCommitTime = waitMetrics.TotalCommitTime
				metrics.TotalRollbackTime = waitMetrics.TotalRollbackTime
				
				d.mx.connections[currentAppID] = metrics
			}
			
			// Reset for next row
			currentAppID = ""
			waitMetrics = struct {
				TotalWaitTime       int64
				LockWaitTime        int64
				LogDiskWaitTime     int64
				LogBufferWaitTime   int64
				PoolReadTime        int64
				PoolWriteTime       int64
				DirectReadTime      int64
				DirectWriteTime     int64
				FCMRecvWaitTime     int64
				FCMSendWaitTime     int64
				TotalRoutineTime    int64
				TotalCompileTime    int64
				TotalSectionTime    int64
				TotalCommitTime     int64
				TotalRollbackTime   int64
			}{}
		}
	})
	
	if err != nil {
		return fmt.Errorf("failed to collect connection wait metrics: %w", err)
	}
	
	return nil
}

// collectTableIOInstances collects table I/O statistics
func (d *DB2) collectTableIOInstances(ctx context.Context) error {
	if d.MaxTables <= 0 {
		return nil
	}

	// Initialize maps if needed
	if d.mx.tableIOs == nil {
		d.mx.tableIOs = make(map[string]tableIOInstanceMetrics)
	}
	
	d.Debugf("collectTableIOInstances: starting collection, MaxTables=%d", d.MaxTables)

	// Mark all tables as not updated for I/O metrics
	for _, table := range d.tables {
		table.ioUpdated = false
	}

	query := queryMonGetTable
	
	var currentTableName string
	var currentMetrics tableIOInstanceMetrics
	collected := 0
	
	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "TABSCHEMA":
			if value != "" {
				currentTableName = strings.TrimSpace(value) + "."
			}
			
		case "TABNAME":
			currentTableName += strings.TrimSpace(value)
			cleanName := cleanName(currentTableName)
			
			// Create table metadata if new
			if _, exists := d.tables[cleanName]; !exists {
				d.tables[cleanName] = &tableMetrics{
					name: currentTableName,
				}
				// Add charts for new table
				d.addTableIOCharts(d.tables[cleanName])
			}
			d.tables[cleanName].ioUpdated = true
			collected++
			
		case "TABLE_SCANS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.TableScans = v
			}
			
		case "ROWS_READ":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.RowsRead = v
			}
			
		case "ROWS_INSERTED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.RowsInserted = v
			}
			
		case "ROWS_UPDATED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.RowsUpdated = v
			}
			
		case "ROWS_DELETED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.RowsDeleted = v
			}
			
		case "OVERFLOW_ACCESSES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				currentMetrics.OverflowAccesses = v
			}
		}
		
		// At end of row, save metrics
		if lineEnd && currentTableName != "" {
			cleanName := cleanName(currentTableName)
			d.mx.tableIOs[cleanName] = currentMetrics
			currentTableName = ""
			currentMetrics = tableIOInstanceMetrics{}
		}
	})
	
	if err != nil {
		return err
	}
	
	// Remove stale table I/O metrics
	for name, table := range d.tables {
		if table.ioUpdated == false && d.mx.tableIOs[name].RowsRead > 0 {
			// Only remove if it was being tracked for I/O
			d.removeTableIOCharts(name)
			delete(d.mx.tableIOs, name)
		}
	}
	
	if collected == d.MaxTables {
		d.Debugf("reached max_tables limit (%d) for I/O metrics, some tables may not be collected", d.MaxTables)
	}
	
	d.Debugf("collectTableIOInstances: completed, collected=%d tables, tableIOs map size=%d", collected, len(d.mx.tableIOs))
	
	return nil
}

