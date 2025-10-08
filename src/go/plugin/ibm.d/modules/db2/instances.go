// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package db2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (c *Collector) collectDatabaseInstances(ctx context.Context) error {
	// Handle MaxDatabases == 0 (collect all)
	if c.MaxDatabases == 0 {
		return c.doCollectDatabaseInstances(ctx, false, nil)
	}

	// Handle MaxDatabases == -1 (always filter)
	if c.MaxDatabases == -1 {
		c.databaseFilterMode = true // Force filter mode
		return c.doCollectDatabaseInstances(ctx, true, nil)
	}

	// Handle MaxDatabases > 0 (dynamic threshold)
	if c.databaseFilterMode {
		// Already in filter mode, apply selector
		return c.doCollectDatabaseInstances(ctx, true, nil)
	}

	// Not in filter mode yet, try to collect all and check count
	collectedCount := 0
	err := c.doCollectDatabaseInstances(ctx, false, &collectedCount)
	if err != nil {
		return err
	}

	if collectedCount > c.MaxDatabases {
		c.databaseFilterMode = true // Exceeded limit, switch to filter mode
		c.Warningf("Number of databases (%d) exceeded MaxDatabases (%d). Switching to filter mode. Only databases matching '%s' will be collected.", collectedCount, c.MaxDatabases, c.CollectDatabasesMatching)
		// Re-collect with filtering applied for this cycle
		return c.doCollectDatabaseInstances(ctx, true, nil)
	}

	return nil // Already collected in the check phase
}

// Helper function to encapsulate the actual database instance collection logic
func (c *Collector) doCollectDatabaseInstances(ctx context.Context, applySelector bool, collectedCount *int) error {
	// Reset metrics for this collection pass
	c.mx.databases = make(map[string]databaseInstanceMetrics)

	var currentDB string
	var currentMetrics databaseInstanceMetrics

	// Reset collectedCount if provided
	if collectedCount != nil {
		*collectedCount = 0
	}

	err := c.doQuery(ctx, queryDatabaseInstances, func(column, value string, lineEnd bool) {
		switch column {
		case "DB_NAME":
			// Save previous database metrics if we have any
			if currentDB != "" {
				c.mx.databases[currentDB] = currentMetrics
			}

			dbName := strings.TrimSpace(value)
			if dbName == "" {
				currentDB = ""
				return
			}

			// Apply selector if applySelector is true AND selector is configured
			if applySelector && c.databaseSelector != nil && !c.databaseSelector.MatchString(dbName) {
				currentDB = ""
				return // Skip this database
			}

			// Increment count if we are counting
			if collectedCount != nil {
				*collectedCount++
			}

			currentDB = dbName
			currentMetrics = databaseInstanceMetrics{}

			if _, exists := c.databases[dbName]; !exists {
				c.databases[dbName] = &databaseMetrics{name: dbName}
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

				c.databases[currentDB].status = value
				currentMetrics.Status = statusValue
			}

		case "APPLS_CUR_CONS":
			if currentDB != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					c.databases[currentDB].applications = v
					currentMetrics.Applications = v
				}
			}
		}

		// At end of row, save the metrics
		if lineEnd && currentDB != "" {
			c.mx.databases[currentDB] = currentMetrics
			currentDB = ""
		}
	})

	// Save last database if query ended without lineEnd
	if currentDB != "" {
		c.mx.databases[currentDB] = currentMetrics
	}

	// Count active and inactive databases
	for dbName, db := range c.databases {
		if _, exists := c.mx.databases[dbName]; exists {
			// Database was collected this cycle
			if strings.ToUpper(db.status) == "ACTIVE" {
				c.mx.DatabaseCountActive++
			} else {
				c.mx.DatabaseCountInactive++
			}
		}
	}

	return err
}

func (c *Collector) collectBufferpoolInstances(ctx context.Context) error {
	if c.MaxBufferpools <= 0 {
		return nil
	}

	// Always use MON_GET_BUFFERPOOL for bufferpool instances
	// Note: MON_GET_BUFFERPOOL doesn't support FETCH FIRST, so we'll handle limit in post-processing
	query := queryMonGetBufferpool
	c.Debugf("using MON_GET_BUFFERPOOL for bufferpool instances")

	var currentBP string
	count := 0
	err := c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		// Handle limit for MON_GET queries
		if count >= c.MaxBufferpools {
			return
		}
		switch column {
		case "BP_NAME":
			currentBP = strings.TrimSpace(value)
			if currentBP == "" {
				return
			}

			// Apply selector if configured
			if c.bufferpoolSelector != nil && !c.bufferpoolSelector.MatchString(currentBP) {
				currentBP = "" // Skip this bufferpool
				return
			}

			if _, exists := c.bufferpools[currentBP]; !exists {
				c.bufferpools[currentBP] = &bufferpoolMetrics{name: currentBP}
				count++
			}
			if _, exists := c.mx.bufferpools[currentBP]; !exists {
				c.mx.bufferpools[currentBP] = bufferpoolInstanceMetrics{}
			}

		case "PAGESIZE":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					c.bufferpools[currentBP].pageSize = v
					metrics := c.mx.bufferpools[currentBP]
					metrics.PageSize = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "TOTAL_PAGES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.TotalPages = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "USED_PAGES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.UsedPages = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "DATA_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.DataHits = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "INDEX_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.IndexHits = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "XDA_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.XDAHits = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "COL_PAGES_FOUND":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.ColumnHits = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.LogicalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.PhysicalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "TOTAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.TotalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "DATA_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.DataLogicalReads = v
					// Calculate data misses
					metrics.DataMisses = v - metrics.DataHits
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "DATA_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.DataPhysicalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "INDEX_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.IndexLogicalReads = v
					// Calculate index misses
					metrics.IndexMisses = v - metrics.IndexHits
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "INDEX_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.IndexPhysicalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "XDA_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.XDALogicalReads = v
					// Calculate XDA misses
					metrics.XDAMisses = v - metrics.XDAHits
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "XDA_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.XDAPhysicalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "COLUMN_LOGICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.ColumnLogicalReads = v
					// Calculate column misses
					metrics.ColumnMisses = v - metrics.ColumnHits
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "COLUMN_PHYSICAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.ColumnPhysicalReads = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "WRITES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.bufferpools[currentBP]
					metrics.Writes = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		// MON_GET specific fields for buffer pool instances
		case "POOL_CUR_SIZE":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// For MON_GET, this is already in pages
					metrics := c.mx.bufferpools[currentBP]
					metrics.TotalPages = v
					c.mx.bufferpools[currentBP] = metrics
				}
			}

		case "POOL_WATERMARK":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// Watermark can be used as an indicator of usage
					metrics := c.mx.bufferpools[currentBP]
					metrics.UsedPages = v // Approximation for MON_GET
					c.mx.bufferpools[currentBP] = metrics
				}
			}
		}

		// Calculate hit/miss metrics at end of line
		if lineEnd && currentBP != "" {
			metrics := c.mx.bufferpools[currentBP]

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

			c.mx.bufferpools[currentBP] = metrics
		}
	})

	return err
}

func (c *Collector) collectTablespaceInstances(ctx context.Context) error {
	if c.MaxTablespaces <= 0 {
		return nil
	}

	// Always use MON_GET_TABLESPACE for tablespace metrics
	query := queryMonGetTablespace
	c.Debugf("using MON_GET_TABLESPACE for tablespace metrics")

	var currentTbsp string
	err := c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "TBSP_NAME":
			currentTbsp = strings.TrimSpace(value)
			if currentTbsp == "" {
				return
			}

			if _, exists := c.tablespaces[currentTbsp]; !exists {
				c.tablespaces[currentTbsp] = &tablespaceMetrics{name: currentTbsp}
			}
			c.mx.tablespaces[currentTbsp] = tablespaceInstanceMetrics{}

		case "TBSP_TYPE":
			if currentTbsp != "" {
				c.tablespaces[currentTbsp].tbspType = value
			}

		case "TBSP_CONTENT_TYPE":
			if currentTbsp != "" {
				c.tablespaces[currentTbsp].contentType = value
			}

		case "TBSP_STATE":
			if currentTbsp != "" {
				c.tablespaces[currentTbsp].state = value
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
				metrics := c.mx.tablespaces[currentTbsp]
				metrics.State = stateValue
				c.mx.tablespaces[currentTbsp] = metrics
			}

		case "TOTAL_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tablespaces[currentTbsp]
					metrics.TotalSize = v
					c.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USED_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tablespaces[currentTbsp]
					metrics.UsedSize = v
					c.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "FREE_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tablespaces[currentTbsp]
					metrics.FreeSize = v
					c.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USABLE_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tablespaces[currentTbsp]
					metrics.UsableSize = v
					c.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USED_PERCENT":
			if currentTbsp != "" {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					metrics := c.mx.tablespaces[currentTbsp]
					metrics.UsedPercent = int64(v * Precision)
					c.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "TBSP_PAGE_SIZE":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.tablespaces[currentTbsp]
					metrics.PageSize = v
					c.mx.tablespaces[currentTbsp] = metrics
				}
			}
		}
	})

	return err
}

func (c *Collector) collectConnectionInstances(ctx context.Context) error {
	if c.MaxConnections <= 0 {
		return nil
	}

	// Always use MON_GET_CONNECTION for connection instances
	query := queryMonGetConnectionDetails
	c.Debugf("using MON_GET_CONNECTION for connection instances")

	var currentAppID string
	err := c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "APPLICATION_ID":
			currentAppID = strings.TrimSpace(value)
			if currentAppID == "" {
				return
			}

			if _, exists := c.connections[currentAppID]; !exists {
				c.connections[currentAppID] = &connectionMetrics{applicationID: currentAppID}
			}
			c.mx.connections[currentAppID] = connectionInstanceMetrics{}

		case "APPLICATION_NAME":
			if currentAppID != "" {
				c.connections[currentAppID].applicationName = value
			}

		case "CLIENT_HOSTNAME":
			if currentAppID != "" {
				c.connections[currentAppID].clientHostname = value
			}

		case "CLIENT_IPADDR":
			if currentAppID != "" {
				c.connections[currentAppID].clientIP = value
			}

		case "SESSION_AUTH_ID":
			if currentAppID != "" {
				c.connections[currentAppID].clientUser = value
			}

		case "APPL_STATUS":
			if currentAppID != "" {
				c.connections[currentAppID].connectionState = value
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
				metrics := c.mx.connections[currentAppID]
				metrics.State = stateValue
				metrics.ExecutingQueries = execQueries
				c.mx.connections[currentAppID] = metrics
			}

		case "ROWS_READ":
			if currentAppID != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.connections[currentAppID]
					metrics.RowsRead = v
					c.mx.connections[currentAppID] = metrics
				}
			}

		case "ROWS_WRITTEN":
			if currentAppID != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.connections[currentAppID]
					metrics.RowsWritten = v
					c.mx.connections[currentAppID] = metrics
				}
			}

		case "TOTAL_CPU_TIME":
			if currentAppID != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := c.mx.connections[currentAppID]
					metrics.TotalCPUTime = v
					c.mx.connections[currentAppID] = metrics
				}
			}
		}
	})

	return err
}

// Screen 26: Instance Memory Sets collection using MON_GET_MEMORY_SET
func (c *Collector) collectMemorySetInstances(ctx context.Context) error {
	// Always use MON_GET_MEMORY_SET for memory set metrics
	query := queryMonGetMemorySet

	c.Debugf("collecting memory set instances using MON_GET_MEMORY_SET")

	// Track seen memory sets for lifecycle management
	seen := make(map[string]bool)

	err := c.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "HOST_NAME" {
			c.currentMemorySetHostName = value
			return
		}

		if c.currentMemorySetHostName == "" {
			return // Skip until we have a host name
		}

		switch column {
		case "DB_NAME":
			c.currentMemorySetDBName = value
		case "MEMORY_SET_TYPE":
			c.currentMemorySetType = value
		case "MEMBER":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				c.currentMemorySetMember = v
			}
		}

		// Process metrics when we have complete record
		if lineEnd && c.currentMemorySetHostName != "" && c.currentMemorySetDBName != "" {
			// Use DATABASE as default type if not set (ODBC panic might prevent collection)
			if c.currentMemorySetType == "" {
				c.currentMemorySetType = "DATABASE"
			}

			// Create unique identifier for this memory set
			setKey := fmt.Sprintf("%s.%s.%s.%d",
				c.currentMemorySetHostName,
				c.currentMemorySetDBName,
				c.currentMemorySetType,
				c.currentMemorySetMember)

			seen[setKey] = true

			// Initialize memory set if not seen before
			if _, exists := c.memorySets[setKey]; !exists {
				c.memorySets[setKey] = &memorySetInstanceMetrics{
					hostName: c.currentMemorySetHostName,
					dbName:   c.currentMemorySetDBName,
					setType:  c.currentMemorySetType,
					member:   c.currentMemorySetMember,
				}
				c.Debugf("created memory set instance: %s (type: %s)", setKey, c.currentMemorySetType)
			}

			// Reset for next record
			c.currentMemorySetHostName = ""
			c.currentMemorySetDBName = ""
			c.currentMemorySetType = ""
			c.currentMemorySetMember = 0
		}

		// Process individual metrics
		if c.currentMemorySetHostName != "" && c.currentMemorySetDBName != "" {
			// Use DATABASE as default type if not set
			memSetType := c.currentMemorySetType
			if memSetType == "" {
				memSetType = "DATABASE"
			}

			setKey := fmt.Sprintf("%s.%s.%s.%d",
				c.currentMemorySetHostName,
				c.currentMemorySetDBName,
				memSetType,
				c.currentMemorySetMember)

			// Initialize memory set if it doesn't exist
			if _, exists := c.memorySets[setKey]; !exists {
				c.memorySets[setKey] = &memorySetInstanceMetrics{
					hostName: c.currentMemorySetHostName,
					dbName:   c.currentMemorySetDBName,
					setType:  memSetType,
					member:   c.currentMemorySetMember,
				}
			}

			// Update the metrics
			ms := c.memorySets[setKey]
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

	// Remove stale memory sets
	for setKey := range c.memorySets {
		if !seen[setKey] {
			delete(c.memorySets, setKey)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to collect memory set instances: %w", err)
	}

	c.Debugf("collected %d memory set instances", len(seen))
	for k := range seen {
		c.Debugf("  memory set: %s", k)
	}
	return nil
}

func (c *Collector) collectPrefetcherInstances(ctx context.Context) error {
	// Always use MON_GET functions for prefetcher metrics
	// Track seen prefetcher instances
	seen := make(map[string]bool)
	c.currentBufferPoolName = ""

	err := c.doQuery(ctx, queryPrefetcherMetrics, func(column, value string, lineEnd bool) {
		switch column {
		case "BUFFERPOOL_NAME":
			bufferPoolName := strings.TrimSpace(value)
			if bufferPoolName == "" {
				return
			}

			c.currentBufferPoolName = bufferPoolName
			seen[bufferPoolName] = true

			// Create prefetcher instance if new
			if _, exists := c.prefetchers[bufferPoolName]; !exists {
				c.prefetchers[bufferPoolName] = &prefetcherInstanceMetrics{}
			}

		case "PREFETCH_RATIO_PCT":
			if c.currentBufferPoolName != "" && c.prefetchers[c.currentBufferPoolName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					c.prefetchers[c.currentBufferPoolName].PrefetchRatio = int64(v * Precision)
				}
			}

		case "CLEANER_RATIO_PCT":
			if c.currentBufferPoolName != "" && c.prefetchers[c.currentBufferPoolName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					c.prefetchers[c.currentBufferPoolName].CleanerRatio = int64(v * Precision)
				}
			}

		case "PHYSICAL_READS":
			if c.currentBufferPoolName != "" && c.prefetchers[c.currentBufferPoolName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					c.prefetchers[c.currentBufferPoolName].PhysicalReads = v
				}
			}

		case "ASYNCHRONOUS_READS":
			if c.currentBufferPoolName != "" && c.prefetchers[c.currentBufferPoolName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					c.prefetchers[c.currentBufferPoolName].AsyncReads = v
				}
			}

		case "PREFETCH_WAITS_TIME_MS":
			if c.currentBufferPoolName != "" && c.prefetchers[c.currentBufferPoolName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					c.prefetchers[c.currentBufferPoolName].AvgWaitTime = int64(v * Precision)
				}
			}

		case "UNREAD_PREFETCH_PAGES":
			if c.currentBufferPoolName != "" && c.prefetchers[c.currentBufferPoolName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					c.prefetchers[c.currentBufferPoolName].UnreadPages = v
				}
			}
		}
	})

	// Remove stale prefetcher instances
	for bufferPoolName := range c.prefetchers {
		if !seen[bufferPoolName] {
			delete(c.prefetchers, bufferPoolName)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to collect prefetcher instances: %w", err)
	}

	c.Debugf("collected %d prefetcher instances", len(seen))
	return nil
}
