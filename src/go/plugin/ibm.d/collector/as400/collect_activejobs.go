// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"context"
	"fmt"
	"strconv"
)

// countActiveJobs returns the number of active jobs for cardinality check
func (a *AS400) countActiveJobs(ctx context.Context) (int, error) {
	var count int
	err := a.doQueryRow(ctx, queryCountActiveJobs, func(column, value string) {
		if column == "COUNT" {
			if v, err := strconv.Atoi(value); err == nil {
				count = v
			}
		}
	})
	return count, err
}

// collectActiveJobs collects metrics for top CPU-consuming active jobs
func (a *AS400) collectActiveJobs(ctx context.Context) error {
	// Check if feature is enabled
	if a.CollectActiveJobs == nil || !*a.CollectActiveJobs {
		return nil
	}

	// First check cardinality if we haven't yet
	if len(a.activeJobs) == 0 && a.MaxActiveJobs > 0 {
		count, err := a.countActiveJobs(ctx)
		if err != nil {
			return fmt.Errorf("failed to count active jobs: %v", err)
		}
		a.Debugf("Found %d active jobs in system", count)
		// We'll collect only top N by CPU usage
	}

	// Mark all jobs as not updated
	for _, job := range a.activeJobs {
		job.updated = false
	}

	// Query for top active jobs by CPU usage
	query := fmt.Sprintf(queryTopActiveJobs, a.MaxActiveJobs)
	
	var currentJobName string
	err := a.doQuery(ctx, query, func(column, value string, lineEnd bool) {
		switch column {
		case "JOB_NAME":
			currentJobName = value
			
			job := a.getActiveJobMetrics(currentJobName)
			job.updated = true
			
			// Add charts on first encounter
			if !job.hasCharts {
				job.hasCharts = true
				a.addActiveJobCharts(job)
			}
			
		case "JOB_STATUS":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				a.activeJobs[currentJobName].jobStatus = value
			}
			
		case "SUBSYSTEM":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				a.activeJobs[currentJobName].subsystem = value
			}
			
		case "JOB_TYPE":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				a.activeJobs[currentJobName].jobType = value
			}
			
		case "ELAPSED_CPU_TIME":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					a.activeJobs[currentJobName].elapsedCPUTime = int64(v)
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
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					a.activeJobs[currentJobName].elapsedTime = int64(v)
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedTime = int64(v)
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}
			
		case "TEMPORARY_STORAGE":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					// Convert KB to MB
					vMB := v / 1024
					a.activeJobs[currentJobName].temporaryStorage = vMB
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.TemporaryStorage = vMB
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}
			
		case "CPU_PERCENTAGE":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseFloat(value, 64); err == nil {
					a.activeJobs[currentJobName].cpuPercentage = int64(v * precision)
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.CPUPercentage = int64(v * precision)
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}
			
		case "ELAPSED_INTERACTIVE_TRANSACTIONS":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.activeJobs[currentJobName].interactiveTransactions = v
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedInteractiveTransactions = v
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}
			
		case "ELAPSED_TOTAL_DISK_IO_COUNT":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.activeJobs[currentJobName].diskIO = v
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ElapsedDiskIO = v
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}
			
		case "THREAD_COUNT":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.activeJobs[currentJobName].threadCount = v
					if m, ok := a.mx.activeJobs[currentJobName]; ok {
						m.ThreadCount = v
						a.mx.activeJobs[currentJobName] = m
					}
				}
			}
			
		case "RUN_PRIORITY":
			if currentJobName != "" && a.activeJobs[currentJobName] != nil {
				if v, err := strconv.ParseInt(value, 10, 64); err == nil {
					a.activeJobs[currentJobName].runPriority = v
				}
			}
		}
	})

	if err != nil {
		return err
	}

	// Remove stale jobs
	for jobName, job := range a.activeJobs {
		if !job.updated {
			delete(a.activeJobs, jobName)
			delete(a.mx.activeJobs, jobName)
			a.removeActiveJobCharts(jobName)
			a.Debugf("Removed stale active job: %s", jobName)
		}
	}

	return nil
}