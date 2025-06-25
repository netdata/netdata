// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"fmt"
	"strings"
)

// Chart lifecycle management

// Disk charts
func (a *AS400) addDiskCharts(disk *diskMetrics) {
	charts := newDiskCharts(disk)
	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) removeDiskCharts(disk *diskMetrics) {
	prefix := fmt.Sprintf("disk_%s_", cleanName(disk.unit))
	for _, chart := range *a.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

// Subsystem charts
func (a *AS400) addSubsystemCharts(subsystem *subsystemMetrics) {
	charts := newSubsystemCharts(subsystem)
	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) removeSubsystemCharts(subsystem *subsystemMetrics) {
	prefix := fmt.Sprintf("subsystem_%s_", cleanName(subsystem.name))
	for _, chart := range *a.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

// Job queue charts
func (a *AS400) addJobQueueCharts(jobQueue *jobQueueMetrics, key string) {
	charts := newJobQueueCharts(jobQueue, key)
	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) removeJobQueueCharts(key string) {
	prefix := fmt.Sprintf("jobqueue_%s_", cleanName(key))
	for _, chart := range *a.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

// Cleanup stale instances
func (a *AS400) cleanupStaleInstances() {
	// Cleanup disks
	for unit, disk := range a.disks {
		if !disk.updated {
			delete(a.disks, unit)
			if disk.hasCharts {
				a.removeDiskCharts(disk)
			}
		} else {
			disk.updated = false
		}
	}

	// Cleanup subsystems
	for name, subsystem := range a.subsystems {
		if !subsystem.updated {
			delete(a.subsystems, name)
			if subsystem.hasCharts {
				a.removeSubsystemCharts(subsystem)
			}
		} else {
			subsystem.updated = false
		}
	}

	// Cleanup job queues
	for key, jobQueue := range a.jobQueues {
		if !jobQueue.updated {
			delete(a.jobQueues, key)
			if jobQueue.hasCharts {
				a.removeJobQueueCharts(key)
			}
		} else {
			jobQueue.updated = false
		}
	}
}
