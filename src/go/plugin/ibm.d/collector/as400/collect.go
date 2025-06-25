// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func (a *AS400) collect(ctx context.Context) (map[string]int64, error) {
	if a.db == nil {
		db, err := a.initDatabase(ctx)
		if err != nil {
			return nil, err
		}
		a.db = db
	}

	// Reset metrics
	a.mx = &metricsData{
		disks:      make(map[string]diskInstanceMetrics),
		subsystems: make(map[string]subsystemInstanceMetrics),
		jobQueues:  make(map[string]jobQueueInstanceMetrics),
	}

	// Test connection with a ping before proceeding
	if err := a.ping(ctx); err != nil {
		// Try to reconnect once
		a.Cleanup(ctx)
		db, err := a.initDatabase(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to reconnect to database: %v", err)
		}
		a.db = db
		
		// Retry ping
		if err := a.ping(ctx); err != nil {
			return nil, fmt.Errorf("database connection failed after reconnect: %v", err)
		}
	}
	
	// Collect system-wide metrics
	if err := a.collectSystemStatus(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect system status: %v", err)
	}

	if err := a.collectMemoryPools(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect memory pools: %v", err)
	}

	// Collect aggregate disk status
	if err := a.collectDiskStatus(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect disk status: %v", err)
	}

	// Collect aggregate job info
	if err := a.collectJobInfo(ctx); err != nil {
		return nil, fmt.Errorf("failed to collect job info: %v", err)
	}

	// Collect per-instance metrics if enabled
	if a.CollectDiskMetrics {
		if err := a.collectDiskInstances(ctx); err != nil {
			a.Errorf("failed to collect disk instances: %v", err)
		}
	}

	if a.CollectSubsystemMetrics {
		if err := a.collectSubsystemInstances(ctx); err != nil {
			a.Errorf("failed to collect subsystem instances: %v", err)
		}
	}

	if a.CollectJobQueueMetrics {
		if err := a.collectJobQueueInstances(ctx); err != nil {
			a.Errorf("failed to collect job queue instances: %v", err)
		}
	}

	// Cleanup stale instances
	a.cleanupStaleInstances()

	// Build final metrics map
	mx := stm.ToMap(a.mx)
	
	// Add per-instance metrics
	for unit, metrics := range a.mx.disks {
		cleanUnit := cleanName(unit)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("disk_%s_%s", cleanUnit, k)] = v
		}
	}
	
	for name, metrics := range a.mx.subsystems {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("subsystem_%s_%s", cleanName, k)] = v
		}
	}
	
	for key, metrics := range a.mx.jobQueues {
		cleanKey := cleanName(key)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("jobqueue_%s_%s", cleanKey, k)] = v
		}
	}

	return mx, nil
}

func (a *AS400) collectSystemStatus(ctx context.Context) error {
	query := `
		SELECT 
			AVERAGE_CPU_UTILIZATION,
			SYSTEM_ASP_USED,
			ACTIVE_JOBS_IN_SYSTEM
		FROM QSYS2.SYSTEM_STATUS_INFO
	`

	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "AVERAGE_CPU_UTILIZATION":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.CPUPercentage = int64(v)
			}
		case "SYSTEM_ASP_USED":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.SystemASPUsed = int64(v)
			}
		case "ACTIVE_JOBS_IN_SYSTEM":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.ActiveJobsCount = v
			}
		}
	})
}

func (a *AS400) collectMemoryPools(ctx context.Context) error {
	query := `
		SELECT 
			POOL_NAME,
			CURRENT_SIZE
		FROM QSYS2.MEMORY_POOL_INFO
		WHERE POOL_NAME IN ('*MACHINE', '*BASE', '*INTERACT', '*SPOOL')
	`

	var currentPoolName string
	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "POOL_NAME":
			currentPoolName = value
		case "CURRENT_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolSize = v
				case "*BASE":
					a.mx.BasePoolSize = v
				case "*INTERACT":
					a.mx.InteractivePoolSize = v
				case "*SPOOL":
					a.mx.SpoolPoolSize = v
				}
			}
		}
	})
}

func (a *AS400) collectDiskStatus(ctx context.Context) error {
	query := `
		SELECT 
			AVG(PERCENT_BUSY) as AVG_DISK_BUSY
		FROM QSYS2.DISK_STATUS
	`

	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "AVG_DISK_BUSY" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.DiskBusyPercentage = int64(v)
			}
		}
	})
}

func (a *AS400) collectJobInfo(ctx context.Context) error {
	query := `
		SELECT 
			COUNT(*) as JOB_QUEUE_LENGTH
		FROM QSYS2.JOB_INFO
		WHERE JOB_STATUS = 'JOBQ'
	`

	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "JOB_QUEUE_LENGTH" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.JobQueueLength = v
			}
		}
	})
}

func (a *AS400) doQuery(ctx context.Context, query string, assign func(column, value string, lineEnd bool)) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout))
	defer cancel()

	rows, err := a.db.QueryContext(queryCtx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	return a.readRows(rows, assign)
}

func (a *AS400) readRows(rows *sql.Rows, assign func(column, value string, lineEnd bool)) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := make([]sql.NullString, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}

		for i, col := range columns {
			val := "NULL"
			if values[i].Valid {
				val = values[i].String
			}
			assign(col, val, i == len(columns)-1)
		}
	}

	return rows.Err()
}

// Per-instance collection methods

func (a *AS400) collectDiskInstances(ctx context.Context) error {
	// First check cardinality if we haven't yet
	if len(a.disks) == 0 && a.MaxDisks > 0 {
		count, err := a.countDisks()
		if err != nil {
			return err
		}
		if count > a.MaxDisks {
			return fmt.Errorf("disk count (%d) exceeds limit (%d), skipping per-disk metrics", count, a.MaxDisks)
		}
	}

	query := `
		SELECT 
			UNIT_NUMBER,
			UNIT_TYPE,
			UNIT_MODEL,
			PERCENT_BUSY,
			READ_REQUESTS,
			WRITE_REQUESTS,
			READ_BYTES,
			WRITE_BYTES,
			AVERAGE_REQUEST_TIME
		FROM QSYS2.DISK_STATUS
	`

	// Note: We apply selector in the result processing, not in the SQL query

	var currentUnit string
	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		
		switch column {
		case "UNIT_NUMBER":
			currentUnit = value
			
			// Apply selector if configured
			if a.diskSelector != nil && !a.diskSelector.MatchString(currentUnit) {
				currentUnit = "" // Skip this disk
				return
			}
			
			disk := a.getDiskMetrics(currentUnit)
			disk.updated = true
			
			// Add charts on first encounter
			if !disk.hasCharts {
				disk.hasCharts = true
				a.addDiskCharts(disk)
			}
			
		case "UNIT_TYPE":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				a.disks[currentUnit].typeField = value
			}
		case "UNIT_MODEL":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				a.disks[currentUnit].model = value
			}
		case "PERCENT_BUSY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					disk.busyPercent = int64(v)
					a.mx.disks[currentUnit] = diskInstanceMetrics{
						BusyPercent: disk.busyPercent,
					}
				}
			}
		case "READ_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					disk.readRequests = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.ReadRequests = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "WRITE_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					disk.writeRequests = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.WriteRequests = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "READ_BYTES":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					disk.readBytes = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.ReadBytes = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "WRITE_BYTES":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					disk.writeBytes = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.WriteBytes = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "AVERAGE_REQUEST_TIME":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					disk.averageTime = int64(v * 1000) // Convert to milliseconds
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.AverageTime = disk.averageTime
						a.mx.disks[currentUnit] = m
					}
				}
			}
		}
	})
}

func (a *AS400) countDisks() (int, error) {
	query := "SELECT COUNT(DISTINCT UNIT_NUMBER) as COUNT FROM QSYS2.DISK_STATUS"
	
	var count int
	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})
	
	return count, err
}

func (a *AS400) collectSubsystemInstances(ctx context.Context) error {
	// Check cardinality
	if len(a.subsystems) == 0 && a.MaxSubsystems > 0 {
		count, err := a.countSubsystems()
		if err != nil {
			return err
		}
		if count > a.MaxSubsystems {
			return fmt.Errorf("subsystem count (%d) exceeds limit (%d), skipping per-subsystem metrics", count, a.MaxSubsystems)
		}
	}

	query := `
		SELECT 
			SUBSYSTEM_NAME,
			SUBSYSTEM_LIBRARY_NAME,
			STATUS,
			CURRENT_ACTIVE_JOBS,
			JOBS_IN_SUBSYSTEM_HELD,
			STORAGE_USED,
			SUBSYSTEM_POOL_ID,
			MAXIMUM_JOBS
		FROM QSYS2.SUBSYSTEM_INFO
		WHERE STATUS = 'ACTIVE'
	`

	// Note: We apply selector in the result processing, not in the SQL query

	var currentName string
	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		
		switch column {
		case "SUBSYSTEM_NAME":
			currentName = value
			
			// Apply selector if configured
			if a.subsystemSelector != nil && !a.subsystemSelector.MatchString(currentName) {
				currentName = "" // Skip this subsystem
				return
			}
			
			subsystem := a.getSubsystemMetrics(currentName)
			subsystem.updated = true
			
			if !subsystem.hasCharts {
				subsystem.hasCharts = true
				a.addSubsystemCharts(subsystem)
			}
			
		case "SUBSYSTEM_LIBRARY_NAME":
			if currentName != "" && a.subsystems[currentName] != nil {
				subsystem := a.subsystems[currentName]
				subsystem.library = value
			}
		case "STATUS":
			if currentName != "" && a.subsystems[currentName] != nil {
				subsystem := a.subsystems[currentName]
				subsystem.status = value
			}
		case "CURRENT_ACTIVE_JOBS":
			if currentName != "" && a.subsystems[currentName] != nil {
				subsystem := a.subsystems[currentName]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					subsystem.jobsActive = v
					a.mx.subsystems[currentName] = subsystemInstanceMetrics{
						JobsActive: v,
					}
				}
			}
		case "JOBS_IN_SUBSYSTEM_HELD":
			if currentName != "" && a.subsystems[currentName] != nil {
				subsystem := a.subsystems[currentName]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					subsystem.jobsHeld = v
					if m, ok := a.mx.subsystems[currentName]; ok {
						m.JobsHeld = v
						a.mx.subsystems[currentName] = m
					}
				}
			}
		case "STORAGE_USED":
			if currentName != "" && a.subsystems[currentName] != nil {
				subsystem := a.subsystems[currentName]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					subsystem.storageUsed = v
					if m, ok := a.mx.subsystems[currentName]; ok {
						m.StorageUsedKB = v
						a.mx.subsystems[currentName] = m
					}
				}
			}
		case "MAXIMUM_JOBS":
			if currentName != "" && a.subsystems[currentName] != nil {
				subsystem := a.subsystems[currentName]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					subsystem.maxJobs = v
					subsystem.currentJobs = subsystem.jobsActive + subsystem.jobsHeld
					if m, ok := a.mx.subsystems[currentName]; ok {
						m.CurrentJobs = subsystem.currentJobs
						a.mx.subsystems[currentName] = m
					}
				}
			}
		}
	})
}

func (a *AS400) countSubsystems() (int, error) {
	query := "SELECT COUNT(*) as COUNT FROM QSYS2.SUBSYSTEM_INFO WHERE STATUS = 'ACTIVE'"
	
	var count int
	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})
	
	return count, err
}

func (a *AS400) collectJobQueueInstances(ctx context.Context) error {
	// Check cardinality
	if len(a.jobQueues) == 0 && a.MaxJobQueues > 0 {
		count, err := a.countJobQueues()
		if err != nil {
			return err
		}
		if count > a.MaxJobQueues {
			return fmt.Errorf("job queue count (%d) exceeds limit (%d), skipping per-queue metrics", count, a.MaxJobQueues)
		}
	}

	query := `
		SELECT 
			JOB_QUEUE_NAME,
			JOB_QUEUE_LIBRARY,
			JOB_QUEUE_STATUS,
			NUMBER_OF_JOBS,
			HELD_JOB_COUNT,
			SCHEDULED_JOB_COUNT,
			MAXIMUM_JOBS,
			SEQUENCE_NUMBER
		FROM QSYS2.JOB_QUEUE_INFO
	`

	// Note: We apply selector in the result processing, not in the SQL query

	var currentName, currentLib, key string
	return a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		
		switch column {
		case "JOB_QUEUE_NAME":
			currentName = value
		case "JOB_QUEUE_LIBRARY":
			currentLib = value
			key = fmt.Sprintf("%s_%s", currentName, currentLib)
			
			// Apply selector if configured (matches on queue name)
			if a.jobQueueSelector != nil && !a.jobQueueSelector.MatchString(currentName) {
				key = "" // Skip this job queue
				return
			}
			
			jobQueue := a.getJobQueueMetrics(key)
			jobQueue.updated = true
			jobQueue.name = currentName
			jobQueue.library = currentLib
			
			if !jobQueue.hasCharts {
				jobQueue.hasCharts = true
				a.addJobQueueCharts(jobQueue, key)
			}
			
		case "JOB_QUEUE_STATUS":
			if key != "" && a.jobQueues[key] != nil {
				jobQueue := a.jobQueues[key]
				jobQueue.status = value
			}
		case "NUMBER_OF_JOBS":
			if key != "" && a.jobQueues[key] != nil {
				jobQueue := a.jobQueues[key]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					jobQueue.jobsWaiting = v
					a.mx.jobQueues[key] = jobQueueInstanceMetrics{
						JobsWaiting: v,
					}
				}
			}
		case "HELD_JOB_COUNT":
			if key != "" && a.jobQueues[key] != nil {
				jobQueue := a.jobQueues[key]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					jobQueue.jobsHeld = v
					if m, ok := a.mx.jobQueues[key]; ok {
						m.JobsHeld = v
						a.mx.jobQueues[key] = m
					}
				}
			}
		case "SCHEDULED_JOB_COUNT":
			if key != "" && a.jobQueues[key] != nil {
				jobQueue := a.jobQueues[key]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					jobQueue.jobsScheduled = v
					if m, ok := a.mx.jobQueues[key]; ok {
						m.JobsScheduled = v
						a.mx.jobQueues[key] = m
					}
				}
			}
		case "SEQUENCE_NUMBER":
			if key != "" && a.jobQueues[key] != nil {
				jobQueue := a.jobQueues[key]
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					jobQueue.priority = v
				}
			}
		}
	})
}

func (a *AS400) countJobQueues() (int, error) {
	query := "SELECT COUNT(*) as COUNT FROM QSYS2.JOB_QUEUE_INFO"
	
	var count int
	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})
	
	return count, err
}