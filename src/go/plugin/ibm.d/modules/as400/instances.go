// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package as400

// diskMetrics holds metrics for an individual disk
type diskMetrics struct {
	unit             string
	typeField        string // HDD, SSD, etc.
	model            string
	busyPercent      int64
	readRequests     int64
	writeRequests    int64
	readBytes        int64
	writeBytes       int64
	averageTime      int64
	ssdLifeRemaining int64  // -1 if not SSD
	ssdPowerOnDays   int64  // -1 if not SSD
	hardwareStatus   string // Hardware status from SYSDISKSTAT
	diskModel        string // Disk model from SYSDISKSTAT
	serialNumber     string // Serial number from SYSDISKSTAT
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
}

// messageQueueMetrics holds metrics for an individual message queue
type messageQueueMetrics struct {
	name         string
	library      string
	messageCount int64
	oldestHours  int64
}

// tempStorageMetrics holds metrics for a named temporary storage bucket
type tempStorageMetrics struct {
	name        string
	currentSize int64
	peakSize    int64
}

// activeJobMetrics holds metrics for an individual active job
type activeJobMetrics struct {
	jobName                 string
	jobStatus               string
	subsystem               string
	jobType                 string
	runPriority             int64
	elapsedCPUTime          int64 // seconds
	elapsedTime             int64 // seconds
	temporaryStorage        int64 // MB
	cpuPercentage           int64 // percentage * precision
	interactiveTransactions int64
	diskIO                  int64
	threadCount             int64
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
}

// Per-instance metric methods
func (a *Collector) getDiskMetrics(unit string) *diskMetrics {
	if _, ok := a.disks[unit]; !ok {
		a.disks[unit] = &diskMetrics{
			unit:             unit,
			typeField:        "UNKNOWN", // Default for disk type when not specified
			ssdLifeRemaining: -1,        // Default to -1 (not SSD)
			ssdPowerOnDays:   -1,        // Default to -1 (not SSD)
			hardwareStatus:   "UNKNOWN", // Default for hardware status
			diskModel:        "UNKNOWN", // Default for disk model
			serialNumber:     "UNKNOWN", // Default for serial number
		}
	}
	return a.disks[unit]
}

func (a *Collector) getSubsystemMetrics(name string) *subsystemMetrics {
	if _, ok := a.subsystems[name]; !ok {
		a.subsystems[name] = &subsystemMetrics{name: name}
	}
	return a.subsystems[name]
}

func (a *Collector) getJobQueueMetrics(key string) *jobQueueMetrics {
	if _, ok := a.jobQueues[key]; !ok {
		a.jobQueues[key] = &jobQueueMetrics{}
	}
	return a.jobQueues[key]
}

func (a *Collector) getMessageQueueMetrics(key string) *messageQueueMetrics {
	if _, ok := a.messageQueues[key]; !ok {
		a.messageQueues[key] = &messageQueueMetrics{}
	}
	return a.messageQueues[key]
}

func (a *Collector) getTempStorageMetrics(name string) *tempStorageMetrics {
	if _, ok := a.tempStorageNamed[name]; !ok {
		a.tempStorageNamed[name] = &tempStorageMetrics{name: name}
	}
	return a.tempStorageNamed[name]
}

func (a *Collector) getActiveJobMetrics(jobName string) *activeJobMetrics {
	if _, ok := a.activeJobs[jobName]; !ok {
		a.activeJobs[jobName] = &activeJobMetrics{jobName: jobName}
	}
	return a.activeJobs[jobName]
}

func (a *Collector) getNetworkInterfaceMetrics(name string) *networkInterfaceMetrics {
	if _, ok := a.networkInterfaces[name]; !ok {
		a.networkInterfaces[name] = &networkInterfaceMetrics{name: name}
	}
	return a.networkInterfaces[name]
}
