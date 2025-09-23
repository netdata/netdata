//go:build cgo
// +build cgo

package as400

// SPDX-License-Identifier: GPL-3.0-or-later

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/as400/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
)

// Collector implements the IBM i module on top of the ibm.d framework.
type Collector struct {
	framework.Collector

	Config

	client *as400proto.Client

	// Per-iteration metrics
	mx *metricsData

	// Metadata caches (reset every iteration)
	disks             map[string]*diskMetrics
	subsystems        map[string]*subsystemMetrics
	jobQueues         map[string]*jobQueueMetrics
	messageQueues     map[string]*messageQueueMetrics
	tempStorageNamed  map[string]*tempStorageMetrics
	activeJobs        map[string]*activeJobMetrics
	networkInterfaces map[string]*networkInterfaceMetrics

	// Selectors
	diskSelector      matcher.Matcher
	subsystemSelector matcher.Matcher
	jobQueueSelector  matcher.Matcher

	// System identity
	systemName        string
	serialNumber      string
	model             string
	osVersion         string
	technologyRefresh string
	versionMajor      int
	versionRelease    int
	versionMod        int

	// Feature flags and logging guards
	disabled map[string]bool

	once sync.Once
}

func (c *Collector) initOnce() {
	c.once.Do(func() {
		c.disabled = make(map[string]bool)
		c.mx = &metricsData{}
		c.resetInstanceCaches()
	})
}

func (c *Collector) resetInstanceCaches() {
	c.disks = make(map[string]*diskMetrics)
	c.subsystems = make(map[string]*subsystemMetrics)
	c.jobQueues = make(map[string]*jobQueueMetrics)
	c.messageQueues = make(map[string]*messageQueueMetrics)
	c.tempStorageNamed = make(map[string]*tempStorageMetrics)
	c.activeJobs = make(map[string]*activeJobMetrics)
	c.networkInterfaces = make(map[string]*networkInterfaceMetrics)
	c.mx.disks = make(map[string]diskInstanceMetrics)
	c.mx.subsystems = make(map[string]subsystemInstanceMetrics)
	c.mx.jobQueues = make(map[string]jobQueueInstanceMetrics)
	c.mx.tempStorageNamed = make(map[string]tempStorageInstanceMetrics)
	c.mx.activeJobs = make(map[string]activeJobInstanceMetrics)
	c.mx.networkInterfaces = make(map[string]networkInterfaceInstanceMetrics)
}

// CollectOnce implements framework.CollectorImpl.
func (c *Collector) CollectOnce() error {
	c.initOnce()

	ctx := context.Background()
	if err := c.client.Connect(ctx); err != nil {
		return err
	}
	if err := c.client.Ping(ctx); err != nil {
		_ = c.client.Close()
		if err := c.client.Connect(ctx); err != nil {
			return err
		}
		if err := c.client.Ping(ctx); err != nil {
			return err
		}
	}

	// Reset state for the iteration
	c.mx = &metricsData{
		disks:             make(map[string]diskInstanceMetrics),
		subsystems:        make(map[string]subsystemInstanceMetrics),
		jobQueues:         make(map[string]jobQueueInstanceMetrics),
		tempStorageNamed:  make(map[string]tempStorageInstanceMetrics),
		activeJobs:        make(map[string]activeJobInstanceMetrics),
		networkInterfaces: make(map[string]networkInterfaceInstanceMetrics),
		systemActivity:    systemActivityMetrics{},
	}
	c.resetInstanceCaches()

	metricsMap, err := c.collect(ctx)
	if err != nil {
		return err
	}

	// Populate contexts from metric struct maps
	c.exportSystemMetrics()
	c.exportDiskMetrics()
	c.exportSubsystemMetrics()
	c.exportJobQueueMetrics()
	c.exportTempStorageMetrics()
	c.exportActiveJobMetrics()
	c.exportNetworkInterfaceMetrics()
	c.exportSystemActivityMetrics()
	c.applyGlobalLabels()

	// Some collectors expect legacy map for compatibility (e.g. tests);
	// ensure we at least touch the metrics map so linter doesn't complain.
	_ = metricsMap

	return nil
}

func (c *Collector) exportSystemMetrics() {
	labels := contexts.EmptyLabels{}

	contexts.System.CPUUtilization.Set(c.State, labels, contexts.SystemCPUUtilizationValues{
		Utilization: c.mx.CPUPercentage,
	})

	contexts.System.CPUDetails.Set(c.State, labels, contexts.SystemCPUDetailsValues{
		Configured: c.mx.ConfiguredCPUs,
	})

	contexts.System.CPUCapacity.Set(c.State, labels, contexts.SystemCPUCapacityValues{
		Capacity: c.mx.CurrentCPUCapacity,
	})

	contexts.System.TotalJobs.Set(c.State, labels, contexts.SystemTotalJobsValues{
		Total: c.mx.TotalJobsInSystem,
	})

	contexts.System.ActiveJobsByType.Set(c.State, labels, contexts.SystemActiveJobsByTypeValues{
		Batch:       c.mx.BatchJobsRunning,
		Interactive: c.mx.InteractiveJobsInSystem,
		Active:      c.mx.ActiveJobsInSystem,
	})

	contexts.System.JobQueueLength.Set(c.State, labels, contexts.SystemJobQueueLengthValues{
		Waiting: c.mx.JobQueueLength,
	})

	contexts.System.MainStorageSize.Set(c.State, labels, contexts.SystemMainStorageSizeValues{
		Total: c.mx.MainStorageSize,
	})

	contexts.System.TemporaryStorage.Set(c.State, labels, contexts.SystemTemporaryStorageValues{
		Current: c.mx.CurrentTemporaryStorage,
		Maximum: c.mx.MaximumTemporaryStorageUsed,
	})

	contexts.System.MemoryPoolUsage.Set(c.State, labels, contexts.SystemMemoryPoolUsageValues{
		Machine:     c.mx.MachinePoolSize,
		Base:        c.mx.BasePoolSize,
		Interactive: c.mx.InteractivePoolSize,
		Spool:       c.mx.SpoolPoolSize,
	})

	contexts.System.MemoryPoolDefined.Set(c.State, labels, contexts.SystemMemoryPoolDefinedValues{
		Machine: c.mx.MachinePoolDefinedSize,
		Base:    c.mx.BasePoolDefinedSize,
	})

	contexts.System.MemoryPoolReserved.Set(c.State, labels, contexts.SystemMemoryPoolReservedValues{
		Machine: c.mx.MachinePoolReservedSize,
		Base:    c.mx.BasePoolReservedSize,
	})

	contexts.System.MemoryPoolThreads.Set(c.State, labels, contexts.SystemMemoryPoolThreadsValues{
		Machine: c.mx.MachinePoolThreads,
		Base:    c.mx.BasePoolThreads,
	})

	contexts.System.MemoryPoolMaxThreads.Set(c.State, labels, contexts.SystemMemoryPoolMaxThreadsValues{
		Machine: c.mx.MachinePoolMaxThreads,
		Base:    c.mx.BasePoolMaxThreads,
	})

	contexts.System.DiskBusyAverage.Set(c.State, labels, contexts.SystemDiskBusyAverageValues{
		Busy: c.mx.DiskBusyPercentage,
	})

	contexts.System.SystemASPUsage.Set(c.State, labels, contexts.SystemSystemASPUsageValues{
		Used: c.mx.SystemASPUsed,
	})

	contexts.System.SystemASPStorage.Set(c.State, labels, contexts.SystemSystemASPStorageValues{
		Total: c.mx.SystemASPStorage,
	})

	contexts.System.TotalAuxiliaryStorage.Set(c.State, labels, contexts.SystemTotalAuxiliaryStorageValues{
		Total: c.mx.TotalAuxiliaryStorage,
	})

	contexts.System.SystemThreads.Set(c.State, labels, contexts.SystemSystemThreadsValues{
		Active:        c.mx.ActiveThreadsInSystem,
		Per_processor: c.mx.ThreadsPerProcessor,
	})

	contexts.System.NetworkConnections.Set(c.State, labels, contexts.SystemNetworkConnectionsValues{
		Remote: c.mx.RemoteConnections,
		Total:  c.mx.TotalConnections,
	})

	contexts.System.NetworkConnectionStates.Set(c.State, labels, contexts.SystemNetworkConnectionStatesValues{
		Listen:     c.mx.ListenConnections,
		Close_wait: c.mx.CloseWaitConnections,
	})

	contexts.System.TempStorageTotal.Set(c.State, labels, contexts.SystemTempStorageTotalValues{
		Current: c.mx.TempStorageCurrentTotal,
		Peak:    c.mx.TempStoragePeakTotal,
	})

	if c.mx.systemActivity.AverageCPURate != 0 || c.mx.systemActivity.AverageCPUUtilization != 0 {
		contexts.System.SystemActivityCPURate.Set(c.State, labels, contexts.SystemSystemActivityCPURateValues{
			Average: c.mx.systemActivity.AverageCPURate,
		})
		contexts.System.SystemActivityCPUUtilization.Set(c.State, labels, contexts.SystemSystemActivityCPUUtilizationValues{
			Average: c.mx.systemActivity.AverageCPUUtilization,
			Minimum: c.mx.systemActivity.MinimumCPUUtilization,
			Maximum: c.mx.systemActivity.MaximumCPUUtilization,
		})
	}
}

func (c *Collector) exportDiskMetrics() {
	for unit, values := range c.mx.disks {
		meta := c.disks[unit]
		diskUnit := unit
		diskType := ""
		diskModel := ""
		hardwareStatus := ""
		diskSerial := ""
		if meta != nil {
			if meta.unit != "" {
				diskUnit = meta.unit
			}
			diskType = meta.typeField
			diskModel = meta.diskModel
			hardwareStatus = meta.hardwareStatus
			diskSerial = meta.serialNumber
		}

		labels := contexts.DiskLabels{
			Disk_unit:          diskUnit,
			Disk_type:          diskType,
			Disk_model:         diskModel,
			Hardware_status:    hardwareStatus,
			Disk_serial_number: diskSerial,
		}

		contexts.Disk.Busy.Set(c.State, labels, contexts.DiskBusyValues{
			Busy: values.BusyPercent,
		})

		contexts.Disk.IORequests.Set(c.State, labels, contexts.DiskIORequestsValues{
			Read:  values.ReadRequests,
			Write: values.WriteRequests,
		})

		contexts.Disk.SpaceUsage.Set(c.State, labels, contexts.DiskSpaceUsageValues{
			Used: values.PercentUsed,
		})

		contexts.Disk.Capacity.Set(c.State, labels, contexts.DiskCapacityValues{
			Available: values.AvailableGB,
			Used:      values.UsedGB,
		})

		contexts.Disk.Blocks.Set(c.State, labels, contexts.DiskBlocksValues{
			Read:  values.BlocksRead,
			Write: values.BlocksWritten,
		})

		if values.SSDLifeRemaining > 0 {
			contexts.Disk.SSDHealth.Set(c.State, labels, contexts.DiskSSDHealthValues{
				Life_remaining: values.SSDLifeRemaining,
			})
		}

		if values.SSDPowerOnDays > 0 {
			contexts.Disk.SSDPowerOn.Set(c.State, labels, contexts.DiskSSDPowerOnValues{
				Power_on_days: values.SSDPowerOnDays,
			})
		}
	}
}

func (c *Collector) exportSubsystemMetrics() {
	for name, values := range c.mx.subsystems {
		meta := c.subsystems[name]
		subsystemName := name
		library := ""
		status := "ACTIVE"
		if meta != nil {
			if meta.name != "" {
				subsystemName = meta.name
			}
			library = meta.library
			if meta.status != "" {
				status = meta.status
			}
		}
		labels := contexts.SubsystemLabels{
			Subsystem: subsystemName,
			Library:   library,
			Status:    status,
		}
		contexts.Subsystem.Jobs.Set(c.State, labels, contexts.SubsystemJobsValues{
			Active:  values.CurrentActiveJobs,
			Maximum: values.MaximumActiveJobs,
		})
	}
}

func (c *Collector) exportJobQueueMetrics() {
	for key, values := range c.mx.jobQueues {
		meta := c.jobQueues[key]
		queueName := key
		library := ""
		status := "RELEASED"
		if meta != nil {
			if meta.name != "" {
				queueName = meta.name
			}
			library = meta.library
			if meta.status != "" {
				status = meta.status
			}
		}
		labels := contexts.JobQueueLabels{
			Job_queue: queueName,
			Library:   library,
			Status:    status,
		}
		contexts.JobQueue.Length.Set(c.State, labels, contexts.JobQueueLengthValues{
			Jobs: values.NumberOfJobs,
		})
	}
}

func (c *Collector) exportTempStorageMetrics() {
	for name, values := range c.mx.tempStorageNamed {
		labels := contexts.TempStorageBucketLabels{Bucket: name}
		contexts.TempStorageBucket.Usage.Set(c.State, labels, contexts.TempStorageBucketUsageValues{
			Current: values.CurrentSize,
			Peak:    values.PeakSize,
		})
	}
}

func (c *Collector) exportActiveJobMetrics() {
	for jobName, values := range c.mx.activeJobs {
		meta := c.activeJobs[jobName]
		jobNameLabel := jobName
		jobStatus := ""
		subsystem := ""
		jobType := ""
		if meta != nil {
			if meta.jobName != "" {
				jobNameLabel = meta.jobName
			}
			jobStatus = meta.jobStatus
			subsystem = meta.subsystem
			jobType = meta.jobType
		}
		labels := contexts.ActiveJobLabels{
			Job_name:   jobNameLabel,
			Job_status: jobStatus,
			Subsystem:  subsystem,
			Job_type:   jobType,
		}

		contexts.ActiveJob.CPU.Set(c.State, labels, contexts.ActiveJobCPUValues{
			Cpu: values.CPUPercentage,
		})

		contexts.ActiveJob.Resources.Set(c.State, labels, contexts.ActiveJobResourcesValues{
			Temp_storage: values.TemporaryStorage,
		})

		contexts.ActiveJob.Time.Set(c.State, labels, contexts.ActiveJobTimeValues{
			Cpu_time:   values.ElapsedCPUTime,
			Total_time: values.ElapsedTime,
		})

		contexts.ActiveJob.Activity.Set(c.State, labels, contexts.ActiveJobActivityValues{
			Disk_io:                  values.ElapsedDiskIO,
			Interactive_transactions: values.ElapsedInteractiveTransactions,
		})

		contexts.ActiveJob.Threads.Set(c.State, labels, contexts.ActiveJobThreadsValues{
			Threads: values.ThreadCount,
		})
	}
}

func (c *Collector) exportNetworkInterfaceMetrics() {
	for name, values := range c.mx.networkInterfaces {
		meta := c.networkInterfaces[name]
		interfaceType := ""
		connectionType := ""
		internetAddr := ""
		networkAddr := ""
		subnetMask := ""
		if meta != nil {
			interfaceType = meta.interfaceType
			connectionType = meta.connectionType
			internetAddr = meta.internetAddress
			networkAddr = meta.networkAddress
			subnetMask = meta.subnetMask
		}
		labels := contexts.NetworkInterfaceLabels{
			Interface:        name,
			Interface_type:   interfaceType,
			Connection_type:  connectionType,
			Internet_address: internetAddr,
			Network_address:  networkAddr,
			Subnet_mask:      subnetMask,
		}

		contexts.NetworkInterface.Status.Set(c.State, labels, contexts.NetworkInterfaceStatusValues{
			Active: values.InterfaceStatus,
		})

		contexts.NetworkInterface.MTU.Set(c.State, labels, contexts.NetworkInterfaceMTUValues{
			Mtu: values.MTU,
		})
	}
}

func (c *Collector) exportSystemActivityMetrics() {
	if c.mx.systemActivity.AverageCPURate == 0 && c.mx.systemActivity.AverageCPUUtilization == 0 {
		return
	}
	labels := contexts.EmptyLabels{}
	contexts.System.SystemActivityCPURate.Set(c.State, labels, contexts.SystemSystemActivityCPURateValues{
		Average: c.mx.systemActivity.AverageCPURate,
	})
	contexts.System.SystemActivityCPUUtilization.Set(c.State, labels, contexts.SystemSystemActivityCPUUtilizationValues{
		Average: c.mx.systemActivity.AverageCPUUtilization,
		Minimum: c.mx.systemActivity.MinimumCPUUtilization,
		Maximum: c.mx.systemActivity.MaximumCPUUtilization,
	})
}

func (c *Collector) verifyConfig() error {
	if strings.TrimSpace(c.DSN) == "" {
		return errors.New("dsn is required but not set")
	}
	return nil
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			c.Errorf("cleanup: error closing database: %v", err)
		}
	}
	c.Collector.Cleanup(ctx)
}

func (c *Collector) buildDSNIfNeeded() error {
	if strings.TrimSpace(c.DSN) != "" {
		return nil
	}

	if c.Hostname == "" || c.Username == "" || c.Password == "" {
		return fmt.Errorf("dsn required but not set, and insufficient connection parameters provided")
	}

	cfg := &dbdriver.ConnectionConfig{
		Hostname:   c.Hostname,
		Port:       c.Port,
		Username:   c.Username,
		Password:   c.Password,
		Database:   c.Database,
		SystemType: "AS400",
		ODBCDriver: c.ODBCDriver,
		UseSSL:     c.UseSSL,
	}

	c.DSN = dbdriver.BuildODBCDSN(cfg)
	return nil
}
