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
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/as400/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
)

// Collector implements the IBM i module on top of the ibm.d framework.
type Collector struct {
	framework.Collector

	Config `yaml:",inline" json:",inline"`

	client *as400proto.Client

	// Per-iteration metrics
	mx *metricsData

	fastQueryLatencyCounters map[string]int64
	batchLatencyValues       contexts.ObservabilityQueryLatencyBatchValues
	batchLatencyValid        bool

	// Metadata caches (reset every iteration)
	disks             map[string]*diskMetrics
	subsystems        map[string]*subsystemMetrics
	jobQueues         map[string]*jobQueueMetrics
	messageQueues     map[string]*messageQueueMetrics
	outputQueues      map[string]*outputQueueMetrics
	tempStorageNamed  map[string]*tempStorageMetrics
	activeJobs        map[string]*activeJobMetrics
	networkInterfaces map[string]*networkInterfaceMetrics
	httpServers       map[string]*httpServerMetrics
	planCache         map[string]*planCacheMetrics

	// Selectors
	diskSelector      matcher.Matcher
	subsystemSelector matcher.Matcher

	slow struct {
		client *as400proto.Client
		cancel context.CancelFunc
		wg     sync.WaitGroup
		config slowPathConfig
		cache  slowCache
	}

	batch struct {
		client *as400proto.Client
		cancel context.CancelFunc
		wg     sync.WaitGroup
		config batchPathConfig
		cache  batchCache
	}

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
	disabled    map[string]bool
	errorLogged map[string]bool
	muErrorLog  sync.Mutex

	// Cardinality guards to avoid repeated expensive counts
	diskCardinality              cardinalityGuard
	networkInterfacesCardinality cardinalityGuard
	httpServersCardinality       cardinalityGuard

	dump   *dumpContext
	groups []collectionGroup

	messageQueueTargets []queueTarget
	jobQueueTargets     []queueTarget
	outputQueueTargets  []queueTarget
	activeJobTargets    []activeJobTarget

	// CPU collection state for delta-based calculation
	cpuCollectionMethod   string // "total_cpu_time" or "elapsed_cpu_used"
	prevTotalCPUTime      int64  // Previous TOTAL_CPU_TIME value (nanoseconds)
	prevElapsedTime       int64  // Previous ELAPSED_TIME value (seconds)
	prevElapsedCPUProduct int64  // Previous ELAPSED_CPU_USED * ELAPSED_TIME product
	hasCPUBaseline        bool   // Whether we have a previous measurement

	once sync.Once
}

func (c *Collector) initOnce() {
	c.once.Do(func() {
		c.disabled = make(map[string]bool)
		c.errorLogged = make(map[string]bool)
		c.mx = &metricsData{}
		c.resetInstanceCaches()
		c.initGroups()
	})
}

func (c *Collector) resetInstanceCaches() {
	c.disks = make(map[string]*diskMetrics)
	c.subsystems = make(map[string]*subsystemMetrics)
	c.jobQueues = make(map[string]*jobQueueMetrics)
	c.messageQueues = make(map[string]*messageQueueMetrics)
	c.outputQueues = make(map[string]*outputQueueMetrics)
	c.tempStorageNamed = make(map[string]*tempStorageMetrics)
	c.activeJobs = make(map[string]*activeJobMetrics)
	c.networkInterfaces = make(map[string]*networkInterfaceMetrics)
	c.httpServers = make(map[string]*httpServerMetrics)
	c.planCache = make(map[string]*planCacheMetrics)
	c.mx.disks = make(map[string]diskInstanceMetrics)
	c.mx.subsystems = make(map[string]subsystemInstanceMetrics)
	c.mx.jobQueues = make(map[string]jobQueueInstanceMetrics)
	c.mx.messageQueues = make(map[string]messageQueueInstanceMetrics)
	c.mx.outputQueues = make(map[string]outputQueueInstanceMetrics)
	c.mx.tempStorageNamed = make(map[string]tempStorageInstanceMetrics)
	c.mx.activeJobs = make(map[string]activeJobInstanceMetrics)
	c.mx.networkInterfaces = make(map[string]networkInterfaceInstanceMetrics)
	c.mx.httpServers = make(map[string]httpServerInstanceMetrics)
	c.mx.planCache = make(map[string]planCacheInstanceMetrics)
}

func (c *Collector) prepareIterationState() {
	c.mx = &metricsData{
		systemActivity: systemActivityMetrics{},
	}
	c.resetInstanceCaches()
	c.diskCardinality.Configure(c.MaxDisks)
	c.networkInterfacesCardinality.Configure(networkInterfaceLimit)
	c.httpServersCardinality.Configure(httpServerLimit)
}

func (c *Collector) initGroups() {
	if c.groups != nil {
		return
	}
	c.groups = []collectionGroup{
		&systemGroup{c},
		&diskGroup{c},
		&subsystemGroup{c},
		&jobQueueGroup{c},
		&messageQueueGroup{c},
		&outputQueueGroup{c},
		&activeJobGroup{c},
		&networkInterfaceGroup{c},
		&systemActivityGroup{c},
		&httpServerGroup{c},
		&planCacheGroup{c},
	}
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

	if err := c.collect(ctx); err != nil {
		return err
	}
	if c.dump != nil {
		c.dump.recordMetrics(c.snapshotMetrics())
	}

	// Populate contexts from metric struct maps
	c.exportSystemMetrics()
	c.exportDiskMetrics()
	c.exportSubsystemMetrics()
	c.exportJobQueueMetrics()
	c.exportMessageQueueMetrics()
	c.exportOutputQueueMetrics()
	c.exportQueueTotalsMetrics()
	c.exportTempStorageMetrics()
	c.exportActiveJobMetrics()
	c.exportNetworkInterfaceMetrics()
	c.exportSystemActivityMetrics()
	c.exportHTTPServerMetrics()
	c.exportPlanCacheMetrics()
	c.exportQueryLatencyMetrics()
	c.applyGlobalLabels()

	return nil
}

func (c *Collector) snapshotMetrics() map[string]int64 {
	metrics := make(map[string]int64)

	for k, v := range stm.ToMap(c.mx) {
		if isSystemMetric(k) {
			metrics[k] = v
		}
	}

	for unit, values := range c.mx.disks {
		clean := cleanName(unit)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("disk_%s_%s", clean, k)] = v
		}
		if values.SSDLifeRemaining >= 0 {
			metrics[fmt.Sprintf("disk_%s_ssd_life_remaining", clean)] = values.SSDLifeRemaining
		}
		if values.SSDPowerOnDays >= 0 {
			metrics[fmt.Sprintf("disk_%s_ssd_power_on_days", clean)] = values.SSDPowerOnDays
		}
	}

	for name, values := range c.mx.subsystems {
		clean := cleanName(name)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("subsystem_%s_%s", clean, k)] = v
		}
	}

	for name, values := range c.mx.jobQueues {
		clean := cleanName(name)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("jobqueue_%s_%s", clean, k)] = v
		}
	}

	for key, values := range c.mx.messageQueues {
		clean := cleanName(key)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("message_queue_%s_%s", clean, k)] = v
		}
	}

	for key, values := range c.mx.outputQueues {
		clean := cleanName(key)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("output_queue_%s_%s", clean, k)] = v
		}
	}

	for bucket, values := range c.mx.tempStorageNamed {
		clean := cleanName(bucket)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("tempstorage_%s_%s", clean, k)] = v
		}
	}

	for jobName, values := range c.mx.activeJobs {
		clean := cleanName(jobName)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("activejob_%s_%s", clean, k)] = v
		}
	}

	for name, values := range c.mx.networkInterfaces {
		clean := cleanName(name)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("netintf_%s_%s", clean, k)] = v
		}
	}

	for id, values := range c.mx.httpServers {
		clean := cleanName(id)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("httpserver_%s_%s", clean, k)] = v
		}
	}

	for heading, values := range c.mx.planCache {
		clean := cleanName(heading)
		for k, v := range stm.ToMap(values) {
			metrics[fmt.Sprintf("plan_cache_%s_%s", clean, k)] = v
		}
	}

	for k, v := range stm.ToMap(c.mx.systemActivity) {
		metrics[fmt.Sprintf("system_activity_%s", k)] = v
	}

	return metrics
}

func isSystemMetric(key string) bool {
	systemMetrics := map[string]bool{
		"cpu_percentage":                          true,
		"configured_cpus":                         true,
		"current_cpu_capacity":                    true,
		"main_storage_size":                       true,
		"current_temporary_storage":               true,
		"maximum_temporary_storage_used":          true,
		"total_jobs_in_system":                    true,
		"active_jobs_in_system":                   true,
		"interactive_jobs_in_system":              true,
		"batch_jobs_running":                      true,
		"job_queue_length":                        true,
		"system_asp_used":                         true,
		"system_asp_storage":                      true,
		"total_auxiliary_storage":                 true,
		"active_threads_in_system":                true,
		"threads_per_processor":                   true,
		"machine_pool_size":                       true,
		"base_pool_size":                          true,
		"interactive_pool_size":                   true,
		"spool_pool_size":                         true,
		"machine_pool_defined_size":               true,
		"machine_pool_reserved_size":              true,
		"base_pool_defined_size":                  true,
		"base_pool_reserved_size":                 true,
		"machine_pool_threads":                    true,
		"machine_pool_max_threads":                true,
		"base_pool_threads":                       true,
		"base_pool_max_threads":                   true,
		"remote_connections":                      true,
		"total_connections":                       true,
		"listen_connections":                      true,
		"closewait_connections":                   true,
		"temp_storage_current_total":              true,
		"temp_storage_peak_total":                 true,
		"disk_busy_percentage":                    true,
		"system_activity_average_cpu_rate":        true,
		"system_activity_average_cpu_utilization": true,
		"system_activity_minimum_cpu_utilization": true,
		"system_activity_maximum_cpu_utilization": true,
	}

	return systemMetrics[key]
}

func (c *Collector) exportSystemMetrics() {
	labels := contexts.EmptyLabels{}

	contexts.System.CPUUtilization.Set(c.State, labels, contexts.SystemCPUUtilizationValues{
		Utilization: c.mx.CPUPercentage,
	})

	contexts.System.CPUEntitledUtilization.Set(c.State, labels, contexts.SystemCPUEntitledUtilizationValues{
		Utilization: c.mx.EntitledCPUPercentage,
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

	avgDiskBusy := c.mx.DiskBusyPercentage
	if avgDiskBusy == 0 {
		var (
			sum   int64
			count int64
		)
		for _, values := range c.mx.disks {
			sum += values.BusyPercent
			count++
		}
		if count > 0 {
			avgDiskBusy = sum / count
		}
	}

	if avgDiskBusy != c.mx.DiskBusyPercentage {
		c.mx.DiskBusyPercentage = avgDiskBusy
	}

	contexts.System.DiskBusyAverage.Set(c.State, labels, contexts.SystemDiskBusyAverageValues{
		Busy: avgDiskBusy,
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
		if interval := c.slowPathIntervalSeconds(); interval > 0 {
			contexts.Subsystem.Jobs.SetUpdateEvery(c.State, labels, interval)
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
		if interval := c.slowPathIntervalSeconds(); interval > 0 {
			contexts.JobQueue.Length.SetUpdateEvery(c.State, labels, interval)
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

func (c *Collector) exportMessageQueueMetrics() {
	for key, values := range c.mx.messageQueues {
		meta := c.messageQueues[key]
		library := ""
		queue := key
		if meta != nil {
			library = meta.library
			if meta.name != "" {
				queue = meta.name
			}
		}
		labels := contexts.MessageQueueLabels{
			Library: library,
			Queue:   queue,
		}
		if interval := c.slowPathIntervalSeconds(); interval > 0 {
			contexts.MessageQueue.Messages.SetUpdateEvery(c.State, labels, interval)
			contexts.MessageQueue.Severity.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.MessageQueue.Messages.Set(c.State, labels, contexts.MessageQueueMessagesValues{
			Total:         values.Total,
			Informational: values.Informational,
			Inquiry:       values.Inquiry,
			Diagnostic:    values.Diagnostic,
			Escape:        values.Escape,
			Notify:        values.Notify,
			Sender_copy:   values.SenderCopy,
		})
		contexts.MessageQueue.Severity.Set(c.State, labels, contexts.MessageQueueSeverityValues{
			Max: values.MaxSeverity,
		})
	}
}

func (c *Collector) exportOutputQueueMetrics() {
	for key, values := range c.mx.outputQueues {
		meta := c.outputQueues[key]
		library := ""
		queue := key
		status := ""
		if meta != nil {
			library = meta.library
			status = meta.status
			if meta.name != "" {
				queue = meta.name
			}
		}
		labels := contexts.OutputQueueLabels{
			Library: library,
			Queue:   queue,
			Status:  status,
		}
		if interval := c.slowPathIntervalSeconds(); interval > 0 {
			contexts.OutputQueue.Files.SetUpdateEvery(c.State, labels, interval)
			contexts.OutputQueue.Writers.SetUpdateEvery(c.State, labels, interval)
			contexts.OutputQueue.Status.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.OutputQueue.Files.Set(c.State, labels, contexts.OutputQueueFilesValues{
			Files: values.Files,
		})
		contexts.OutputQueue.Writers.Set(c.State, labels, contexts.OutputQueueWritersValues{
			Writers: values.Writers,
		})
		contexts.OutputQueue.Status.Set(c.State, labels, contexts.OutputQueueStatusValues{
			Released: values.Released,
		})
	}
}

func (c *Collector) exportQueueTotalsMetrics() {
	if !c.batchPathActive() {
		return
	}

	snapshot := c.batch.cache.getTotals()
	if snapshot.timestamp.IsZero() && len(snapshot.queues) == 0 && len(snapshot.items) == 0 && snapshot.err == nil {
		return
	}

	interval := c.batchPathIntervalSeconds()
	types := []struct {
		queueType string
		itemType  string
		enabled   bool
	}{
		{"message_queue", "message", c.CollectMessageQueueTotals.IsEnabled()},
		{"job_queue", "job", c.CollectJobQueueTotals.IsEnabled()},
		{"output_queue", "spooled_file", c.CollectOutputQueueTotals.IsEnabled()},
	}

	for _, entry := range types {
		if !entry.enabled {
			continue
		}

		labels := contexts.QueueOverviewLabels{
			Queue_type: entry.queueType,
			Item_type:  entry.itemType,
		}
		if interval > 0 {
			contexts.QueueOverview.Count.SetUpdateEvery(c.State, labels, interval)
			contexts.QueueOverview.Items.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.QueueOverview.Count.Set(c.State, labels, contexts.QueueOverviewCountValues{
			Queues: snapshot.queues[entry.queueType],
		})
		contexts.QueueOverview.Items.Set(c.State, labels, contexts.QueueOverviewItemsValues{
			Items: snapshot.items[entry.queueType],
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
			if meta.qualifiedName != "" {
				jobNameLabel = meta.qualifiedName
			} else if meta.jobNumber != "" && meta.jobUser != "" && meta.jobName != "" {
				jobNameLabel = fmt.Sprintf("%s/%s/%s", meta.jobNumber, meta.jobUser, meta.jobName)
			} else if meta.jobName != "" {
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

func (c *Collector) exportHTTPServerMetrics() {
	for key, values := range c.mx.httpServers {
		meta := c.httpServers[key]
		server := ""
		function := ""
		if meta != nil {
			server = meta.serverName
			function = meta.httpFunction
		}
		labels := contexts.HTTPServerLabels{
			Server:   server,
			Function: function,
		}
		contexts.HTTPServer.Connections.Set(c.State, labels, contexts.HTTPServerConnectionsValues{
			Normal: values.NormalConnections,
			Ssl:    values.SSLConnections,
		})
		contexts.HTTPServer.Threads.Set(c.State, labels, contexts.HTTPServerThreadsValues{
			Active: values.ActiveThreads,
			Idle:   values.IdleThreads,
		})
		contexts.HTTPServer.Requests.Set(c.State, labels, contexts.HTTPServerRequestsValues{
			Requests:  values.TotalRequests,
			Responses: values.TotalResponses,
			Rejected:  values.TotalRequestsRejected,
		})
		contexts.HTTPServer.Bytes.Set(c.State, labels, contexts.HTTPServerBytesValues{
			Received: values.BytesReceived,
			Sent:     values.BytesSent,
		})
	}
}

func (c *Collector) exportPlanCacheMetrics() {
	for key, values := range c.mx.planCache {
		meta := c.planCache[key]
		metricLabel := key
		if meta != nil && meta.heading != "" {
			metricLabel = meta.heading
		}
		labels := contexts.PlanCacheLabels{Metric: metricLabel}
		if interval := c.slowPathIntervalSeconds(); interval > 0 {
			contexts.PlanCache.Summary.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.PlanCache.Summary.Set(c.State, labels, contexts.PlanCacheSummaryValues{
			Value: values.Value,
		})
	}
}

func (c *Collector) logUnknownQueryLatency(path, name string) {
	c.logOnce("unknown_query_latency_"+path+"_"+name, "%s path query latency not mapped to chart dimension: %s", path, name)
}

func (c *Collector) splitLatencyCounters(counters map[string]int64) (
	contexts.ObservabilityQueryLatencyFastValues,
	contexts.ObservabilityQueryLatencySlowValues,
	contexts.ObservabilityQueryLatencyBatchValues,
	bool, bool, bool,
) {
	var (
		fast  contexts.ObservabilityQueryLatencyFastValues
		slow  contexts.ObservabilityQueryLatencySlowValues
		batch contexts.ObservabilityQueryLatencyBatchValues
	)

	if len(counters) == 0 {
		return fast, slow, batch, false, false, false
	}

	fastDirect := map[string]*int64{
		"count_disks":                  &fast.Count_disks,
		"count_http_servers":           &fast.Count_http_servers,
		"count_network_interfaces":     &fast.Count_network_interfaces,
		"detect_ibmi_version_primary":  &fast.Detect_ibmi_version_primary,
		"detect_ibmi_version_fallback": &fast.Detect_ibmi_version_fallback,
		"disk_instances":               &fast.Disk_instances,
		"disk_instances_enhanced":      &fast.Disk_instances_enhanced,
		"disk_status":                  &fast.Disk_status,
		"http_server_info":             &fast.Http_server_info,
		"job_info":                     &fast.Job_info,
		"memory_pools":                 &fast.Memory_pools,
		"network_connections":          &fast.Network_connections,
		"network_interfaces":           &fast.Network_interfaces,
		"serial_number":                &fast.Serial_number,
		"system_activity":              &fast.System_activity,
		"system_model":                 &fast.System_model,
		"system_status":                &fast.System_status,
		"system_name_metric":           &fast.System_name,
		"temp_storage_named":           &fast.Temp_storage_named,
		"temp_storage_total":           &fast.Temp_storage_total,
		"technology_refresh_level":     &fast.Technology_refresh_level,
	}

	slowDirect := map[string]*int64{
		"analyze_plan_cache": &slow.Analyze_plan_cache,
		"count_subsystems":   &slow.Count_subsystems,
		"subsystems":         &slow.Subsystems,
		"plan_cache_summary": &slow.Plan_cache_summary,
	}

	var (
		fastSet  bool
		slowSet  bool
		batchSet bool
	)

	for name, total := range counters {
		if total <= 0 {
			continue
		}

		if target, ok := fastDirect[name]; ok {
			*target += total
			fastSet = true
			continue
		}

		if target, ok := slowDirect[name]; ok {
			*target += total
			slowSet = true
			continue
		}

		switch {
		case name == queryNameMessageQueueTotals:
			batch.Message_queue_totals += total
			batchSet = true
		case name == queryNameJobQueueTotals:
			batch.Job_queue_totals += total
			batchSet = true
		case name == queryNameOutputQueueTotals:
			batch.Output_queue_totals += total
			batchSet = true
		case strings.HasPrefix(name, "message_queue_"):
			slow.Message_queue_aggregates += total
			slowSet = true
		case strings.HasPrefix(name, "job_queue_"):
			slow.Job_queues += total
			slowSet = true
		case strings.HasPrefix(name, "output_queue_"):
			slow.Output_queue_info += total
			slowSet = true
		case strings.HasPrefix(name, "active_job_"):
			fast.Active_job += total
			fastSet = true
		default:
			c.logOnce("unknown_query_latency_"+name, "query latency not mapped to chart dimension: %s", name)
		}
	}

	return fast, slow, batch, fastSet, slowSet, batchSet
}

func (c *Collector) exportQueryLatencyMetrics() {
	fastValues, slowFallback, batchFallback, fastHasData, slowFallbackHasData, batchFallbackHasData := c.splitLatencyCounters(c.fastQueryLatencyCounters)

	if fastHasData {
		labels := contexts.EmptyLabels{}
		if interval := c.fastPathIntervalSeconds(); interval > 0 {
			contexts.Observability.QueryLatencyFast.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.Observability.QueryLatencyFast.Set(c.State, labels, fastValues)
	}

	var slowSet bool
	if c.slowPathActive() {
		if counters, _ := c.slow.cache.getLatencies(); len(counters) > 0 {
			_, slowValues, _, _, slowHasData, _ := c.splitLatencyCounters(counters)
			if slowHasData {
				labels := contexts.EmptyLabels{}
				if interval := c.slowPathIntervalSeconds(); interval > 0 {
					contexts.Observability.QueryLatencySlow.SetUpdateEvery(c.State, labels, interval)
				}
				contexts.Observability.QueryLatencySlow.Set(c.State, labels, slowValues)
				slowSet = true
			}
		}
	}
	if !slowSet && slowFallbackHasData {
		labels := contexts.EmptyLabels{}
		if interval := c.fastPathIntervalSeconds(); interval > 0 {
			contexts.Observability.QueryLatencySlow.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.Observability.QueryLatencySlow.Set(c.State, labels, slowFallback)
	} else if !slowSet && c.slowPathActive() {
		labels := contexts.EmptyLabels{}
		if interval := c.slowPathIntervalSeconds(); interval > 0 {
			contexts.Observability.QueryLatencySlow.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.Observability.QueryLatencySlow.Set(c.State, labels, contexts.ObservabilityQueryLatencySlowValues{})
	}

	var batchSet bool
	if c.batchPathActive() {
		if counters, _ := c.batch.cache.getLatencies(); len(counters) > 0 {
			_, _, batchValues, _, _, batchHasData := c.splitLatencyCounters(counters)
			if batchHasData {
				c.batchLatencyValues = batchValues
				c.batchLatencyValid = true
				labels := contexts.EmptyLabels{}
				if interval := c.batchPathIntervalSeconds(); interval > 0 {
					contexts.Observability.QueryLatencyBatch.SetUpdateEvery(c.State, labels, interval)
				}
				contexts.Observability.QueryLatencyBatch.Set(c.State, labels, c.batchLatencyValues)
				batchSet = true
			}
		}
	}
	if !batchSet && c.batchLatencyValid {
		labels := contexts.EmptyLabels{}
		if interval := c.batchPathIntervalSeconds(); interval > 0 {
			contexts.Observability.QueryLatencyBatch.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.Observability.QueryLatencyBatch.Set(c.State, labels, c.batchLatencyValues)
		batchSet = true
	}
	if !batchSet && batchFallbackHasData {
		labels := contexts.EmptyLabels{}
		if interval := c.fastPathIntervalSeconds(); interval > 0 {
			contexts.Observability.QueryLatencyBatch.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.Observability.QueryLatencyBatch.Set(c.State, labels, batchFallback)
	} else if c.batchPathActive() && !batchSet {
		labels := contexts.EmptyLabels{}
		if interval := c.batchPathIntervalSeconds(); interval > 0 {
			contexts.Observability.QueryLatencyBatch.SetUpdateEvery(c.State, labels, interval)
		}
		contexts.Observability.QueryLatencyBatch.Set(c.State, labels, contexts.ObservabilityQueryLatencyBatchValues{})
	}
}

func (c *Collector) fastPathIntervalSeconds() int {
	if c == nil {
		return 0
	}
	if c.Collector.Config.UpdateEvery > 0 {
		return c.Collector.Config.UpdateEvery
	}
	if c.Config.UpdateEvery > 0 {
		return c.Config.UpdateEvery
	}
	return 1
}

func (c *Collector) slowPathIntervalSeconds() int {
	if c == nil {
		return 0
	}
	if !c.slowPathActive() {
		return 0
	}
	interval := int(c.slow.config.interval / time.Second)
	if interval < 1 {
		interval = c.fastPathIntervalSeconds()
		if interval < 1 {
			interval = 1
		}
	}
	return interval
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
	c.stopBatchPath()
	c.stopSlowPath()
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			c.Errorf("cleanup: error closing database: %v", err)
		}
	}
	c.Collector.Cleanup(ctx)
}

// EnableDump allows the collector to emit structured dump artifacts when requested.
func (c *Collector) EnableDump(dir string) {
	ctx, err := newDumpContext(dir, &c.Config)
	if err != nil {
		c.Errorf("failed to initialise dump context: %v", err)
		return
	}
	c.dump = ctx
	c.Infof("dump data enabled, writing artifacts to %s", dir)
}

func (c *Collector) buildDSNIfNeeded(ctx context.Context) error {
	if strings.TrimSpace(c.DSN) != "" {
		return nil
	}

	if strings.TrimSpace(c.Hostname) == "" || strings.TrimSpace(c.Username) == "" || c.Password == "" {
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
