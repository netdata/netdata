// SPDX-License-Identifier: GPL-3.0-or-later

package as400

// diskMetrics holds metrics for an individual disk
type diskMetrics struct {
	unit          string
	typeField     string // HDD, SSD, etc.
	model         string
	busyPercent   int64
	readRequests  int64
	writeRequests int64
	readBytes     int64
	writeBytes    int64
	averageTime   int64
	ssdLifeRemaining int64 // -1 if not SSD
	ssdPowerOnDays   int64 // -1 if not SSD
	hardwareStatus   string // Hardware status from SYSDISKSTAT
	diskModel        string // Disk model from SYSDISKSTAT
	serialNumber     string // Serial number from SYSDISKSTAT

	updated   bool
	hasCharts bool
}

// subsystemMetrics holds metrics for an individual subsystem
type subsystemMetrics struct {
	name        string
	library     string
	status      string
	jobsActive  int64
	jobsHeld    int64
	storageUsed int64 // KB
	poolID      int64
	maxJobs     int64
	currentJobs int64

	updated   bool
	hasCharts bool
}

// jobQueueMetrics holds metrics for an individual job queue
type jobQueueMetrics struct {
	name          string
	library       string
	status        string
	jobsWaiting   int64
	jobsHeld      int64
	jobsScheduled int64
	maxJobs       int64
	priority      int64

	updated   bool
	hasCharts bool
}

// messageQueueMetrics holds metrics for an individual message queue
type messageQueueMetrics struct {
	name         string
	library      string
	messageCount int64
	oldestHours  int64

	updated   bool
	hasCharts bool
}

// tempStorageMetrics holds metrics for a named temporary storage bucket
type tempStorageMetrics struct {
	name        string
	currentSize int64
	peakSize    int64

	updated   bool
	hasCharts bool
}

// activeJobMetrics holds metrics for an individual active job
type activeJobMetrics struct {
	jobName                string
	jobStatus              string
	subsystem              string
	jobType                string
	runPriority            int64
	elapsedCPUTime         int64  // seconds
	elapsedTime            int64  // seconds
	temporaryStorage       int64  // MB
	cpuPercentage          int64  // percentage * precision
	interactiveTransactions int64
	diskIO                 int64
	threadCount            int64

	updated   bool
	hasCharts bool
}

type networkInterfaceMetrics struct {
	name            string
	interfaceType   string // INTERFACE_LINE_TYPE
	interfaceStatus string // INTERFACE_STATUS
	connectionType  string // CONNECTION_TYPE
	internetAddress string // INTERNET_ADDRESS
	networkAddress  string // NETWORK_ADDRESS
	subnetMask      string // INTERFACE_SUBNET_MASK
	mtu             int64  // MAXIMUM_TRANSMISSION_UNIT

	updated   bool
	hasCharts bool
}

// Per-instance metric methods
func (a *AS400) getDiskMetrics(unit string) *diskMetrics {
	if _, ok := a.disks[unit]; !ok {
		a.disks[unit] = &diskMetrics{
			unit: unit,
			ssdLifeRemaining: -1, // Default to -1 (not SSD)
			ssdPowerOnDays: -1,   // Default to -1 (not SSD)
			hardwareStatus: "UNKNOWN", // Default for hardware status
			diskModel: "UNKNOWN",      // Default for disk model
			serialNumber: "UNKNOWN",   // Default for serial number
		}
	}
	return a.disks[unit]
}

func (a *AS400) getSubsystemMetrics(name string) *subsystemMetrics {
	if _, ok := a.subsystems[name]; !ok {
		a.subsystems[name] = &subsystemMetrics{name: name}
	}
	return a.subsystems[name]
}

func (a *AS400) getJobQueueMetrics(key string) *jobQueueMetrics {
	if _, ok := a.jobQueues[key]; !ok {
		a.jobQueues[key] = &jobQueueMetrics{}
	}
	return a.jobQueues[key]
}

func (a *AS400) getMessageQueueMetrics(key string) *messageQueueMetrics {
	if _, ok := a.messageQueues[key]; !ok {
		a.messageQueues[key] = &messageQueueMetrics{}
	}
	return a.messageQueues[key]
}

func (a *AS400) getTempStorageMetrics(name string) *tempStorageMetrics {
	if _, ok := a.tempStorageNamed[name]; !ok {
		a.tempStorageNamed[name] = &tempStorageMetrics{name: name}
	}
	return a.tempStorageNamed[name]
}

func (a *AS400) getActiveJobMetrics(jobName string) *activeJobMetrics {
	if _, ok := a.activeJobs[jobName]; !ok {
		a.activeJobs[jobName] = &activeJobMetrics{jobName: jobName}
	}
	return a.activeJobs[jobName]
}

func (a *AS400) getNetworkInterfaceMetrics(name string) *networkInterfaceMetrics {
	if _, ok := a.networkInterfaces[name]; !ok {
		a.networkInterfaces[name] = &networkInterfaceMetrics{name: name}
	}
	return a.networkInterfaces[name]
}
