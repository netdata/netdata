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

const precision = 1000 // Precision multiplier for floating-point values

// boolPtr returns a pointer to the bool value
func boolPtr(v bool) *bool {
	return &v
}

func (a *AS400) collect(ctx context.Context) (map[string]int64, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		a.Debugf("collection iteration completed in %v", duration)
	}()
	
	if a.db == nil {
		db, err := a.initDatabase(ctx)
		if err != nil {
			return nil, err
		}
		a.db = db
	}

	// Reset metrics - initialize all instance maps
	a.mx = &metricsData{
		disks:            make(map[string]diskInstanceMetrics),
		subsystems:       make(map[string]subsystemInstanceMetrics),
		jobQueues:        make(map[string]jobQueueInstanceMetrics),
		tempStorageNamed: make(map[string]tempStorageInstanceMetrics),
		activeJobs:       make(map[string]activeJobInstanceMetrics),
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

	// Collect aggregate disk status - optional, gracefully handle missing disk tables
	if err := a.collectDiskStatus(ctx); err != nil {
		if isSQLFeatureError(err) {
			a.Warningf("disk status monitoring not available on this IBM i version: %v", err)
		} else {
			return nil, fmt.Errorf("failed to collect disk status: %v", err)
		}
	}

	// Collect aggregate job info - optional, gracefully handle missing job tables
	if err := a.collectJobInfo(ctx); err != nil {
		if isSQLFeatureError(err) {
			a.Warningf("job info monitoring not available on this IBM i version: %v", err)
		} else {
			return nil, fmt.Errorf("failed to collect job info: %v", err)
		}
	}

	// Collect network connections - optional
	if err := a.collectNetworkConnections(ctx); err != nil {
		if isSQLFeatureError(err) {
			a.Warningf("network connections monitoring not available on this IBM i version: %v", err)
		} else {
			a.Errorf("failed to collect network connections: %v", err)
		}
	}

	// Collect temporary storage - optional
	if err := a.collectTempStorage(ctx); err != nil {
		if isSQLFeatureError(err) {
			a.Warningf("temporary storage monitoring not available on this IBM i version: %v", err)
		} else {
			a.Errorf("failed to collect temporary storage: %v", err)
		}
	}

	// Collect subsystems - optional  
	if a.CollectSubsystemMetrics != nil && *a.CollectSubsystemMetrics {
		if err := a.collectSubsystems(ctx); err != nil {
			if isSQLFeatureError(err) {
				a.Warningf("subsystem monitoring not available on this IBM i version: %v", err)
			} else {
				a.Errorf("failed to collect subsystems: %v", err)
			}
		}
	}

	// Collect job queues - optional
	if a.CollectJobQueueMetrics != nil && *a.CollectJobQueueMetrics {
		if err := a.collectJobQueues(ctx); err != nil {
			if isSQLFeatureError(err) {
				a.Warningf("job queue monitoring not available on this IBM i version: %v", err)
			} else {
				a.Errorf("failed to collect job queues: %v", err)
			}
		}
	}

	// Collect per-instance metrics if enabled
	if a.CollectDiskMetrics != nil && *a.CollectDiskMetrics {
		if err := a.collectDiskInstancesEnhanced(ctx); err != nil {
			if isSQLFeatureError(err) {
				// Fall back to basic disk collection
				a.Warningf("enhanced disk metrics not available, using basic collection: %v", err)
				if err := a.collectDiskInstances(ctx); err != nil {
					a.Errorf("failed to collect disk instances: %v", err)
				}
			} else {
				a.Errorf("failed to collect enhanced disk instances: %v", err)
			}
		}
	}

	// Collect active jobs if enabled (requires IBM i 7.3+)
	if a.CollectActiveJobs != nil && *a.CollectActiveJobs {
		if err := a.collectActiveJobs(ctx); err != nil {
			if isSQLFeatureError(err) {
				a.Warningf("active job metrics not available on this IBM i version: %v", err)
				// Disable it for future collections
				a.CollectActiveJobs = boolPtr(false)
			} else {
				a.Errorf("failed to collect active jobs: %v", err)
			}
		}
	}

	// Cleanup stale instances
	a.cleanupStaleInstances()

	// Build final metrics map
	mx := stm.ToMap(a.mx)

	// Add per-instance metrics
	// Disks
	for unit, metrics := range a.mx.disks {
		cleanUnit := cleanName(unit)
		metricsMap := stm.ToMap(metrics)
		
		// Check if this disk has SSD metrics - if not, remove them from the map
		if disk, ok := a.disks[unit]; ok {
			if disk.ssdLifeRemaining < 0 {
				delete(metricsMap, "ssd_life_remaining")
			}
			if disk.ssdPowerOnDays < 0 {
				delete(metricsMap, "ssd_power_on_days")
			}
		}
		
		for k, v := range metricsMap {
			mx[fmt.Sprintf("disk_%s_%s", cleanUnit, k)] = v
		}
		// Calculate used_gb from capacity - available
		if metrics.CapacityGB > 0 && metrics.AvailableGB > 0 {
			mx[fmt.Sprintf("disk_%s_used_gb", cleanUnit)] = metrics.CapacityGB - metrics.AvailableGB
		}
	}

	// Subsystems
	for name, metrics := range a.mx.subsystems {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("subsystem_%s_%s", cleanName, k)] = v
		}
	}

	// Job queues
	for name, metrics := range a.mx.jobQueues {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("jobqueue_%s_%s", cleanName, k)] = v
		}
	}

	// Temp storage buckets
	for name, metrics := range a.mx.tempStorageNamed {
		cleanName := cleanName(name)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("tempstorage_%s_%s", cleanName, k)] = v
		}
	}

	// Active jobs
	for jobName, metrics := range a.mx.activeJobs {
		cleanJobName := cleanName(jobName)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("activejob_%s_%s", cleanJobName, k)] = v
		}
	}

	return mx, nil
}

func (a *AS400) collectSystemStatus(ctx context.Context) error {
	// Use comprehensive query to get all system status metrics at once
	err := a.doQuery(ctx, querySystemStatus, func(column, value string, lineEnd bool) {
		// Skip empty values
		if value == "" {
			return
		}

		switch column {
		// CPU metrics
		case "AVERAGE_CPU_UTILIZATION":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.CPUPercentage = int64(v * precision)
			}
		case "CURRENT_CPU_CAPACITY":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.CurrentCPUCapacity = int64(v * precision)
			}
		case "CONFIGURED_CPUS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.ConfiguredCPUs = v
			}

		// Memory metrics
		case "MAIN_STORAGE_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.MainStorageSize = v // KB
			}
		case "CURRENT_TEMPORARY_STORAGE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.CurrentTemporaryStorage = v // MB
			}
		case "MAXIMUM_TEMPORARY_STORAGE_USED":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.MaximumTemporaryStorageUsed = v // MB
			}

		// Job metrics
		case "TOTAL_JOBS_IN_SYSTEM":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.TotalJobsInSystem = v
			}
		case "ACTIVE_JOBS_IN_SYSTEM":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.ActiveJobsInSystem = v
			}
		case "INTERACTIVE_JOBS_IN_SYSTEM":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.InteractiveJobsInSystem = v
			}
		case "BATCH_RUNNING":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.BatchJobsRunning = v
			}

		// Storage metrics
		case "SYSTEM_ASP_USED":
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.SystemASPUsed = int64(v * precision)
			}
		case "SYSTEM_ASP_STORAGE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.SystemASPStorage = v // MB
			}
		case "TOTAL_AUXILIARY_STORAGE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.TotalAuxiliaryStorage = v // MB
			}

		// Thread metrics
		case "ACTIVE_THREADS_IN_SYSTEM":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.ActiveThreadsInSystem = v
			}
		case "THREADS_PER_PROCESSOR":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.ThreadsPerProcessor = v
			}
		}
	})

	if err != nil {
		return fmt.Errorf("failed to collect system status: %v", err)
	}

	return nil
}

func (a *AS400) collectMemoryPools(ctx context.Context) error {
	var currentPoolName string
	return a.doQuery(ctx, queryMemoryPools, func(column, value string, lineEnd bool) {
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
		case "DEFINED_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolDefinedSize = v
				case "*BASE":
					a.mx.BasePoolDefinedSize = v
				}
			}
		case "RESERVED_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolReservedSize = v
				case "*BASE":
					a.mx.BasePoolReservedSize = v
				}
			}
		case "CURRENT_THREADS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolThreads = v
				case "*BASE":
					a.mx.BasePoolThreads = v
				}
			}
		case "MAXIMUM_ACTIVE_THREADS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolMaxThreads = v
				case "*BASE":
					a.mx.BasePoolMaxThreads = v
				}
			}
		}
	})
}

func (a *AS400) collectDiskStatus(ctx context.Context) error {
	// Try modern query first
	err := a.doQuery(ctx, queryDiskStatus, func(column, value string, lineEnd bool) {
		if column == "AVG_DISK_BUSY" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.DiskBusyPercentage = int64(v * precision)
			}
		}
	})
	
	
	return err
}

func (a *AS400) collectJobInfo(ctx context.Context) error {
	// Try modern query first
	err := a.doQuery(ctx, queryJobInfo, func(column, value string, lineEnd bool) {
		if column == "JOB_QUEUE_LENGTH" {
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.JobQueueLength = v
			}
		}
	})
	
	
	return err
}

func (a *AS400) doQuery(ctx context.Context, query string, assign func(column, value string, lineEnd bool)) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout))
	defer cancel()

	rows, err := a.db.QueryContext(queryCtx, query)
	if err != nil {
		// Log expected SQL feature errors at DEBUG level to reduce noise
		if isSQLFeatureError(err) {
			a.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		} else {
			a.Errorf("failed to execute query: %s, error: %v", query, err)
		}
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

// doQueryRow executes a query that returns a single row
func (a *AS400) doQueryRow(ctx context.Context, query string, assign func(column, value string)) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(a.Timeout))
	defer cancel()

	rows, err := a.db.QueryContext(queryCtx, query)
	if err != nil {
		if isSQLFeatureError(err) {
			a.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		} else {
			a.Errorf("failed to execute query: %s, error: %v", query, err)
		}
		return err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := make([]sql.NullString, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	if rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return err
		}

		for i, col := range columns {
			val := "NULL"
			if values[i].Valid {
				val = values[i].String
			}
			assign(col, val)
		}
	}

	return rows.Err()
}

// Per-instance collection methods

func (a *AS400) collectDiskInstances(ctx context.Context) error {
	// First check cardinality if we haven't yet
	if len(a.disks) == 0 && a.MaxDisks > 0 {
		count, err := a.countDisks(ctx)
		if err != nil {
			return err
		}
		if count > a.MaxDisks {
			return fmt.Errorf("disk count (%d) exceeds limit (%d), skipping per-disk metrics", count, a.MaxDisks)
		}
	}

	var currentUnit string
	return a.doQuery(ctx, queryDiskInstances, func(column, value string, lineEnd bool) {

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
				disk := a.disks[currentUnit]
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					disk.busyPercent = int64(v * precision)
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BusyPercent = disk.busyPercent
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							BusyPercent: disk.busyPercent,
						}
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
		}
	})
}

func (a *AS400) countDisks(ctx context.Context) (int, error) {
	var count int
	err := a.doQuery(ctx, queryCountDisks, func(column, value string, lineEnd bool) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})
	return count, err
}

// Network connections collection
func (a *AS400) collectNetworkConnections(ctx context.Context) error {
	return a.doQuery(ctx, queryNetworkConnections, func(column, value string, lineEnd bool) {
		switch column {
		case "REMOTE_CONNECTIONS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.RemoteConnections = v
			}
		case "TOTAL_CONNECTIONS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.TotalConnections = v
			}
		case "LISTEN_CONNECTIONS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.ListenConnections = v
			}
		case "CLOSEWAIT_CONNECTIONS":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.CloseWaitConnections = v
			}
		}
	})
}

// Temporary storage collection
func (a *AS400) collectTempStorage(ctx context.Context) error {
	// Collect total temp storage
	err := a.doQuery(ctx, queryTempStorageTotal, func(column, value string, lineEnd bool) {
		switch column {
		case "CURRENT_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.TempStorageCurrentTotal = v
			}
		case "PEAK_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.TempStoragePeakTotal = v
			}
		}
	})
	if err != nil {
		return err
	}

	// Collect named temp storage buckets
	var currentBucket string
	return a.doQuery(ctx, queryTempStorageNamed, func(column, value string, lineEnd bool) {
		switch column {
		case "NAME":
			currentBucket = value
			bucket := a.getTempStorageMetrics(currentBucket)
			bucket.updated = true
			// Note: Charts will be created when we have actual data

		case "CURRENT_SIZE":
			if currentBucket != "" && a.tempStorageNamed[currentBucket] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.tempStorageNamed[currentBucket]; ok {
						m.CurrentSize = v
						a.mx.tempStorageNamed[currentBucket] = m
					} else {
						a.mx.tempStorageNamed[currentBucket] = tempStorageInstanceMetrics{
							CurrentSize: v,
						}
					}
					
					// Create charts only when we have actual data
					bucket := a.tempStorageNamed[currentBucket]
					if !bucket.hasCharts && v > 0 {
						bucket.hasCharts = true
						a.addTempStorageCharts(bucket)
					}
				}
			}
		case "PEAK_SIZE":
			if currentBucket != "" && a.tempStorageNamed[currentBucket] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.tempStorageNamed[currentBucket]; ok {
						m.PeakSize = v
						a.mx.tempStorageNamed[currentBucket] = m
					}
					
					// Create charts only when we have actual data
					bucket := a.tempStorageNamed[currentBucket]
					if !bucket.hasCharts && v > 0 {
						bucket.hasCharts = true
						a.addTempStorageCharts(bucket)
					}
				}
			}
		}
	})
}

// Subsystems collection
func (a *AS400) collectSubsystems(ctx context.Context) error {
	var currentSubsystem string
	return a.doQuery(ctx, querySubsystems, func(column, value string, lineEnd bool) {
		switch column {
		case "SUBSYSTEM_NAME":
			currentSubsystem = value
			subsystem := a.getSubsystemMetrics(currentSubsystem)
			subsystem.updated = true

			// Add charts on first encounter
			if !subsystem.hasCharts {
				subsystem.hasCharts = true
				a.addSubsystemCharts(subsystem)
			}

		case "CURRENT_ACTIVE_JOBS":
			if currentSubsystem != "" && a.subsystems[currentSubsystem] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.subsystems[currentSubsystem]; ok {
						m.CurrentActiveJobs = v
						a.mx.subsystems[currentSubsystem] = m
					} else {
						a.mx.subsystems[currentSubsystem] = subsystemInstanceMetrics{
							CurrentActiveJobs: v,
						}
					}
				}
			}
		case "MAXIMUM_ACTIVE_JOBS":
			if currentSubsystem != "" && a.subsystems[currentSubsystem] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.subsystems[currentSubsystem]; ok {
						m.MaximumActiveJobs = v
						a.mx.subsystems[currentSubsystem] = m
					}
				}
			}
		case "HELD_JOB_COUNT":
			if currentSubsystem != "" && a.subsystems[currentSubsystem] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.subsystems[currentSubsystem]; ok {
						m.HeldJobCount = v
						a.mx.subsystems[currentSubsystem] = m
					}
				}
			}
		case "STORAGE_USED_KB":
			if currentSubsystem != "" && a.subsystems[currentSubsystem] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.subsystems[currentSubsystem]; ok {
						m.StorageUsedKB = v
						a.mx.subsystems[currentSubsystem] = m
					}
				}
			}
		}
	})
}

// Job queues collection
func (a *AS400) collectJobQueues(ctx context.Context) error {
	var currentQueue string
	return a.doQuery(ctx, queryJobQueues, func(column, value string, lineEnd bool) {
		switch column {
		case "QUEUE_NAME":
			currentQueue = value
			queue := a.getJobQueueMetrics(currentQueue)
			queue.updated = true

			// Add charts on first encounter
			if !queue.hasCharts {
				queue.hasCharts = true
				a.addJobQueueCharts(queue, currentQueue)
			}

		case "NUMBER_OF_JOBS":
			if currentQueue != "" && a.jobQueues[currentQueue] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.jobQueues[currentQueue]; ok {
						m.NumberOfJobs = v
						a.mx.jobQueues[currentQueue] = m
					} else {
						a.mx.jobQueues[currentQueue] = jobQueueInstanceMetrics{
							NumberOfJobs: v,
						}
					}
				}
			}
		case "HELD_JOB_COUNT":
			if currentQueue != "" && a.jobQueues[currentQueue] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.jobQueues[currentQueue]; ok {
						m.HeldJobCount = v
						a.mx.jobQueues[currentQueue] = m
					}
				}
			}
		}
	})
}

// Enhanced disk collection with all metrics
func (a *AS400) collectDiskInstancesEnhanced(ctx context.Context) error {
	// First check cardinality if we haven't yet
	if len(a.disks) == 0 && a.MaxDisks > 0 {
		count, err := a.countDisks(ctx)
		if err != nil {
			return err
		}
		if count > a.MaxDisks {
			return fmt.Errorf("disk count (%d) exceeds limit (%d), skipping per-disk metrics", count, a.MaxDisks)
		}
	}

	var currentUnit string
	return a.doQuery(ctx, queryDiskInstancesEnhanced, func(column, value string, lineEnd bool) {
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
		case "PERCENT_USED":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.PercentUsed = int64(v * precision)
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							PercentUsed: int64(v * precision),
						}
					}
				}
			}
		case "UNIT_SPACE_AVAILABLE_GB":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.AvailableGB = int64(v * precision)
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "UNIT_STORAGE_CAPACITY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.CapacityGB = int64(v * precision)
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "TOTAL_READ_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.ReadRequests = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "TOTAL_WRITE_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.WriteRequests = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "TOTAL_BLOCKS_READ":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BlocksRead = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "TOTAL_BLOCKS_WRITTEN":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BlocksWritten = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "ELAPSED_PERCENT_BUSY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BusyPercent = int64(v * precision)
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "SSD_LIFE_REMAINING":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.disks[currentUnit].ssdLifeRemaining = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.SSDLifeRemaining = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "SSD_POWER_ON_DAYS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.disks[currentUnit].ssdPowerOnDays = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.SSDPowerOnDays = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "HARDWARE_STATUS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				a.disks[currentUnit].hardwareStatus = value
				if m, ok := a.mx.disks[currentUnit]; ok {
					m.HardwareStatus = value
					a.mx.disks[currentUnit] = m
				}
			}
		case "DISK_MODEL":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				a.disks[currentUnit].diskModel = value
				if m, ok := a.mx.disks[currentUnit]; ok {
					m.DiskModel = value
					a.mx.disks[currentUnit] = m
				}
			}
		case "SERIAL_NUMBER":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				a.disks[currentUnit].serialNumber = value
				if m, ok := a.mx.disks[currentUnit]; ok {
					m.SerialNumber = value
					a.mx.disks[currentUnit] = m
				}
			}
		}
	})
}
