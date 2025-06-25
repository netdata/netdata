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

// Per-instance metric methods
func (a *AS400) getDiskMetrics(unit string) *diskMetrics {
	if _, ok := a.disks[unit]; !ok {
		a.disks[unit] = &diskMetrics{unit: unit}
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
