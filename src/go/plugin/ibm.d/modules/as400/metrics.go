// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

type metricsData struct {
	// CPU metrics from SYSTEM_STATUS()
	CPUPercentage         int64 `stm:"cpu_percentage"`       // AVERAGE_CPU_UTILIZATION
	CurrentCPUCapacity    int64 `stm:"current_cpu_capacity"` // CURRENT_CPU_CAPACITY
	ConfiguredCPUs        int64 `stm:"configured_cpus"`      // CONFIGURED_CPUS
	EntitledCPUPercentage int64 `stm:"entitled_cpu_percentage"`

	// Memory metrics from SYSTEM_STATUS()
	MainStorageSize             int64 `stm:"main_storage_size"`              // MAIN_STORAGE_SIZE (KB)
	CurrentTemporaryStorage     int64 `stm:"current_temporary_storage"`      // CURRENT_TEMPORARY_STORAGE (MB)
	MaximumTemporaryStorageUsed int64 `stm:"maximum_temporary_storage_used"` // MAXIMUM_TEMPORARY_STORAGE_USED (MB)

	// Job metrics from SYSTEM_STATUS()
	TotalJobsInSystem       int64 `stm:"total_jobs_in_system"`       // TOTAL_JOBS_IN_SYSTEM
	ActiveJobsInSystem      int64 `stm:"active_jobs_in_system"`      // ACTIVE_JOBS_IN_SYSTEM
	InteractiveJobsInSystem int64 `stm:"interactive_jobs_in_system"` // INTERACTIVE_JOBS_IN_SYSTEM
	BatchJobsRunning        int64 `stm:"batch_jobs_running"`         // BATCH_RUNNING
	JobQueueLength          int64 `stm:"job_queue_length"`           // From JOB_INFO query

	// Storage metrics from SYSTEM_STATUS()
	SystemASPUsed         int64 `stm:"system_asp_used"`         // SYSTEM_ASP_USED (percentage)
	SystemASPStorage      int64 `stm:"system_asp_storage"`      // SYSTEM_ASP_STORAGE (MB)
	TotalAuxiliaryStorage int64 `stm:"total_auxiliary_storage"` // TOTAL_AUXILIARY_STORAGE (MB)

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
	RemoteConnections    int64 `stm:"remote_connections"`    // Count of established remote connections
	TotalConnections     int64 `stm:"total_connections"`     // Total TCP connections
	ListenConnections    int64 `stm:"listen_connections"`    // Connections in LISTEN state
	CloseWaitConnections int64 `stm:"closewait_connections"` // Connections in CLOSE_WAIT state

	// Temporary storage from SYSTMPSTG
	TempStorageCurrentTotal int64 `stm:"temp_storage_current_total"` // Total current temp storage
	TempStoragePeakTotal    int64 `stm:"temp_storage_peak_total"`    // Total peak temp storage

	// Per-instance metrics (not included in stm)
	disks             map[string]diskInstanceMetrics
	subsystems        map[string]subsystemInstanceMetrics
	jobQueues         map[string]jobQueueInstanceMetrics
	messageQueues     map[string]messageQueueInstanceMetrics
	outputQueues      map[string]outputQueueInstanceMetrics
	tempStorageNamed  map[string]tempStorageInstanceMetrics
	activeJobs        map[string]activeJobInstanceMetrics
	networkInterfaces map[string]networkInterfaceInstanceMetrics
	httpServers       map[string]httpServerInstanceMetrics
	planCache         map[string]planCacheInstanceMetrics
	systemActivity    systemActivityMetrics

	queryLatencies map[string]int64 `stm:"-"`
}

// Per-instance metric structures for stm conversion
type diskInstanceMetrics struct {
	BusyPercent      int64  `stm:"busy_percent"`
	ReadRequests     int64  `stm:"read_requests"`
	WriteRequests    int64  `stm:"write_requests"`
	PercentUsed      int64  `stm:"percent_used"` // PERCENT_USED
	AvailableGB      int64  `stm:"available_gb"` // UNIT_SPACE_AVAILABLE_GB
	CapacityGB       int64  // UNIT_STORAGE_CAPACITY - used only for calculation
	UsedGB           int64  `stm:"used_gb"`        // Calculated: CapacityGB - AvailableGB
	BlocksRead       int64  `stm:"blocks_read"`    // TOTAL_BLOCKS_READ
	BlocksWritten    int64  `stm:"blocks_written"` // TOTAL_BLOCKS_WRITTEN
	SSDLifeRemaining int64  // SSD_LIFE_REMAINING - manually added to map when > 0
	SSDPowerOnDays   int64  // SSD_POWER_ON_DAYS - manually added to map when > 0
	HardwareStatus   string // HARDWARE_STATUS - for labels only
	DiskModel        string // DISK_MODEL - for labels only
	SerialNumber     string // SERIAL_NUMBER - for labels only
}

type subsystemInstanceMetrics struct {
	CurrentActiveJobs int64 `stm:"current_active_jobs"` // CURRENT_ACTIVE_JOBS
	MaximumActiveJobs int64 `stm:"maximum_active_jobs"` // MAXIMUM_ACTIVE_JOBS
	// Note: HELD_JOB_COUNT and STORAGE_USED_KB removed - columns don't exist in SUBSYSTEM_INFO table
}

type jobQueueInstanceMetrics struct {
	NumberOfJobs int64 `stm:"number_of_jobs"` // NUMBER_OF_JOBS
	// Note: HELD_JOB_COUNT removed - column doesn't exist in JOB_QUEUE_INFO table
}

type messageQueueInstanceMetrics struct {
	Total         int64 `stm:"total"`
	Informational int64 `stm:"informational"`
	Inquiry       int64 `stm:"inquiry"`
	Diagnostic    int64 `stm:"diagnostic"`
	Escape        int64 `stm:"escape"`
	Notify        int64 `stm:"notify"`
	SenderCopy    int64 `stm:"sender_copy"`
	MaxSeverity   int64 `stm:"max_severity"`
}

type outputQueueInstanceMetrics struct {
	Files    int64 `stm:"files"`
	Writers  int64 `stm:"writers"`
	Released int64 `stm:"released"`
}

type tempStorageInstanceMetrics struct {
	CurrentSize int64 `stm:"current_size"` // BUCKET_CURRENT_SIZE
	PeakSize    int64 `stm:"peak_size"`    // BUCKET_PEAK_SIZE
}

type activeJobInstanceMetrics struct {
	CPUPercentage                  int64 `stm:"cpu_percentage"`                   // CPU_PERCENTAGE
	ElapsedCPUTime                 int64 `stm:"elapsed_cpu_time"`                 // ELAPSED_CPU_TIME (seconds)
	ElapsedTime                    int64 `stm:"elapsed_time"`                     // ELAPSED_TIME (seconds)
	TemporaryStorage               int64 `stm:"temporary_storage"`                // TEMPORARY_STORAGE (MB)
	ThreadCount                    int64 `stm:"thread_count"`                     // THREAD_COUNT
	ElapsedDiskIO                  int64 `stm:"elapsed_disk_io"`                  // ELAPSED_TOTAL_DISK_IO_COUNT
	ElapsedInteractiveTransactions int64 `stm:"elapsed_interactive_transactions"` // ELAPSED_INTERACTIVE_TRANSACTIONS
}

type networkInterfaceInstanceMetrics struct {
	InterfaceStatus int64  `stm:"interface_status"` // 1 if ACTIVE, 0 if not
	MTU             int64  `stm:"mtu"`              // MAXIMUM_TRANSMISSION_UNIT
	InterfaceType   string // INTERFACE_LINE_TYPE - for labels only
	ConnectionType  string // CONNECTION_TYPE - for labels only
	InternetAddress string // INTERNET_ADDRESS - for labels only
	NetworkAddress  string // NETWORK_ADDRESS - for labels only
	SubnetMask      string // INTERFACE_SUBNET_MASK - for labels only
}

type systemActivityMetrics struct {
	AverageCPURate        int64 `stm:"average_cpu_rate"`        // AVERAGE_CPU_RATE (percentage with precision)
	AverageCPUUtilization int64 `stm:"average_cpu_utilization"` // AVERAGE_CPU_UTILIZATION (percentage with precision)
	MinimumCPUUtilization int64 `stm:"minimum_cpu_utilization"` // MINIMUM_CPU_UTILIZATION (percentage with precision)
	MaximumCPUUtilization int64 `stm:"maximum_cpu_utilization"` // MAXIMUM_CPU_UTILIZATION (percentage with precision)
}

type httpServerInstanceMetrics struct {
	NormalConnections     int64 `stm:"normal_connections"`
	SSLConnections        int64 `stm:"ssl_connections"`
	ActiveThreads         int64 `stm:"active_threads"`
	IdleThreads           int64 `stm:"idle_threads"`
	TotalRequests         int64 `stm:"total_requests"`
	TotalResponses        int64 `stm:"total_responses"`
	TotalRequestsRejected int64 `stm:"total_requests_rejected"`
	BytesReceived         int64 `stm:"bytes_received"`
	BytesSent             int64 `stm:"bytes_sent"`
}

type planCacheInstanceMetrics struct {
	Value int64 `stm:"value"`
}
