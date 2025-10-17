// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const precision = 1000 // Precision multiplier for floating-point values

// cleanNumericString removes all non-numeric characters except digits, minus, and decimal point
func cleanNumericString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "NULL") || strings.EqualFold(trimmed, "N/A") {
		return ""
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '.':
			return r
		case r == 'e' || r == 'E' || r == '+':
			return r
		default:
			return -1
		}
	}, trimmed)
	return cleaned
}

// parseInt64Value parses a value as int64 with optional multiplier, returns (result, ok)
// Automatically handles both integers and floats from IBM i
// Logs all parse attempts in debug mode
func (a *Collector) parseInt64Value(value string, multiplier int64) (int64, bool) {
	cleaned := cleanNumericString(value)
	if cleaned == "" || cleaned == "-" || cleaned == "." || cleaned == "+" {
		a.Debugf("parseInt64Value: empty/invalid value='%s', cleaned='%s'", value, cleaned)
		return 0, false
	}
	if strings.Count(cleaned, ".") > 1 {
		a.Debugf("parseInt64Value: too many decimal points, value='%s', cleaned='%s'", value, cleaned)
		return 0, false
	}
	if strings.Count(cleaned, "e")+strings.Count(cleaned, "E") > 1 {
		a.Debugf("parseInt64Value: too many exponents, value='%s', cleaned='%s'", value, cleaned)
		return 0, false
	}
	if multiplier <= 0 {
		multiplier = 1
	}

	// Handle floats/exponentials from IBM i (like memory sizes: "8192.00" or "7.8e+09")
	if strings.Contains(cleaned, ".") || strings.ContainsAny(cleaned, "eE") {
		f, err := strconv.ParseFloat(cleaned, 64)
		if err != nil {
			a.Debugf("parseInt64Value: ParseFloat failed, value='%s', cleaned='%s', error=%v", value, cleaned, err)
			return 0, false
		}
		return int64(math.Round(f * float64(multiplier))), true
	}

	// Handle integers
	v, err := strconv.ParseInt(cleaned, 10, 64)
	if err != nil {
		a.Debugf("parseInt64Value: ParseInt failed, value='%s', cleaned='%s', error=%v", value, cleaned, err)
		return 0, false
	}
	if multiplier != 1 {
		return v * multiplier, true
	}
	return v, true
}

// parseFloat64Value parses a value as float64, returns (result, ok)
// Logs all parse attempts in debug mode
func (a *Collector) computeEntitledCPUPercentage(cpuUtilization float64) int64 {
	if a.mx.CurrentCPUCapacity <= 0 {
		return 0
	}
	capacityPercent := float64(a.mx.CurrentCPUCapacity) / float64(precision)
	if capacityPercent <= 0 {
		return 0
	}
	perCorePercent := cpuUtilization / float64(precision)
	entitled := (perCorePercent / capacityPercent) * 100.0
	if entitled < 0 {
		entitled = 0
	}
	return int64(math.Round(entitled * float64(precision)))
}

func (a *Collector) parseFloat64Value(value string) (float64, bool) {
	cleaned := cleanNumericString(value)
	if cleaned == "" || cleaned == "-" || cleaned == "." || cleaned == "+" {
		a.Debugf("parseFloat64Value: empty/invalid value='%s', cleaned='%s'", value, cleaned)
		return 0, false
	}
	if strings.Count(cleaned, ".") > 1 {
		a.Debugf("parseFloat64Value: too many decimal points, value='%s', cleaned='%s'", value, cleaned)
		return 0, false
	}
	if strings.Count(cleaned, "e")+strings.Count(cleaned, "E") > 1 {
		a.Debugf("parseFloat64Value: too many exponents, value='%s', cleaned='%s'", value, cleaned)
		return 0, false
	}

	f, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		a.Debugf("parseFloat64Value: ParseFloat failed, value='%s', cleaned='%s', error=%v", value, cleaned, err)
		return 0, false
	}
	return f, true
}

func planCacheMetricKey(heading string) string {
	trimmed := strings.TrimSpace(heading)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ToLower(trimmed)
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastUnderscore = false
		case r == ' ' || r == '-' || r == '/' || r == '\\' || r == ':' || r == '%' || r == '(' || r == ')' || r == '.':
			if !lastUnderscore {
				builder.WriteRune('_')
				lastUnderscore = true
			}
		default:
			// skip other punctuation
		}
	}
	result := builder.String()
	result = strings.Trim(result, "_")
	if result == "" {
		result = cleanName(trimmed)
	}
	if result == "" {
		return "plan_cache_metric"
	}
	return result
}

func normalizeValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "NULL") {
		return ""
	}
	return trimmed
}

// parseInt64OrZero parses value as int64, returns 0 on any error (no logging)
func parseInt64OrZero(value string) int64 {
	cleaned := cleanNumericString(value)
	if cleaned == "" || cleaned == "-" || cleaned == "." || cleaned == "+" {
		return 0
	}
	if strings.Count(cleaned, ".") > 1 {
		return 0
	}
	if strings.Count(cleaned, "e")+strings.Count(cleaned, "E") > 1 {
		return 0
	}

	// Handle floats/exponentials
	if strings.Contains(cleaned, ".") || strings.ContainsAny(cleaned, "eE") {
		if f, err := strconv.ParseFloat(cleaned, 64); err == nil {
			return int64(math.Round(f))
		}
		return 0
	}

	// Handle integers
	if v, err := strconv.ParseInt(cleaned, 10, 64); err == nil {
		return v
	}
	return 0
}

func boolToInt(cond bool) int64 {
	if cond {
		return 1
	}
	return 0
}

func (a *Collector) collect(ctx context.Context) error {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		a.Debugf("collection iteration completed in %v", duration)
	}()

	a.prepareIterationState()
	a.initGroups()

	for _, grp := range a.groups {
		if grp == nil || !grp.Enabled() {
			continue
		}
		if err := grp.Collect(ctx); err != nil {
			return fmt.Errorf("%s: %w", grp.Name(), err)
		}
	}

	return nil
}

func (a *Collector) recordQueryLatency(queryName string, duration time.Duration) {
	if a.mx == nil {
		return
	}
	if queryName == "" {
		queryName = "unknown_query"
	}

	sanitized := cleanName(queryName)
	if sanitized == "" {
		sanitized = "unknown_query"
	}

	if a.mx.queryLatencies == nil {
		a.mx.queryLatencies = make(map[string]int64)
	}

	latency := duration.Microseconds()
	if latency == 0 && duration > 0 {
		latency = 1
	}

	a.mx.queryLatencies[sanitized] += latency
}

func (a *Collector) collectSystemStatus(ctx context.Context) error {
	// Use comprehensive query to get all system status metrics at once
	err := a.doQuery(ctx, "system_status", a.systemStatusQuery(), func(column, value string, lineEnd bool) {
		// Debug log all columns to see what we're receiving
		if strings.Contains(column, "STORAGE") || strings.Contains(column, "MEMORY") {
			a.Debugf("collectSystemStatus: column='%s', value='%s'", column, value)
		}

		// Skip empty values
		if value == "" {
			return
		}

		switch column {
		// CPU metrics
		case "AVERAGE_CPU_UTILIZATION":
			// AVERAGE_CPU_UTILIZATION is system-wide 0-100% (deprecated in IBM i 7.4+)
			if v, ok := a.parseInt64Value(value, precision); ok {
				a.mx.CPUPercentage = v
				a.mx.EntitledCPUPercentage = a.computeEntitledCPUPercentage(float64(v))
			}
		case "CURRENT_CPU_CAPACITY":
			// CURRENT_CPU_CAPACITY comes from IBM as decimal fraction (0.0-1.0)
			// Convert to percentage by multiplying by 100
			if v, ok := a.parseInt64Value(value, precision); ok {
				// Convert decimal fraction (e.g., 0.20) to percentage scale
				a.mx.CurrentCPUCapacity = v * 100
			}
		case "CONFIGURED_CPUS":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.ConfiguredCPUs = v
			}

		// Memory metrics
		case "MAIN_STORAGE_SIZE":
			if v, ok := a.parseInt64Value(value, 1024); ok { // Convert KB to bytes
				a.mx.MainStorageSize = v
			}
		case "CURRENT_TEMPORARY_STORAGE":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.CurrentTemporaryStorage = v // MB
			}
		case "MAXIMUM_TEMPORARY_STORAGE_USED":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.MaximumTemporaryStorageUsed = v // MB
			}

		// Job metrics
		case "TOTAL_JOBS_IN_SYSTEM":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.TotalJobsInSystem = v
			}
		case "ACTIVE_JOBS_IN_SYSTEM":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.ActiveJobsInSystem = v
			}
		case "INTERACTIVE_JOBS_IN_SYSTEM":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.InteractiveJobsInSystem = v
			}
		case "BATCH_RUNNING":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.BatchJobsRunning = v
			}

		// Storage metrics
		case "SYSTEM_ASP_USED":
			if v, ok := a.parseInt64Value(value, precision); ok {
				a.mx.SystemASPUsed = v
			}
		case "SYSTEM_ASP_STORAGE":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.SystemASPStorage = v // MB
			}
		case "TOTAL_AUXILIARY_STORAGE":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.TotalAuxiliaryStorage = v // MB
			}

		// Thread metrics
		case "ACTIVE_THREADS_IN_SYSTEM":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.ActiveThreadsInSystem = v
			}
		case "THREADS_PER_PROCESSOR":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.ThreadsPerProcessor = v
			}
		}
	})

	if err != nil {
		return fmt.Errorf("failed to collect system status: %v", err)
	}

	return nil
}

func (a *Collector) collectMemoryPools(ctx context.Context) error {
	var currentPoolName string
	return a.doQuery(ctx, "memory_pools", a.memoryPoolQuery(), func(column, value string, lineEnd bool) {
		switch column {
		case "POOL_NAME":
			currentPoolName = strings.TrimSpace(value)
		case "CURRENT_SIZE":
			if v, ok := a.parseInt64Value(value, 1024*1024); ok { // Convert MB to bytes
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
			if v, ok := a.parseInt64Value(value, 1024*1024); ok { // Convert MB to bytes
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolDefinedSize = v
				case "*BASE":
					a.mx.BasePoolDefinedSize = v
				}
			}
		case "RESERVED_SIZE":
			if v, ok := a.parseInt64Value(value, 1024*1024); ok { // Convert MB to bytes
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolReservedSize = v
				case "*BASE":
					a.mx.BasePoolReservedSize = v
				}
			}
		case "CURRENT_THREADS":
			if v, ok := a.parseInt64Value(value, 1); ok {
				switch currentPoolName {
				case "*MACHINE":
					a.mx.MachinePoolThreads = v
				case "*BASE":
					a.mx.BasePoolThreads = v
				}
			}
		case "MAXIMUM_ACTIVE_THREADS":
			if v, ok := a.parseInt64Value(value, 1); ok {
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

func (a *Collector) collectDiskStatus(ctx context.Context) error {
	// Try modern query first
	err := a.doQuery(ctx, "disk_status", queryDiskStatus, func(column, value string, lineEnd bool) {
		if column == "AVG_DISK_BUSY" {
			if v, ok := a.parseInt64Value(value, precision); ok {
				a.mx.DiskBusyPercentage = v
			}
		}
	})

	return err
}

func (a *Collector) collectJobInfo(ctx context.Context) error {
	// Try modern query first
	err := a.doQuery(ctx, "job_info", queryJobInfo, func(column, value string, lineEnd bool) {
		if column == "JOB_QUEUE_LENGTH" {
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.JobQueueLength = v
			}
		}
	})

	return err
}

func (a *Collector) collectMessageQueues(ctx context.Context) error {
	if len(a.messageQueueTargets) == 0 {
		return nil
	}

	var firstErr error

	for _, target := range a.messageQueueTargets {
		key := target.ID()
		meta := a.getMessageQueueMetrics(key)
		meta.library = target.Library
		meta.name = target.Name
		a.messageQueues[key] = meta

		metrics := messageQueueInstanceMetrics{}
		found := false

		queryName := fmt.Sprintf("message_queue_%s_%s", target.Library, target.Name)
		query := buildMessageQueueQuery(target, a.supportsMessageQueueTableFunction())

		err := a.doQuery(ctx, queryName, query, func(column, value string, lineEnd bool) {
			switch column {
			case "MESSAGE_COUNT":
				metrics.Total = parseInt64OrZero(value)
			case "INFORMATIONAL_MESSAGES":
				metrics.Informational = parseInt64OrZero(value)
			case "INQUIRY_MESSAGES":
				metrics.Inquiry = parseInt64OrZero(value)
			case "DIAGNOSTIC_MESSAGES":
				metrics.Diagnostic = parseInt64OrZero(value)
			case "ESCAPE_MESSAGES":
				metrics.Escape = parseInt64OrZero(value)
			case "NOTIFY_MESSAGES":
				metrics.Notify = parseInt64OrZero(value)
			case "SENDER_COPY_MESSAGES":
				metrics.SenderCopy = parseInt64OrZero(value)
			case "MAX_SEVERITY":
				metrics.MaxSeverity = parseInt64OrZero(value)
			}

			if lineEnd {
				found = true
			}
		})

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("message queue %s: %w", key, err)
			}
			continue
		}

		if !found {
			metrics = messageQueueInstanceMetrics{}
		}

		a.mx.messageQueues[key] = metrics
	}

	return firstErr
}

func (a *Collector) collectOutputQueues(ctx context.Context) error {
	if len(a.outputQueueTargets) == 0 {
		return nil
	}

	var firstErr error

	for _, target := range a.outputQueueTargets {
		key := target.ID()
		meta := a.getOutputQueueMetrics(key)
		meta.library = target.Library
		meta.name = target.Name
		meta.status = "UNKNOWN"
		a.outputQueues[key] = meta

		metrics := outputQueueInstanceMetrics{}
		entriesCount := int64(0)
		entriesUsed := false

		queryName := fmt.Sprintf("output_queue_%s_%s", target.Library, target.Name)
		err := a.doQuery(ctx, queryName, buildOutputQueueEntriesQuery(target), func(column, value string, lineEnd bool) {
			if lineEnd {
				entriesCount++
			}
		})
		if err != nil {
			if isSQLFeatureError(err) {
				a.Debugf("output queue entries function unavailable for %s/%s, falling back to view", target.Library, target.Name)
			} else if firstErr == nil {
				firstErr = fmt.Errorf("output queue %s (entries): %w", key, err)
			}
		} else {
			entriesUsed = true
			metrics.Files = entriesCount
		}

		viewErr := a.doQuery(ctx, queryName+"_view", buildOutputQueueInfoQuery(target), func(column, value string, lineEnd bool) {
			switch column {
			case "OUTPUT_QUEUE_STATUS":
				meta.status = strings.TrimSpace(value)
			case "NUMBER_OF_WRITERS":
				metrics.Writers = parseInt64OrZero(value)
			case "NUMBER_OF_FILES":
				if !entriesUsed {
					metrics.Files = parseInt64OrZero(value)
				}
			}
		})
		if viewErr != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("output queue %s (info): %w", key, viewErr)
			}
			continue
		}

		metrics.Released = boolToInt(strings.EqualFold(meta.status, "RELEASED"))
		a.outputQueues[key] = meta
		a.mx.outputQueues[key] = metrics
	}

	return firstErr
}

func (a *Collector) doQuery(ctx context.Context, queryName, query string, assign func(column, value string, lineEnd bool)) error {
	var (
		capture      bool
		columnsSaved []string
		rowsSaved    [][]string
	)
	if a.dump != nil {
		capture = true
	}

	start := time.Now()
	defer func() {
		a.recordQueryLatency(queryName, time.Since(start))
	}()

	err := a.client.Query(ctx, query, func(columns []string, values []string) error {
		for i, col := range columns {
			assign(col, values[i], i == len(columns)-1)
		}
		if capture {
			if columnsSaved == nil {
				columnsSaved = append(columnsSaved, columns...)
			}
			rowCopy := make([]string, len(values))
			copy(rowCopy, values)
			rowsSaved = append(rowsSaved, rowCopy)
		}
		return nil
	})
	if err != nil {
		if isSQLFeatureError(err) {
			a.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		} else if isSQLTemporaryError(err) {
			a.Debugf("query failed with temporary database error: %s, error: %v", query, err)
		} else {
			a.Errorf("failed to execute query: %s, error: %v", query, err)
		}
		return err
	}
	if capture && columnsSaved != nil {
		a.dump.recordQuery(query, columnsSaved, rowsSaved)
	}
	return nil
}

// doQueryRow executes a query that returns a single row
func (a *Collector) doQueryRow(ctx context.Context, queryName, query string, assign func(column, value string)) error {
	var (
		capture      bool
		columnsSaved []string
		rowsSaved    [][]string
	)
	if a.dump != nil {
		capture = true
	}

	start := time.Now()
	defer func() {
		a.recordQueryLatency(queryName, time.Since(start))
	}()

	err := a.client.QueryWithLimit(ctx, query, 1, func(columns []string, values []string) error {
		for i, col := range columns {
			assign(col, values[i])
		}
		if capture {
			columnsSaved = append([]string{}, columns...)
			rowCopy := make([]string, len(values))
			copy(rowCopy, values)
			rowsSaved = append(rowsSaved, rowCopy)
		}
		return nil
	})
	if err != nil {
		if isSQLFeatureError(err) {
			a.Debugf("query failed with expected feature error: %s, error: %v", query, err)
		} else if isSQLTemporaryError(err) {
			a.Debugf("query failed with temporary database error: %s, error: %v", query, err)
		} else {
			a.Errorf("failed to execute query: %s, error: %v", query, err)
		}
		return err
	}
	if capture && columnsSaved != nil {
		a.dump.recordQuery(query, columnsSaved, rowsSaved)
	}
	return nil
}

// Per-instance collection methods

func (a *Collector) collectDiskInstances(ctx context.Context) error {
	allowed, count, err := a.diskCardinality.Allow(ctx, a.countDisks)
	if err != nil {
		return err
	}
	if !allowed {
		a.logOnce("disk_cardinality", "disk count (%d) exceeds limit (%d), skipping per-disk metrics", count, a.MaxDisks)
		return nil
	}

	var currentUnit string
	return a.doQuery(ctx, "disk_instances", queryDiskInstances, func(column, value string, lineEnd bool) {

		switch column {
		case "UNIT_NUMBER":
			currentUnit = value

			// Apply selector if configured
			if a.diskSelector != nil && !a.diskSelector.MatchString(currentUnit) {
				currentUnit = "" // Skip this disk
				return
			}

			_ = a.getDiskMetrics(currentUnit)

		case "UNIT_TYPE":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				// Map IBM i disk type values to meaningful labels
				switch value {
				case "0":
					a.disks[currentUnit].typeField = "HDD"
				case "1":
					a.disks[currentUnit].typeField = "SSD"
				case "":
					a.disks[currentUnit].typeField = "UNKNOWN"
				default:
					a.disks[currentUnit].typeField = value // Keep unknown values as-is
				}
			}
		case "UNIT_MODEL":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				a.disks[currentUnit].model = value
			}
		case "PERCENT_BUSY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, ok := a.parseInt64Value(value, precision); ok {
					disk.busyPercent = v
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
				if v, ok := a.parseInt64Value(value, 1); ok {
					disk.readRequests = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.ReadRequests = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							ReadRequests: v,
						}
					}
				}
			}
		case "WRITE_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				if v, ok := a.parseInt64Value(value, 1); ok {
					disk.writeRequests = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.WriteRequests = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							WriteRequests: v,
						}
					}
				}
			}
		case "PERCENT_USED":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.PercentUsed = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							PercentUsed: v,
						}
					}
				}
			}
		case "UNIT_SPACE_AVAILABLE_GB":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.AvailableGB = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							AvailableGB: v,
						}
					}
				}
			}
		case "UNIT_STORAGE_CAPACITY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.CapacityGB = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							CapacityGB: v,
						}
					}
				}
			}
		case "TOTAL_BLOCKS_READ":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BlocksRead = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							BlocksRead: v,
						}
					}
				}
			}
		case "TOTAL_BLOCKS_WRITTEN":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BlocksWritten = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							BlocksWritten: v,
						}
					}
				}
			}
		case "SSD_LIFE_REMAINING":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v := parseInt64OrZero(value); v > 0 {
					disk := a.disks[currentUnit]
					disk.ssdLifeRemaining = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.SSDLifeRemaining = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							SSDLifeRemaining: v,
						}
					}
				}
			}
		case "SSD_POWER_ON_DAYS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v := parseInt64OrZero(value); v > 0 {
					disk := a.disks[currentUnit]
					disk.ssdPowerOnDays = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.SSDPowerOnDays = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							SSDPowerOnDays: v,
						}
					}
				}
			}
		case "HARDWARE_STATUS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				disk.hardwareStatus = value
				if m, ok := a.mx.disks[currentUnit]; ok {
					m.HardwareStatus = value
					a.mx.disks[currentUnit] = m
				} else {
					a.mx.disks[currentUnit] = diskInstanceMetrics{
						HardwareStatus: value,
					}
				}
			}
		case "DISK_MODEL":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				disk.diskModel = value
				if m, ok := a.mx.disks[currentUnit]; ok {
					m.DiskModel = value
					a.mx.disks[currentUnit] = m
				} else {
					a.mx.disks[currentUnit] = diskInstanceMetrics{
						DiskModel: value,
					}
				}
			}
		case "SERIAL_NUMBER":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				disk := a.disks[currentUnit]
				disk.serialNumber = value
				if m, ok := a.mx.disks[currentUnit]; ok {
					m.SerialNumber = value
					a.mx.disks[currentUnit] = m
				} else {
					a.mx.disks[currentUnit] = diskInstanceMetrics{
						SerialNumber: value,
					}
				}
			}
		}

		// After processing all columns for this disk, calculate used_gb if we have the required data
		if lineEnd && currentUnit != "" {
			if m, ok := a.mx.disks[currentUnit]; ok {
				// Calculate used_gb from capacity - available
				// Always calculate used_gb if we have capacity information
				if m.CapacityGB > 0 {
					usedGB := m.CapacityGB - m.AvailableGB
					// Ensure used_gb is not negative
					if usedGB < 0 {
						usedGB = 0
					}
					m.UsedGB = usedGB
					a.mx.disks[currentUnit] = m
				}
			}
		}
	})
}

func (a *Collector) countDisks(ctx context.Context) (int, error) {
	var count int
	err := a.doQuery(ctx, "count_disks", queryCountDisks, func(column, value string, lineEnd bool) {
		if column == "COUNT" {
			count = int(parseInt64OrZero(value))
		}
	})
	return count, err
}

// Network connections collection
func (a *Collector) collectNetworkConnections(ctx context.Context) error {
	return a.doQuery(ctx, "network_connections", queryNetworkConnections, func(column, value string, lineEnd bool) {
		switch column {
		case "REMOTE_CONNECTIONS":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.RemoteConnections = v
			}
		case "TOTAL_CONNECTIONS":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.TotalConnections = v
			}
		case "LISTEN_CONNECTIONS":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.ListenConnections = v
			}
		case "CLOSEWAIT_CONNECTIONS":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.CloseWaitConnections = v
			}
		}
	})
}

func (a *Collector) countNetworkInterfaces(ctx context.Context) (int, error) {
	var count int
	err := a.doQueryRow(ctx, "count_network_interfaces", queryCountNetworkInterfaces, func(column, value string) {
		if column == "COUNT" {
			if v, ok := a.parseInt64Value(value, 1); ok {
				count = int(v)
			}
		}
	})
	return count, err
}

func (a *Collector) countHTTPServers(ctx context.Context) (int, error) {
	var count int64
	err := a.doQueryRow(ctx, "count_http_servers", queryCountHTTPServers, func(column, value string) {
		if column == "COUNT" {
			if v, ok := a.parseInt64Value(value, 1); ok {
				count = v
			}
		}
	})
	return int(count), err
}

func withFetchLimit(query string, limit int) string {
	if limit <= 0 {
		return query
	}
	trimmed := strings.TrimSpace(query)
	upper := strings.ToUpper(trimmed)
	if strings.Contains(upper, "FETCH FIRST") {
		return trimmed
	}
	return fmt.Sprintf("%s FETCH FIRST %d ROWS ONLY", trimmed, limit)
}

func (a *Collector) countSubsystems(ctx context.Context) (int, error) {
	var count int64
	err := a.doQueryRow(ctx, "count_subsystems", queryCountSubsystems, func(column, value string) {
		if column == "COUNT" {
			if v, ok := a.parseInt64Value(value, 1); ok {
				count = v
			}
		}
	})
	return int(count), err
}

// Temporary storage collection
func (a *Collector) collectTempStorage(ctx context.Context) error {
	// Collect total temp storage
	err := a.doQuery(ctx, "temp_storage_total", queryTempStorageTotal, func(column, value string, lineEnd bool) {
		switch column {
		case "CURRENT_SIZE":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.TempStorageCurrentTotal = v
			}
		case "PEAK_SIZE":
			if v, ok := a.parseInt64Value(value, 1); ok {
				a.mx.TempStoragePeakTotal = v
			}
		}
	})
	if err != nil {
		return err
	}

	// Collect named temp storage buckets
	var currentBucket string
	return a.doQuery(ctx, "temp_storage_named", queryTempStorageNamed, func(column, value string, lineEnd bool) {
		switch column {
		case "NAME":
			currentBucket = value
			_ = a.getTempStorageMetrics(currentBucket)

		case "CURRENT_SIZE":
			if currentBucket != "" && a.tempStorageNamed[currentBucket] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.tempStorageNamed[currentBucket]; ok {
						m.CurrentSize = v
						a.mx.tempStorageNamed[currentBucket] = m
					} else {
						a.mx.tempStorageNamed[currentBucket] = tempStorageInstanceMetrics{
							CurrentSize: v,
						}
					}
				}
			}
		case "PEAK_SIZE":
			if currentBucket != "" && a.tempStorageNamed[currentBucket] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.tempStorageNamed[currentBucket]; ok {
						m.PeakSize = v
						a.mx.tempStorageNamed[currentBucket] = m
					}
				}
			}
		}
	})
}

// Subsystems collection
func (a *Collector) collectSubsystems(ctx context.Context) error {
	query := querySubsystems
	if a.MaxSubsystems > 0 {
		if total, err := a.countSubsystems(ctx); err != nil {
			a.logOnce("subsystem_count_failed", "failed to count subsystems before applying limit: %v", err)
		} else if total > a.MaxSubsystems {
			a.logOnce("subsystem_limit", "subsystem count (%d) exceeds limit (%d); truncating results", total, a.MaxSubsystems)
		}
		query = withFetchLimit(query, a.MaxSubsystems)
	}

	var currentSubsystem string
	return a.doQuery(ctx, "subsystems", query, func(column, value string, lineEnd bool) {
		switch column {
		case "SUBSYSTEM_NAME":
			name := strings.TrimSpace(value)
			if name == "" {
				currentSubsystem = ""
				return
			}
			if a.subsystemSelector != nil && !a.subsystemSelector.MatchString(name) {
				currentSubsystem = ""
				return
			}
			currentSubsystem = name
			subsystem := a.getSubsystemMetrics(currentSubsystem)
			parts := strings.SplitN(name, "/", 2)
			if len(parts) == 2 {
				subsystem.library = parts[0]
				subsystem.name = parts[1]
			} else {
				subsystem.name = name
				subsystem.library = ""
			}
			subsystem.status = "ACTIVE"

		case "CURRENT_ACTIVE_JOBS":
			if currentSubsystem != "" && a.subsystems[currentSubsystem] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
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
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.subsystems[currentSubsystem]; ok {
						m.MaximumActiveJobs = v
						a.mx.subsystems[currentSubsystem] = m
					}
				}
			}
			// Note: HELD_JOB_COUNT and STORAGE_USED_KB columns removed - they don't exist in SUBSYSTEM_INFO table
		}

		if lineEnd {
			currentSubsystem = ""
		}
	})
}

// Job queues collection
func (a *Collector) collectJobQueues(ctx context.Context) error {
	if len(a.jobQueueTargets) == 0 {
		return nil
	}

	var firstErr error

	for _, target := range a.jobQueueTargets {
		key := target.ID()
		queue := a.getJobQueueMetrics(key)
		queue.library = target.Library
		queue.name = target.Name
		queue.status = "UNKNOWN"
		a.jobQueues[key] = queue

		metrics := jobQueueInstanceMetrics{}
		found := false

		queryName := fmt.Sprintf("job_queue_%s_%s", target.Library, target.Name)
		err := a.doQuery(ctx, queryName, buildJobQueueQuery(target), func(column, value string, lineEnd bool) {
			switch column {
			case "JOB_QUEUE_STATUS":
				queue.status = strings.TrimSpace(value)
			case "NUMBER_OF_JOBS":
				metrics.NumberOfJobs = parseInt64OrZero(value)
			case "RELEASED_JOBS":
				queue.jobsWaiting = parseInt64OrZero(value)
			case "SCHEDULED_JOBS":
				queue.jobsScheduled = parseInt64OrZero(value)
			case "HELD_JOBS":
				queue.jobsHeld = parseInt64OrZero(value)
			}

			if lineEnd {
				found = true
			}
		})

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("job queue %s: %w", key, err)
			}
			continue
		}

		if !found {
			queue.status = "NOT_FOUND"
		}

		a.mx.jobQueues[key] = metrics
	}

	return firstErr
}

// Enhanced disk collection with all metrics
func (a *Collector) collectDiskInstancesEnhanced(ctx context.Context) error {
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
	return a.doQuery(ctx, "disk_instances_enhanced", queryDiskInstancesEnhanced, func(column, value string, lineEnd bool) {
		switch column {
		case "UNIT_NUMBER":
			currentUnit = value

			// Apply selector if configured
			if a.diskSelector != nil && !a.diskSelector.MatchString(currentUnit) {
				currentUnit = "" // Skip this disk
				return
			}

			_ = a.getDiskMetrics(currentUnit)

		case "UNIT_TYPE":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				// Map IBM i disk type values to meaningful labels
				switch value {
				case "0":
					a.disks[currentUnit].typeField = "HDD"
				case "1":
					a.disks[currentUnit].typeField = "SSD"
				case "":
					a.disks[currentUnit].typeField = "UNKNOWN"
				default:
					a.disks[currentUnit].typeField = value // Keep unknown values as-is
				}
			}
		case "PERCENT_USED":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.PercentUsed = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							PercentUsed: v,
						}
					}
				}
			}
		case "UNIT_SPACE_AVAILABLE_GB":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.AvailableGB = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "UNIT_STORAGE_CAPACITY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.CapacityGB = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "TOTAL_READ_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.ReadRequests = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							ReadRequests: v,
						}
					}
				}
			}
		case "TOTAL_WRITE_REQUESTS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.WriteRequests = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							WriteRequests: v,
						}
					}
				}
			}
		case "TOTAL_BLOCKS_READ":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BlocksRead = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							BlocksRead: v,
						}
					}
				}
			}
		case "TOTAL_BLOCKS_WRITTEN":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, 1); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BlocksWritten = v
						a.mx.disks[currentUnit] = m
					} else {
						a.mx.disks[currentUnit] = diskInstanceMetrics{
							BlocksWritten: v,
						}
					}
				}
			}
		case "ELAPSED_PERCENT_BUSY":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v, ok := a.parseInt64Value(value, precision); ok {
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.BusyPercent = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "SSD_LIFE_REMAINING":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v := parseInt64OrZero(value); v > 0 {
					disk := a.disks[currentUnit]
					disk.ssdLifeRemaining = v
					if m, ok := a.mx.disks[currentUnit]; ok {
						m.SSDLifeRemaining = v
						a.mx.disks[currentUnit] = m
					}
				}
			}
		case "SSD_POWER_ON_DAYS":
			if currentUnit != "" && a.disks[currentUnit] != nil {
				if v := parseInt64OrZero(value); v > 0 {
					disk := a.disks[currentUnit]
					disk.ssdPowerOnDays = v
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

func (a *Collector) collectNetworkInterfaces(ctx context.Context) error {
	allowed, count, err := a.networkInterfacesCardinality.Allow(ctx, a.countNetworkInterfaces)
	if err != nil {
		return fmt.Errorf("failed to count network interfaces: %w", err)
	}
	if !allowed {
		a.logOnce("network_interfaces_cardinality", "too many network interfaces (%d), skipping collection to avoid performance issues", count)
		return nil
	}

	var currentInterface string
	return a.doQuery(ctx, "network_interfaces", queryNetworkInterfaces, func(column, value string, lineEnd bool) {
		switch column {
		case "LINE_DESCRIPTION":
			iface := strings.TrimSpace(value)
			if iface == "" {
				currentInterface = ""
				return
			}
			currentInterface = iface
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			intf.name = iface

		case "INTERFACE_LINE_TYPE":
			if currentInterface == "" {
				return
			}
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			intf.interfaceType = value
			clean := cleanName(currentInterface)
			entry := a.mx.networkInterfaces[clean]
			entry.InterfaceType = value
			a.mx.networkInterfaces[clean] = entry

		case "INTERFACE_STATUS":
			if currentInterface == "" {
				return
			}
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			intf.interfaceStatus = value
			clean := cleanName(currentInterface)
			entry := a.mx.networkInterfaces[clean]
			if strings.EqualFold(value, "ACTIVE") {
				entry.InterfaceStatus = 1
			} else {
				entry.InterfaceStatus = 0
			}
			a.mx.networkInterfaces[clean] = entry

		case "CONNECTION_TYPE":
			if currentInterface == "" {
				return
			}
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			intf.connectionType = value
			clean := cleanName(currentInterface)
			entry := a.mx.networkInterfaces[clean]
			entry.ConnectionType = value
			a.mx.networkInterfaces[clean] = entry

		case "INTERNET_ADDRESS":
			if currentInterface == "" {
				return
			}
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			intf.internetAddress = value
			clean := cleanName(currentInterface)
			entry := a.mx.networkInterfaces[clean]
			entry.InternetAddress = value
			a.mx.networkInterfaces[clean] = entry

		case "NETWORK_ADDRESS":
			if currentInterface == "" {
				return
			}
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			intf.networkAddress = value
			clean := cleanName(currentInterface)
			entry := a.mx.networkInterfaces[clean]
			entry.NetworkAddress = value
			a.mx.networkInterfaces[clean] = entry

		case "MAXIMUM_TRANSMISSION_UNIT":
			if currentInterface == "" {
				return
			}
			intf := a.getNetworkInterfaceMetrics(currentInterface)
			if v, ok := a.parseInt64Value(value, 1); ok {
				intf.mtu = v
				clean := cleanName(currentInterface)
				entry := a.mx.networkInterfaces[clean]
				entry.MTU = v
				a.mx.networkInterfaces[clean] = entry
			}
		}

		if lineEnd {
			currentInterface = ""
		}
	})
}

func (a *Collector) collectHTTPServerInfo(ctx context.Context) error {
	if !a.CollectHTTPServerMetrics.IsEnabled() {
		return nil
	}

	allowed, count, err := a.httpServersCardinality.Allow(ctx, a.countHTTPServers)
	if err != nil {
		return fmt.Errorf("failed to count HTTP server rows: %w", err)
	}
	if !allowed {
		a.logOnce("http_server_cardinality", "too many HTTP server entries (%d), skipping collection to avoid performance issues", count)
		return nil
	}

	var (
		serverName   string
		functionName string
		currentKey   string
	)

	return a.doQuery(ctx, "http_server_info", queryHTTPServerInfo, func(column, value string, lineEnd bool) {
		switch column {
		case "SERVER_NAME":
			serverName = strings.TrimSpace(value)
		case "HTTP_FUNCTION":
			functionName = strings.TrimSpace(value)
			if serverName == "" {
				serverName = "UNKNOWN"
			}
			if functionName == "" {
				functionName = "UNKNOWN"
			}
			currentKey = httpServerKey(serverName, functionName)
			if meta := a.getHTTPServerMetrics(serverName, functionName); meta != nil {
				meta.serverName = serverName
				meta.httpFunction = functionName
			}
		case "SERVER_NORMAL_CONNECTIONS":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.NormalConnections = v
				a.mx.httpServers[currentKey] = entry
			}
		case "SERVER_SSL_CONNECTIONS":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.SSLConnections = v
				a.mx.httpServers[currentKey] = entry
			}
		case "SERVER_ACTIVE_THREADS":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.ActiveThreads = v
				a.mx.httpServers[currentKey] = entry
			}
		case "SERVER_IDLE_THREADS":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.IdleThreads = v
				a.mx.httpServers[currentKey] = entry
			}
		case "SERVER_TOTAL_REQUESTS":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.TotalRequests = v
				a.mx.httpServers[currentKey] = entry
			}
		case "SERVER_TOTAL_RESPONSES":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.TotalResponses = v
				a.mx.httpServers[currentKey] = entry
			}
		case "SERVER_TOTAL_REQUESTS_REJECTED":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.TotalRequestsRejected = v
				a.mx.httpServers[currentKey] = entry
			}
		case "BYTES_RECEIVED":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.BytesReceived = v
				a.mx.httpServers[currentKey] = entry
			}
		case "BYTES_SENT":
			if currentKey == "" {
				return
			}
			if v, ok := a.parseInt64Value(value, 1); ok {
				entry := a.mx.httpServers[currentKey]
				entry.BytesSent = v
				a.mx.httpServers[currentKey] = entry
			}
		}

		if lineEnd {
			serverName = ""
			functionName = ""
			currentKey = ""
		}
	})
}

func (a *Collector) collectPlanCache(ctx context.Context) error {
	if !a.CollectPlanCacheMetrics.IsEnabled() {
		return nil
	}

	start := time.Now()
	if err := a.client.Exec(ctx, callAnalyzePlanCache); err != nil {
		return fmt.Errorf("failed to analyze plan cache: %w", err)
	}
	elapsed := time.Since(start)
	a.Debugf("plan cache analysis completed in %v", elapsed)
	a.recordQueryLatency("analyze_plan_cache", elapsed)

	var currentHeading string
	return a.doQuery(ctx, "plan_cache_summary", queryPlanCacheSummary, func(column, value string, lineEnd bool) {
		switch column {
		case "HEADING":
			currentHeading = strings.TrimSpace(value)
		case "VALUE":
			if currentHeading == "" {
				return
			}
			key := planCacheMetricKey(currentHeading)
			if key == "" {
				return
			}
			if parsed, ok := a.parseInt64Value(value, precision); ok {
				if meta := a.getPlanCacheMetrics(key, currentHeading); meta != nil {
					meta.heading = currentHeading
				}
				entry := a.mx.planCache[key]
				entry.Value = parsed
				a.mx.planCache[key] = entry
			}
		}
		if lineEnd {
			currentHeading = ""
		}
	})
}

func (a *Collector) collectSystemActivity(ctx context.Context) error {
	// IBM deprecated AVERAGE_CPU_* columns in 7.4, so we use a hybrid approach:
	// 1. Try TOTAL_CPU_TIME (requires *JOBCTL authority) - monotonic counter, most accurate
	// 2. Fall back to ELAPSED_CPU_USED with reset detection if *JOBCTL unavailable

	// Query both potential data sources in one query
	query := a.systemActivityQuery()

	a.mx.EntitledCPUPercentage = 0

	var (
		totalCPUTime    int64   // Nanoseconds since IPL (NULL if no *JOBCTL)
		elapsedTime     int64   // Seconds since last reset
		elapsedCPUUsed  float64 // Average CPU% since last reset
		hasTotalCPUTime bool
		hasElapsedData  bool
	)

	err := a.doQuery(ctx, "system_activity", query, func(column, value string, lineEnd bool) {
		switch column {
		case "TOTAL_CPU_TIME":
			// This will be NULL if user doesn't have *JOBCTL authority
			if value != "" && !strings.EqualFold(value, "NULL") {
				if v, ok := a.parseInt64Value(value, 1); ok {
					totalCPUTime = v
					hasTotalCPUTime = true
				}
			}
		case "ELAPSED_TIME":
			if value != "" && !strings.EqualFold(value, "NULL") {
				if v, ok := a.parseInt64Value(value, 1); ok {
					elapsedTime = v
					hasElapsedData = true
				}
			}
		case "ELAPSED_CPU_USED":
			if value != "" && !strings.EqualFold(value, "NULL") {
				if v, ok := a.parseFloat64Value(value); ok {
					elapsedCPUUsed = v
				}
			}
		}
	})

	if err != nil {
		return fmt.Errorf("failed to collect system activity: %w", err)
	}

	// Determine which method to use
	if hasTotalCPUTime {
		// Primary method: Use TOTAL_CPU_TIME (requires *JOBCTL)
		if a.cpuCollectionMethod == "" {
			a.cpuCollectionMethod = "total_cpu_time"
			a.Debugf("CPU collection: using TOTAL_CPU_TIME method (*JOBCTL authority available)")
		}

		if a.hasCPUBaseline {
			// Calculate CPU utilization from delta
			deltaNanos := totalCPUTime - a.prevTotalCPUTime

			// Convert to per-core percentage based on update_every interval
			// TOTAL_CPU_TIME is cumulative CPU-seconds across all processors in nanoseconds
			// The delta/interval ratio directly gives us cores consumed (already in per-core scale)
			// Formula: (delta_nanoseconds / 1e9) / update_every_seconds * 100
			// Example: 2.0 CPU-seconds consumed in 1 second = 200% (2 cores fully utilized)
			if a.UpdateEvery > 0 {
				deltaSeconds := float64(deltaNanos) / 1e9
				intervalSeconds := float64(a.UpdateEvery)

				// TOTAL_CPU_TIME is naturally in per-core scale - do NOT divide by ConfiguredCPUs
				cpuUtilization := (deltaSeconds / intervalSeconds) * 100.0 * precision
				maxAllowed := float64(a.mx.ConfiguredCPUs) * 100.0 * precision
				if cpuUtilization >= 0 && (a.mx.ConfiguredCPUs <= 0 || cpuUtilization <= maxAllowed) {
					a.mx.systemActivity.AverageCPUUtilization = int64(cpuUtilization)
					a.mx.systemActivity.AverageCPURate = int64(cpuUtilization)
					a.mx.CPUPercentage = int64(cpuUtilization)
					a.mx.EntitledCPUPercentage = a.computeEntitledCPUPercentage(cpuUtilization)
				} else {
					if cpuUtilization < 0 {
						a.Warningf("CPU collection: calculated utilization negative (%.2f%%), skipping this sample", cpuUtilization/precision)
					} else {
						a.Warningf("CPU collection: calculated utilization (%.2f%%) exceeds configured capacity (%d CPUs), skipping this sample",
							cpuUtilization/precision, a.mx.ConfiguredCPUs)
					}
				}
			}
		} else {
			a.Debugf("CPU collection: establishing baseline for TOTAL_CPU_TIME method")
		}

		// Save current values for next iteration
		a.prevTotalCPUTime = totalCPUTime
		a.hasCPUBaseline = true

	} else if hasElapsedData {
		// Fallback method: Use ELAPSED_CPU_USED with reset detection
		if a.cpuCollectionMethod == "" {
			a.cpuCollectionMethod = "elapsed_cpu_used"
			a.Warningf("CPU collection: *JOBCTL authority not available, using ELAPSED_CPU_USED fallback method")
			a.Warningf("CPU collection: This method is affected by SYSTEM_STATUS(RESET_STATISTICS=>'YES') calls")
		}

		// Calculate product for reset detection
		cpuProduct := int64(elapsedCPUUsed * float64(elapsedTime) * precision)

		if a.hasCPUBaseline {
			// Detect if statistics were reset
			resetDetected := false
			if elapsedTime < a.prevElapsedTime {
				resetDetected = true
				a.Warningf("CPU collection: statistics reset detected (ELAPSED_TIME decreased from %d to %d)", a.prevElapsedTime, elapsedTime)
			} else if cpuProduct < a.prevElapsedCPUProduct {
				resetDetected = true
				a.Warningf("CPU collection: statistics reset detected (CPU product decreased from %d to %d)", a.prevElapsedCPUProduct, cpuProduct)
			}

			if !resetDetected {
				// Calculate delta-based CPU utilization
				deltaProduct := cpuProduct - a.prevElapsedCPUProduct
				deltaTime := elapsedTime - a.prevElapsedTime

				if deltaTime > 0 {
					// ELAPSED_CPU_USED is already in per-core scaling
					intervalCPU := float64(deltaProduct) / float64(deltaTime)
					cpuUtilization := intervalCPU
					maxAllowed := float64(a.mx.ConfiguredCPUs) * 100.0 * precision
					if cpuUtilization >= 0 && (a.mx.ConfiguredCPUs <= 0 || cpuUtilization <= maxAllowed) {
						a.mx.systemActivity.AverageCPUUtilization = int64(cpuUtilization)
						a.mx.systemActivity.AverageCPURate = int64(cpuUtilization)
						a.mx.CPUPercentage = int64(cpuUtilization)
						a.mx.EntitledCPUPercentage = a.computeEntitledCPUPercentage(cpuUtilization)
					} else {
						if cpuUtilization < 0 {
							a.Warningf("CPU collection: interval utilization negative (%.2f%%), skipping this sample", cpuUtilization/precision)
						} else {
							a.Warningf("CPU collection: interval utilization (%.2f%%) exceeds configured capacity (%d CPUs), skipping this sample",
								cpuUtilization/precision, a.mx.ConfiguredCPUs)
						}
					}
				}
			} else {
				a.Debugf("CPU collection: re-establishing baseline after reset")
				a.hasCPUBaseline = false
			}
		} else {
			a.Debugf("CPU collection: establishing baseline for ELAPSED_CPU_USED method")
		}

		// Save current values for next iteration
		a.prevElapsedTime = elapsedTime
		a.prevElapsedCPUProduct = cpuProduct
		a.hasCPUBaseline = true

	} else {
		return fmt.Errorf("failed to collect CPU data: no usable CPU metrics available")
	}

	return nil
}
