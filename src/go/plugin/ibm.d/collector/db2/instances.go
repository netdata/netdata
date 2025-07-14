// SPDX-License-Identifier: GPL-3.0-or-later

package db2

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func (d *DB2) collectDatabaseInstances(ctx context.Context) error {
	if d.MaxDatabases <= 0 {
		return nil
	}

	var currentDB string
	var currentMetrics databaseInstanceMetrics
	
	err := d.doQuery(ctx, fmt.Sprintf(queryDatabaseInstances, d.MaxDatabases), func(column, value string, lineEnd bool) {
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

			// Apply selector if configured
			if d.databaseSelector != nil && !d.databaseSelector.MatchString(dbName) {
				currentDB = ""
				return // Skip this database
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

	return err
}

func (d *DB2) collectBufferpoolInstances(ctx context.Context) error {
	if d.MaxBufferpools <= 0 {
		return nil
	}

	// Choose query based on monitoring approach and column support
	var query string
	if d.useMonGetFunctions {
		// Note: MON_GET_BUFFERPOOL doesn't support FETCH FIRST, so we'll handle limit in post-processing
		query = queryMonGetBufferpool
		d.Debugf("using MON_GET_BUFFERPOOL for bufferpool instances (modern approach)")
	} else if d.edition == "Cloud" {
		query = fmt.Sprintf(queryBufferpoolInstancesCloud, d.MaxBufferpools)
		d.Debugf("using Cloud-specific bufferpool query for Db2 on Cloud")
	} else if d.supportsColumnOrganizedTables {
		query = fmt.Sprintf(queryBufferpoolInstances, d.MaxBufferpools)
		d.Debugf("using SNAP views with column-organized metrics for bufferpool instances")
	} else {
		query = fmt.Sprintf(queryBufferpoolInstancesLegacy, d.MaxBufferpools)
		d.Debugf("using legacy SNAP views without column-organized metrics for bufferpool instances")
	}

	var currentBP string
	count := 0
	err := d.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		// Handle limit for MON_GET queries
		if d.useMonGetFunctions && count >= d.MaxBufferpools {
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
			d.mx.bufferpools[currentBP] = bufferpoolInstanceMetrics{}

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
			if currentBP != "" && d.useMonGetFunctions {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// For MON_GET, this is already in pages
					metrics := d.mx.bufferpools[currentBP]
					metrics.TotalPages = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "POOL_WATERMARK":
			if currentBP != "" && d.useMonGetFunctions {
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
			if d.useMonGetFunctions {
				// Calculate hits for each type (hits = logical - physical)
				metrics.DataHits = metrics.DataLogicalReads - metrics.DataPhysicalReads
				metrics.DataMisses = metrics.DataPhysicalReads
				
				metrics.IndexHits = metrics.IndexLogicalReads - metrics.IndexPhysicalReads
				metrics.IndexMisses = metrics.IndexPhysicalReads
				
				metrics.XDAHits = metrics.XDALogicalReads - metrics.XDAPhysicalReads
				metrics.XDAMisses = metrics.XDAPhysicalReads
				
				metrics.ColumnHits = metrics.ColumnLogicalReads - metrics.ColumnPhysicalReads
				metrics.ColumnMisses = metrics.ColumnPhysicalReads
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

	// Choose query based on monitoring approach
	var query string
	if d.useMonGetFunctions {
		query = fmt.Sprintf(queryMonGetTablespace, d.MaxTablespaces)
		d.Debugf("using MON_GET_TABLESPACE for tablespace metrics (modern approach)")
	} else {
		query = fmt.Sprintf(queryTablespaceInstances, d.MaxTablespaces)
		d.Debugf("using SNAP views for tablespace metrics (legacy approach)")
	}

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

	// Choose query based on monitoring approach
	var query string
	if d.useMonGetFunctions {
		query = fmt.Sprintf(queryMonGetConnectionDetails, d.MaxConnections)
		d.Debugf("using MON_GET_CONNECTION for connection instances (modern approach)")
	} else if d.edition == "Cloud" {
		query = fmt.Sprintf(queryConnectionInstancesCloud, d.MaxConnections)
		d.Debugf("using Cloud-specific connection query for Db2 on Cloud")
	} else {
		query = fmt.Sprintf(queryConnectionInstances, d.MaxConnections)
		d.Debugf("using SNAP views for connection instances (legacy approach)")
	}

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
