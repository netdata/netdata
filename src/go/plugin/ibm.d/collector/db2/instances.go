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

	count := 0
	err := d.doQuery(ctx, fmt.Sprintf(queryDatabaseInstances, d.MaxDatabases), func(column, value string, lineEnd bool) {
		var dbName string

		switch column {
		case "DB_NAME":
			dbName = strings.TrimSpace(value)
			if dbName == "" {
				return
			}

			// Apply selector if configured
			if d.databaseSelector != nil && !d.databaseSelector.MatchString(dbName) {
				return // Skip this database
			}

			if _, exists := d.databases[dbName]; !exists {
				d.databases[dbName] = &databaseMetrics{name: dbName}
				d.addDatabaseCharts(d.databases[dbName])
			}
			count++

		case "DB_STATUS":
			if count > 0 && len(d.databases) > 0 {
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

				// Find the current database
				for name, db := range d.databases {
					if db.status == "" { // Not yet set
						db.status = value
						d.mx.databases[name] = databaseInstanceMetrics{
							Status: statusValue,
						}
						break
					}
				}
			}

		case "APPLS_CUR_CONS":
			if count > 0 && len(d.databases) > 0 {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// Find the current database
					for name, db := range d.databases {
						if db.applications == 0 { // Not yet set
							db.applications = v
							if metrics, exists := d.mx.databases[name]; exists {
								metrics.Applications = v
								d.mx.databases[name] = metrics
							}
							break
						}
					}
				}
			}
		}
	})

	return err
}

func (d *DB2) collectBufferpoolInstances(ctx context.Context) error {
	if d.MaxBufferpools <= 0 {
		return nil
	}

	var currentBP string
	err := d.doQuery(ctx, fmt.Sprintf(queryBufferpoolInstances, d.MaxBufferpools), func(column, value string, lineEnd bool) {
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
			}
			d.mx.bufferpools[currentBP] = bufferpoolInstanceMetrics{}

		case "TOTAL_READS":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.Reads = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "TOTAL_WRITES":
			if currentBP != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.Writes = v
					d.mx.bufferpools[currentBP] = metrics
				}
			}

		case "HIT_RATIO":
			if currentBP != "" {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					metrics := d.mx.bufferpools[currentBP]
					metrics.HitRatio = int64(v * precision)
					d.mx.bufferpools[currentBP] = metrics
				}
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

		case "NPAGES":
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
		}
	})

	return err
}

func (d *DB2) collectTablespaceInstances(ctx context.Context) error {
	if d.MaxTablespaces <= 0 {
		return nil
	}

	var currentTbsp string
	err := d.doQuery(ctx, fmt.Sprintf(queryTablespaceInstances, d.MaxTablespaces), func(column, value string, lineEnd bool) {
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

		case "TBSP_TOTAL_SIZE_KB":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.TotalSizeKB = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "TBSP_USED_SIZE_KB":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.UsedSizeKB = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "TBSP_FREE_SIZE_KB":
			if currentTbsp != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.FreeSizeKB = v
					d.mx.tablespaces[currentTbsp] = metrics
				}
			}

		case "USED_PERCENT":
			if currentTbsp != "" {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					metrics := d.mx.tablespaces[currentTbsp]
					metrics.UsedPercent = int64(v * precision)
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

	var currentAppID string
	err := d.doQuery(ctx, fmt.Sprintf(queryConnectionInstances, d.MaxConnections), func(column, value string, lineEnd bool) {
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
