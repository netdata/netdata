// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (d *DB2) collectDatabaseInstances(ctx context.Context) error {
	// Handle MaxDatabases == 0 (collect all)
	if d.MaxDatabases == 0 {
		return d.doCollectDatabaseInstances(ctx, false, nil)
	}

	// Handle MaxDatabases == -1 (always filter)
	if d.MaxDatabases == -1 {
		d.databaseFilterMode = true // Force filter mode
		return d.doCollectDatabaseInstances(ctx, true, nil)
	}

	// Handle MaxDatabases > 0 (dynamic threshold)
	if d.databaseFilterMode {
		// Already in filter mode, apply selector
		return d.doCollectDatabaseInstances(ctx, true, nil)
	}

	// Not in filter mode yet, try to collect all and check count
	collectedCount := 0
	err := d.doCollectDatabaseInstances(ctx, false, &collectedCount)
	if err != nil {
		return err
	}

	if collectedCount > d.MaxDatabases {
		d.databaseFilterMode = true // Exceeded limit, switch to filter mode
		d.Warningf("Number of databases (%d) exceeded MaxDatabases (%d). Switching to filter mode. Only databases matching '%s' will be collected.", collectedCount, d.MaxDatabases, d.CollectDatabasesMatching)
		// Re-collect with filtering applied for this cycle
		return d.doCollectDatabaseInstances(ctx, true, nil)
	}

	return nil // Already collected in the check phase
}

// Helper function to encapsulate the actual database instance collection logic
func (d *DB2) doCollectDatabaseInstances(ctx context.Context, applySelector bool, collectedCount *int) error {
	// Reset metrics for this collection pass
	d.mx.databases = make(map[string]databaseInstanceMetrics)

	var currentDB string
	var currentMetrics databaseInstanceMetrics

	// Reset collectedCount if provided
	if collectedCount != nil {
		*collectedCount = 0
	}

	err := d.doQuery(ctx, queryDatabaseInstances, func(column, value string, lineEnd bool) {
		switch column {
		case "DB_NAME":
			// Save previous database metrics if we have any
			if currentDB != "" {
				d.mx.databases[currentDB] = currentMetrics
			}

			dbName := strings.TrimSpace(value)
			if dbName == "" {
				currentDB = ""
				return
			}

			// Apply selector if applySelector is true AND selector is configured
			if applySelector && d.databaseSelector != nil && !d.databaseSelector.MatchString(dbName) {
				currentDB = ""
				return // Skip this database
			}

			// Increment count if we are counting
			if collectedCount != nil {
				*collectedCount++
			}

			currentDB = dbName
			currentMetrics = databaseInstanceMetrics{}

			if _, exists := d.databases[dbName]; !exists {
				d.databases[dbName] = &databaseMetrics{name: dbName}
				d.addDatabaseCharts(d.databases[dbName])
			}

		case "DB_STATUS":
			if currentDB != "" {
				// Map status to numeric value
				statusValue := int64(0)
				switch strings.ToUpper(value) {
				case "ACTIVE":
					statusValue = 1
				case "INACTIVE":
					statusValue = 0
				default:
					statusValue = -1
				}

				d.databases[currentDB].status = value
				currentMetrics.Status = statusValue
			}

		case "APPLS_CUR_CONS":
			if currentDB != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					d.databases[currentDB].applications = v
					currentMetrics.Applications = v
				}
			}
		}

		// At end of row, save the metrics
		if lineEnd && currentDB != "" {
			d.mx.databases[currentDB] = currentMetrics
			currentDB = ""
		}
	})

	// Save last database if query ended without lineEnd
	if currentDB != "" {
		d.mx.databases[currentDB] = currentMetrics
	}

	// Count active and inactive databases
	for dbName, db := range d.databases {
		if _, exists := d.mx.databases[dbName]; exists {
			// Database was collected this cycle
			if strings.ToUpper(db.status) == "ACTIVE" {
				d.mx.DatabaseCountActive++
			} else {
				d.mx.DatabaseCountInactive++
			}
		}
	}

	return err
}

func (d *DB2) collectBufferpoolInstances(ctx context.Context) error {
	if d.MaxBufferpools <= 0 {
		return nil
	}

	// Always use MON_GET_BUFFERPOOL for bufferpool instances
	// Note: MON_GET_BUFFERPOOL doesn't support FETCH FIRST, so we'll handle limit in post-processing
	query := queryMonGetBufferpool
	d.Debugf("using MON_GET_BUFFERPOOL for bufferpool instances")

	var currentBP string
	count := 0
	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		// Handle limit for MON_GET queries
		if count >= d.MaxBufferpools {
			return
		}
		switch column {
		case "BP_NAME":
			currentBP = strings.TrimSpace(value)
			if currentBP == "" {
				return
			}

			// Apply selector if configured
			if d.bufferpoolSelector != nil && !d.bufferpoolSelector.MatchString(currentBP) {
				currentBP = "" // Skip this bufferpool
				return
			}

			if _, exists := d.bufferpools[currentBP]; !exists {
				d.bufferpools[currentBP] = &bufferpoolMetrics{name: currentBP}
				d.addBufferpoolCharts(d.bufferpools[currentBP])
				count++
			}
			if _, exists := d.mx.bufferpools[currentBP]; !exists {
				d.mx.bufferpools[currentBP] = bufferpoolInstanceMetrics{}
			}

		case "PAGESIZE":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					d.bufferpools[currentBP].pageSize = v
					metrics := d.mx.bufferpools[currentBP]
					metrics.PageSize = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "TOTAL_PAGES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.TotalPages = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "USED_PAGES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.UsedPages = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "DATA_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.DataHits = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "INDEX_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.IndexHits = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "XDA_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.XDAHits = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "COL_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.ColumnHits = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.LogicalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.PhysicalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "TOTAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.TotalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "DATA_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.DataLogicalReads = v
					// Calculate data misses
					metrics.DataMisses = v - metrics.DataHits
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "DATA_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.DataPhysicalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "INDEX_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.IndexLogicalReads = v
					// Calculate index misses
					metrics.IndexMisses = v - metrics.IndexHits
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "INDEX_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.IndexPhysicalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "XDA_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.XDALogicalReads = v
					// Calculate XDA misses
					metrics.XDAMisses = v - metrics.XDAHits
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "XDA_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.XDAPhysicalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "COLUMN_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.ColumnLogicalReads = v
					// Calculate column misses
					metrics.ColumnMisses = v - metrics.ColumnHits
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "COLUMN_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.ColumnPhysicalReads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "WRITES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.Writes = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		// MON_GET specific fields for buffer pool instances
		case "POOL_CUR_SIZE":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// For MON_GET, this is already in pages
					metrics := d.mx.bufferpools[currentBP]
					metrics.TotalPages = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "POOL_WATERMARK":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// Watermark can be used as an indicator of usage
					metrics := d.mx.bufferpools[currentBP]
					metrics.UsedPages = v // Approximation for MON_GET
					d.mx.bufferpools[currentBP] = metrics
				}
			}
		}

		// Calculate hit/miss metrics at end of line
		if lineEnd && currentBP != "" {
			metrics := d.mx.bufferpools[currentBP]

			// For MON_GET queries, we calculate hits from logical - physical
			// Calculate hits for each type (hits = logical - physical)
			metrics.DataHits = metrics.DataLogicalReads - metrics.DataPhysicalReads
			metrics.DataMisses = metrics.DataPhysicalReads

			metrics.IndexHits = metrics.IndexLogicalReads - metrics.IndexPhysicalReads
			metrics.IndexMisses = metrics.IndexPhysicalReads

			metrics.XDAHits = metrics.XDALogicalReads - metrics.XDAPhysicalReads
			metrics.XDAMisses = metrics.XDAPhysicalReads

			metrics.ColumnHits = metrics.ColumnLogicalReads - metrics.ColumnPhysicalReads
			metrics.ColumnMisses = metrics.ColumnPhysicalReads

			// Ensure hits are not negative (shouldn't happen with MON_GET approach but safety check)
			if metrics.DataHits < 0 {
				metrics.DataHits = 0
				metrics.DataMisses = metrics.DataLogicalReads
			}
			if metrics.IndexHits < 0 {
				metrics.IndexHits = 0
				metrics.IndexMisses = metrics.IndexLogicalReads
			}
			if metrics.XDAHits < 0 {
				metrics.XDAHits = 0
				metrics.XDAMisses = metrics.XDALogicalReads
			}
			if metrics.ColumnHits < 0 {
				metrics.ColumnHits = 0
				metrics.ColumnMisses = metrics.ColumnLogicalReads
			}

			// Calculate overall hits and misses
			metrics.Hits = metrics.DataHits + metrics.IndexHits + metrics.XDAHits + metrics.ColumnHits
			metrics.Misses = metrics.DataMisses + metrics.IndexMisses + metrics.XDAMisses + metrics.ColumnMisses

			// Calculate total reads
			totalLogical := metrics.DataLogicalReads + metrics.IndexLogicalReads + metrics.XDALogicalReads + metrics.ColumnLogicalReads
			totalPhysical := metrics.DataPhysicalReads + metrics.IndexPhysicalReads + metrics.XDAPhysicalReads + metrics.ColumnPhysicalReads
			metrics.LogicalReads = totalLogical
			metrics.PhysicalReads = totalPhysical
			metrics.TotalReads = totalLogical + totalPhysical

			d.mx.bufferpools[currentBP] = metrics
		}
	})

	return err
}

func (d *DB2) collectTablespaceInstances(ctx context.Context) error {
	if d.MaxTablespaces <= 0 {
		return nil
	}

	// Always use MON_GET_TABLESPACE for tablespace metrics
	query := queryMonGetTablespace
	d.Debugf("using MON_GET_TABLESPACE for tablespace metrics")

	var currentTbsp string
	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "TBSP_NAME":
			currentTbsp = strings.TrimSpace(value)
			if currentTbsp == "" {
				return
			}

			if _, exists := d.tablespaces[currentTbsp]; !exists {
				d.tablespaces[currentTbsp] = &tablespaceMetrics{name: currentTbsp}
				d.addTablespaceCharts(d.tablespaces[currentTbsp])
			}
			d.mx.tablespaces[currentTbsp] = tablespaceInstanceMetrics{}

		case "TBSP_TYPE":
			if currentTbsp != "" {
				d.tablespaces[currentTbsp].tbspType = value
			}

		case "TBSP_CONTENT_TYPE":
			if currentTbsp != "" {
				d.tablespaces[currentTbsp].contentType = value
			}

		case "TBSP_STATE":
			if currentTbsp != "" {
				d.tablespaces[currentTbsp].state = value
				// Map state to numeric
				stateValue := int64(0)
				switch strings.ToUpper(value) {
				case "NORMAL":
					stateValue = 1
				case "OFFLINE":
					stateValue = 0
				default:
					stateValue = -1
				}
				metrics := d.mx.tablespaces[currentTbsp]
				metrics.State = stateValue
				d.mx.tablespaces[currentTbsp] = metrics
			}

		case "TOTAL_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.TotalSize = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USED_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.UsedSize = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "FREE_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.FreeSize = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USABLE_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.UsableSize = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USED_PERCENT":
			if currentTbsp != "" {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.UsedPercent = int64(v * Precision)
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "TBSP_PAGE_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.PageSize = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}
		}
	})

	return err
}

func (d *DB2) collectConnectionInstances(ctx context.Context) error {
	if d.MaxConnections <= 0 {
		return nil
	}

	// Always use MON_GET_CONNECTION for connection instances
	query := queryMonGetConnectionDetails
	d.Debugf("using MON_GET_CONNECTION for connection instances")

	var currentAppID string
	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "APPLICATION_ID":
			currentAppID = strings.TrimSpace(value)
			if currentAppID == "" {
				return
			}

			if _, exists := d.connections[currentAppID]; !exists {
				d.connections[currentAppID] = &connectionMetrics{applicationID: currentAppID}
				d.addConnectionCharts(d.connections[currentAppID])
			}
			d.mx.connections[currentAppID] = connectionInstanceMetrics{}

		case "APPLICATION_NAME":
			if currentAppID != "" {
				d.connections[currentAppID].applicationName = value
			}

		case "CLIENT_HOSTNAME":
			if currentAppID != "" {
				d.connections[currentAppID].clientHostname = value
			}

		case "CLIENT_IPADDR":
			if currentAppID != "" {
				d.connections[currentAppID].clientIP = value
			}

		case "SESSION_AUTH_ID":
			if currentAppID != "" {
				d.connections[currentAppID].clientUser = value
			}

		case "APPL_STATUS":
			if currentAppID != "" {
				d.connections[currentAppID].connectionState = value
				// Map state to numeric
				stateValue := int64(0)
				execQueries := int64(0)
				switch strings.ToUpper(value) {
				case "CONNECTED":
					stateValue = 1
				case "UOWEXEC":
					stateValue = 2
					execQueries = 1
				}
				metrics := d.mx.connections[currentAppID]
				metrics.State = stateValue
				metrics.ExecutingQueries = execQueries
				d.mx.connections[currentAppID] = metrics
			}

		case "ROWS_READ":
			if currentAppID != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.connections[currentAppID]
					metrics.RowsRead = v
					d.mx.connections[currentAppID] = metrics
				}
			}

		case "ROWS_WRITTEN":
			if currentAppID != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.connections[currentAppID]
					metrics.RowsWritten = v
					d.mx.connections[currentAppID] = metrics
				}
			}

		case "TOTAL_CPU_TIME":
			if currentAppID != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.connections[currentAppID]
					metrics.TotalCPUTime = v
					d.mx.connections[currentAppID] = metrics
				}
			}
		}
	})

	return err
}

// Screen 26: Instance Memory Sets collection using MON_GET_MEMORY_SET
func (d *DB2) collectMemorySetInstances(ctx context.Context) error {
	// Always use MON_GET_MEMORY_SET for memory set metrics
	query := queryMonGetMemorySet

	d.Debugf("collecting memory set instances using MON_GET_MEMORY_SET")

	// Track seen memory sets for lifecycle management
	seen := make(map[string]bool)

	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "HOST_NAME" {
			d.currentMemorySetHostName = value
			return
		}

		if d.currentMemorySetHostName == "" {
			return // Skip until we have a host name
		}

		switch column {
		case "DB_NAME":
			d.currentMemorySetDBName = value
		case "MEMORY_SET_TYPE":
			d.currentMemorySetType = value
		case "MEMBER":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				d.currentMemorySetMember = v
			}
		}

		// Process metrics when we have complete record
		if lineEnd && d.currentMemorySetHostName != "" && d.currentMemorySetDBName != "" {
			// Use DATABASE as default type if not set (ODBC panic might prevent collection)
			if d.currentMemorySetType == "" {
				d.currentMemorySetType = "DATABASE"
			}

			// Create unique identifier for this memory set
			setKey := fmt.Sprintf("%s.%s.%s.%d",
				d.currentMemorySetHostName,
				d.currentMemorySetDBName,
				d.currentMemorySetType,
				d.currentMemorySetMember)

			seen[setKey] = true

			// Initialize memory set if not seen before
			if _, exists := d.memorySets[setKey]; !exists {
				d.memorySets[setKey] = &memorySetInstanceMetrics{
					hostName: d.currentMemorySetHostName,
					dbName:   d.currentMemorySetDBName,
					setType:  d.currentMemorySetType,
					member:   d.currentMemorySetMember,
				}
				d.addMemorySetCharts(setKey)
				d.Debugf("created memory set instance: %s (type: %s)", setKey, d.currentMemorySetType)
			}

			// Reset for next record
			d.currentMemorySetHostName = ""
			d.currentMemorySetDBName = ""
			d.currentMemorySetType = ""
			d.currentMemorySetMember = 0
		}

		// Process individual metrics
		if d.currentMemorySetHostName != "" && d.currentMemorySetDBName != "" {
			// Use DATABASE as default type if not set
			memSetType := d.currentMemorySetType
			if memSetType == "" {
				memSetType = "DATABASE"
			}

			setKey := fmt.Sprintf("%s.%s.%s.%d",
				d.currentMemorySetHostName,
				d.currentMemorySetDBName,
				memSetType,
				d.currentMemorySetMember)

			// Initialize memory set if it doesn't exist
			if _, exists := d.memorySets[setKey]; !exists {
				d.memorySets[setKey] = &memorySetInstanceMetrics{
					hostName: d.currentMemorySetHostName,
					dbName:   d.currentMemorySetDBName,
					setType:  memSetType,
					member:   d.currentMemorySetMember,
				}
			}

			// Update the metrics
			ms := d.memorySets[setKey]
			switch column {
			case "MEMORY_SET_USED":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					ms.Used = v
				}
			case "MEMORY_SET_COMMITTED":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					ms.Committed = v
				}
			case "MEMORY_SET_USED_HWM":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					ms.HighWaterMark = v
				}
			case "ADDITIONAL_COMMITTED":
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					ms.AdditionalCommitted = v
				}
			case "PERCENT_USED_HWM":
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					ms.PercentUsedHWM = int64(v * Precision)
				}
			}
		}
	})

	// Ensure charts are created for all memory sets that were seen
	// This is needed because ODBC panic might prevent the normal chart creation
	for setKey := range seen {
		if _, exists := d.memorySets[setKey]; exists {
			// Check if charts exist for this memory set
			// Need to match the format used in newMemorySetCharts
			parts := strings.Split(setKey, ".")
			if len(parts) >= 4 {
				setIdentifier := fmt.Sprintf("%s_%s_%s_%s",
					cleanName(parts[0]), // host name
					cleanName(parts[1]), // db name
					cleanName(parts[2]), // set type
					parts[3])            // member number
				chartID := fmt.Sprintf("memory_set_%s_usage", setIdentifier)
				if d.charts.Get(chartID) == nil {
					// Charts don't exist, create them now
					d.addMemorySetCharts(setKey)
					d.Debugf("created missing charts for memory set %s", setKey)
				}
			}
		}
	}

	// Remove stale memory sets
	for setKey := range d.memorySets {
		if !seen[setKey] {
			delete(d.memorySets, setKey)
			d.removeMemorySetCharts(setKey)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to collect memory set instances: %w", err)
	}

	d.Debugf("collected %d memory set instances", len(seen))
	for k := range seen {
		d.Debugf("  memory set: %s", k)
	}
	return nil
}

func (d *DB2) collectPrefetcherInstances(ctx context.Context) error {
	// Always use MON_GET functions for prefetcher metrics
	// Track seen prefetcher instances
	seen := make(map[string]bool)
	d.currentBufferPoolName = ""

	err := d.doQuery(ctx, queryPrefetcherMetrics, func(column, value string, lineEnd bool) {
		switch column {
		case "BUFFERPOOL_NAME":
			bufferPoolName := strings.TrimSpace(value)
			if bufferPoolName == "" {
				return
			}

			d.currentBufferPoolName = bufferPoolName
			seen[bufferPoolName] = true

			// Create prefetcher instance if new
			if _, exists := d.prefetchers[bufferPoolName]; !exists {
				d.prefetchers[bufferPoolName] = &prefetcherInstanceMetrics{}
				d.addPrefetcherCharts(d.prefetchers[bufferPoolName], bufferPoolName)
			}

		case "PREFETCH_RATIO_PCT":
			if d.currentBufferPoolName != "" && d.prefetchers[d.currentBufferPoolName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					d.prefetchers[d.currentBufferPoolName].PrefetchRatio = int64(v * Precision)
				}
			}

		case "CLEANER_RATIO_PCT":
			if d.currentBufferPoolName != "" && d.prefetchers[d.currentBufferPoolName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					d.prefetchers[d.currentBufferPoolName].CleanerRatio = int64(v * Precision)
				}
			}

		case "PHYSICAL_READS":
			if d.currentBufferPoolName != "" && d.prefetchers[d.currentBufferPoolName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					d.prefetchers[d.currentBufferPoolName].PhysicalReads = v
				}
			}

		case "ASYNCHRONOUS_READS":
			if d.currentBufferPoolName != "" && d.prefetchers[d.currentBufferPoolName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					d.prefetchers[d.currentBufferPoolName].AsyncReads = v
				}
			}

		case "PREFETCH_WAITS_TIME_MS":
			if d.currentBufferPoolName != "" && d.prefetchers[d.currentBufferPoolName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					d.prefetchers[d.currentBufferPoolName].AvgWaitTime = int64(v * Precision)
				}
			}

		case "UNREAD_PREFETCH_PAGES":
			if d.currentBufferPoolName != "" && d.prefetchers[d.currentBufferPoolName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					d.prefetchers[d.currentBufferPoolName].UnreadPages = v
				}
			}
		}
	})

	// Remove stale prefetcher instances
	for bufferPoolName := range d.prefetchers {
		if !seen[bufferPoolName] {
			delete(d.prefetchers, bufferPoolName)
			d.removePrefetcherCharts(bufferPoolName)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to collect prefetcher instances: %w", err)
	}

	d.Debugf("collected %d prefetcher instances", len(seen))
	return nil
}
