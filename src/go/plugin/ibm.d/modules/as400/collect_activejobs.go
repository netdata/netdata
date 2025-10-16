// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

import (
	"context"
	"fmt"
	"strconv"
)

// countActiveJobs returns the number of active jobs for cardinality check
func (a *Collector) countActiveJobs(ctx context.Context) (int, error) {
	var count int
	err := a.doQueryRow(ctx, "count_active_jobs", queryCountActiveJobs, func(column, value string) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})
	return count, err
}

// collectActiveJobs collects metrics for top CPU-consuming active jobs
func (a *Collector) collectActiveJobs(ctx context.Context) error {
	// Check if feature is enabled
	if !a.CollectActiveJobs.IsEnabled() {
		return nil
	}

	allowed, count, err := a.activeJobsCardinality.Allow(ctx, a.countActiveJobs)
	if err != nil {
		return fmt.Errorf("failed to count active jobs: %v", err)
	}
	if !allowed {
		a.logOnce("active_jobs_cardinality", "active jobs (%d) exceed configured limit (%d), skipping detailed collection", count, a.MaxActiveJobs)
		return nil
	}
	a.Debugf("Found %d active jobs in system", count)

	// Query for top active jobs by CPU usage
	query := fmt.Sprintf(queryTopActiveJobs, a.MaxActiveJobs)

	var (
		currentJobName string
		currentJob     *activeJobMetrics
	)
	err = a.doQuery(ctx, "top_active_jobs", query, func(column, value string, lineEnd bool) {
		switch column {
		case "JOB_NAME":
			currentJobName = value
			currentJob = a.getActiveJobMetrics(currentJobName)
			if currentJob != nil {
				currentJob.jobName = value
			}

		case "JOB_STATUS":
			if currentJob != nil {
				currentJob.jobStatus = value
			}

		case "SUBSYSTEM":
			if currentJob != nil {
				currentJob.subsystem = value
			}

		case "JOB_TYPE":
			if currentJob != nil {
				currentJob.jobType = value
			}

		case "ELAPSED_CPU_TIME":
			if currentJob != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					currentJob.elapsedCPUTime = int64(v)
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedCPUTime = int64(v)
						a.mx.activeJobs[currentJobName] = m
					} else {
						a.mx.activeJobs[currentJobName] = activeJobInstanceMetrics{
							ElapsedCPUTime: int64(v),
						}
					}
				}
			}

		case "ELAPSED_TIME":
			if currentJob != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					currentJob.elapsedTime = int64(v)
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedTime = int64(v)
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}

		case "TEMPORARY_STORAGE":
			if currentJob != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// Convert KB to MB
					vMB := v / 1024
					currentJob.temporaryStorage = vMB
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.TemporaryStorage = vMB
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}

		case "CPU_PERCENTAGE":
			if currentJob != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					currentJob.cpuPercentage = int64(v * precision)
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.CPUPercentage = int64(v * precision)
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}

		case "ELAPSED_INTERACTIVE_TRANSACTIONS":
			if currentJob != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					currentJob.interactiveTransactions = v
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedInteractiveTransactions = v
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}

		case "ELAPSED_TOTAL_DISK_IO_COUNT":
			if currentJob != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					currentJob.diskIO = v
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedDiskIO = v
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}

		case "THREAD_COUNT":
			if currentJob != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					currentJob.threadCount = v
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ThreadCount = v
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}

		case "RUN_PRIORITY":
			if currentJob != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					currentJob.runPriority = v
				}
			}
		}
	})

	if err != nil {
		return err
	}

	return nil
}
