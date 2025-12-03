// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

import (
	"context"
	"fmt"
	"math"
	"strings"
)

// collectActiveJobs collects metrics for explicitly configured active jobs
func (a *Collector) collectActiveJobs(ctx context.Context) error {
	if len(a.activeJobTargets) == 0 {
		return nil
	}
	if !a.CollectActiveJobs.IsEnabled() {
		return nil
	}

	var firstErr error

	for _, target := range a.activeJobTargets {
		key := target.ID()
		meta := a.getActiveJobMetrics(key)
		meta.qualifiedName = key
		meta.jobNumber = target.Number
		meta.jobUser = target.User
		meta.jobName = target.Name

		metrics := activeJobInstanceMetrics{}
		found := false

		queryName := fmt.Sprintf("active_job_%s_%s_%s", target.Number, target.User, target.Name)
		query := buildActiveJobQuery(target)

		err := a.doQuery(ctx, queryName, query, func(column, value string, lineEnd bool) {
			switch column {
			case "JOB_NAME":
				qualified := strings.TrimSpace(value)
				if qualified != "" {
					meta.qualifiedName = qualified
				}
			case "JOB_USER":
				user := strings.TrimSpace(value)
				if user != "" {
					meta.jobUser = strings.ToUpper(user)
				}
			case "JOB_NUMBER":
				number := strings.TrimSpace(value)
				if number != "" {
					meta.jobNumber = number
				}
			case "JOB_STATUS":
				meta.jobStatus = strings.TrimSpace(value)
			case "SUBSYSTEM":
				meta.subsystem = strings.TrimSpace(value)
			case "JOB_TYPE":
				meta.jobType = strings.TrimSpace(value)
			case "ELAPSED_CPU_TIME":
				if v, ok := a.parseInt64Value(value, 1); ok {
					metrics.ElapsedCPUTime = v
				}
			case "ELAPSED_TIME":
				if v, ok := a.parseInt64Value(value, 1); ok {
					metrics.ElapsedTime = v
				}
			case "TEMPORARY_STORAGE":
				if v, ok := a.parseInt64Value(value, 1); ok {
					metrics.TemporaryStorage = v / 1024 // Convert KB to MB
				}
			case "CPU_PERCENTAGE":
				if f, ok := a.parseFloat64Value(value); ok {
					metrics.CPUPercentage = int64(math.Round(f * float64(precision)))
				}
			case "ELAPSED_INTERACTIVE_TRANSACTIONS":
				if v, ok := a.parseInt64Value(value, 1); ok {
					metrics.ElapsedInteractiveTransactions = v
				}
			case "ELAPSED_TOTAL_DISK_IO_COUNT":
				if v, ok := a.parseInt64Value(value, 1); ok {
					metrics.ElapsedDiskIO = v
				}
			case "THREAD_COUNT":
				if v, ok := a.parseInt64Value(value, 1); ok {
					metrics.ThreadCount = v
				}
			}

			if lineEnd {
				found = true
			}
		})

		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("active job %s: %w", key, err)
			}
			continue
		}

		if !found {
			meta.jobStatus = "NOT FOUND"
			meta.subsystem = ""
			meta.jobType = ""
			metrics = activeJobInstanceMetrics{}
		}

		a.mx.activeJobs[key] = metrics
	}

	return firstErr
}
