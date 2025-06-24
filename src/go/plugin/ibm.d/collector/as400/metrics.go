// SPDX-License-Identifier: GPL-3.0-or-later

package as400

type metricsData struct {
	// System-wide metrics
	CPUPercentage int64 `stm:"cpu_percentage"`

	// Job metrics
	ActiveJobsCount int64 `stm:"active_jobs_count"`
	JobQueueLength  int64 `stm:"job_queue_length"` // Aggregate

	// Storage metrics
	SystemASPUsed int64 `stm:"system_asp_used"`

	// Memory pool metrics
	MachinePoolSize     int64 `stm:"machine_pool_size"`
	BasePoolSize        int64 `stm:"base_pool_size"`
	InteractivePoolSize int64 `stm:"interactive_pool_size"`
	SpoolPoolSize       int64 `stm:"spool_pool_size"`

	// Aggregate disk metrics
	DiskBusyPercentage int64 `stm:"disk_busy_percentage"`
	
	// Per-instance metrics (not included in stm)
	disks      map[string]diskInstanceMetrics
	subsystems map[string]subsystemInstanceMetrics
	jobQueues  map[string]jobQueueInstanceMetrics
}

// Per-instance metric structures for stm conversion
type diskInstanceMetrics struct {
	BusyPercent   int64 `stm:"busy_percent"`
	ReadRequests  int64 `stm:"read_requests"`
	WriteRequests int64 `stm:"write_requests"`
	ReadBytes     int64 `stm:"read_bytes"`
	WriteBytes    int64 `stm:"write_bytes"`
	AverageTime   int64 `stm:"average_time"`
}

type subsystemInstanceMetrics struct {
	JobsActive    int64 `stm:"jobs_active"`
	JobsHeld      int64 `stm:"jobs_held"`
	StorageUsedKB int64 `stm:"storage_used_kb"`
	CurrentJobs   int64 `stm:"current_jobs"`
}

type jobQueueInstanceMetrics struct {
	JobsWaiting   int64 `stm:"jobs_waiting"`
	JobsHeld      int64 `stm:"jobs_held"`
	JobsScheduled int64 `stm:"jobs_scheduled"`
}