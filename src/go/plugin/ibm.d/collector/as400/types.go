// SPDX-License-Identifier: GPL-3.0-or-later

package as400

// Instance metadata structures for chart management
type diskMetrics struct {
	unit          string
	typeField     string
	model         string
	hasCharts     bool
	updated       bool
	busyPercent   int64
	readRequests  int64
	writeRequests int64
	readBytes     int64
	writeBytes    int64
	averageTime   int64
}

type subsystemMetrics struct {
	name        string
	library     string
	status      string
	hasCharts   bool
	updated     bool
	jobsActive  int64
	jobsHeld    int64
	storageUsed int64
	maxJobs     int64
	currentJobs int64
}

type jobQueueMetrics struct {
	name          string
	library       string
	status        string
	hasCharts     bool
	updated       bool
	jobsWaiting   int64
	jobsHeld      int64
	jobsScheduled int64
	priority      int64
}

type messageQueueMetrics struct {
	name         string
	library      string
	hasCharts    bool
	updated      bool
	messageCount int64
	oldestHours  int64
}
