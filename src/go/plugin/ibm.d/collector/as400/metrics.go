// SPDX-License-Identifier: GPL-3.0-or-later

package as400

type metricsData struct {
	// CPU metrics from SYSTEM_STATUS()
	CPUPercentage       int64 `stm:"cpu_percentage"`        // AVERAGE_CPU_UTILIZATION
	CurrentCPUCapacity  int64 `stm:"current_cpu_capacity"`  // CURRENT_CPU_CAPACITY
	ConfiguredCPUs      int64 `stm:"configured_cpus"`        // CONFIGURED_CPUS
	
	// Memory metrics from SYSTEM_STATUS()
	MainStorageSize            int64 `stm:"main_storage_size"`             // MAIN_STORAGE_SIZE (KB)
	CurrentTemporaryStorage    int64 `stm:"current_temporary_storage"`     // CURRENT_TEMPORARY_STORAGE (MB)
	MaximumTemporaryStorageUsed int64 `stm:"maximum_temporary_storage_used"` // MAXIMUM_TEMPORARY_STORAGE_USED (MB)
	
	// Job metrics from SYSTEM_STATUS()
	TotalJobsInSystem       int64 `stm:"total_jobs_in_system"`       // TOTAL_JOBS_IN_SYSTEM
	ActiveJobsInSystem      int64 `stm:"active_jobs_in_system"`      // ACTIVE_JOBS_IN_SYSTEM
	InteractiveJobsInSystem int64 `stm:"interactive_jobs_in_system"` // INTERACTIVE_JOBS_IN_SYSTEM
	BatchJobsRunning        int64 `stm:"batch_jobs_running"`         // BATCH_RUNNING
	JobQueueLength          int64 `stm:"job_queue_length"`           // From JOB_INFO query
	
	// Storage metrics from SYSTEM_STATUS()
	SystemASPUsed          int64 `stm:"system_asp_used"`           // SYSTEM_ASP_USED (percentage)
	SystemASPStorage       int64 `stm:"system_asp_storage"`        // SYSTEM_ASP_STORAGE (MB)
	TotalAuxiliaryStorage  int64 `stm:"total_auxiliary_storage"`   // TOTAL_AUXILIARY_STORAGE (MB)
	
	// Thread metrics from SYSTEM_STATUS()
	ActiveThreadsInSystem int64 `stm:"active_threads_in_system"` // ACTIVE_THREADS_IN_SYSTEM
	ThreadsPerProcessor   int64 `stm:"threads_per_processor"`    // THREADS_PER_PROCESSOR
	
	// Legacy job type breakdown (removed - no corresponding charts)

	// Memory pool metrics from MEMORY_POOL()
	MachinePoolSize     int64 `stm:"machine_pool_size"`
	BasePoolSize        int64 `stm:"base_pool_size"`
	InteractivePoolSize int64 `stm:"interactive_pool_size"`
	SpoolPoolSize       int64 `stm:"spool_pool_size"`
	
	MachinePoolDefinedSize  int64 `stm:"machine_pool_defined_size"`
	MachinePoolReservedSize int64 `stm:"machine_pool_reserved_size"`
	BasePoolDefinedSize     int64 `stm:"base_pool_defined_size"`
	BasePoolReservedSize    int64 `stm:"base_pool_reserved_size"`
	
	// Memory pool thread metrics (if available)
	MachinePoolThreads    int64 `stm:"machine_pool_threads"`     // CURRENT_THREADS
	MachinePoolMaxThreads int64 `stm:"machine_pool_max_threads"` // MAXIMUM_ACTIVE_THREADS
	BasePoolThreads       int64 `stm:"base_pool_threads"`
	BasePoolMaxThreads    int64 `stm:"base_pool_max_threads"`

	// Aggregate disk metrics
	DiskBusyPercentage int64 `stm:"disk_busy_percentage"`
	
	// Network metrics from NETSTAT_INFO
	RemoteConnections  int64 `stm:"remote_connections"`   // Count of established remote connections
	TotalConnections   int64 `stm:"total_connections"`    // Total TCP connections
	ListenConnections  int64 `stm:"listen_connections"`   // Connections in LISTEN state
	CloseWaitConnections int64 `stm:"closewait_connections"` // Connections in CLOSE_WAIT state
	
	// Temporary storage from SYSTMPSTG
	TempStorageCurrentTotal int64 `stm:"temp_storage_current_total"` // Total current temp storage
	TempStoragePeakTotal    int64 `stm:"temp_storage_peak_total"`    // Total peak temp storage

	// Per-instance metrics (not included in stm)
	disks           map[string]diskInstanceMetrics
	subsystems      map[string]subsystemInstanceMetrics
	jobQueues       map[string]jobQueueInstanceMetrics
	tempStorageNamed map[string]tempStorageInstanceMetrics
}

// Per-instance metric structures for stm conversion
type diskInstanceMetrics struct {
	BusyPercent      int64 `stm:"busy_percent"`
	ReadRequests     int64 `stm:"read_requests"`
	WriteRequests    int64 `stm:"write_requests"`
	PercentUsed      int64 `stm:"percent_used"`        // PERCENT_USED
	AvailableGB      int64 `stm:"available_gb"`        // UNIT_SPACE_AVAILABLE_GB
	CapacityGB       int64                              // UNIT_STORAGE_CAPACITY - used only for calculation
	BlocksRead       int64 `stm:"blocks_read"`         // TOTAL_BLOCKS_READ
	BlocksWritten    int64 `stm:"blocks_written"`      // TOTAL_BLOCKS_WRITTEN
	SSDLifeRemaining int64 `stm:"ssd_life_remaining"`  // SSD_LIFE_REMAINING
	SSDPowerOnDays   int64 `stm:"ssd_power_on_days"`   // SSD_POWER_ON_DAYS
}

type subsystemInstanceMetrics struct {
	CurrentActiveJobs  int64 `stm:"current_active_jobs"`  // CURRENT_ACTIVE_JOBS
	MaximumActiveJobs  int64 `stm:"maximum_active_jobs"`  // MAXIMUM_ACTIVE_JOBS
	HeldJobCount       int64 `stm:"held_job_count"`       // HELD_JOB_COUNT
	StorageUsedKB      int64 `stm:"storage_used_kb"`      // STORAGE_USED_KB
}

type jobQueueInstanceMetrics struct {
	NumberOfJobs    int64 `stm:"number_of_jobs"`    // NUMBER_OF_JOBS
	HeldJobCount    int64 `stm:"held_job_count"`    // HELD_JOB_COUNT
}

type tempStorageInstanceMetrics struct {
	CurrentSize int64 `stm:"current_size"` // BUCKET_CURRENT_SIZE
	PeakSize    int64 `stm:"peak_size"`    // BUCKET_PEAK_SIZE
}