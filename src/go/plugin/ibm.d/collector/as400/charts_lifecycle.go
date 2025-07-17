// SPDX-License-Identifier: GPL-3.0-or-later

package as400

import (
	"fmt"
	"strings"
)

// Chart lifecycle management

// Disk charts
func (a *AS400) addDiskCharts(disk *diskMetrics) {
	charts := a.newDiskCharts(disk)
	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) addSSDHealthChart(disk *diskMetrics) {
	ssdHealthChart := a.newSSDHealthChart(disk)
	if err := a.Charts().Add(ssdHealthChart); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) addSSDPowerOnChart(disk *diskMetrics) {
	ssdPowerOnChart := a.newSSDPowerOnChart(disk)
	if err := a.Charts().Add(ssdPowerOnChart); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) updateDiskChartLabels(unit string) {
	disk, ok := a.disks[unit]
	if !ok {
		return
	}
	
	cleanUnit := cleanName(unit)
	prefix := fmt.Sprintf("disk_%s_", cleanUnit)
	
	// Update labels on all charts for this disk
	for _, chart := range *a.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			// Update the disk_type label with the new value
			for i, label := range chart.Labels {
				if label.Key == "disk_type" {
					chart.Labels[i].Value = disk.typeField
					break
				}
			}
		}
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
	charts := a.newSubsystemCharts(subsystem)
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
	charts := a.newJobQueueCharts(jobQueue, key)
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

// Temp storage charts
func (a *AS400) addTempStorageCharts(bucket *tempStorageMetrics) {
	charts := a.newTempStorageCharts(bucket)
	if err := a.Charts().Add(*charts...); err != nil {
		a.Warning(err)
	}
}

func (a *AS400) removeTempStorageCharts(bucket *tempStorageMetrics) {
	prefix := fmt.Sprintf("tempstorage_%s_", cleanName(bucket.name))
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

	// Cleanup temp storage buckets
	for name, bucket := range a.tempStorageNamed {
		if !bucket.updated {
			delete(a.tempStorageNamed, name)
			if bucket.hasCharts {
				a.removeTempStorageCharts(bucket)
			}
		} else {
			bucket.updated = false
		}
	}

	// Cleanup message queues (if collecting)
	for key, queue := range a.messageQueues {
		if !queue.updated {
			delete(a.messageQueues, key)
			// Message queue charts not implemented yet
		} else {
			queue.updated = false
		}
	}

	// Cleanup network interfaces
	for name, intf := range a.networkInterfaces {
		if !intf.updated {
			delete(a.networkInterfaces, name)
			if intf.hasCharts {
				a.removeNetworkInterfaceCharts(intf)
			}
		} else {
			intf.updated = false
		}
	}
}
