// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

const precision = 1000 // Precision multiplier for floating-point values

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
		disks:             make(map[string]diskInstanceMetrics),
		subsystems:        make(map[string]subsystemInstanceMetrics),
		jobQueues:         make(map[string]jobQueueInstanceMetrics),
		jobs:              make(map[string]jobMetrics),
		aspPools:          make(map[string]aspMetrics),
		IFSDirectoryUsage: make(map[string]int64),
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

	// Collect job type breakdown
	if err := a.collectJobTypeBreakdown(ctx); err != nil {
		a.Warningf("failed to collect job type breakdown: %v", err)
	}

	// Collect IFS usage
	if err := a.collectIFSUsage(ctx); err != nil {
		a.Warningf("failed to collect IFS usage: %v", err)
	}

	if a.IFSTopNDirectories > 0 {
		if err := a.collectIFSTopNDirectories(ctx); err != nil {
			if isSQLFeatureError(err) {
				a.logOnce("ifs_top_directories_unavailable", "IFS top directories collection failed (likely unsupported on this IBM i version): %v", err)
			} else {
				a.Warningf("failed to collect IFS top N directories: %v", err)
			}
		}
	}

	// Collect message queue depths
	if err := a.collectMessageQueues(ctx); err != nil {
		if isSQLFeatureError(err) {
			a.logOnce("message_queues_unavailable", "Message queue collection failed (likely unsupported on this IBM i version): %v", err)
		} else {
			a.Warningf("failed to collect message queues: %v", err)
		}
	}

	// Collect critical message counts
	if err := a.collectCriticalMessages(ctx); err != nil {
		a.Warningf("failed to collect critical messages: %v", err)
	}

	// Collect ASP (storage pool) information
	if err := a.collectASPInfo(ctx); err != nil {
		a.Warningf("failed to collect ASP info: %v", err)
	}

	// Collect top active jobs if enabled
	if a.CollectActiveJobs != nil && *a.CollectActiveJobs && a.MaxActiveJobs > 0 {
		if err := a.collectActiveJobs(ctx); err != nil {
			if isSQLFeatureError(err) {
				a.logOnce("active_jobs_unavailable", "Active job collection failed (likely unsupported on this IBM i version): %v", err)
			} else {
				a.Warningf("failed to collect active jobs: %v", err)
			}
		}
	}

	// Collect per-instance metrics if enabled
	if a.CollectDiskMetrics != nil && *a.CollectDiskMetrics {
		if err := a.collectDiskInstances(ctx); err != nil {
			a.Errorf("failed to collect disk instances: %v", err)
		}
	}

	if a.CollectSubsystemMetrics != nil && *a.CollectSubsystemMetrics {
		if err := a.collectSubsystemInstances(ctx); err != nil {
			a.Errorf("failed to collect subsystem instances: %v", err)
		}
	}

	if a.CollectJobQueueMetrics != nil && *a.CollectJobQueueMetrics {
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

	// Add individual job metrics if enabled
	if a.CollectActiveJobs != nil && *a.CollectActiveJobs {
		for jobName, metrics := range a.mx.jobs {
			cleanJobName := cleanName(jobName)
			for k, v := range stm.ToMap(metrics) {
				mx[fmt.Sprintf("job_%s_%s", cleanJobName, k)] = v
			}
		}
	}

	// Add ASP pool metrics
	for aspNum, metrics := range a.mx.aspPools {
		cleanASP := cleanName(aspNum)
		for k, v := range stm.ToMap(metrics) {
			mx[fmt.Sprintf("asp_%s_%s", cleanASP, k)] = v
		}
	}

	// Add IFS directory usage metrics
	for dir, size := range a.mx.IFSDirectoryUsage {
		cleanDir := cleanName(dir)
		mx[fmt.Sprintf("ifs_directory_%s_size", cleanDir)] = size
	}

	return mx, nil
}

func (a *AS400) collectSystemStatus(ctx context.Context) error {
	// Core metrics - should always be available
	if err := a.collectSingleMetric(ctx, "average_cpu", queryAverageCPU, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.CPUPercentage = int64(v * precision)
		}
	}); err != nil {
		return err // Fatal - basic metric must work
	}

	if err := a.collectSingleMetric(ctx, "system_asp", querySystemASP, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.SystemASPUsed = int64(v * precision)
		}
	}); err != nil {
		return err // Fatal - basic metric must work
	}

	if err := a.collectSingleMetric(ctx, "active_jobs", queryActiveJobs, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			a.mx.ActiveJobsCount = v
		}
	}); err != nil {
		return err // Fatal - basic metric must work
	}

	// Version-specific metrics - gracefully handle missing columns
	_ = a.collectSingleMetric(ctx, "configured_cpus", queryConfiguredCPUs, func(value string) {
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			a.mx.ConfiguredCPUs = v
		}
	})

	_ = a.collectSingleMetric(ctx, "current_processing_capacity", queryCurrentProcessingCapacity, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.CurrentProcessingCapacity = int64(v * precision)
		}
	})

	_ = a.collectSingleMetric(ctx, "shared_processor_pool_util", querySharedProcessorPoolUtil, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.SharedProcessorPoolUsage = int64(v * precision)
		}
	})

	_ = a.collectSingleMetric(ctx, "partition_cpu_util", queryPartitionCPUUtil, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.PartitionCPUUtilization = int64(v * precision)
		}
	})

	_ = a.collectSingleMetric(ctx, "interactive_cpu_util", queryInteractiveCPUUtil, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.InteractiveCPUUtilization = int64(v * precision)
		}
	})

	_ = a.collectSingleMetric(ctx, "database_cpu_util", queryDatabaseCPUUtil, func(value string) {
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			a.mx.DatabaseCPUUtilization = int64(v * precision)
		}
	})

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
		}
	})
}

func (a *AS400) collectDiskStatus(ctx context.Context) error {
	return a.doQuery(ctx, queryDiskStatus, func(column, value string, lineEnd bool) {
		if column == "AVG_DISK_BUSY" {
			if v, err := strconv.ParseFloat(value, 64); err == nil {
				a.mx.DiskBusyPercentage = int64(v * precision)
			}
		}
	})
}

func (a *AS400) collectJobInfo(ctx context.Context) error {
	return a.doQuery(ctx, queryJobInfo, func(column, value string, lineEnd bool) {
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

func (a *AS400) collectSubsystemInstances(ctx context.Context) error {
	// Check cardinality
	if len(a.subsystems) == 0 && a.MaxSubsystems > 0 {
		count, err := a.countSubsystems(ctx)
		if err != nil {
			return err
		}
		if count > a.MaxSubsystems {
			return fmt.Errorf("subsystem count (%d) exceeds limit (%d), skipping per-subsystem metrics", count, a.MaxSubsystems)
		}
	}

	// Note: We apply selector in the result processing, not in the SQL query

	var currentName string
	return a.doQuery(ctx, querySubsystemInstances, func(column, value string, lineEnd bool) {

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
					if m, ok := a.mx.subsystems[currentName]; ok {
						m.JobsActive = v
						a.mx.subsystems[currentName] = m
					} else {
						a.mx.subsystems[currentName] = subsystemInstanceMetrics{
							JobsActive: v,
						}
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

func (a *AS400) countSubsystems(ctx context.Context) (int, error) {
	var count int
	err := a.doQuery(ctx, queryCountSubsystems, func(column, value string, lineEnd bool) {
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
		count, err := a.countJobQueues(ctx)
		if err != nil {
			return err
		}
		if count > a.MaxJobQueues {
			return fmt.Errorf("job queue count (%d) exceeds limit (%d), skipping per-queue metrics", count, a.MaxJobQueues)
		}
	}

	// Note: We apply selector in the result processing, not in the SQL query

	var currentName, currentLib, key string
	return a.doQuery(ctx, queryJobQueueInstances, func(column, value string, lineEnd bool) {

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
					if m, ok := a.mx.jobQueues[key]; ok {
						m.JobsWaiting = v
						a.mx.jobQueues[key] = m
					} else {
						a.mx.jobQueues[key] = jobQueueInstanceMetrics{
							JobsWaiting: v,
						}
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

func (a *AS400) countJobQueues(ctx context.Context) (int, error) {
	var count int
	err := a.doQuery(ctx, queryCountJobQueues, func(column, value string, lineEnd bool) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})

	return count, err
}

func (a *AS400) collectJobTypeBreakdown(ctx context.Context) error {
	// Check if ACTIVE_JOB_INFO is disabled
	if a.isDisabled("active_job_info") {
		return nil
	}

	// Reset job type counts
	a.mx.BatchJobs = 0
	a.mx.InteractiveJobs = 0
	a.mx.SystemJobs = 0
	a.mx.SpooledJobs = 0
	a.mx.OtherJobs = 0

	var currentJobType string
	err := a.doQuery(ctx, queryJobTypeBreakdown, func(column, value string, lineEnd bool) {
		switch column {
		case "JOB_TYPE":
			// Store current job type for next COUNT value
			currentJobType = value
		case "COUNT":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				switch currentJobType {
				case "BAT":
					a.mx.BatchJobs = v
				case "INT":
					a.mx.InteractiveJobs = v
				case "SYS":
					a.mx.SystemJobs = v
				case "WTR", "RDR":
					a.mx.SpooledJobs += v
				default:
					a.mx.OtherJobs += v
				}
			}
		}
	})

	// Handle table function errors
	if err != nil && isSQLFeatureError(err) {
		a.logOnce("active_job_info", "ACTIVE_JOB_INFO function not available on this IBM i version: %v", err)
		a.disabled["active_job_info"] = true
		return nil
	}

	return err
}

func (a *AS400) collectIFSUsage(ctx context.Context) error {
	// Check if IFS_OBJECT_STATISTICS is disabled
	if a.isDisabled("ifs_object_statistics") {
		return nil
	}
	err := a.doQuery(ctx, queryIFSUsage, func(column, value string, lineEnd bool) {
		switch column {
		case "FILE_COUNT":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.IFSFileCount = v
			}
		case "TOTAL_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.IFSTotalSize = v
			}
		case "ALLOCATED_SIZE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				a.mx.IFSUsedSize = v
			}
		}
	})

	// Handle table function errors
	if err != nil && isSQLFeatureError(err) {
		a.logOnce("ifs_object_statistics", "IFS_OBJECT_STATISTICS function not available on this IBM i version: %v", err)
		a.disabled["ifs_object_statistics"] = true
		return nil
	}

	return err
}

func (a *AS400) collectMessageQueues(ctx context.Context) error {
	// Collect system message queue depth
	var currentQueue string
	var currentLib string

	return a.doQuery(ctx, queryMessageQueues, func(column, value string, lineEnd bool) {
		switch column {
		case "MESSAGE_QUEUE_NAME":
			currentQueue = value
		case "MESSAGE_QUEUE_LIBRARY":
			currentLib = value
		case "NUMBER_OF_MESSAGES":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				if currentQueue == "QSYSMSG" && currentLib == "QSYS" {
					a.mx.SystemMessageQueueDepth = v
				} else if currentQueue == "QSYSOPR" && currentLib == "QSYS" {
					a.mx.QSYSOPRMessageQueueDepth = v
				}
			}
		}
	})
}

// New collection functions for enhanced metrics
func (a *AS400) collectCriticalMessages(ctx context.Context) error {
	var currentQueue, currentLib string
	return a.doQuery(ctx, queryMessageQueueCritical, func(column, value string, lineEnd bool) {
		switch column {
		case "MESSAGE_QUEUE_NAME":
			currentQueue = value
		case "MESSAGE_QUEUE_LIBRARY":
			currentLib = value
		case "CRITICAL_COUNT":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				if currentQueue == "QSYSMSG" && currentLib == "QSYS" {
					a.mx.SystemCriticalMessages = v
				} else if currentQueue == "QSYSOPR" && currentLib == "QSYS" {
					a.mx.QSYSOPRCriticalMessages = v
				}
			}
		}
	})
}

func (a *AS400) collectASPInfo(ctx context.Context) error {
	var currentASP string
	return a.doQuery(ctx, queryASPInfo, func(column, value string, lineEnd bool) {
		switch column {
		case "ASP_NUMBER":
			currentASP = value
		case "TOTAL_CAPACITY_AVAILABLE":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				if currentASP == "1" { // System ASP
					if asp, ok := a.mx.aspPools[currentASP]; ok {
						asp.TotalCapacityAvailable = v
						a.mx.aspPools[currentASP] = asp
					} else {
						a.mx.aspPools[currentASP] = aspMetrics{TotalCapacityAvailable: v}
					}
				}
			}
		case "TOTAL_CAPACITY":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				if currentASP == "1" { // System ASP
					if asp, ok := a.mx.aspPools[currentASP]; ok {
						asp.TotalCapacity = v
						a.mx.aspPools[currentASP] = asp
					} else {
						a.mx.aspPools[currentASP] = aspMetrics{TotalCapacity: v}
					}
				}
			}
		}
	})
}

func (a *AS400) collectActiveJobs(ctx context.Context) error {
	// Check if ACTIVE_JOB_INFO is disabled
	if a.isDisabled("active_job_info") {
		return nil
	}

	query := fmt.Sprintf(queryActiveJobsDetails, a.MaxActiveJobs)
	var currentJobName string
	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "JOB_NAME":
			currentJobName = value
			if _, exists := a.mx.jobs[currentJobName]; !exists {
				a.mx.jobs[currentJobName] = jobMetrics{}
			}
		case "CPU_TIME":
			if currentJobName != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if job, ok := a.mx.jobs[currentJobName]; ok {
						job.CPUTime = v
						a.mx.jobs[currentJobName] = job
					}
				}
			}
		case "TEMPORARY_STORAGE":
			if currentJobName != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if job, ok := a.mx.jobs[currentJobName]; ok {
						job.TemporaryStorage = v
						a.mx.jobs[currentJobName] = job
					}
				}
			}
		case "JOB_ACTIVE_TIME":
			if currentJobName != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					if job, ok := a.mx.jobs[currentJobName]; ok {
						job.JobActiveTime = v
						a.mx.jobs[currentJobName] = job
					}
				}
			}
		case "ELAPSED_CPU_PERCENTAGE":
			if currentJobName != "" {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					if job, ok := a.mx.jobs[currentJobName]; ok {
						job.ElapsedCPUPercentage = int64(v * precision)
						a.mx.jobs[currentJobName] = job
					}
				}
			}
		}
	})

	// Handle table function errors
	if err != nil && isSQLFeatureError(err) {
		a.logOnce("active_job_info", "ACTIVE_JOB_INFO function not available on this IBM i version: %v", err)
		a.disabled["active_job_info"] = true
		return nil
	}

	return err
}

func (a *AS400) collectIFSTopNDirectories(ctx context.Context) error {
	// Check if IFS_OBJECT_STATISTICS is disabled
	if a.isDisabled("ifs_object_statistics") {
		return nil
	}
	query := fmt.Sprintf(queryIFSTopNDirectories, a.IFSStartPath, a.IFSTopNDirectories)
	var currentDir string
	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "DIR":
			currentDir = value
		case "TOTAL_SIZE":
			if currentDir != "" {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.mx.IFSDirectoryUsage[currentDir] = v
					// Add directory to chart if not already present
					if !a.hasIFSDirectoryDim(currentDir) {
						a.addIFSDirectoryDim(currentDir)
					}
				}
			}
		}
	})

	// Handle table function errors
	if err != nil && isSQLFeatureError(err) {
		a.logOnce("ifs_object_statistics", "IFS_OBJECT_STATISTICS function not available on this IBM i version: %v", err)
		a.disabled["ifs_object_statistics"] = true
		return nil
	}

	return err
}

func (a *AS400) hasIFSDirectoryDim(dir string) bool {
	cleanDir := cleanName(dir)
	dimID := fmt.Sprintf("ifs_directory_%s_size", cleanDir)

	for _, chart := range *a.charts {
		if chart.ID == "ifs_directory_usage" {
			for _, dim := range chart.Dims {
				if dim.ID == dimID {
					return true
				}
			}
			break
		}
	}
	return false
}

func (a *AS400) addIFSDirectoryDim(dir string) {
	cleanDir := cleanName(dir)
	dimID := fmt.Sprintf("ifs_directory_%s_size", cleanDir)

	for _, chart := range *a.charts {
		if chart.ID == "ifs_directory_usage" {
			dim := &module.Dim{
				ID:   dimID,
				Name: dir,
			}
			if err := chart.AddDim(dim); err != nil {
				a.Warningf("failed to add IFS directory dimension for %s: %v", dir, err)
			}
			break
		}
	}
}
