// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioCPUUtil = module.Priority + iota
	prioCPUCoreUtil
	prioCPUInterrupts
	prioCPUDPCs
	prioCPUCoreCState

	prioMemUtil
	prioMemPageFaults
	prioMemSwapUtil
	prioMemSwapOperations
	prioMemSwapPages
	prioMemCache
	prioMemCacheFaults
	prioMemSystemPool

	prioDiskSpaceUsage
	prioDiskBandwidth
	prioDiskOperations
	prioDiskAvgLatency

	prioNICBandwidth
	prioNICPackets
	prioNICErrors
	prioNICDiscards

	prioTCPConnsEstablished
	prioTCPConnsActive
	prioTCPConnsPassive
	prioTCPConnsFailure
	prioTCPConnsReset
	prioTCPSegmentsReceived
	prioTCPSegmentsSent
	prioTCPSegmentsRetransmitted

	prioOSProcesses
	prioOSUsers
	prioOSVisibleMemoryUsage
	prioOSPagingUsage

	prioSystemThreads
	prioSystemUptime

	prioLogonSessions

	prioThermalzoneTemperature

	prioProcessesCPUUtilization
	prioProcessesMemoryUsage
	prioProcessesIOBytes
	prioProcessesIOOperations
	prioProcessesPageFaults
	prioProcessesPageFileBytes
	prioProcessesThreads
	prioProcessesHandles

	prioIISWebsiteTraffic
	prioIISWebsiteFTPFileTransferRate
	prioIISWebsiteActiveConnectionsCount
	prioIISWebsiteRequestsRate
	prioIISWebsiteConnectionAttemptsRate
	prioIISWebsiteUsersCount
	prioIISWebsiteISAPIExtRequestsCount
	prioIISWebsiteISAPIExtRequestsRate
	prioIISWebsiteErrorsRate
	prioIISWebsiteLogonAttemptsRate
	prioIISWebsiteUptime

	// Connections
	prioMSSQLUserConnections

	// Transactions
	prioMSSQLDatabaseTransactions
	prioMSSQLDatabaseActiveTransactions
	prioMSSQLDatabaseWriteTransactions
	prioMSSQLDatabaseBackupRestoreOperations
	prioMSSQLDatabaseLogFlushes
	prioMSSQLDatabaseLogFlushed

	// Size
	prioMSSQLDatabaseDataFileSize

	// SQL activity
	prioMSSQLStatsBatchRequests
	prioMSSQLStatsCompilations
	prioMSSQLStatsRecompilations
	prioMSSQLStatsAutoParameterization
	prioMSSQLStatsSafeAutoParameterization

	// Processes
	prioMSSQLBlockedProcess

	// Buffer Cache
	prioMSSQLCacheHitRatio
	prioMSSQLBufManIOPS
	prioMSSQLBufferCheckpointPages
	prioMSSQLAccessMethodPageSplits
	prioMSSQLBufferPageLifeExpectancy

	// Memory
	prioMSSQLMemmgrConnectionMemoryBytes
	prioMSSQLMemTotalServer
	prioMSSQLMemmgrExternalBenefitOfMemory
	prioMSSQLMemmgrPendingMemoryGrants

	// Locks
	prioMSSQLLocksLockWait
	prioMSSQLLocksDeadLocks

	// Error
	prioMSSQLSqlErrorsTotal

	// NET Framework
	// Exceptions
	prioNETFrameworkCLRExceptionsThrown
	prioNETFrameworkCLRExceptionsFilters
	prioNETFrameworkCLRExceptionsFinallys
	prioNETFrameworkCLRExceptionsThrowToCatchDepth

	// InterOP
	prioNETFrameworkCLRInteropCOMCallableWrappers
	prioNETFrameworkCLRInteropMarshalling
	prioNETFrameworkCLRInteropStubsCreated
	prioNETFrameworkCLRJITMethods

	// JIT
	prioNETFrameworkCLRJITTime
	prioNETFrameworkCLRJITStandardFailures
	prioNETFrameworkCLRJITILBytes

	// Loading
	prioNETFrameworkCLRLoadingLoaderHeapSize
	prioNETFrameworkCLRLoadingAppDomainsLoaded
	prioNETFrameworkCLRLoadingAppDomainsUnloaded
	prioNETFrameworkCLRLoadingAssembliesLoaded
	prioNETFrameworkCLRLoadingClassesLoaded
	prioNETFrameworkCLRLoadingClassLoadFailure

	// Locks and threads
	prioNETFrameworkCLRLocksAndThreadsQueueLength
	prioNETFrameworkCLRLocksAndThreadsCurrentLogicalThreads
	prioNETFrameworkCLRLocksAndThreadsCurrentPhysicalThreads
	prioNETFrameworkCLRLocksAndThreadsRecognizedThreads
	prioNETFrameworkCLRLocksAndThreadsContentions

	// Memory
	prioNETFrameworkCLRMemoryAllocatedBytes
	prioNETFrameworkCLRMemoryFinalizationSurvivors
	prioNETFrameworkCLRMemoryHeapSize
	prioNETFrameworkCLRMemoryPromoted
	prioNETFrameworkCLRMemoryNumberGCHandles
	prioNETFrameworkCLRMemoryCollections
	prioNETFrameworkCLRMemoryInducedGC
	prioNETFrameworkCLRMemoryNumberPinnedObjects
	prioNETFrameworkCLRMemoryNumberSinkBlocksInUse
	prioNETFrameworkCLRMemoryCommitted
	prioNETFrameworkCLRMemoryReserved
	prioNETFrameworkCLRMemoryGCTime

	// Remoting
	prioNETFrameworkCLRRemotingChannels
	prioNETFrameworkCLRRemotingContextBoundClassesLoaded
	prioNETFrameworkCLRRemotingContextBoundObjects
	prioNETFrameworkCLRRemotingContextProxies
	prioNETFrameworkCLRRemotingContexts
	prioNETFrameworkCLRRemotingRemoteCalls

	// Security
	prioNETFrameworkCLRSecurityLinkTimeChecks
	prioNETFrameworkCLRSecurityRTChecksTime
	prioNETFrameworkCLRSecurityStackWalkDepth
	prioNETFrameworkCLRSecurityRuntimeChecks

	prioServiceState
	prioServiceStatus

	// Database
	prioADDatabaseOperations
	prioADDirectoryOperations
	prioADNameCacheLookups
	prioADCacheHits

	// Replication
	prioADDRAReplicationIntersiteCompressedTraffic
	prioADDRAReplicationIntrasiteCompressedTraffic
	prioADDRAReplicationSyncObjectsRemaining
	prioADDRAReplicationPropertiesUpdated
	prioADDRAReplicationPropertiesFiltered
	prioADDRAReplicationObjectsFiltered
	prioADReplicationPendingSyncs
	prioADDRASyncRequests
	prioADDirectoryServiceThreadsInUse

	// Bind
	prioADLDAPBindTime
	prioADBindsTotal

	// LDAP
	prioADLDAPSearchesTotal

	// Thread Queue
	prioADATQAverageRequestLatency
	prioADATQOutstandingRequests

	// Requests
	prioADCSCertTemplateRequests
	prioADCSCertTemplateRequestProcessingTime
	prioADCSCertTemplateRetrievals
	prioADCSCertTemplateFailedRequests
	prioADCSCertTemplateIssuesRequests
	prioADCSCertTemplatePendingRequests

	// Response
	prioADCSCertTemplateChallengeResponses

	// Retrieval
	prioADCSCertTemplateRetrievalProcessingTime

	// Timing
	prioADCSCertTemplateRequestCryptoSigningTime
	prioADCSCertTemplateRequestPolicyModuleProcessingTime
	prioADCSCertTemplateChallengeResponseProcessingTime
	prioADCSCertTemplateSignedCertificateTimestampLists
	prioADCSCertTemplateSignedCertificateTimestampListProcessingTime

	// ADFS
	// AD
	prioADFSADLoginConnectionFailures

	// DB Artifacts
	prioADFSDBArtifactFailures
	prioADFSDBArtifactQueryTimeSeconds

	// DB Config
	prioADFSDBConfigFailures
	prioADFSDBConfigQueryTimeSeconds

	// Auth
	prioADFSDeviceAuthentications
	prioADFSExternalAuthentications
	prioADFSOauthAuthorizationRequests
	prioADFSCertificateAuthentications
	prioADFSOauthClientAuthentications
	prioADFSPassportAuthentications
	prioADFSSSOAuthentications
	prioADFSUserPasswordAuthentications
	prioADFSWindowsIntegratedAuthentications

	// OAuth
	prioADFSOauthClientCredentials
	prioADFSOauthClientPrivkeyJwtAuthentication
	prioADFSOauthClientSecretBasicAuthentications
	prioADFSOauthClientSecretPostAuthentications
	prioADFSOauthClientWindowsAuthentications
	prioADFSOauthLogonCertificateRequests
	prioADFSOauthPasswordGrantRequests
	prioADFSOauthTokenRequestsSuccess
	prioADFSFederatedAuthentications

	// Requests
	prioADFSFederationMetadataRequests
	prioADFSPassiveRequests
	prioADFSPasswordChangeRequests
	prioADFSSAMLPTokenRequests
	prioADFSWSTrustTokenRequestsSuccess
	prioADFSTokenRequests
	prioADFSWSFedTokenRequestsSuccess

	// Exchange
	// Transport Queue
	prioExchangeTransportQueuesActiveMailboxDelivery
	prioExchangeTransportQueuesExternalActiveRemoteDelivery
	prioExchangeTransportQueuesExternalLargestDelivery
	prioExchangeTransportQueuesInternalActiveRemoteDeliery
	prioExchangeTransportQueuesInternalLargestDelivery
	prioExchangeTransportQueuesRetryMailboxDelivery
	prioExchangeTransportQueuesUnreachable
	prioExchangeTransportQueuesPoison

	// LDAP
	prioExchangeLDAPLongRunningOPS
	prioExchangeLDAPReadTime
	prioExchangeLDAPSearchTime
	prioExchangeLDAPWriteTime
	prioExchangeLDAPTimeoutErrors

	//  OWA
	prioExchangeOWACurrentUniqueUsers
	prioExchangeOWARequestsTotal

	// Sync
	prioExchangeActiveSyncPingCMDsPending
	prioExchangeActiveSyncRequests
	prioExchangeActiveSyncSyncCMDs

	// RPC
	prioExchangeRPCActiveUserCount
	prioExchangeRPCAvgLatency
	prioExchangeRPCConnectionCount
	prioExchangeRPCOperationsTotal
	prioExchangeRPCRequests
	prioExchangeRpcUserCount

	// Workload
	prioExchangeWorkloadActiveTasks
	prioExchangeWorkloadCompleteTasks
	prioExchangeWorkloadQueueTasks
	prioExchangeWorkloadYieldedTasks
	prioExchangeWorkloadActivityStatus

	// HTTP Proxy
	prioExchangeHTTPProxyAVGAuthLatency
	prioExchangeHTTPProxyAVGCASProcessingLatency
	prioExchangeHTTPProxyMailboxProxyFailureRate
	prioExchangeHTTPProxyServerLocatorAvgLatency
	prioExchangeHTTPProxyOutstandingProxyRequests
	prioExchangeHTTPProxyRequestsTotal

	// Request
	prioExchangeAutoDiscoverRequests
	prioExchangeAvailServiceRequests

	// Hyperv Health
	prioHypervVMHealth

	// Hyperv Partition
	prioHypervRootPartitionDeviceSpacePages
	prioHypervRootPartitionGPASpacePages
	prioHypervRootPartitionGPASpaceModifications
	prioHypervRootPartitionAttachedDevices
	prioHypervRootPartitionDepositedPages
	prioHypervRootPartitionSkippedInterrupts
	prioHypervRootPartitionDeviceDMAErrors
	prioHypervRootPartitionDeviceInterruptErrors
	prioHypervRootPartitionDeviceInterruptThrottleEvents
	prioHypervRootPartitionIOTlbFlush
	prioHypervRootPartitionAddressSpace
	prioHypervRootPartitionVirtualTlbFlushEntires
	prioHypervRootPartitionVirtualTlbPages

	// Hyperv VM (Memory)
	prioHypervVMCPUUsage
	prioHypervVMMemoryPhysical
	prioHypervVMMemoryPhysicalGuestVisible
	prioHypervVMMemoryPressureCurrent
	prioHypervVIDPhysicalPagesAllocated
	prioHypervVIDRemotePhysicalPages

	// Hyperv Device
	prioHypervVMDeviceBytes
	prioHypervVMDeviceOperations
	prioHypervVMDeviceErrors

	// Hyperv Interface
	prioHypervVMInterfaceBytes
	prioHypervVMInterfacePacketsDropped
	prioHypervVMInterfacePackets

	// Hyperv Vswitch
	prioHypervVswitchTrafficTotal
	prioHypervVswitchPackets
	prioHypervVswitchDirectedPackets
	prioHypervVswitchBroadcastPackets
	prioHypervVswitchMulticastPackets
	prioHypervVswitchDroppedPackets
	prioHypervVswitchExtensionsDroppedPackets
	prioHypervVswitchPacketsFlooded
	prioHypervVswitchLearnedMACAddresses
	prioHypervVswitchPurgeMACAddress

	prioCollectorDuration
	prioCollectorStatus
)

// CPU
var (
	cpuCharts = module.Charts{
		cpuUtilChart.Copy(),
	}
	cpuUtilChart = module.Chart{
		ID:       "cpu_utilization_total",
		Title:    "Total CPU Utilization (all cores)",
		Units:    "percentage",
		Fam:      "cpu",
		Ctx:      "windows.cpu_utilization_total",
		Type:     module.Stacked,
		Priority: prioCPUUtil,
		Dims: module.Dims{
			{ID: "cpu_idle_time", Name: "idle", Algo: module.PercentOfIncremental, Div: 1000, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "cpu_dpc_time", Name: "dpc", Algo: module.PercentOfIncremental, Div: 1000},
			{ID: "cpu_user_time", Name: "user", Algo: module.PercentOfIncremental, Div: 1000},
			{ID: "cpu_privileged_time", Name: "privileged", Algo: module.PercentOfIncremental, Div: 1000},
			{ID: "cpu_interrupt_time", Name: "interrupt", Algo: module.PercentOfIncremental, Div: 1000},
		},
	}
)

// CPU core
var (
	cpuCoreChartsTmpl = module.Charts{
		cpuCoreUtilChartTmpl.Copy(),
		cpuCoreInterruptsChartTmpl.Copy(),
		cpuDPCsChartTmpl.Copy(),
		cpuCoreCStateChartTmpl.Copy(),
	}
	cpuCoreUtilChartTmpl = module.Chart{
		ID:       "core_%s_cpu_utilization",
		Title:    "Core CPU Utilization",
		Units:    "percentage",
		Fam:      "cpu",
		Ctx:      "windows.cpu_core_utilization",
		Type:     module.Stacked,
		Priority: prioCPUCoreUtil,
		Dims: module.Dims{
			{ID: "cpu_core_%s_idle_time", Name: "idle", Algo: module.PercentOfIncremental, Div: precision, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "cpu_core_%s_dpc_time", Name: "dpc", Algo: module.PercentOfIncremental, Div: precision},
			{ID: "cpu_core_%s_user_time", Name: "user", Algo: module.PercentOfIncremental, Div: precision},
			{ID: "cpu_core_%s_privileged_time", Name: "privileged", Algo: module.PercentOfIncremental, Div: precision},
			{ID: "cpu_core_%s_interrupt_time", Name: "interrupt", Algo: module.PercentOfIncremental, Div: precision},
		},
	}
	cpuCoreInterruptsChartTmpl = module.Chart{
		ID:       "cpu_core_%s_interrupts",
		Title:    "Received and Serviced Hardware Interrupts",
		Units:    "interrupts/s",
		Fam:      "cpu",
		Ctx:      "windows.cpu_core_interrupts",
		Priority: prioCPUInterrupts,
		Dims: module.Dims{
			{ID: "cpu_core_%s_interrupts", Name: "interrupts", Algo: module.Incremental},
		},
	}
	cpuDPCsChartTmpl = module.Chart{
		ID:       "cpu_core_%s_dpcs",
		Title:    "Received and Serviced Deferred Procedure Calls (DPC)",
		Units:    "dpc/s",
		Fam:      "cpu",
		Ctx:      "windows.cpu_core_dpcs",
		Priority: prioCPUDPCs,
		Dims: module.Dims{
			{ID: "cpu_core_%s_dpcs", Name: "dpcs", Algo: module.Incremental},
		},
	}
	cpuCoreCStateChartTmpl = module.Chart{
		ID:       "cpu_core_%s_cpu_cstate",
		Title:    "Core Time Spent in Low-Power Idle State",
		Units:    "percentage",
		Fam:      "cpu",
		Ctx:      "windows.cpu_core_cstate",
		Type:     module.Stacked,
		Priority: prioCPUCoreCState,
		Dims: module.Dims{
			{ID: "cpu_core_%s_cstate_c1", Name: "c1", Algo: module.PercentOfIncremental, Div: precision},
			{ID: "cpu_core_%s_cstate_c2", Name: "c2", Algo: module.PercentOfIncremental, Div: precision},
			{ID: "cpu_core_%s_cstate_c3", Name: "c3", Algo: module.PercentOfIncremental, Div: precision},
		},
	}
)

// Memory
var (
	memCharts = module.Charts{
		memUtilChart.Copy(),
		memPageFaultsChart.Copy(),
		memSwapUtilChart.Copy(),
		memSwapOperationsChart.Copy(),
		memSwapPagesChart.Copy(),
		memCacheChart.Copy(),
		memCacheFaultsChart.Copy(),
		memSystemPoolChart.Copy(),
	}
	memUtilChart = module.Chart{
		ID:       "memory_utilization",
		Title:    "Memory Utilization",
		Units:    "bytes",
		Fam:      "mem",
		Ctx:      "windows.memory_utilization",
		Type:     module.Stacked,
		Priority: prioMemUtil,
		Dims: module.Dims{
			{ID: "memory_available_bytes", Name: "available"},
			{ID: "memory_used_bytes", Name: "used"},
		},
	}
	memPageFaultsChart = module.Chart{
		ID:       "memory_page_faults",
		Title:    "Memory Page Faults",
		Units:    "pgfaults/s",
		Fam:      "mem",
		Ctx:      "windows.memory_page_faults",
		Priority: prioMemPageFaults,
		Dims: module.Dims{
			{ID: "memory_page_faults_total", Name: "page_faults", Algo: module.Incremental},
		},
	}
	memSwapUtilChart = module.Chart{
		ID:       "memory_swap_utilization",
		Title:    "Swap Utilization",
		Units:    "bytes",
		Fam:      "mem",
		Ctx:      "windows.memory_swap_utilization",
		Type:     module.Stacked,
		Priority: prioMemSwapUtil,
		Dims: module.Dims{
			{ID: "memory_not_committed_bytes", Name: "available"},
			{ID: "memory_committed_bytes", Name: "used"},
		},
		Vars: module.Vars{
			{ID: "memory_commit_limit"},
		},
	}
	memSwapOperationsChart = module.Chart{
		ID:       "memory_swap_operations",
		Title:    "Swap Operations",
		Units:    "operations/s",
		Fam:      "mem",
		Ctx:      "windows.memory_swap_operations",
		Type:     module.Area,
		Priority: prioMemSwapOperations,
		Dims: module.Dims{
			{ID: "memory_swap_page_reads_total", Name: "read", Algo: module.Incremental},
			{ID: "memory_swap_page_writes_total", Name: "write", Algo: module.Incremental, Mul: -1},
		},
	}
	memSwapPagesChart = module.Chart{
		ID:       "memory_swap_pages",
		Title:    "Swap Pages",
		Units:    "pages/s",
		Fam:      "mem",
		Ctx:      "windows.memory_swap_pages",
		Priority: prioMemSwapPages,
		Dims: module.Dims{
			{ID: "memory_swap_pages_read_total", Name: "read", Algo: module.Incremental},
			{ID: "memory_swap_pages_written_total", Name: "written", Algo: module.Incremental, Mul: -1},
		},
	}
	memCacheChart = module.Chart{
		ID:       "memory_cached",
		Title:    "Cached",
		Units:    "bytes",
		Fam:      "mem",
		Ctx:      "windows.memory_cached",
		Type:     module.Area,
		Priority: prioMemCache,
		Dims: module.Dims{
			{ID: "memory_cache_total", Name: "cached"},
		},
	}
	memCacheFaultsChart = module.Chart{
		ID:       "memory_cache_faults",
		Title:    "Cache Faults",
		Units:    "faults/s",
		Fam:      "mem",
		Ctx:      "windows.memory_cache_faults",
		Priority: prioMemCacheFaults,
		Dims: module.Dims{
			{ID: "memory_cache_faults_total", Name: "cache_faults", Algo: module.Incremental},
		},
	}
	memSystemPoolChart = module.Chart{
		ID:       "memory_system_pool",
		Title:    "System Memory Pool",
		Units:    "bytes",
		Fam:      "mem",
		Ctx:      "windows.memory_system_pool",
		Type:     module.Stacked,
		Priority: prioMemSystemPool,
		Dims: module.Dims{
			{ID: "memory_pool_paged_bytes", Name: "paged"},
			{ID: "memory_pool_nonpaged_bytes_total", Name: "non-paged"},
		},
	}
)

// Logical Disks
var (
	diskChartsTmpl = module.Charts{
		diskSpaceUsageChartTmpl.Copy(),
		diskBandwidthChartTmpl.Copy(),
		diskOperationsChartTmpl.Copy(),
		diskAvgLatencyChartTmpl.Copy(),
	}
	diskSpaceUsageChartTmpl = module.Chart{
		ID:       "logical_disk_%s_space_usage",
		Title:    "Space usage",
		Units:    "bytes",
		Fam:      "disk",
		Ctx:      "windows.logical_disk_space_usage",
		Type:     module.Stacked,
		Priority: prioDiskSpaceUsage,
		Dims: module.Dims{
			{ID: "logical_disk_%s_free_space", Name: "free"},
			{ID: "logical_disk_%s_used_space", Name: "used"},
		},
	}
	diskBandwidthChartTmpl = module.Chart{
		ID:       "logical_disk_%s_bandwidth",
		Title:    "Bandwidth",
		Units:    "bytes/s",
		Fam:      "disk",
		Ctx:      "windows.logical_disk_bandwidth",
		Type:     module.Area,
		Priority: prioDiskBandwidth,
		Dims: module.Dims{
			{ID: "logical_disk_%s_read_bytes_total", Name: "read", Algo: module.Incremental},
			{ID: "logical_disk_%s_write_bytes_total", Name: "write", Algo: module.Incremental, Mul: -1},
		},
	}
	diskOperationsChartTmpl = module.Chart{
		ID:       "logical_disk_%s_operations",
		Title:    "Operations",
		Units:    "operations/s",
		Fam:      "disk",
		Ctx:      "windows.logical_disk_operations",
		Priority: prioDiskOperations,
		Dims: module.Dims{
			{ID: "logical_disk_%s_reads_total", Name: "reads", Algo: module.Incremental},
			{ID: "logical_disk_%s_writes_total", Name: "writes", Algo: module.Incremental, Mul: -1},
		},
	}
	diskAvgLatencyChartTmpl = module.Chart{
		ID:       "logical_disk_%s_latency",
		Title:    "Average Read/Write Latency",
		Units:    "seconds",
		Fam:      "disk",
		Ctx:      "windows.logical_disk_latency",
		Priority: prioDiskAvgLatency,
		Dims: module.Dims{
			{ID: "logical_disk_%s_read_latency", Name: "read", Algo: module.Incremental, Div: precision},
			{ID: "logical_disk_%s_write_latency", Name: "write", Algo: module.Incremental, Div: precision},
		},
	}
)

// Network interfaces
var (
	nicChartsTmpl = module.Charts{
		nicBandwidthChartTmpl.Copy(),
		nicPacketsChartTmpl.Copy(),
		nicErrorsChartTmpl.Copy(),
		nicDiscardsChartTmpl.Copy(),
	}
	nicBandwidthChartTmpl = module.Chart{
		ID:       "nic_%s_bandwidth",
		Title:    "Bandwidth",
		Units:    "kilobits/s",
		Fam:      "net",
		Ctx:      "windows.net_nic_bandwidth",
		Type:     module.Area,
		Priority: prioNICBandwidth,
		Dims: module.Dims{
			{ID: "net_nic_%s_bytes_received", Name: "received", Algo: module.Incremental, Div: 1000},
			{ID: "net_nic_%s_bytes_sent", Name: "sent", Algo: module.Incremental, Mul: -1, Div: 1000},
		},
	}
	nicPacketsChartTmpl = module.Chart{
		ID:       "nic_%s_packets",
		Title:    "Packets",
		Units:    "packets/s",
		Fam:      "net",
		Ctx:      "windows.net_nic_packets",
		Priority: prioNICPackets,
		Dims: module.Dims{
			{ID: "net_nic_%s_packets_received_total", Name: "received", Algo: module.Incremental},
			{ID: "net_nic_%s_packets_sent_total", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	nicErrorsChartTmpl = module.Chart{
		ID:       "nic_%s_errors",
		Title:    "Errors",
		Units:    "errors/s",
		Fam:      "net",
		Ctx:      "windows.net_nic_errors",
		Priority: prioNICErrors,
		Dims: module.Dims{
			{ID: "net_nic_%s_packets_received_errors", Name: "inbound", Algo: module.Incremental},
			{ID: "net_nic_%s_packets_outbound_errors", Name: "outbound", Algo: module.Incremental, Mul: -1},
		},
	}
	nicDiscardsChartTmpl = module.Chart{
		ID:       "nic_%s_discarded",
		Title:    "Discards",
		Units:    "discards/s",
		Fam:      "net",
		Ctx:      "windows.net_nic_discarded",
		Priority: prioNICDiscards,
		Dims: module.Dims{
			{ID: "net_nic_%s_packets_received_discarded", Name: "inbound", Algo: module.Incremental},
			{ID: "net_nic_%s_packets_outbound_discarded", Name: "outbound", Algo: module.Incremental, Mul: -1},
		},
	}
)

// TCP
var (
	tcpCharts = module.Charts{
		tcpConnsActiveChart.Copy(),
		tcpConnsEstablishedChart.Copy(),
		tcpConnsFailuresChart.Copy(),
		tcpConnsPassiveChart.Copy(),
		tcpConnsResetsChart.Copy(),
		tcpSegmentsReceivedChart.Copy(),
		tcpSegmentsRetransmittedChart.Copy(),
		tcpSegmentsSentChart.Copy(),
	}
	tcpConnsEstablishedChart = module.Chart{
		ID:       "tcp_conns_established",
		Title:    "TCP established connections",
		Units:    "connections",
		Fam:      "tcp",
		Ctx:      "windows.tcp_conns_established",
		Priority: prioTCPConnsEstablished,
		Dims: module.Dims{
			{ID: "tcp_ipv4_conns_established", Name: "ipv4"},
			{ID: "tcp_ipv6_conns_established", Name: "ipv6"},
		},
	}
	tcpConnsActiveChart = module.Chart{
		ID:       "tcp_conns_active",
		Title:    "TCP active connections",
		Units:    "connections/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_conns_active",
		Priority: prioTCPConnsActive,
		Dims: module.Dims{
			{ID: "tcp_ipv4_conns_active", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_conns_active", Name: "ipv6", Algo: module.Incremental},
		},
	}
	tcpConnsPassiveChart = module.Chart{
		ID:       "tcp_conns_passive",
		Title:    "TCP passive connections",
		Units:    "connections/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_conns_passive",
		Priority: prioTCPConnsPassive,
		Dims: module.Dims{
			{ID: "tcp_ipv4_conns_passive", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_conns_passive", Name: "ipv6", Algo: module.Incremental},
		},
	}
	tcpConnsFailuresChart = module.Chart{
		ID:       "tcp_conns_failures",
		Title:    "TCP connection failures",
		Units:    "failures/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_conns_failures",
		Priority: prioTCPConnsFailure,
		Dims: module.Dims{
			{ID: "tcp_ipv4_conns_failures", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_conns_failures", Name: "ipv6", Algo: module.Incremental},
		},
	}
	tcpConnsResetsChart = module.Chart{
		ID:       "tcp_conns_resets",
		Title:    "TCP connections resets",
		Units:    "resets/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_conns_resets",
		Priority: prioTCPConnsReset,
		Dims: module.Dims{
			{ID: "tcp_ipv4_conns_resets", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_conns_resets", Name: "ipv6", Algo: module.Incremental},
		},
	}
	tcpSegmentsReceivedChart = module.Chart{
		ID:       "tcp_segments_received",
		Title:    "Number of TCP segments received",
		Units:    "segments/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_segments_received",
		Priority: prioTCPSegmentsReceived,
		Dims: module.Dims{
			{ID: "tcp_ipv4_segments_received", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_segments_received", Name: "ipv6", Algo: module.Incremental},
		},
	}
	tcpSegmentsSentChart = module.Chart{
		ID:       "tcp_segments_sent",
		Title:    "Number of TCP segments sent",
		Units:    "segments/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_segments_sent",
		Priority: prioTCPSegmentsSent,
		Dims: module.Dims{
			{ID: "tcp_ipv4_segments_sent", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_segments_sent", Name: "ipv6", Algo: module.Incremental},
		},
	}
	tcpSegmentsRetransmittedChart = module.Chart{
		ID:       "tcp_segments_retransmitted",
		Title:    "Number of TCP segments retransmitted",
		Units:    "segments/s",
		Fam:      "tcp",
		Ctx:      "windows.tcp_segments_retransmitted",
		Priority: prioTCPSegmentsRetransmitted,
		Dims: module.Dims{
			{ID: "tcp_ipv4_segments_retransmitted", Name: "ipv4", Algo: module.Incremental},
			{ID: "tcp_ipv6_segments_retransmitted", Name: "ipv6", Algo: module.Incremental},
		},
	}
)

// OS
var (
	osCharts = module.Charts{
		osProcessesChart.Copy(),
		osUsersChart.Copy(),
		osMemoryUsage.Copy(),
		osPagingFilesUsageChart.Copy(),
	}
	osProcessesChart = module.Chart{
		ID:       "os_processes",
		Title:    "Processes",
		Units:    "number",
		Fam:      "os",
		Ctx:      "windows.os_processes",
		Priority: prioOSProcesses,
		Dims: module.Dims{
			{ID: "os_processes", Name: "processes"},
		},
		Vars: module.Vars{
			{ID: "os_processes_limit"},
		},
	}
	osUsersChart = module.Chart{
		ID:       "os_users",
		Title:    "Number of Users",
		Units:    "users",
		Fam:      "os",
		Ctx:      "windows.os_users",
		Priority: prioOSUsers,
		Dims: module.Dims{
			{ID: "os_users", Name: "users"},
		},
	}
	osMemoryUsage = module.Chart{
		ID:       "os_visible_memory_usage",
		Title:    "Visible Memory Usage",
		Units:    "bytes",
		Fam:      "os",
		Ctx:      "windows.os_visible_memory_usage",
		Type:     module.Stacked,
		Priority: prioOSVisibleMemoryUsage,
		Dims: module.Dims{
			{ID: "os_physical_memory_free_bytes", Name: "free"},
			{ID: "os_visible_memory_used_bytes", Name: "used"},
		},
		Vars: module.Vars{
			{ID: "os_visible_memory_bytes"},
		},
	}
	osPagingFilesUsageChart = module.Chart{
		ID:       "os_paging_files_usage",
		Title:    "Paging Files Usage",
		Units:    "bytes",
		Fam:      "os",
		Ctx:      "windows.os_paging_files_usage",
		Type:     module.Stacked,
		Priority: prioOSPagingUsage,
		Dims: module.Dims{
			{ID: "os_paging_free_bytes", Name: "free"},
			{ID: "os_paging_used_bytes", Name: "used"},
		},
		Vars: module.Vars{
			{ID: "os_paging_limit_bytes"},
		},
	}
)

// System
var (
	systemCharts = module.Charts{
		systemThreadsChart.Copy(),
		systemUptimeChart.Copy(),
	}
	systemThreadsChart = module.Chart{
		ID:       "system_threads",
		Title:    "Threads",
		Units:    "number",
		Fam:      "system",
		Ctx:      "windows.system_threads",
		Priority: prioSystemThreads,
		Dims: module.Dims{
			{ID: "system_threads", Name: "threads"},
		},
	}
	systemUptimeChart = module.Chart{
		ID:       "system_uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "system",
		Ctx:      "windows.system_uptime",
		Priority: prioSystemUptime,
		Dims: module.Dims{
			{ID: "system_up_time", Name: "time"},
		},
	}
)

// IIS
var (
	iisWebsiteChartsTmpl = module.Charts{
		iisWebsiteTrafficChartTempl.Copy(),
		iisWebsiteRequestsRateChartTmpl.Copy(),
		iisWebsiteActiveConnectionsCountChartTmpl.Copy(),
		iisWebsiteUsersCountChartTmpl.Copy(),
		iisWebsiteConnectionAttemptsRate.Copy(),
		iisWebsiteISAPIExtRequestsCountChartTmpl.Copy(),
		iisWebsiteISAPIExtRequestsRateChartTmpl.Copy(),
		iisWebsiteFTPFileTransferRateChartTempl.Copy(),
		iisWebsiteLogonAttemptsRateChartTmpl.Copy(),
		iisWebsiteErrorsRateChart.Copy(),
		iisWebsiteUptimeChartTmpl.Copy(),
	}
	iisWebsiteTrafficChartTempl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_traffic",
		Title:      "Website traffic",
		Units:      "bytes/s",
		Fam:        "traffic",
		Ctx:        "iis.website_traffic",
		Type:       module.Area,
		Priority:   prioIISWebsiteTraffic,
		Dims: module.Dims{
			{ID: "iis_website_%s_received_bytes_total", Name: "received", Algo: module.Incremental},
			{ID: "iis_website_%s_sent_bytes_total", Name: "sent", Algo: module.Incremental, Mul: -1},
		},
	}
	iisWebsiteFTPFileTransferRateChartTempl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_ftp_file_transfer_rate",
		Title:      "Website FTP file transfer rate",
		Units:      "files/s",
		Fam:        "traffic",
		Ctx:        "iis.website_ftp_file_transfer_rate",
		Priority:   prioIISWebsiteFTPFileTransferRate,
		Dims: module.Dims{
			{ID: "iis_website_%s_files_received_total", Name: "received", Algo: module.Incremental},
			{ID: "iis_website_%s_files_sent_total", Name: "sent", Algo: module.Incremental},
		},
	}
	iisWebsiteActiveConnectionsCountChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_active_connections_count",
		Title:      "Website active connections",
		Units:      "connections",
		Fam:        "connections",
		Ctx:        "iis.website_active_connections_count",
		Priority:   prioIISWebsiteActiveConnectionsCount,
		Dims: module.Dims{
			{ID: "iis_website_%s_current_connections", Name: "active"},
		},
	}
	iisWebsiteConnectionAttemptsRate = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_connection_attempts_rate",
		Title:      "Website connections attempts",
		Units:      "attempts/s",
		Fam:        "connections",
		Ctx:        "iis.website_connection_attempts_rate",
		Priority:   prioIISWebsiteConnectionAttemptsRate,
		Dims: module.Dims{
			{ID: "iis_website_%s_connection_attempts_all_instances_total", Name: "connection", Algo: module.Incremental},
		},
	}
	iisWebsiteRequestsRateChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_requests_rate",
		Title:      "Website requests rate",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "iis.website_requests_rate",
		Priority:   prioIISWebsiteRequestsRate,
		Dims: module.Dims{
			{ID: "iis_website_%s_requests_total", Name: "requests", Algo: module.Incremental},
		},
	}
	iisWebsiteUsersCountChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_users_count",
		Title:      "Website users with pending requests",
		Units:      "users",
		Fam:        "requests",
		Ctx:        "iis.website_users_count",
		Type:       module.Stacked,
		Priority:   prioIISWebsiteUsersCount,
		Dims: module.Dims{
			{ID: "iis_website_%s_current_anonymous_users", Name: "anonymous"},
			{ID: "iis_website_%s_current_non_anonymous_users", Name: "non_anonymous"},
		},
	}
	iisWebsiteISAPIExtRequestsCountChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_isapi_extension_requests_count",
		Title:      "ISAPI extension requests",
		Units:      "requests",
		Fam:        "requests",
		Ctx:        "iis.website_isapi_extension_requests_count",
		Priority:   prioIISWebsiteISAPIExtRequestsCount,
		Dims: module.Dims{
			{ID: "iis_website_%s_current_isapi_extension_requests", Name: "isapi"},
		},
	}
	iisWebsiteISAPIExtRequestsRateChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_isapi_extension_requests_rate",
		Title:      "Website extensions request",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "iis.website_isapi_extension_requests_rate",
		Priority:   prioIISWebsiteISAPIExtRequestsRate,
		Dims: module.Dims{
			{ID: "iis_website_%s_isapi_extension_requests_total", Name: "isapi", Algo: module.Incremental},
		},
	}
	iisWebsiteErrorsRateChart = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_errors_rate",
		Title:      "Website errors",
		Units:      "errors/s",
		Fam:        "requests",
		Ctx:        "iis.website_errors_rate",
		Type:       module.Stacked,
		Priority:   prioIISWebsiteErrorsRate,
		Dims: module.Dims{
			{ID: "iis_website_%s_locked_errors_total", Name: "document_locked", Algo: module.Incremental},
			{ID: "iis_website_%s_not_found_errors_total", Name: "document_not_found", Algo: module.Incremental},
		},
	}
	iisWebsiteLogonAttemptsRateChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_logon_attempts_rate",
		Title:      "Website logon attempts",
		Units:      "attempts/s",
		Fam:        "logon",
		Ctx:        "iis.website_logon_attempts_rate",
		Priority:   prioIISWebsiteLogonAttemptsRate,
		Dims: module.Dims{
			{ID: "iis_website_%s_logon_attempts_total", Name: "logon", Algo: module.Incremental},
		},
	}
	iisWebsiteUptimeChartTmpl = module.Chart{
		OverModule: "iis",
		ID:         "iis_website_%s_uptime",
		Title:      "Website uptime",
		Units:      "seconds",
		Fam:        "uptime",
		Ctx:        "iis.website_uptime",
		Priority:   prioIISWebsiteUptime,
		Dims: module.Dims{
			{ID: "iis_website_%s_service_uptime", Name: "uptime"},
		},
	}
)

// MS-SQL
var (
	mssqlInstanceChartsTmpl = module.Charts{
		mssqlAccessMethodPageSplitsChart.Copy(),
		mssqlCacheHitRatioChart.Copy(),
		mssqlBufferCheckpointPageChart.Copy(),
		mssqlBufferPageLifeExpectancyChart.Copy(),
		mssqlBufManIOPSChart.Copy(),
		mssqlBlockedProcessChart.Copy(),
		mssqlLocksWaitChart.Copy(),
		mssqlDeadLocksChart.Copy(),
		mssqlMemmgrConnectionMemoryBytesChart.Copy(),
		mssqlMemmgrExternalBenefitOfMemoryChart.Copy(),
		mssqlMemmgrPendingMemoryChart.Copy(),
		mssqlMemmgrTotalServerChart.Copy(),
		mssqlSQLErrorsTotalChart.Copy(),
		mssqlStatsAutoParamChart.Copy(),
		mssqlStatsBatchRequestsChart.Copy(),
		mssqlStatsSafeAutoChart.Copy(),
		mssqlStatsCompilationChart.Copy(),
		mssqlStatsRecompilationChart.Copy(),
		mssqlUserConnectionChart.Copy(),
	}
	mssqlDatabaseChartsTmpl = module.Charts{
		mssqlDatabaseActiveTransactionsChart.Copy(),
		mssqlDatabaseBackupRestoreOperationsChart.Copy(),
		mssqlDatabaseSizeChart.Copy(),
		mssqlDatabaseLogFlushedChart.Copy(),
		mssqlDatabaseLogFlushesChart.Copy(),
		mssqlDatabaseTransactionsChart.Copy(),
		mssqlDatabaseWriteTransactionsChart.Copy(),
	}
	// Access Method:
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-access-methods-object?view=sql-server-ver16
	mssqlAccessMethodPageSplitsChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_accessmethods_page_splits",
		Title:      "Page splits",
		Units:      "splits/s",
		Fam:        "buffer cache",
		Ctx:        "mssql.instance_accessmethods_page_splits",
		Priority:   prioMSSQLAccessMethodPageSplits,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_accessmethods_page_splits", Name: "page", Algo: module.Incremental},
		},
	}
	// Buffer Management
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-buffer-manager-object?view=sql-server-ver16
	mssqlCacheHitRatioChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_cache_hit_ratio",
		Title:      "Buffer Cache hit ratio",
		Units:      "percentage",
		Fam:        "buffer cache",
		Ctx:        "mssql.instance_cache_hit_ratio",
		Priority:   prioMSSQLCacheHitRatio,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_cache_hit_ratio", Name: "hit_ratio"},
		},
	}
	mssqlBufferCheckpointPageChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_bufman_checkpoint_pages",
		Title:      "Flushed pages",
		Units:      "pages/s",
		Fam:        "buffer cache",
		Ctx:        "mssql.instance_bufman_checkpoint_pages",
		Priority:   prioMSSQLBufferCheckpointPages,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_bufman_checkpoint_pages", Name: "flushed", Algo: module.Incremental},
		},
	}
	mssqlBufferPageLifeExpectancyChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_bufman_page_life_expectancy",
		Title:      "Page life expectancy",
		Units:      "seconds",
		Fam:        "buffer cache",
		Ctx:        "mssql.instance_bufman_page_life_expectancy",
		Priority:   prioMSSQLBufferPageLifeExpectancy,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_bufman_page_life_expectancy_seconds", Name: "life_expectancy"},
		},
	}
	mssqlBufManIOPSChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_bufman_iops",
		Title:      "Number of pages input and output",
		Units:      "pages/s",
		Fam:        "buffer cache",
		Ctx:        "mssql.instance_bufman_iops",
		Priority:   prioMSSQLBufManIOPS,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_bufman_page_reads", Name: "read", Algo: module.Incremental},
			{ID: "mssql_instance_%s_bufman_page_writes", Name: "written", Mul: -1, Algo: module.Incremental},
		},
	}
	// General Statistic
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-general-statistics-object?view=sql-server-ver16
	mssqlBlockedProcessChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_blocked_process",
		Title:      "Blocked processes",
		Units:      "process",
		Fam:        "processes",
		Ctx:        "mssql.instance_blocked_processes",
		Priority:   prioMSSQLBlockedProcess,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_genstats_blocked_processes", Name: "blocked"},
		},
	}
	mssqlUserConnectionChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_user_connection",
		Title:      "User connections",
		Units:      "connections",
		Fam:        "connections",
		Ctx:        "mssql.instance_user_connection",
		Priority:   prioMSSQLUserConnections,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_genstats_user_connections", Name: "user"},
		},
	}
	// Lock Wait
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-locks-object?view=sql-server-ver16
	mssqlLocksWaitChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_locks_lock_wait",
		Title:      "Lock requests that required the caller to wait",
		Units:      "locks/s",
		Fam:        "locks",
		Ctx:        "mssql.instance_locks_lock_wait",
		Priority:   prioMSSQLLocksLockWait,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_resource_AllocUnit_locks_lock_wait_seconds", Name: "alloc_unit", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Application_locks_lock_wait_seconds", Name: "application", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Database_locks_lock_wait_seconds", Name: "database", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Extent_locks_lock_wait_seconds", Name: "extent", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_File_locks_lock_wait_seconds", Name: "file", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_HoBT_locks_lock_wait_seconds", Name: "hobt", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Key_locks_lock_wait_seconds", Name: "key", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Metadata_locks_lock_wait_seconds", Name: "metadata", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_OIB_locks_lock_wait_seconds", Name: "oib", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Object_locks_lock_wait_seconds", Name: "object", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Page_locks_lock_wait_seconds", Name: "page", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_RID_locks_lock_wait_seconds", Name: "rid", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_RowGroup_locks_lock_wait_seconds", Name: "row_group", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Xact_locks_lock_wait_seconds", Name: "xact", Algo: module.Incremental},
		},
	}
	mssqlDeadLocksChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_locks_deadlocks",
		Title:      "Lock requests that resulted in deadlock",
		Units:      "locks/s",
		Fam:        "locks",
		Ctx:        "mssql.instance_locks_deadlocks",
		Priority:   prioMSSQLLocksDeadLocks,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_resource_AllocUnit_locks_deadlocks", Name: "alloc_unit", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Application_locks_deadlocks", Name: "application", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Database_locks_deadlocks", Name: "database", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Extent_locks_deadlocks", Name: "extent", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_File_locks_deadlocks", Name: "file", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_HoBT_locks_deadlocks", Name: "hobt", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Key_locks_deadlocks", Name: "key", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Metadata_locks_deadlocks", Name: "metadata", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_OIB_locks_deadlocks", Name: "oib", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Object_locks_deadlocks", Name: "object", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Page_locks_deadlocks", Name: "page", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_RID_locks_deadlocks", Name: "rid", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_RowGroup_locks_deadlocks", Name: "row_group", Algo: module.Incremental},
			{ID: "mssql_instance_%s_resource_Xact_locks_deadlocks", Name: "xact", Algo: module.Incremental},
		},
	}

	// Memory Manager
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-memory-manager-object?view=sql-server-ver16
	mssqlMemmgrConnectionMemoryBytesChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_memmgr_connection_memory_bytes",
		Title:      "Amount of dynamic memory to maintain connections",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "mssql.instance_memmgr_connection_memory_bytes",
		Priority:   prioMSSQLMemmgrConnectionMemoryBytes,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_memmgr_connection_memory_bytes", Name: "memory", Algo: module.Incremental},
		},
	}
	mssqlMemmgrExternalBenefitOfMemoryChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_memmgr_external_benefit_of_memory",
		Title:      "Performance benefit from adding memory to a specific cache",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "mssql.instance_memmgr_external_benefit_of_memory",
		Priority:   prioMSSQLMemmgrExternalBenefitOfMemory,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_memmgr_external_benefit_of_memory", Name: "benefit", Algo: module.Incremental},
		},
	}
	mssqlMemmgrPendingMemoryChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_memmgr_pending_memory_grants",
		Title:      "Process waiting for memory grant",
		Units:      "process",
		Fam:        "memory",
		Ctx:        "mssql.instance_memmgr_pending_memory_grants",
		Priority:   prioMSSQLMemmgrPendingMemoryGrants,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_memmgr_pending_memory_grants", Name: "pending"},
		},
	}
	mssqlMemmgrTotalServerChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_memmgr_server_memory",
		Title:      "Memory committed",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "mssql.instance_memmgr_server_memory",
		Priority:   prioMSSQLMemTotalServer,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_memmgr_total_server_memory_bytes", Name: "memory"},
		},
	}

	// SQL errors
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-sql-errors-object?view=sql-server-ver16
	mssqlSQLErrorsTotalChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_sql_errors_total",
		Title:      "Errors",
		Units:      "errors/s",
		Fam:        "errors",
		Ctx:        "mssql.instance_sql_errors",
		Priority:   prioMSSQLSqlErrorsTotal,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_sql_errors_total_db_offline_errors", Name: "db_offline", Algo: module.Incremental},
			{ID: "mssql_instance_%s_sql_errors_total_info_errors", Name: "info", Algo: module.Incremental},
			{ID: "mssql_instance_%s_sql_errors_total_kill_connection_errors", Name: "kill_connection", Algo: module.Incremental},
			{ID: "mssql_instance_%s_sql_errors_total_user_errors", Name: "user", Algo: module.Incremental},
		},
	}

	// SQL Statistic
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-sql-statistics-object?view=sql-server-ver16
	mssqlStatsAutoParamChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_sqlstats_auto_parameterization_attempts",
		Title:      "Failed auto-parameterization attempts",
		Units:      "attempts/s",
		Fam:        "sql activity",
		Ctx:        "mssql.instance_sqlstats_auto_parameterization_attempts",
		Priority:   prioMSSQLStatsAutoParameterization,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_sqlstats_auto_parameterization_attempts", Name: "failed", Algo: module.Incremental},
		},
	}
	mssqlStatsBatchRequestsChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_sqlstats_batch_requests",
		Title:      "Total of batches requests",
		Units:      "requests/s",
		Fam:        "sql activity",
		Ctx:        "mssql.instance_sqlstats_batch_requests",
		Priority:   prioMSSQLStatsBatchRequests,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_sqlstats_batch_requests", Name: "batch", Algo: module.Incremental},
		},
	}
	mssqlStatsSafeAutoChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_sqlstats_safe_auto_parameterization_attempts",
		Title:      "Safe auto-parameterization attempts",
		Units:      "attempts/s",
		Fam:        "sql activity",
		Ctx:        "mssql.instance_sqlstats_safe_auto_parameterization_attempts",
		Priority:   prioMSSQLStatsSafeAutoParameterization,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_sqlstats_safe_auto_parameterization_attempts", Name: "safe", Algo: module.Incremental},
		},
	}
	mssqlStatsCompilationChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_sqlstats_sql_compilations",
		Title:      "SQL compilations",
		Units:      "compilations/s",
		Fam:        "sql activity",
		Ctx:        "mssql.instance_sqlstats_sql_compilations",
		Priority:   prioMSSQLStatsCompilations,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_sqlstats_sql_compilations", Name: "compilations", Algo: module.Incremental},
		},
	}
	mssqlStatsRecompilationChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_instance_%s_sqlstats_sql_recompilations",
		Title:      "SQL re-compilations",
		Units:      "recompiles/s",
		Fam:        "sql activity",
		Ctx:        "mssql.instance_sqlstats_sql_recompilations",
		Priority:   prioMSSQLStatsRecompilations,
		Dims: module.Dims{
			{ID: "mssql_instance_%s_sqlstats_sql_recompilations", Name: "recompiles", Algo: module.Incremental},
		},
	}

	// Database
	// Source: https://learn.microsoft.com/en-us/sql/relational-databases/performance-monitor/sql-server-databases-object?view=sql-server-2017
	mssqlDatabaseActiveTransactionsChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_active_transactions",
		Title:      "Active transactions per database",
		Units:      "transactions",
		Fam:        "transactions",
		Ctx:        "mssql.database_active_transactions",
		Priority:   prioMSSQLDatabaseActiveTransactions,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_active_transactions", Name: "active"},
		},
	}
	mssqlDatabaseBackupRestoreOperationsChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_backup_restore_operations",
		Title:      "Backup IO per database",
		Units:      "operations/s",
		Fam:        "transactions",
		Ctx:        "mssql.database_backup_restore_operations",
		Priority:   prioMSSQLDatabaseBackupRestoreOperations,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_backup_restore_operations", Name: "backup", Algo: module.Incremental},
		},
	}
	mssqlDatabaseSizeChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_data_files_size",
		Title:      "Current database size",
		Units:      "bytes",
		Fam:        "size",
		Ctx:        "mssql.database_data_files_size",
		Priority:   prioMSSQLDatabaseDataFileSize,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_data_files_size_bytes", Name: "size"},
		},
	}
	mssqlDatabaseLogFlushedChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_log_flushed",
		Title:      "Log flushed",
		Units:      "bytes/s",
		Fam:        "transactions",
		Ctx:        "mssql.database_log_flushed",
		Priority:   prioMSSQLDatabaseLogFlushed,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_log_flushed_bytes", Name: "flushed", Algo: module.Incremental},
		},
	}
	mssqlDatabaseLogFlushesChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_log_flushes",
		Title:      "Log flushes",
		Units:      "flushes/s",
		Fam:        "transactions",
		Ctx:        "mssql.database_log_flushes",
		Priority:   prioMSSQLDatabaseLogFlushes,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_log_flushes", Name: "log", Algo: module.Incremental},
		},
	}
	mssqlDatabaseTransactionsChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_transactions",
		Title:      "Transactions",
		Units:      "transactions/s",
		Fam:        "transactions",
		Ctx:        "mssql.database_transactions",
		Priority:   prioMSSQLDatabaseTransactions,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_transactions", Name: "transactions", Algo: module.Incremental},
		},
	}
	mssqlDatabaseWriteTransactionsChart = module.Chart{
		OverModule: "mssql",
		ID:         "mssql_db_%s_instance_%s_write_transactions",
		Title:      "Write transactions",
		Units:      "transactions/s",
		Fam:        "transactions",
		Ctx:        "mssql.database_write_transactions",
		Priority:   prioMSSQLDatabaseWriteTransactions,
		Dims: module.Dims{
			{ID: "mssql_db_%s_instance_%s_write_transactions", Name: "write", Algo: module.Incremental},
		},
	}
)

// AD
var (
	adCharts = module.Charts{
		adDatabaseOperationsChart.Copy(),
		adDirectoryOperationsChart.Copy(),
		adNameCacheLookupsChart.Copy(),
		adNameCacheHitsChart.Copy(),
		adDRAReplicationIntersiteCompressedTrafficChart.Copy(),
		adDRAReplicationIntrasiteCompressedTrafficChart.Copy(),
		adDRAReplicationSyncObjectRemainingChart.Copy(),
		adDRAReplicationObjectsFilteredChart.Copy(),
		adDRAReplicationPropertiesUpdatedChart.Copy(),
		adDRAReplicationPropertiesFilteredChart.Copy(),
		adDRAReplicationPendingSyncsChart.Copy(),
		adDRAReplicationSyncRequestsChart.Copy(),
		adDirectoryServiceThreadsChart.Copy(),
		adLDAPLastBindTimeChart.Copy(),
		adBindsTotalChart.Copy(),
		adLDAPSearchesChart.Copy(),
		adATQAverageRequestLatencyChart.Copy(),
		adATQOutstandingRequestsChart.Copy(),
	}
	adDatabaseOperationsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_database_operations",
		Title:      "AD database operations",
		Units:      "operations/s",
		Fam:        "database",
		Ctx:        "ad.database_operations",
		Priority:   prioADDatabaseOperations,
		Dims: module.Dims{
			{ID: "ad_database_operations_total_add", Name: "add", Algo: module.Incremental},
			{ID: "ad_database_operations_total_delete", Name: "delete", Algo: module.Incremental},
			{ID: "ad_database_operations_total_modify", Name: "modify", Algo: module.Incremental},
			{ID: "ad_database_operations_total_recycle", Name: "recycle", Algo: module.Incremental},
		},
	}
	adDirectoryOperationsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_directory_operations_read",
		Title:      "AD directory operations",
		Units:      "operations/s",
		Fam:        "database",
		Ctx:        "ad.directory_operations",
		Priority:   prioADDirectoryOperations,
		Dims: module.Dims{
			{ID: "ad_directory_operations_total_read", Name: "read", Algo: module.Incremental},
			{ID: "ad_directory_operations_total_write", Name: "write", Algo: module.Incremental},
			{ID: "ad_directory_operations_total_search", Name: "search", Algo: module.Incremental},
		},
	}
	adNameCacheLookupsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_name_cache_lookups",
		Title:      "Name cache lookups",
		Units:      "lookups/s",
		Fam:        "database",
		Ctx:        "ad.name_cache_lookups",
		Priority:   prioADNameCacheLookups,
		Dims: module.Dims{
			{ID: "ad_name_cache_lookups_total", Name: "lookups", Algo: module.Incremental},
		},
	}
	adNameCacheHitsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_name_cache_hits",
		Title:      "Name cache hits",
		Units:      "hits/s",
		Fam:        "database",
		Ctx:        "ad.name_cache_hits",
		Priority:   prioADCacheHits,
		Dims: module.Dims{
			{ID: "ad_name_cache_hits_total", Name: "hits", Algo: module.Incremental},
		},
	}
	adDRAReplicationIntersiteCompressedTrafficChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_intersite_compressed_traffic",
		Title:      "DRA replication compressed traffic withing site",
		Units:      "bytes/s",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_intersite_compressed_traffic",
		Priority:   prioADDRAReplicationIntersiteCompressedTraffic,
		Type:       module.Area,
		Dims: module.Dims{
			{ID: "ad_replication_data_intersite_bytes_total_inbound", Name: "inbound", Algo: module.Incremental},
			{ID: "ad_replication_data_intersite_bytes_total_outbound", Name: "outbound", Algo: module.Incremental, Mul: -1},
		},
	}
	adDRAReplicationIntrasiteCompressedTrafficChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_intrasite_compressed_traffic",
		Title:      "DRA replication compressed traffic between sites",
		Units:      "bytes/s",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_intrasite_compressed_traffic",
		Priority:   prioADDRAReplicationIntrasiteCompressedTraffic,
		Type:       module.Area,
		Dims: module.Dims{
			{ID: "ad_replication_data_intrasite_bytes_total_inbound", Name: "inbound", Algo: module.Incremental},
			{ID: "ad_replication_data_intrasite_bytes_total_outbound", Name: "outbound", Algo: module.Incremental, Mul: -1},
		},
	}
	adDRAReplicationSyncObjectRemainingChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_sync_objects_remaining",
		Title:      "DRA replication full sync objects remaining",
		Units:      "objects",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_sync_objects_remaining",
		Priority:   prioADDRAReplicationSyncObjectsRemaining,
		Dims: module.Dims{
			{ID: "ad_replication_inbound_sync_objects_remaining", Name: "inbound"},
		},
	}
	adDRAReplicationObjectsFilteredChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_objects_filtered",
		Title:      "DRA replication objects filtered",
		Units:      "objects/s",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_objects_filtered",
		Priority:   prioADDRAReplicationObjectsFiltered,
		Dims: module.Dims{
			{ID: "ad_replication_inbound_objects_filtered_total", Name: "inbound", Algo: module.Incremental},
		},
	}
	adDRAReplicationPropertiesUpdatedChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_properties_updated",
		Title:      "DRA replication properties updated",
		Units:      "properties/s",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_properties_updated",
		Priority:   prioADDRAReplicationPropertiesUpdated,
		Dims: module.Dims{
			{ID: "ad_replication_inbound_properties_updated_total", Name: "inbound", Algo: module.Incremental},
		},
	}
	adDRAReplicationPropertiesFilteredChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_properties_filtered",
		Title:      "DRA replication properties filtered",
		Units:      "properties/s",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_properties_filtered",
		Priority:   prioADDRAReplicationPropertiesFiltered,
		Dims: module.Dims{
			{ID: "ad_replication_inbound_properties_filtered_total", Name: "inbound", Algo: module.Incremental},
		},
	}
	adDRAReplicationPendingSyncsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_pending_syncs",
		Title:      "DRA replication pending syncs",
		Units:      "syncs",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_pending_syncs",
		Priority:   prioADReplicationPendingSyncs,
		Dims: module.Dims{
			{ID: "ad_replication_pending_synchronizations", Name: "pending"},
		},
	}
	adDRAReplicationSyncRequestsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_dra_replication_sync_requests",
		Title:      "DRA replication sync requests",
		Units:      "requests/s",
		Fam:        "replication",
		Ctx:        "ad.dra_replication_sync_requests",
		Priority:   prioADDRASyncRequests,
		Dims: module.Dims{
			{ID: "ad_replication_sync_requests_total", Name: "request", Algo: module.Incremental},
		},
	}
	adDirectoryServiceThreadsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_ds_threads",
		Title:      "Directory Service threads",
		Units:      "threads",
		Fam:        "replication",
		Ctx:        "ad.ds_threads",
		Priority:   prioADDirectoryServiceThreadsInUse,
		Dims: module.Dims{
			{ID: "ad_directory_service_threads", Name: "in_use"},
		},
	}
	adLDAPLastBindTimeChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_ldap_last_bind_time",
		Title:      "LDAP last successful bind time",
		Units:      "seconds",
		Fam:        "bind",
		Ctx:        "ad.ldap_last_bind_time",
		Priority:   prioADLDAPBindTime,
		Dims: module.Dims{
			{ID: "ad_ldap_last_bind_time_seconds", Name: "last_bind"},
		},
	}
	adBindsTotalChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_binds",
		Title:      "Successful binds",
		Units:      "bind/s",
		Fam:        "bind",
		Ctx:        "ad.binds",
		Priority:   prioADBindsTotal,
		Dims: module.Dims{
			{ID: "ad_binds_total", Name: "binds", Algo: module.Incremental},
		},
	}
	adLDAPSearchesChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_ldap_searches",
		Title:      "LDAP client search operations",
		Units:      "searches/s",
		Fam:        "ldap",
		Ctx:        "ad.ldap_searches",
		Priority:   prioADLDAPSearchesTotal,
		Dims: module.Dims{
			{ID: "ad_ldap_searches_total", Name: "searches", Algo: module.Incremental},
		},
	}
	// https://techcommunity.microsoft.com/t5/ask-the-directory-services-team/understanding-atq-performance-counters-yet-another-twist-in-the/ba-p/400293
	adATQAverageRequestLatencyChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_atq_average_request_latency",
		Title:      "Average request processing time",
		Units:      "seconds",
		Fam:        "queue",
		Ctx:        "ad.atq_average_request_latency",
		Priority:   prioADATQAverageRequestLatency,
		Dims: module.Dims{
			{ID: "ad_atq_average_request_latency", Name: "time", Div: precision},
		},
	}
	adATQOutstandingRequestsChart = module.Chart{
		OverModule: "ad",
		ID:         "ad_atq_outstanding_requests",
		Title:      "Outstanding requests",
		Units:      "requests",
		Fam:        "queue",
		Ctx:        "ad.atq_outstanding_requests",
		Priority:   prioADATQOutstandingRequests,
		Dims: module.Dims{
			{ID: "ad_atq_outstanding_requests", Name: "outstanding"},
		},
	}
)

// AD CS
var (
	adcsCertTemplateChartsTmpl = module.Charts{
		adcsCertTemplateRequestsChartTmpl.Copy(),
		adcsCertTemplateFailedRequestsChartTmpl.Copy(),
		adcsCertTemplateIssuedRequestsChartTmpl.Copy(),
		adcsCertTemplatePendingRequestsChartTmpl.Copy(),
		adcsCertTemplateRequestProcessingTimeChartTmpl.Copy(),

		adcsCertTemplateRetrievalsChartTmpl.Copy(),
		adcsCertificateRetrievalsTimeChartTmpl.Copy(),
		adcsCertTemplateRequestCryptoSigningTimeChartTmpl.Copy(),
		adcsCertTemplateRequestPolicyModuleProcessingTimeChartTmpl.Copy(),
		adcsCertTemplateChallengeResponseChartTmpl.Copy(),
		adcsCertTemplateChallengeResponseProcessingTimeChartTmpl.Copy(),
		adcsCertTemplateSignedCertificateTimestampListsChartTmpl.Copy(),
		adcsCertTemplateSignedCertificateTimestampListProcessingTimeChartTmpl.Copy(),
	}
	adcsCertTemplateRequestsChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template%s_requests",
		Title:      "Certificate requests processed",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adcs.cert_template_requests",
		Priority:   prioADCSCertTemplateRequests,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_requests_total", Name: "requests", Algo: module.Incremental},
		},
	}
	adcsCertTemplateFailedRequestsChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_failed_requests",
		Title:      "Certificate failed requests processed",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adcs.cert_template_failed_requests",
		Priority:   prioADCSCertTemplateFailedRequests,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_failed_requests_total", Name: "failed", Algo: module.Incremental},
		},
	}
	adcsCertTemplateIssuedRequestsChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_issued_requests",
		Title:      "Certificate issued requests processed",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adcs.cert_template_issued_requests",
		Priority:   prioADCSCertTemplateIssuesRequests,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_issued_requests_total", Name: "issued", Algo: module.Incremental},
		},
	}
	adcsCertTemplatePendingRequestsChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_pending_requests",
		Title:      "Certificate pending requests processed",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adcs.cert_template_pending_requests",
		Priority:   prioADCSCertTemplatePendingRequests,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_pending_requests_total", Name: "pending", Algo: module.Incremental},
		},
	}
	adcsCertTemplateRequestProcessingTimeChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_request_processing_time",
		Title:      "Certificate last request processing time",
		Units:      "seconds",
		Fam:        "requests",
		Ctx:        "adcs.cert_template_request_processing_time",
		Priority:   prioADCSCertTemplateRequestProcessingTime,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_request_processing_time_seconds", Name: "processing_time", Div: precision},
		},
	}
	adcsCertTemplateChallengeResponseChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_challenge_responses",
		Title:      "Certificate challenge responses",
		Units:      "responses/s",
		Fam:        "responses",
		Ctx:        "adcs.cert_template_challenge_responses",
		Priority:   prioADCSCertTemplateChallengeResponses,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_challenge_responses_total", Name: "challenge", Algo: module.Incremental},
		},
	}
	adcsCertTemplateRetrievalsChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_retrievals",
		Title:      "Total of certificate retrievals",
		Units:      "retrievals/s",
		Fam:        "retrievals",
		Ctx:        "adcs.cert_template_retrievals",
		Priority:   prioADCSCertTemplateRetrievals,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_retrievals_total", Name: "retrievals", Algo: module.Incremental},
		},
	}
	adcsCertificateRetrievalsTimeChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_retrievals_processing_time",
		Title:      "Certificate last retrieval processing time",
		Units:      "seconds",
		Fam:        "retrievals",
		Ctx:        "adcs.cert_template_retrieval_processing_time",
		Priority:   prioADCSCertTemplateRetrievalProcessingTime,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_retrievals_processing_time_seconds", Name: "processing_time", Div: precision},
		},
	}
	adcsCertTemplateRequestCryptoSigningTimeChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_request_cryptographic_signing_time",
		Title:      "Certificate last signing operation request time",
		Units:      "seconds",
		Fam:        "timings",
		Ctx:        "adcs.cert_template_request_cryptographic_signing_time",
		Priority:   prioADCSCertTemplateRequestCryptoSigningTime,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_request_cryptographic_signing_time_seconds", Name: "singing_time", Div: precision},
		},
	}
	adcsCertTemplateRequestPolicyModuleProcessingTimeChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_request_policy_module_processing_time",
		Title:      "Certificate last policy module processing request time",
		Units:      "seconds",
		Fam:        "timings",
		Ctx:        "adcs.cert_template_request_policy_module_processing",
		Priority:   prioADCSCertTemplateRequestPolicyModuleProcessingTime,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_request_policy_module_processing_time_seconds", Name: "processing_time", Div: precision},
		},
	}
	adcsCertTemplateChallengeResponseProcessingTimeChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_challenge_response_processing_time",
		Title:      "Certificate last challenge response time",
		Units:      "seconds",
		Fam:        "timings",
		Ctx:        "adcs.cert_template_challenge_response_processing_time",
		Priority:   prioADCSCertTemplateChallengeResponseProcessingTime,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_challenge_response_processing_time_seconds", Name: "processing_time", Div: precision},
		},
	}
	adcsCertTemplateSignedCertificateTimestampListsChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_signed_certificate_timestamp_lists",
		Title:      "Certificate Signed Certificate Timestamp Lists processed",
		Units:      "lists/s",
		Fam:        "timings",
		Ctx:        "adcs.cert_template_signed_certificate_timestamp_lists",
		Priority:   prioADCSCertTemplateSignedCertificateTimestampLists,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_signed_certificate_timestamp_lists_total", Name: "processed", Algo: module.Incremental},
		},
	}
	adcsCertTemplateSignedCertificateTimestampListProcessingTimeChartTmpl = module.Chart{
		OverModule: "adcs",
		ID:         "adcs_cert_template_%s_signed_certificate_timestamp_list_processing_time",
		Title:      "Certificate last Signed Certificate Timestamp List process time",
		Units:      "seconds",
		Fam:        "timings",
		Ctx:        "adcs.cert_template_signed_certificate_timestamp_list_processing_time",
		Priority:   prioADCSCertTemplateSignedCertificateTimestampListProcessingTime,
		Dims: module.Dims{
			{ID: "adcs_cert_template_%s_signed_certificate_timestamp_list_processing_time_seconds", Name: "processing_time", Div: precision},
		},
	}
)

// AD FS
var (
	adfsCharts = module.Charts{
		adfsADLoginConnectionFailuresChart.Copy(),
		adfsCertificateAuthenticationsChart.Copy(),
		adfsDBArtifactFailuresChart.Copy(),
		adfsDBArtifactQueryTimeSecondsChart.Copy(),
		adfsDBConfigFailuresChart.Copy(),
		adfsDBConfigQueryTimeSecondsChart.Copy(),
		adfsDeviceAuthenticationsChart.Copy(),
		adfsExternalAuthenticationsChart.Copy(),
		adfsFederatedAuthenticationsChart.Copy(),
		adfsFederationMetadataRequestsChart.Copy(),

		adfsOAuthAuthorizationRequestsChart.Copy(),
		adfsOAuthClientAuthenticationsChart.Copy(),
		adfsOAuthClientCredentialRequestsChart.Copy(),
		adfsOAuthClientPrivKeyJwtAuthenticationsChart.Copy(),
		adfsOAuthClientSecretBasicAuthenticationsChart.Copy(),
		adfsOAuthClientSecretPostAuthenticationsChart.Copy(),
		adfsOAuthClientWindowsAuthenticationsChart.Copy(),
		adfsOAuthLogonCertificateRequestsChart.Copy(),
		adfsOAuthPasswordGrantRequestsChart.Copy(),
		adfsOAuthTokenRequestsChart.Copy(),

		adfsPassiveRequestsChart.Copy(),
		adfsPassportAuthenticationsChart.Copy(),
		adfsPasswordChangeChart.Copy(),
		adfsSAMLPTokenRequestsChart.Copy(),
		adfsSSOAuthenticationsChart.Copy(),
		adfsTokenRequestsChart.Copy(),
		adfsUserPasswordAuthenticationsChart.Copy(),
		adfsWindowsIntegratedAuthenticationsChart.Copy(),
		adfsWSFedTokenRequestsSuccessChart.Copy(),
		adfsWSTrustTokenRequestsSuccessChart.Copy(),
	}

	adfsADLoginConnectionFailuresChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_ad_login_connection_failures",
		Title:      "Connection failures",
		Units:      "failures/s",
		Fam:        "ad",
		Ctx:        "adfs.ad_login_connection_failures",
		Priority:   prioADFSADLoginConnectionFailures,
		Dims: module.Dims{
			{ID: "adfs_ad_login_connection_failures_total", Name: "connection", Algo: module.Incremental},
		},
	}
	adfsCertificateAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_certificate_authentications",
		Title:      "User Certificate authentications",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.certificate_authentications",
		Priority:   prioADFSCertificateAuthentications,
		Dims: module.Dims{
			{ID: "adfs_certificate_authentications_total", Name: "authentications", Algo: module.Incremental},
		},
	}

	adfsDBArtifactFailuresChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_db_artifact_failures",
		Title:      "Connection failures to the artifact database",
		Units:      "failures/s",
		Fam:        "db artifact",
		Ctx:        "adfs.db_artifact_failures",
		Priority:   prioADFSDBArtifactFailures,
		Dims: module.Dims{
			{ID: "adfs_db_artifact_failure_total", Name: "connection", Algo: module.Incremental},
		},
	}
	adfsDBArtifactQueryTimeSecondsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_db_artifact_query_time_seconds",
		Title:      "Time taken for an artifact database query",
		Units:      "seconds/s",
		Fam:        "db artifact",
		Ctx:        "adfs.db_artifact_query_time_seconds",
		Priority:   prioADFSDBArtifactQueryTimeSeconds,
		Dims: module.Dims{
			{ID: "adfs_db_artifact_query_time_seconds_total", Name: "query_time", Algo: module.Incremental, Div: precision},
		},
	}
	adfsDBConfigFailuresChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_db_config_failures",
		Title:      "Connection failures to the configuration database",
		Units:      "failures/s",
		Fam:        "db config",
		Ctx:        "adfs.db_config_failures",
		Priority:   prioADFSDBConfigFailures,
		Dims: module.Dims{
			{ID: "adfs_db_config_failure_total", Name: "connection", Algo: module.Incremental},
		},
	}
	adfsDBConfigQueryTimeSecondsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_db_config_query_time_seconds",
		Title:      "Time taken for a configuration database query",
		Units:      "seconds/s",
		Fam:        "db config",
		Ctx:        "adfs.db_config_query_time_seconds",
		Priority:   prioADFSDBConfigQueryTimeSeconds,
		Dims: module.Dims{
			{ID: "adfs_db_config_query_time_seconds_total", Name: "query_time", Algo: module.Incremental, Div: precision},
		},
	}
	adfsDeviceAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_device_authentications",
		Title:      "Device authentications",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.device_authentications",
		Priority:   prioADFSDeviceAuthentications,
		Dims: module.Dims{
			{ID: "adfs_device_authentications_total", Name: "authentications", Algo: module.Incremental},
		},
	}
	adfsExternalAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_external_authentications",
		Title:      "Authentications from external MFA providers",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.external_authentications",
		Priority:   prioADFSExternalAuthentications,
		Dims: module.Dims{
			{ID: "adfs_external_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_external_authentications_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsFederatedAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_federated_authentications",
		Title:      "Authentications from Federated Sources",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.federated_authentications",
		Priority:   prioADFSFederatedAuthentications,
		Dims: module.Dims{
			{ID: "adfs_federated_authentications_total", Name: "authentications", Algo: module.Incremental},
		},
	}
	adfsFederationMetadataRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_federation_metadata_requests",
		Title:      "Federation Metadata requests",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.federation_metadata_requests",
		Priority:   prioADFSFederationMetadataRequests,
		Dims: module.Dims{
			{ID: "adfs_federation_metadata_requests_total", Name: "requests", Algo: module.Incremental},
		},
	}

	adfsOAuthAuthorizationRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_authorization_requests",
		Title:      "Incoming requests to the OAuth Authorization endpoint",
		Units:      "requests/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_authorization_requests",
		Priority:   prioADFSOauthAuthorizationRequests,
		Dims: module.Dims{
			{ID: "adfs_oauth_authorization_requests_total", Name: "requests", Algo: module.Incremental},
		},
	}
	adfsOAuthClientAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_client_authentications",
		Title:      "OAuth client authentications",
		Units:      "authentications/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_client_authentications",
		Priority:   prioADFSOauthClientAuthentications,
		Dims: module.Dims{
			{ID: "adfs_oauth_client_authentication_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_client_authentication_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthClientCredentialRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_client_credentials_requests",
		Title:      "OAuth client credentials requests",
		Units:      "requests/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_client_credentials_requests",
		Priority:   prioADFSOauthClientCredentials,
		Dims: module.Dims{
			{ID: "adfs_oauth_client_credentials_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_client_credentials_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthClientPrivKeyJwtAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_client_privkey_jwt_authentications",
		Title:      "OAuth client private key JWT authentications",
		Units:      "authentications/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_client_privkey_jwt_authentications",
		Priority:   prioADFSOauthClientPrivkeyJwtAuthentication,
		Dims: module.Dims{
			{ID: "adfs_oauth_client_privkey_jwt_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_client_privkey_jtw_authentication_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthClientSecretBasicAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_client_secret_basic_authentications",
		Title:      "OAuth client secret basic authentications",
		Units:      "authentications/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_client_secret_basic_authentications",
		Priority:   prioADFSOauthClientSecretBasicAuthentications,
		Dims: module.Dims{
			{ID: "adfs_oauth_client_secret_basic_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_client_secret_basic_authentications_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthClientSecretPostAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_client_secret_post_authentications",
		Title:      "OAuth client secret post authentications",
		Units:      "authentications/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_client_secret_post_authentications",
		Priority:   prioADFSOauthClientSecretPostAuthentications,
		Dims: module.Dims{
			{ID: "adfs_oauth_client_secret_post_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_client_secret_post_authentications_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthClientWindowsAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_client_windows_authentications",
		Title:      "OAuth client windows integrated authentications",
		Units:      "authentications/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_client_windows_authentications",
		Priority:   prioADFSOauthClientWindowsAuthentications,
		Dims: module.Dims{
			{ID: "adfs_oauth_client_windows_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_client_windows_authentications_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthLogonCertificateRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_logon_certificate_requests",
		Title:      "OAuth logon certificate requests",
		Units:      "requests/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_logon_certificate_requests",
		Priority:   prioADFSOauthLogonCertificateRequests,
		Dims: module.Dims{
			{ID: "adfs_oauth_logon_certificate_token_requests_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_logon_certificate_requests_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthPasswordGrantRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_password_grant_requests",
		Title:      "OAuth password grant requests",
		Units:      "requests/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_password_grant_requests",
		Priority:   prioADFSOauthPasswordGrantRequests,
		Dims: module.Dims{
			{ID: "adfs_oauth_password_grant_requests_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_oauth_password_grant_requests_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsOAuthTokenRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_oauth_token_requests_success",
		Title:      "Successful RP token requests over OAuth protocol",
		Units:      "requests/s",
		Fam:        "oauth",
		Ctx:        "adfs.oauth_token_requests_success",
		Priority:   prioADFSOauthTokenRequestsSuccess,
		Dims: module.Dims{
			{ID: "adfs_oauth_token_requests_success_total", Name: "success", Algo: module.Incremental},
		},
	}

	adfsPassiveRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_passive_requests",
		Title:      "Passive requests",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.passive_requests",
		Priority:   prioADFSPassiveRequests,
		Dims: module.Dims{
			{ID: "adfs_passive_requests_total", Name: "passive", Algo: module.Incremental},
		},
	}
	adfsPassportAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_passport_authentications",
		Title:      "Microsoft Passport SSO authentications",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.passport_authentications",
		Priority:   prioADFSPassportAuthentications,
		Dims: module.Dims{
			{ID: "adfs_passport_authentications_total", Name: "passport", Algo: module.Incremental},
		},
	}
	adfsPasswordChangeChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_password_change_requests",
		Title:      "Password change requests",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.password_change_requests",
		Priority:   prioADFSPasswordChangeRequests,
		Dims: module.Dims{
			{ID: "adfs_password_change_succeeded_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_password_change_failed_total", Name: "failed", Algo: module.Incremental},
		},
	}
	adfsSAMLPTokenRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_samlp_token_requests_success",
		Title:      "Successful RP token requests over SAML-P protocol",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.samlp_token_requests_success",
		Priority:   prioADFSSAMLPTokenRequests,
		Dims: module.Dims{
			{ID: "adfs_samlp_token_requests_success_total", Name: "success", Algo: module.Incremental},
		},
	}
	adfsWSTrustTokenRequestsSuccessChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_wstrust_token_requests_success",
		Title:      "Successful RP token requests over WS-Trust protocol",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.wstrust_token_requests_success",
		Priority:   prioADFSWSTrustTokenRequestsSuccess,
		Dims: module.Dims{
			{ID: "adfs_wstrust_token_requests_success_total", Name: "success", Algo: module.Incremental},
		},
	}
	adfsSSOAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_sso_authentications",
		Title:      "SSO authentications",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.sso_authentications",
		Priority:   prioADFSSSOAuthentications,
		Dims: module.Dims{
			{ID: "adfs_sso_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_sso_authentications_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsTokenRequestsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_token_requests",
		Title:      "Token access requests",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.token_requests",
		Priority:   prioADFSTokenRequests,
		Dims: module.Dims{
			{ID: "adfs_token_requests_total", Name: "requests", Algo: module.Incremental},
		},
	}
	adfsUserPasswordAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_userpassword_authentications",
		Title:      "AD U/P authentications",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.userpassword_authentications",
		Priority:   prioADFSUserPasswordAuthentications,
		Dims: module.Dims{
			{ID: "adfs_sso_authentications_success_total", Name: "success", Algo: module.Incremental},
			{ID: "adfs_sso_authentications_failure_total", Name: "failure", Algo: module.Incremental},
		},
	}
	adfsWindowsIntegratedAuthenticationsChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_windows_integrated_authentications",
		Title:      "Windows integrated authentications using Kerberos or NTLM",
		Units:      "authentications/s",
		Fam:        "auth",
		Ctx:        "adfs.windows_integrated_authentications",
		Priority:   prioADFSWindowsIntegratedAuthentications,
		Dims: module.Dims{
			{ID: "adfs_windows_integrated_authentications_total", Name: "authentications", Algo: module.Incremental},
		},
	}
	adfsWSFedTokenRequestsSuccessChart = module.Chart{
		OverModule: "adfs",
		ID:         "adfs_wsfed_token_requests_success",
		Title:      "Successful RP token requests over WS-Fed protocol",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "adfs.wsfed_token_requests_success",
		Priority:   prioADFSWSFedTokenRequestsSuccess,
		Dims: module.Dims{
			{ID: "adfs_wsfed_token_requests_success_total", Name: "success", Algo: module.Incremental},
		},
	}
)

// Exchange
var (
	exchangeCharts = module.Charts{
		exchangeActiveSyncPingCMDsPendingChart.Copy(),
		exchangeActiveSyncRequestsChart.Copy(),
		exchangeActiveSyncCMDsChart.Copy(),
		exchangeAutoDiscoverRequestsChart.Copy(),
		exchangeAvailableServiceRequestsChart.Copy(),
		exchangeOWACurrentUniqueUsersChart.Copy(),
		exchangeOWARequestsChart.Copy(),
		exchangeRPCActiveUsersCountChart.Copy(),
		exchangeRPCAvgLatencyChart.Copy(),
		exchangeRPCConnectionChart.Copy(),
		exchangeRPCOperationsChart.Copy(),
		exchangeRPCRequestsChart.Copy(),
		exchangeRPCUserChart.Copy(),
		exchangeTransportQueuesActiveMailBoxDelivery.Copy(),
		exchangeTransportQueuesExternalActiveRemoteDelivery.Copy(),
		exchangeTransportQueuesExternalLargestDelivery.Copy(),
		exchangeTransportQueuesInternalActiveRemoteDelivery.Copy(),
		exchangeTransportQueuesInternalLargestDelivery.Copy(),
		exchangeTransportQueuesRetryMailboxDelivery.Copy(),
		exchangeTransportQueuesUnreachable.Copy(),
		exchangeTransportQueuesPoison.Copy(),
	}
	exchangeWorkloadChartsTmpl = module.Charts{
		exchangeWorkloadActiveTasks.Copy(),
		exchangeWorkloadCompletedTasks.Copy(),
		exchangeWorkloadQueuedTasks.Copy(),
		exchangeWorkloadYieldedTasks.Copy(),

		exchangeWorkloadActivityStatus.Copy(),
	}
	exchangeLDAPChartsTmpl = module.Charts{
		exchangeLDAPLongRunningOPS.Copy(),
		exchangeLDAPReadTime.Copy(),
		exchangeLDAPSearchTime.Copy(),
		exchangeLDAPTimeoutErrors.Copy(),
		exchangeLDAPWriteTime.Copy(),
	}
	exchangeHTTPProxyChartsTmpl = module.Charts{
		exchangeProxyAvgAuthLatency.Copy(),
		exchangeProxyAvgCasProcessingLatencySec.Copy(),
		exchangeProxyMailboxProxyFailureRace.Copy(),
		exchangeProxyMailboxServerLocatorAvgLatencySec.Copy(),
		exchangeProxyOutstandingProxyRequests.Copy(),
		exchangeProxyRequestsTotal.Copy(),
	}

	exchangeActiveSyncPingCMDsPendingChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_activesync_ping_cmds_pending",
		Title:      "Ping commands pending in queue",
		Units:      "commands",
		Fam:        "sync",
		Ctx:        "exchange.activesync_ping_cmds_pending",
		Priority:   prioExchangeActiveSyncPingCMDsPending,
		Dims: module.Dims{
			{ID: "exchange_activesync_ping_cmds_pending", Name: "pending"},
		},
	}
	exchangeActiveSyncRequestsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_activesync_requests",
		Title:      "HTTP requests received from ASP.NET",
		Units:      "requests/s",
		Fam:        "sync",
		Ctx:        "exchange.activesync_requests",
		Priority:   prioExchangeActiveSyncRequests,
		Dims: module.Dims{
			{ID: "exchange_activesync_requests_total", Name: "received", Algo: module.Incremental},
		},
	}
	exchangeActiveSyncCMDsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_activesync_sync_cmds",
		Title:      "Sync commands processed",
		Units:      "commands/s",
		Fam:        "sync",
		Ctx:        "exchange.activesync_sync_cmds",
		Priority:   prioExchangeActiveSyncSyncCMDs,
		Dims: module.Dims{
			{ID: "exchange_activesync_sync_cmds_total", Name: "processed", Algo: module.Incremental},
		},
	}
	exchangeAutoDiscoverRequestsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_autodiscover_requests",
		Title:      "Autodiscover service requests processed",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "exchange.autodiscover_requests",
		Priority:   prioExchangeAutoDiscoverRequests,
		Dims: module.Dims{
			{ID: "exchange_autodiscover_requests_total", Name: "processed", Algo: module.Incremental},
		},
	}
	exchangeAvailableServiceRequestsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_avail_service_requests",
		Title:      "Requests serviced",
		Units:      "requests/s",
		Fam:        "requests",
		Ctx:        "exchange.avail_service_requests",
		Priority:   prioExchangeAvailServiceRequests,
		Dims: module.Dims{
			{ID: "exchange_avail_service_requests_per_sec", Name: "serviced", Algo: module.Incremental},
		},
	}
	exchangeOWACurrentUniqueUsersChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_owa_current_unique_users",
		Title:      "Unique users currently logged on to Outlook Web App",
		Units:      "users",
		Fam:        "owa",
		Ctx:        "exchange.owa_current_unique_users",
		Priority:   prioExchangeOWACurrentUniqueUsers,
		Dims: module.Dims{
			{ID: "exchange_owa_current_unique_users", Name: "logged-in"},
		},
	}
	exchangeOWARequestsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_owa_requests_total",
		Title:      "Requests handled by Outlook Web App",
		Units:      "requests/s",
		Fam:        "owa",
		Ctx:        "exchange.owa_requests_total",
		Priority:   prioExchangeOWARequestsTotal,
		Dims: module.Dims{
			{ID: "exchange_owa_requests_total", Name: "handled", Algo: module.Incremental},
		},
	}
	exchangeRPCActiveUsersCountChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_rpc_active_user",
		Title:      "Active unique users in the last 2 minutes",
		Units:      "users",
		Fam:        "rpc",
		Ctx:        "exchange.rpc_active_user_count",
		Priority:   prioExchangeRPCActiveUserCount,
		Dims: module.Dims{
			{ID: "exchange_rpc_active_user_count", Name: "active"},
		},
	}
	exchangeRPCAvgLatencyChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_rpc_avg_latency",
		Title:      "Average latency",
		Units:      "seconds",
		Fam:        "rpc",
		Ctx:        "exchange.rpc_avg_latency",
		Priority:   prioExchangeRPCAvgLatency,
		Dims: module.Dims{
			{ID: "exchange_rpc_avg_latency_sec", Name: "latency", Div: precision},
		},
	}
	exchangeRPCConnectionChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_rpc_connection",
		Title:      "Client connections",
		Units:      "connections",
		Fam:        "rpc",
		Ctx:        "exchange.rpc_connection_count",
		Priority:   prioExchangeRPCConnectionCount,
		Dims: module.Dims{
			{ID: "exchange_rpc_connection_count", Name: "connections"},
		},
	}
	exchangeRPCOperationsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_rpc_operations",
		Title:      "RPC operations",
		Units:      "operations/s",
		Fam:        "rpc",
		Ctx:        "exchange.rpc_operations",
		Priority:   prioExchangeRPCOperationsTotal,
		Dims: module.Dims{
			{ID: "exchange_rpc_operations_total", Name: "operations", Algo: module.Incremental},
		},
	}
	exchangeRPCRequestsChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_rpc_requests_total",
		Title:      "Clients requests currently being processed",
		Units:      "requests",
		Fam:        "rpc",
		Ctx:        "exchange.rpc_requests",
		Priority:   prioExchangeRPCRequests,
		Dims: module.Dims{
			{ID: "exchange_rpc_requests", Name: "processed"},
		},
	}
	exchangeRPCUserChart = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_rpc_user",
		Title:      "RPC users",
		Units:      "users",
		Fam:        "rpc",
		Ctx:        "exchange.rpc_user_count",
		Priority:   prioExchangeRpcUserCount,
		Dims: module.Dims{
			{ID: "exchange_rpc_user_count", Name: "users"},
		},
	}

	// Source: https://learn.microsoft.com/en-us/exchange/mail-flow/queues/queues?view=exchserver-2019
	exchangeTransportQueuesActiveMailBoxDelivery = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_active_mailbox_delivery",
		Title:      "Active Mailbox Delivery Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_active_mail_box_delivery",
		Priority:   prioExchangeTransportQueuesActiveMailboxDelivery,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_active_mailbox_delivery_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_active_mailbox_delivery_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_active_mailbox_delivery_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_active_mailbox_delivery_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesExternalActiveRemoteDelivery = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_external_active_remote_delivery",
		Title:      "External Active Remote Delivery Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_external_active_remote_delivery",
		Priority:   prioExchangeTransportQueuesExternalActiveRemoteDelivery,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_external_active_remote_delivery_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_external_active_remote_delivery_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_external_active_remote_delivery_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_external_active_remote_delivery_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesExternalLargestDelivery = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_external_largest_delivery",
		Title:      "External Largest Delivery Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_external_largest_delivery",
		Priority:   prioExchangeTransportQueuesExternalLargestDelivery,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_external_largest_delivery_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_external_largest_delivery_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_external_largest_delivery_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_external_largest_delivery_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesInternalActiveRemoteDelivery = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_internal_active_remote_delivery",
		Title:      "Internal Active Remote Delivery Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_internal_active_remote_delivery",
		Priority:   prioExchangeTransportQueuesInternalActiveRemoteDeliery,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_internal_active_remote_delivery_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_internal_active_remote_delivery_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_internal_active_remote_delivery_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_internal_active_remote_delivery_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesInternalLargestDelivery = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_internal_largest_delivery",
		Title:      "Internal Largest Delivery Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_internal_largest_delivery",
		Priority:   prioExchangeTransportQueuesInternalLargestDelivery,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_internal_largest_delivery_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_internal_largest_delivery_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_internal_largest_delivery_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_internal_largest_delivery_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesRetryMailboxDelivery = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_retry_mailbox_delivery",
		Title:      "Internal Active Remote Delivery Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_retry_mailbox_delivery",
		Priority:   prioExchangeTransportQueuesRetryMailboxDelivery,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_retry_mailbox_delivery_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_retry_mailbox_delivery_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_retry_mailbox_delivery_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_retry_mailbox_delivery_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesUnreachable = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_unreachable",
		Title:      "Unreachable Queue length",
		Units:      "messages",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_unreachable",
		Priority:   prioExchangeTransportQueuesUnreachable,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_unreachable_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_unreachable_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_unreachable_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_unreachable_normal_priority", Name: "normal"},
		},
	}
	exchangeTransportQueuesPoison = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_transport_queues_poison",
		Title:      "Poison Queue Length",
		Units:      "messages/s",
		Fam:        "queue",
		Ctx:        "exchange.transport_queues_poison",
		Priority:   prioExchangeTransportQueuesPoison,
		Dims: module.Dims{
			{ID: "exchange_transport_queues_poison_high_priority", Name: "high"},
			{ID: "exchange_transport_queues_poison_low_priority", Name: "low"},
			{ID: "exchange_transport_queues_poison_none_priority", Name: "none"},
			{ID: "exchange_transport_queues_poison_normal_priority", Name: "normal"},
		},
	}

	exchangeWorkloadActiveTasks = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_workload_%s_tasks",
		Title:      "Workload active tasks",
		Units:      "tasks",
		Fam:        "workload",
		Ctx:        "exchange.workload_active_tasks",
		Priority:   prioExchangeWorkloadActiveTasks,
		Dims: module.Dims{
			{ID: "exchange_workload_%s_active_tasks", Name: "active"},
		},
	}
	exchangeWorkloadCompletedTasks = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_workload_%s_completed_tasks",
		Title:      "Workload completed tasks",
		Units:      "tasks/s",
		Fam:        "workload",
		Ctx:        "exchange.workload_completed_tasks",
		Priority:   prioExchangeWorkloadCompleteTasks,
		Dims: module.Dims{
			{ID: "exchange_workload_%s_completed_tasks", Name: "completed", Algo: module.Incremental},
		},
	}
	exchangeWorkloadQueuedTasks = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_workload_%s_queued_tasks",
		Title:      "Workload queued tasks",
		Units:      "tasks/s",
		Fam:        "workload",
		Ctx:        "exchange.workload_queued_tasks",
		Priority:   prioExchangeWorkloadQueueTasks,
		Dims: module.Dims{
			{ID: "exchange_workload_%s_queued_tasks", Name: "queued", Algo: module.Incremental},
		},
	}
	exchangeWorkloadYieldedTasks = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_workload_%s_yielded_tasks",
		Title:      "Workload yielded tasks",
		Units:      "tasks/s",
		Fam:        "workload",
		Ctx:        "exchange.workload_yielded_tasks",
		Priority:   prioExchangeWorkloadYieldedTasks,
		Dims: module.Dims{
			{ID: "exchange_workload_%s_yielded_tasks", Name: "yielded", Algo: module.Incremental},
		},
	}
	exchangeWorkloadActivityStatus = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_workload_%s_activity_status",
		Title:      "Workload activity status",
		Units:      "status",
		Fam:        "workload",
		Ctx:        "exchange.workload_activity_status",
		Priority:   prioExchangeWorkloadActivityStatus,
		Dims: module.Dims{
			{ID: "exchange_workload_%s_is_active", Name: "active"},
			{ID: "exchange_workload_%s_is_paused", Name: "paused"},
		},
	}

	exchangeLDAPLongRunningOPS = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_ldap_%s_long_running_ops",
		Title:      "Long Running LDAP operations",
		Units:      "operations/s",
		Fam:        "ldap",
		Ctx:        "exchange.ldap_long_running_ops_per_sec",
		Priority:   prioExchangeLDAPLongRunningOPS,
		Dims: module.Dims{
			{ID: "exchange_ldap_%s_long_running_ops_per_sec", Name: "long-running", Algo: module.Incremental},
		},
	}
	exchangeLDAPReadTime = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_ldap_%s_read_time",
		Title:      "Time to send an LDAP read request and receive a response",
		Units:      "seconds",
		Fam:        "ldap",
		Ctx:        "exchange.ldap_read_time",
		Priority:   prioExchangeLDAPReadTime,
		Dims: module.Dims{
			{ID: "exchange_ldap_%s_read_time_sec", Name: "read", Algo: module.Incremental, Div: precision},
		},
	}
	exchangeLDAPSearchTime = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_ldap_%s_search_time",
		Title:      "Time to send an LDAP search request and receive a response",
		Units:      "seconds",
		Fam:        "ldap",
		Ctx:        "exchange.ldap_search_time",
		Priority:   prioExchangeLDAPSearchTime,
		Dims: module.Dims{
			{ID: "exchange_ldap_%s_search_time_sec", Name: "search", Algo: module.Incremental, Div: precision},
		},
	}
	exchangeLDAPWriteTime = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_ldap_%s_write_time",
		Title:      "Time to send an LDAP search request and receive a response",
		Units:      "second",
		Fam:        "ldap",
		Ctx:        "exchange.ldap_write_time",
		Priority:   prioExchangeLDAPWriteTime,
		Dims: module.Dims{
			{ID: "exchange_ldap_%s_write_time_sec", Name: "write", Algo: module.Incremental, Div: precision},
		},
	}
	exchangeLDAPTimeoutErrors = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_ldap_%s_timeout_errors",
		Title:      "LDAP timeout errors",
		Units:      "errors/s",
		Fam:        "ldap",
		Ctx:        "exchange.ldap_timeout_errors",
		Priority:   prioExchangeLDAPTimeoutErrors,
		Dims: module.Dims{
			{ID: "exchange_ldap_%s_timeout_errors_total", Name: "timeout", Algo: module.Incremental},
		},
	}

	exchangeProxyAvgAuthLatency = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_proxy_%s_avg_auth_latency",
		Title:      "Average time spent authenticating CAS",
		Units:      "seconds",
		Fam:        "proxy",
		Ctx:        "exchange.http_proxy_avg_auth_latency",
		Priority:   prioExchangeHTTPProxyAVGAuthLatency,
		Dims: module.Dims{
			{ID: "exchange_http_proxy_%s_avg_auth_latency", Name: "latency"},
		},
	}
	exchangeProxyAvgCasProcessingLatencySec = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_proxy_%s_avg_cas_processing_latency_sec",
		Title:      "Average time spent authenticating CAS",
		Units:      "seconds",
		Fam:        "proxy",
		Ctx:        "exchange.http_proxy_avg_cas_processing_latency_sec",
		Priority:   prioExchangeHTTPProxyAVGCASProcessingLatency,
		Dims: module.Dims{
			{ID: "exchange_http_proxy_%s_avg_cas_proccessing_latency_sec", Name: "latency"},
		},
	}
	exchangeProxyMailboxProxyFailureRace = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_proxy_%s_mailbox_proxy_failure_rate",
		Title:      "Percentage of failures between this CAS and MBX servers",
		Units:      "percentage",
		Fam:        "proxy",
		Ctx:        "exchange.http_proxy_mailbox_proxy_failure_rate",
		Priority:   prioExchangeHTTPProxyMailboxProxyFailureRate,
		Dims: module.Dims{
			{ID: "exchange_http_proxy_%s_mailbox_proxy_failure_rate", Name: "failures", Div: precision},
		},
	}
	exchangeProxyMailboxServerLocatorAvgLatencySec = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_proxy_%s_mailbox_server_locator_avg_latency_sec",
		Title:      "Average latency of MailboxServerLocator web service calls",
		Units:      "seconds",
		Fam:        "proxy",
		Ctx:        "exchange.http_proxy_mailbox_server_locator_avg_latency_sec",
		Priority:   prioExchangeHTTPProxyServerLocatorAvgLatency,
		Dims: module.Dims{
			{ID: "exchange_http_proxy_%s_mailbox_server_locator_avg_latency_sec", Name: "latency", Div: precision},
		},
	}
	exchangeProxyOutstandingProxyRequests = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_proxy_%s_outstanding_proxy_requests",
		Title:      "Concurrent outstanding proxy requests",
		Units:      "requests",
		Fam:        "proxy",
		Ctx:        "exchange.http_proxy_outstanding_proxy_requests",
		Priority:   prioExchangeHTTPProxyOutstandingProxyRequests,
		Dims: module.Dims{
			{ID: "exchange_http_proxy_%s_outstanding_proxy_requests", Name: "outstanding"},
		},
	}
	exchangeProxyRequestsTotal = module.Chart{
		OverModule: "exchange",
		ID:         "exchange_proxy_%s_requests_total",
		Title:      "Number of proxy requests processed each second",
		Units:      "requests/s",
		Fam:        "proxy",
		Ctx:        "exchange.http_proxy_requests",
		Priority:   prioExchangeHTTPProxyRequestsTotal,
		Dims: module.Dims{
			{ID: "exchange_http_proxy_%s_requests_total", Name: "processed", Algo: module.Incremental},
		},
	}
)

// Logon
var (
	logonCharts = module.Charts{
		logonSessionsChart.Copy(),
	}
	logonSessionsChart = module.Chart{
		ID:       "logon_active_sessions_by_type",
		Title:    "Active User Logon Sessions By Type",
		Units:    "sessions",
		Fam:      "logon",
		Ctx:      "windows.logon_type_sessions",
		Type:     module.Stacked,
		Priority: prioLogonSessions,
		Dims: module.Dims{
			{ID: "logon_type_system_sessions", Name: "system"},
			{ID: "logon_type_proxy_sessions", Name: "proxy"},
			{ID: "logon_type_network_sessions", Name: "network"},
			{ID: "logon_type_interactive_sessions", Name: "interactive"},
			{ID: "logon_type_batch_sessions", Name: "batch"},
			{ID: "logon_type_service_sessions", Name: "service"},
			{ID: "logon_type_unlock_sessions", Name: "unlock"},
			{ID: "logon_type_network_clear_text_sessions", Name: "network_clear_text"},
			{ID: "logon_type_new_credentials_sessions", Name: "new_credentials"},
			{ID: "logon_type_remote_interactive_sessions", Name: "remote_interactive"},
			{ID: "logon_type_cached_interactive_sessions", Name: "cached_interactive"},
			{ID: "logon_type_cached_remote_interactive_sessions", Name: "cached_remote_interactive"},
			{ID: "logon_type_cached_unlock_sessions", Name: "cached_unlock"},
		},
	}
)

// Thermal zone
var (
	thermalzoneChartsTmpl = module.Charts{
		thermalzoneTemperatureChartTmpl.Copy(),
	}
	thermalzoneTemperatureChartTmpl = module.Chart{
		ID:       "thermalzone_%s_temperature",
		Title:    "Thermal zone temperature",
		Units:    "Celsius",
		Fam:      "thermalzone",
		Ctx:      "windows.thermalzone_temperature",
		Priority: prioThermalzoneTemperature,
		Dims: module.Dims{
			{ID: "thermalzone_%s_temperature", Name: "temperature"},
		},
	}
)

// Processes
var (
	processesCharts = module.Charts{
		processesCPUUtilizationTotalChart.Copy(),
		processesMemoryUsageChart.Copy(),
		processesHandlesChart.Copy(),
		processesIOBytesChart.Copy(),
		processesIOOperationsChart.Copy(),
		processesPageFaultsChart.Copy(),
		processesPageFileBytes.Copy(),
		processesThreads.Copy(),
	}
	processesCPUUtilizationTotalChart = module.Chart{
		ID:       "processes_cpu_utilization",
		Title:    "CPU usage (100% = 1 core)",
		Units:    "percentage",
		Fam:      "processes",
		Ctx:      "windows.processes_cpu_utilization",
		Type:     module.Stacked,
		Priority: prioProcessesCPUUtilization,
	}
	processesMemoryUsageChart = module.Chart{
		ID:       "processes_memory_usage",
		Title:    "Memory usage",
		Units:    "bytes",
		Fam:      "processes",
		Ctx:      "windows.processes_memory_usage",
		Type:     module.Stacked,
		Priority: prioProcessesMemoryUsage,
	}
	processesIOBytesChart = module.Chart{
		ID:       "processes_io_bytes",
		Title:    "Total of IO bytes (read, write, other)",
		Units:    "bytes/s",
		Fam:      "processes",
		Ctx:      "windows.processes_io_bytes",
		Type:     module.Stacked,
		Priority: prioProcessesIOBytes,
	}
	processesIOOperationsChart = module.Chart{
		ID:       "processes_io_operations",
		Title:    "Total of IO events (read, write, other)",
		Units:    "operations/s",
		Fam:      "processes",
		Ctx:      "windows.processes_io_operations",
		Type:     module.Stacked,
		Priority: prioProcessesIOOperations,
	}
	processesPageFaultsChart = module.Chart{
		ID:       "processes_page_faults",
		Title:    "Number of page faults",
		Units:    "pgfaults/s",
		Fam:      "processes",
		Ctx:      "windows.processes_page_faults",
		Type:     module.Stacked,
		Priority: prioProcessesPageFaults,
	}
	processesPageFileBytes = module.Chart{
		ID:       "processes_page_file_bytes",
		Title:    "Bytes used in page file(s)",
		Units:    "bytes",
		Fam:      "processes",
		Ctx:      "windows.processes_file_bytes",
		Type:     module.Stacked,
		Priority: prioProcessesPageFileBytes,
	}
	processesThreads = module.Chart{
		ID:       "processes_threads",
		Title:    "Active threads",
		Units:    "threads",
		Fam:      "processes",
		Ctx:      "windows.processes_threads",
		Type:     module.Stacked,
		Priority: prioProcessesThreads,
	}
	processesHandlesChart = module.Chart{
		ID:       "processes_handles",
		Title:    "Number of handles open",
		Units:    "handles",
		Fam:      "processes",
		Ctx:      "windows.processes_handles",
		Type:     module.Stacked,
		Priority: prioProcessesHandles,
	}
)

// .NET
var (
	netFrameworkCLRExceptionsChartsTmpl = module.Charts{
		netFrameworkCLRExceptionsThrown.Copy(),
		netFrameworkCLRExceptionsFilters.Copy(),
		netFrameworkCLRExceptionsFinallys.Copy(),
		netFrameworkCLRExceptionsThrowToCatchDepth.Copy(),
	}

	netFrameworkCLRInteropChartsTmpl = module.Charts{
		netFrameworkCLRInteropCOMCallableWrapper.Copy(),
		netFrameworkCLRInteropMarshalling.Copy(),
		netFrameworkCLRInteropStubsCreated.Copy(),
	}

	netFrameworkCLRJITChartsTmpl = module.Charts{
		netFrameworkCLRJITMethods.Copy(),
		netFrameworkCLRJITTime.Copy(),
		netFrameworkCLRJITStandardFailures.Copy(),
		netFrameworkCLRJITILBytes.Copy(),
	}

	netFrameworkCLRLoadingChartsTmpl = module.Charts{
		netFrameworkCLRLoadingLoaderHeapSize.Copy(),
		netFrameworkCLRLoadingAppDomainsLoaded.Copy(),
		netFrameworkCLRLoadingAppDomainsUnloaded.Copy(),
		netFrameworkCLRLoadingAssembliesLoaded.Copy(),
		netFrameworkCLRLoadingClassesLoaded.Copy(),
		netFrameworkCLRLoadingClassLoadFailure.Copy(),
	}

	netFrameworkCLRLocksAndThreadsChartsTmpl = module.Charts{
		netFrameworkCLRLockAndThreadsQueueLength.Copy(),
		netFrameworkCLRLockAndThreadsCurrentLogicalThreads.Copy(),
		netFrameworkCLRLockAndThreadsCurrentPhysicalThreads.Copy(),
		netFrameworkCLRLockAndThreadsRecognizedThreads.Copy(),
		netFrameworkCLRLockAndThreadsContentions.Copy(),
	}

	netFrameworkCLRMemoryChartsTmpl = module.Charts{
		netFrameworkCLRMemoryAllocatedBytes.Copy(),
		netFrameworkCLRMemoryFinalizationSurvivors.Copy(),
		netFrameworkCLRMemoryHeapSize.Copy(),
		netFrameworkCLRMemoryPromoted.Copy(),
		netFrameworkCLRMemoryNumberGCHandles.Copy(),
		netFrameworkCLRMemoryCollections.Copy(),
		netFrameworkCLRMemoryInducedGC.Copy(),
		netFrameworkCLRMemoryNumberPinnedObjects.Copy(),
		netFrameworkCLRMemoryNumberSinkBlocksInUse.Copy(),
		netFrameworkCLRMemoryCommitted.Copy(),
		netFrameworkCLRMemoryReserved.Copy(),
		netFrameworkCLRMemoryGCTime.Copy(),
	}

	netFrameworkCLRRemotingChartsTmpl = module.Charts{
		netFrameworkCLRRemotingChannels.Copy(),
		netFrameworkCLRRemotingContextBoundClassesLoaded.Copy(),
		netFrameworkCLRRemotingContextBoundObjects.Copy(),
		netFrameworkCLRRemotingContextProxies.Copy(),
		netFrameworkCLRRemotingContexts.Copy(),
		netFrameworkCLRRemotingCalls.Copy(),
	}

	netFrameworkCLRSecurityChartsTmpl = module.Charts{
		netFrameworkCLRSecurityLinkTimeChecks.Copy(),
		netFrameworkCLRSecurityChecksTime.Copy(),
		netFrameworkCLRSecurityStackWalkDepth.Copy(),
		netFrameworkCLRSecurityRuntimeChecks.Copy(),
	}

	// Exceptions
	netFrameworkCLRExceptionsThrown = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrexception_thrown",
		Title:      "Thrown exceptions",
		Units:      "exceptions/s",
		Fam:        "exceptions",
		Ctx:        "netframework.clrexception_thrown",
		Priority:   prioNETFrameworkCLRExceptionsThrown,
		Dims: module.Dims{
			{ID: "netframework_%s_clrexception_thrown_total", Name: "exceptions", Algo: module.Incremental},
		},
	}
	netFrameworkCLRExceptionsFilters = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrexception_filters",
		Title:      "Executed exception filters",
		Units:      "filters/s",
		Fam:        "exceptions",
		Ctx:        "netframework.clrexception_filters",
		Priority:   prioNETFrameworkCLRExceptionsFilters,
		Dims: module.Dims{
			{ID: "netframework_%s_clrexception_filters_total", Name: "filters", Algo: module.Incremental},
		},
	}
	netFrameworkCLRExceptionsFinallys = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrexception_finallys",
		Title:      "Executed finally blocks",
		Units:      "finallys/s",
		Fam:        "exceptions",
		Ctx:        "netframework.clrexception_finallys",
		Priority:   prioNETFrameworkCLRExceptionsFinallys,
		Dims: module.Dims{
			{ID: "netframework_%s_clrexception_finallys_total", Name: "finallys", Algo: module.Incremental},
		},
	}
	netFrameworkCLRExceptionsThrowToCatchDepth = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrexception_throw_to_catch_depth",
		Title:      "Traversed stack frames",
		Units:      "stack_frames/s",
		Fam:        "exceptions",
		Ctx:        "netframework.clrexception_throw_to_catch_depth",
		Priority:   prioNETFrameworkCLRExceptionsThrowToCatchDepth,
		Dims: module.Dims{
			{ID: "netframework_%s_clrexception_throw_to_catch_depth_total", Name: "traversed", Algo: module.Incremental},
		},
	}

	// Interop
	netFrameworkCLRInteropCOMCallableWrapper = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrinterop_com_callable_wrappers",
		Title:      "COM callable wrappers (CCW)",
		Units:      "ccw/s",
		Fam:        "interop",
		Ctx:        "netframework.clrinterop_com_callable_wrappers",
		Priority:   prioNETFrameworkCLRInteropCOMCallableWrappers,
		Dims: module.Dims{
			{ID: "netframework_%s_clrinterop_com_callable_wrappers_total", Name: "com_callable_wrappers", Algo: module.Incremental},
		},
	}
	netFrameworkCLRInteropMarshalling = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrinterop_interop_marshalling",
		Title:      "Arguments and return values marshallings",
		Units:      "marshalling/s",
		Fam:        "interop",
		Ctx:        "netframework.clrinterop_interop_marshallings",
		Priority:   prioNETFrameworkCLRInteropMarshalling,
		Dims: module.Dims{
			{ID: "netframework_%s_clrinterop_interop_marshalling_total", Name: "marshallings", Algo: module.Incremental},
		},
	}
	netFrameworkCLRInteropStubsCreated = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrinterop_interop_stubs_created",
		Title:      "Created stubs",
		Units:      "stubs/s",
		Fam:        "interop",
		Ctx:        "netframework.clrinterop_interop_stubs_created",
		Priority:   prioNETFrameworkCLRInteropStubsCreated,
		Dims: module.Dims{
			{ID: "netframework_%s_clrinterop_interop_stubs_created_total", Name: "created", Algo: module.Incremental},
		},
	}

	// JIT
	netFrameworkCLRJITMethods = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrjit_methods",
		Title:      "JIT-compiled methods",
		Units:      "methods/s",
		Fam:        "jit",
		Ctx:        "netframework.clrjit_methods",
		Priority:   prioNETFrameworkCLRJITMethods,
		Dims: module.Dims{
			{ID: "netframework_%s_clrjit_methods_total", Name: "jit-compiled", Algo: module.Incremental},
		},
	}
	netFrameworkCLRJITTime = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrjit_time",
		Title:      "Time spent in JIT compilation",
		Units:      "percentage",
		Fam:        "jit",
		Ctx:        "netframework.clrjit_time",
		Priority:   prioNETFrameworkCLRJITTime,
		Dims: module.Dims{
			{ID: "netframework_%s_clrjit_time_percent", Name: "time"},
		},
	}
	netFrameworkCLRJITStandardFailures = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrjit_standard_failures",
		Title:      "JIT compiler failures",
		Units:      "failures/s",
		Fam:        "jit",
		Ctx:        "netframework.clrjit_standard_failures",
		Priority:   prioNETFrameworkCLRJITStandardFailures,
		Dims: module.Dims{
			{ID: "netframework_%s_clrjit_standard_failures_total", Name: "failures", Algo: module.Incremental},
		},
	}
	netFrameworkCLRJITILBytes = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrjit_il_bytes",
		Title:      "Compiled Microsoft intermediate language (MSIL) bytes",
		Units:      "bytes/s",
		Fam:        "jit",
		Ctx:        "netframework.clrjit_il_bytes",
		Priority:   prioNETFrameworkCLRJITILBytes,
		Dims: module.Dims{
			{ID: "netframework_%s_clrjit_il_bytes_total", Name: "compiled_msil", Algo: module.Incremental},
		},
	}

	// Loading
	netFrameworkCLRLoadingLoaderHeapSize = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrloading_loader_heap_size",
		Title:      "Memory committed by class loader",
		Units:      "bytes",
		Fam:        "loading",
		Ctx:        "netframework.clrloading_loader_heap_size",
		Priority:   prioNETFrameworkCLRLoadingLoaderHeapSize,
		Dims: module.Dims{
			{ID: "netframework_%s_clrloading_loader_heap_size_bytes", Name: "committed"},
		},
	}
	netFrameworkCLRLoadingAppDomainsLoaded = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrloading_appdomains_loaded",
		Title:      "Loaded application domains",
		Units:      "domain/s",
		Fam:        "loading",
		Ctx:        "netframework.clrloading_appdomains_loaded",
		Priority:   prioNETFrameworkCLRLoadingAppDomainsLoaded,
		Dims: module.Dims{
			{ID: "netframework_%s_clrloading_appdomains_loaded_total", Name: "loaded", Algo: module.Incremental},
		},
	}
	netFrameworkCLRLoadingAppDomainsUnloaded = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrloading_appdomains_unloaded",
		Title:      "Unloaded application domains",
		Units:      "domain/s",
		Fam:        "loading",
		Ctx:        "netframework.clrloading_appdomains_unloaded",
		Priority:   prioNETFrameworkCLRLoadingAppDomainsUnloaded,
		Dims: module.Dims{
			{ID: "netframework_%s_clrloading_appdomains_unloaded_total", Name: "unloaded", Algo: module.Incremental},
		},
	}
	netFrameworkCLRLoadingAssembliesLoaded = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrloading_assemblies_loaded",
		Title:      "Loaded assemblies",
		Units:      "assemblies/s",
		Fam:        "loading",
		Ctx:        "netframework.clrloading_assemblies_loaded",
		Priority:   prioNETFrameworkCLRLoadingAssembliesLoaded,
		Dims: module.Dims{
			{ID: "netframework_%s_clrloading_assemblies_loaded_total", Name: "loaded", Algo: module.Incremental},
		},
	}
	netFrameworkCLRLoadingClassesLoaded = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrloading_classes_loaded",
		Title:      "Loaded classes in all assemblies",
		Units:      "classes/s",
		Fam:        "loading",
		Ctx:        "netframework.clrloading_classes_loaded",
		Priority:   prioNETFrameworkCLRLoadingClassesLoaded,
		Dims: module.Dims{
			{ID: "netframework_%s_clrloading_classes_loaded_total", Name: "loaded", Algo: module.Incremental},
		},
	}
	netFrameworkCLRLoadingClassLoadFailure = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrloading_class_load_failure",
		Title:      "Class load failures",
		Units:      "failures/s",
		Fam:        "loading",
		Ctx:        "netframework.clrloading_class_load_failures",
		Priority:   prioNETFrameworkCLRLoadingClassLoadFailure,
		Dims: module.Dims{
			{ID: "netframework_%s_clrloading_class_load_failures_total", Name: "class_load", Algo: module.Incremental},
		},
	}

	// Lock and Threads
	netFrameworkCLRLockAndThreadsQueueLength = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrlocksandthreads_queue_length",
		Title:      "Threads waited to acquire a managed lock",
		Units:      "threads/s",
		Fam:        "locks threads",
		Ctx:        "netframework.clrlocksandthreads_queue_length",
		Priority:   prioNETFrameworkCLRLocksAndThreadsQueueLength,
		Dims: module.Dims{
			{ID: "netframework_%s_clrlocksandthreads_queue_length_total", Name: "threads", Algo: module.Incremental},
		},
	}
	netFrameworkCLRLockAndThreadsCurrentLogicalThreads = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrlocksandthreads_current_logical_threads",
		Title:      "Logical threads",
		Units:      "threads",
		Fam:        "locks threads",
		Ctx:        "netframework.clrlocksandthreads_current_logical_threads",
		Priority:   prioNETFrameworkCLRLocksAndThreadsCurrentLogicalThreads,
		Dims: module.Dims{
			{ID: "netframework_%s_clrlocksandthreads_current_logical_threads", Name: "logical"},
		},
	}
	netFrameworkCLRLockAndThreadsCurrentPhysicalThreads = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrlocksandthreads_current_physical_threads",
		Title:      "Physical threads",
		Units:      "threads",
		Fam:        "locks threads",
		Ctx:        "netframework.clrlocksandthreads_current_physical_threads",
		Priority:   prioNETFrameworkCLRLocksAndThreadsCurrentPhysicalThreads,
		Dims: module.Dims{
			{ID: "netframework_%s_clrlocksandthreads_physical_threads_current", Name: "physical"},
		},
	}
	netFrameworkCLRLockAndThreadsRecognizedThreads = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrlocksandthreads_recognized_threads",
		Title:      "Threads recognized by the runtime",
		Units:      "threads/s",
		Fam:        "locks threads",
		Ctx:        "netframework.clrlocksandthreads_recognized_threads",
		Priority:   prioNETFrameworkCLRLocksAndThreadsRecognizedThreads,
		Dims: module.Dims{
			{ID: "netframework_%s_clrlocksandthreads_recognized_threads_total", Name: "threads", Algo: module.Incremental},
		},
	}
	netFrameworkCLRLockAndThreadsContentions = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrlocksandthreads_contentions",
		Title:      "Fails to acquire a managed lock",
		Units:      "contentions/s",
		Fam:        "locks threads",
		Ctx:        "netframework.clrlocksandthreads_contentions",
		Priority:   prioNETFrameworkCLRLocksAndThreadsContentions,
		Dims: module.Dims{
			{ID: "netframework_%s_clrlocksandthreads_contentions_total", Name: "contentions", Algo: module.Incremental},
		},
	}

	// Memory
	netFrameworkCLRMemoryAllocatedBytes = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_allocated_bytes",
		Title:      "Memory allocated on the garbage collection heap",
		Units:      "bytes/s",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_allocated_bytes",
		Priority:   prioNETFrameworkCLRMemoryAllocatedBytes,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_allocated_bytes_total", Name: "allocated", Algo: module.Incremental},
		},
	}
	netFrameworkCLRMemoryFinalizationSurvivors = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_finalization_survivors",
		Title:      "Objects that survived garbage-collection",
		Units:      "objects",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_finalization_survivors",
		Priority:   prioNETFrameworkCLRMemoryFinalizationSurvivors,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_finalization_survivors", Name: "survived"},
		},
	}
	netFrameworkCLRMemoryHeapSize = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_heap_size",
		Title:      "Maximum bytes that can be allocated",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_heap_size",
		Priority:   prioNETFrameworkCLRMemoryHeapSize,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_heap_size_bytes", Name: "heap"},
		},
	}
	netFrameworkCLRMemoryPromoted = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_promoted",
		Title:      "Memory promoted to the next generation",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_promoted",
		Priority:   prioNETFrameworkCLRMemoryPromoted,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_promoted_bytes", Name: "promoted"},
		},
	}
	netFrameworkCLRMemoryNumberGCHandles = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_number_gc_handles",
		Title:      "Garbage collection handles",
		Units:      "handles",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_number_gc_handles",
		Priority:   prioNETFrameworkCLRMemoryNumberGCHandles,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_number_gc_handles", Name: "used"},
		},
	}
	netFrameworkCLRMemoryCollections = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_collections",
		Title:      "Garbage collections",
		Units:      "gc/s",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_collections",
		Priority:   prioNETFrameworkCLRMemoryCollections,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_collections_total", Name: "gc", Algo: module.Incremental},
		},
	}
	netFrameworkCLRMemoryInducedGC = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_induced_gc",
		Title:      "Garbage collections induced",
		Units:      "gc/s",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_induced_gc",
		Priority:   prioNETFrameworkCLRMemoryInducedGC,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_induced_gc_total", Name: "gc", Algo: module.Incremental},
		},
	}
	netFrameworkCLRMemoryNumberPinnedObjects = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_number_pinned_objects",
		Title:      "Pinned objects encountered",
		Units:      "objects",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_number_pinned_objects",
		Priority:   prioNETFrameworkCLRMemoryNumberPinnedObjects,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_number_pinned_objects", Name: "pinned"},
		},
	}
	netFrameworkCLRMemoryNumberSinkBlocksInUse = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_number_sink_blocks_in_use",
		Title:      "Synchronization blocks in use",
		Units:      "blocks",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_number_sink_blocks_in_use",
		Priority:   prioNETFrameworkCLRMemoryNumberSinkBlocksInUse,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_number_sink_blocksinuse", Name: "used"},
		},
	}
	netFrameworkCLRMemoryCommitted = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_committed",
		Title:      "Virtual memory committed by GC",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_committed",
		Priority:   prioNETFrameworkCLRMemoryCommitted,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_committed_bytes", Name: "committed"},
		},
	}
	netFrameworkCLRMemoryReserved = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_reserved",
		Title:      "Virtual memory reserved by GC",
		Units:      "bytes",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_reserved",
		Priority:   prioNETFrameworkCLRMemoryReserved,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_reserved_bytes", Name: "reserved"},
		},
	}
	netFrameworkCLRMemoryGCTime = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrmemory_gc_time",
		Title:      "Time spent on GC",
		Units:      "percentage",
		Fam:        "memory",
		Ctx:        "netframework.clrmemory_gc_time",
		Priority:   prioNETFrameworkCLRMemoryGCTime,
		Dims: module.Dims{
			{ID: "netframework_%s_clrmemory_gc_time_percent", Name: "time"},
		},
	}

	// Remoting
	netFrameworkCLRRemotingChannels = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrremoting_channels",
		Title:      "Registered channels",
		Units:      "channels/s",
		Fam:        "remoting",
		Ctx:        "netframework.clrremoting_channels",
		Priority:   prioNETFrameworkCLRRemotingChannels,
		Dims: module.Dims{
			{ID: "netframework_%s_clrremoting_channels_total", Name: "registered", Algo: module.Incremental},
		},
	}
	netFrameworkCLRRemotingContextBoundClassesLoaded = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrremoting_context_bound_classes_loaded",
		Title:      "Loaded context-bound classes",
		Units:      "classes",
		Fam:        "remoting",
		Ctx:        "netframework.clrremoting_context_bound_classes_loaded",
		Priority:   prioNETFrameworkCLRRemotingContextBoundClassesLoaded,
		Dims: module.Dims{
			{ID: "netframework_%s_clrremoting_context_bound_classes_loaded", Name: "loaded"},
		},
	}
	netFrameworkCLRRemotingContextBoundObjects = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrremoting_context_bound_objects",
		Title:      "Allocated context-bound objects",
		Units:      "objects/s",
		Fam:        "remoting",
		Ctx:        "netframework.clrremoting_context_bound_objects",
		Priority:   prioNETFrameworkCLRRemotingContextBoundObjects,
		Dims: module.Dims{
			{ID: "netframework_%s_clrremoting_context_bound_objects_total", Name: "allocated", Algo: module.Incremental},
		},
	}
	netFrameworkCLRRemotingContextProxies = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrremoting_context_proxies",
		Title:      "Remoting proxy objects",
		Units:      "objects/s",
		Fam:        "remoting",
		Ctx:        "netframework.clrremoting_context_proxies",
		Priority:   prioNETFrameworkCLRRemotingContextProxies,
		Dims: module.Dims{
			{ID: "netframework_%s_clrremoting_context_proxies_total", Name: "objects", Algo: module.Incremental},
		},
	}
	netFrameworkCLRRemotingContexts = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrremoting_contexts",
		Title:      "Total of remoting contexts",
		Units:      "contexts",
		Fam:        "remoting",
		Ctx:        "netframework.clrremoting_contexts",
		Priority:   prioNETFrameworkCLRRemotingContexts,
		Dims: module.Dims{
			{ID: "netframework_%s_clrremoting_contexts", Name: "contexts"},
		},
	}
	netFrameworkCLRRemotingCalls = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrremoting_calls",
		Title:      "Remote Procedure Calls (RPC) invoked",
		Units:      "calls/s",
		Fam:        "remoting",
		Ctx:        "netframework.clrremoting_remote_calls",
		Priority:   prioNETFrameworkCLRRemotingRemoteCalls,
		Dims: module.Dims{
			{ID: "netframework_%s_clrremoting_remote_calls_total", Name: "rpc", Algo: module.Incremental},
		},
	}

	// Security
	netFrameworkCLRSecurityLinkTimeChecks = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrsecurity_link_time_checks",
		Title:      "Link-time code access security checks",
		Units:      "checks/s",
		Fam:        "security",
		Ctx:        "netframework.clrsecurity_link_time_checks",
		Priority:   prioNETFrameworkCLRSecurityLinkTimeChecks,
		Dims: module.Dims{
			{ID: "netframework_%s_clrsecurity_link_time_checks_total", Name: "linktime", Algo: module.Incremental},
		},
	}
	netFrameworkCLRSecurityChecksTime = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrsecurity_checks_time",
		Title:      "Time spent performing runtime code access security checks",
		Units:      "percentage",
		Fam:        "security",
		Ctx:        "netframework.clrsecurity_checks_time",
		Priority:   prioNETFrameworkCLRSecurityRTChecksTime,
		Dims: module.Dims{
			{ID: "netframework_%s_clrsecurity_checks_time_percent", Name: "time"},
		},
	}
	netFrameworkCLRSecurityStackWalkDepth = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrsecurity_stack_walk_depth",
		Title:      "Depth of the stack",
		Units:      "depth",
		Fam:        "security",
		Ctx:        "netframework.clrsecurity_stack_walk_depth",
		Priority:   prioNETFrameworkCLRSecurityStackWalkDepth,
		Dims: module.Dims{
			{ID: "netframework_%s_clrsecurity_stack_walk_depth", Name: "stack"},
		},
	}
	netFrameworkCLRSecurityRuntimeChecks = module.Chart{
		OverModule: "netframework",
		ID:         "netframework_%s_clrsecurity_runtime_checks",
		Title:      "Runtime code access security checks performed",
		Units:      "checks/s",
		Fam:        "security",
		Ctx:        "netframework.clrsecurity_runtime_checks",
		Priority:   prioNETFrameworkCLRSecurityRuntimeChecks,
		Dims: module.Dims{
			{ID: "netframework_%s_clrsecurity_runtime_checks_total", Name: "runtime", Algo: module.Incremental},
		},
	}
)

// Service
var (
	serviceChartsTmpl = module.Charts{
		serviceStateChartTmpl.Copy(),
		serviceStatusChartTmpl.Copy(),
	}
	serviceStateChartTmpl = module.Chart{
		ID:       "service_%s_state",
		Title:    "Service state",
		Units:    "state",
		Fam:      "services",
		Ctx:      "windows.service_state",
		Priority: prioServiceState,
		Dims: module.Dims{
			{ID: "service_%s_state_running", Name: "running"},
			{ID: "service_%s_state_stopped", Name: "stopped"},
			{ID: "service_%s_state_start_pending", Name: "start_pending"},
			{ID: "service_%s_state_stop_pending", Name: "stop_pending"},
			{ID: "service_%s_state_continue_pending", Name: "continue_pending"},
			{ID: "service_%s_state_pause_pending", Name: "pause_pending"},
			{ID: "service_%s_state_paused", Name: "paused"},
			{ID: "service_%s_state_unknown", Name: "unknown"},
		},
	}
	serviceStatusChartTmpl = module.Chart{
		ID:       "service_%s_status",
		Title:    "Service status",
		Units:    "status",
		Fam:      "services",
		Ctx:      "windows.service_status",
		Priority: prioServiceStatus,
		Dims: module.Dims{
			{ID: "service_%s_status_ok", Name: "ok"},
			{ID: "service_%s_status_error", Name: "error"},
			{ID: "service_%s_status_unknown", Name: "unknown"},
			{ID: "service_%s_status_degraded", Name: "degraded"},
			{ID: "service_%s_status_pred_fail", Name: "pred_fail"},
			{ID: "service_%s_status_starting", Name: "starting"},
			{ID: "service_%s_status_stopping", Name: "stopping"},
			{ID: "service_%s_status_service", Name: "service"},
			{ID: "service_%s_status_stressed", Name: "stressed"},
			{ID: "service_%s_status_nonrecover", Name: "nonrecover"},
			{ID: "service_%s_status_no_contact", Name: "no_contact"},
			{ID: "service_%s_status_lost_comm", Name: "lost_comm"},
		},
	}
)

// HyperV
var (
	hypervChartsTmpl = module.Charts{
		hypervVirtualMachinesHealthChart.Copy(),
		hypervRootPartitionDeviceSpacePagesChart.Copy(),
		hypervRootPartitionGPASpacePagesChart.Copy(),
		hypervRootPartitionGPASpaceModificationsChart.Copy(),
		hypervRootPartitionAttachedDevicesChart.Copy(),
		hypervRootPartitionDepositedPagesChart.Copy(),
		hypervRootPartitionSkippedInterrupts.Copy(),
		hypervRootPartitionDeviceDMAErrorsChart.Copy(),
		hypervRootPartitionDeviceInterruptErrorsChart.Copy(),
		hypervRootPartitionDeviceInterruptThrottledEventsChart.Copy(),
		hypervRootPartitionIOTlbFlushChart.Copy(),
		hypervRootPartitionAddressSpaceChart.Copy(),
		hypervRootPartitionVirtualTlbFlushEntries.Copy(),
		hypervRootPartitionVirtualTlbPages.Copy(),
	}
	hypervVirtualMachinesHealthChart = module.Chart{
		OverModule: "hyperv",
		ID:         "health_vm",
		Title:      "Virtual machines health status",
		Units:      "vms",
		Fam:        "vms health",
		Ctx:        "hyperv.vms_health",
		Priority:   prioHypervVMHealth,
		Type:       module.Stacked,
		Dims: module.Dims{
			{ID: "hyperv_health_ok", Name: "ok"},
			{ID: "hyperv_health_critical", Name: "critical"},
		},
	}
	hypervRootPartitionDeviceSpacePagesChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_device_space_pages",
		Title:      "Root partition pages in the device space",
		Units:      "pages",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_device_space_pages",
		Priority:   prioHypervRootPartitionDeviceSpacePages,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_4K_device_pages", Name: "4K"},
			{ID: "hyperv_root_partition_2M_device_pages", Name: "2M"},
			{ID: "hyperv_root_partition_1G_device_pages", Name: "1G"},
		},
	}
	hypervRootPartitionGPASpacePagesChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_gpa_space_pages",
		Title:      "Root partition pages in the GPA space",
		Units:      "pages",
		Fam:        "root partition",
		Ctx:        "windows.hyperv_root_partition_gpa_space_pages",
		Priority:   prioHypervRootPartitionGPASpacePages,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_4K_gpa_pages", Name: "4K"},
			{ID: "hyperv_root_partition_2M_gpa_pages", Name: "2M"},
			{ID: "hyperv_root_partition_1G_gpa_pages", Name: "1G"},
		},
	}
	hypervRootPartitionGPASpaceModificationsChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_gpa_space_modifications",
		Title:      "Root partition GPA space modifications",
		Units:      "modifications/s",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_gpa_space_modifications",
		Priority:   prioHypervRootPartitionGPASpaceModifications,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_gpa_space_modifications", Name: "gpa", Algo: module.Incremental},
		},
	}
	hypervRootPartitionAttachedDevicesChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_attached_devices",
		Title:      "Root partition attached devices",
		Units:      "devices",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_attached_devices",
		Priority:   prioHypervRootPartitionAttachedDevices,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_attached_devices", Name: "attached"},
		},
	}
	hypervRootPartitionDepositedPagesChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_deposited_pages",
		Title:      "Root partition deposited pages",
		Units:      "pages",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_deposited_pages",
		Priority:   prioHypervRootPartitionDepositedPages,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_deposited_pages", Name: "deposited"},
		},
	}
	hypervRootPartitionSkippedInterrupts = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_skipped_interrupts",
		Title:      "Root partition skipped interrupts",
		Units:      "interrupts",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_skipped_interrupts",
		Priority:   prioHypervRootPartitionSkippedInterrupts,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_physical_pages_allocated", Name: "skipped"},
		},
	}
	hypervRootPartitionDeviceDMAErrorsChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_device_dma_errors",
		Title:      "Root partition illegal DMA requests",
		Units:      "requests",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_device_dma_errors",
		Priority:   prioHypervRootPartitionDeviceDMAErrors,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_deposited_pages", Name: "illegal_dma"},
		},
	}
	hypervRootPartitionDeviceInterruptErrorsChart = module.Chart{
		OverModule: "hyperv",
		ID:         "partition_device_interrupt_errors",
		Title:      "Root partition illegal interrupt requests",
		Units:      "requests",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_device_interrupt_errors",
		Priority:   prioHypervRootPartitionDeviceInterruptErrors,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_device_interrupt_errors", Name: "illegal_interrupt"},
		},
	}
	hypervRootPartitionDeviceInterruptThrottledEventsChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_device_interrupt_throttle_events",
		Title:      "Root partition throttled interrupts",
		Units:      "events",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_device_interrupt_throttle_events",
		Priority:   prioHypervRootPartitionDeviceInterruptThrottleEvents,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_device_interrupt_throttle_events", Name: "throttling"},
		},
	}
	hypervRootPartitionIOTlbFlushChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_io_tbl_flush",
		Title:      "Root partition flushes of I/O TLBs",
		Units:      "flushes/s",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_io_tlb_flush",
		Priority:   prioHypervRootPartitionIOTlbFlush,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_io_tlb_flush", Name: "flushes", Algo: module.Incremental},
		},
	}
	hypervRootPartitionAddressSpaceChart = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_address_space",
		Title:      "Root partition address spaces in the virtual TLB",
		Units:      "address spaces",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_address_space",
		Priority:   prioHypervRootPartitionAddressSpace,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_address_spaces", Name: "address_spaces"},
		},
	}
	hypervRootPartitionVirtualTlbFlushEntries = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_virtual_tbl_flush_entries",
		Title:      "Root partition flushes of the entire virtual TLB",
		Units:      "flushes/s",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_virtual_tlb_flush_entries",
		Priority:   prioHypervRootPartitionVirtualTlbFlushEntires,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_virtual_tlb_flush_entires", Name: "flushes", Algo: module.Incremental},
		},
	}
	hypervRootPartitionVirtualTlbPages = module.Chart{
		OverModule: "hyperv",
		ID:         "root_partition_virtual_tlb_pages",
		Title:      "Root partition pages used by the virtual TLB",
		Units:      "pages",
		Fam:        "root partition",
		Ctx:        "hyperv.root_partition_virtual_tlb_pages",
		Priority:   prioHypervRootPartitionVirtualTlbPages,
		Dims: module.Dims{
			{ID: "hyperv_root_partition_virtual_tlb_pages", Name: "used"},
		},
	}
)

// HyperV VM Memory
var (
	hypervVMChartsTemplate = module.Charts{
		hypervHypervVMCPUUsageChartTmpl.Copy(),
		hypervHypervVMMemoryPhysicalChartTmpl.Copy(),
		hypervHypervVMMemoryPhysicalGuestVisibleChartTmpl.Copy(),
		hypervHypervVMMemoryPressureCurrentChartTmpl.Copy(),
		hypervVIDPhysicalPagesAllocatedChartTmpl.Copy(),
		hypervVIDRemotePhysicalPagesChartTmpl.Copy(),
	}
	hypervHypervVMCPUUsageChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_%s_cpu_usage",
		Title:      "VM CPU usage (100% = 1 core)",
		Units:      "percentage",
		Fam:        "vm cpu",
		Ctx:        "hyperv.vm_cpu_usage",
		Priority:   prioHypervVMCPUUsage,
		Type:       module.Stacked,
		Dims: module.Dims{
			{ID: "hyperv_vm_%s_cpu_guest_run_time", Name: "guest", Div: 1e5, Algo: module.Incremental},
			{ID: "hyperv_vm_%s_cpu_hypervisor_run_time", Name: "hypervisor", Div: 1e5, Algo: module.Incremental},
			{ID: "hyperv_vm_%s_cpu_remote_run_time", Name: "remote", Div: 1e5, Algo: module.Incremental},
		},
	}
	hypervHypervVMMemoryPhysicalChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_%s_memory_physical",
		Title:      "VM assigned memory",
		Units:      "MiB",
		Fam:        "vm mem",
		Ctx:        "hyperv.vm_memory_physical",
		Priority:   prioHypervVMMemoryPhysical,
		Dims: module.Dims{
			{ID: "hyperv_vm_%s_memory_physical", Name: "assigned_memory"},
		},
	}
	hypervHypervVMMemoryPhysicalGuestVisibleChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_%s_memory_physical_guest_visible",
		Title:      "VM guest visible memory",
		Units:      "MiB",
		Fam:        "vm mem",
		Ctx:        "hyperv.vm_memory_physical_guest_visible",
		Priority:   prioHypervVMMemoryPhysicalGuestVisible,
		Dims: module.Dims{
			{ID: "hyperv_vm_%s_memory_physical_guest_visible", Name: "visible_memory"},
		},
	}
	hypervHypervVMMemoryPressureCurrentChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_%s_memory_pressure_current",
		Title:      "VM current pressure",
		Units:      "percentage",
		Fam:        "vm mem",
		Ctx:        "hyperv.vm_memory_pressure_current",
		Priority:   prioHypervVMMemoryPressureCurrent,
		Dims: module.Dims{
			{ID: "hyperv_vm_%s_memory_pressure_current", Name: "pressure"},
		},
	}
	hypervVIDPhysicalPagesAllocatedChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_%s_vid_physical_pages_allocated",
		Title:      "VM physical pages allocated",
		Units:      "pages",
		Fam:        "vm mem",
		Ctx:        "hyperv.vm_vid_physical_pages_allocated",
		Priority:   prioHypervVIDPhysicalPagesAllocated,
		Dims: module.Dims{
			{ID: "hyperv_vid_%s_physical_pages_allocated", Name: "allocated"},
		},
	}
	hypervVIDRemotePhysicalPagesChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_%s_remote_physical_pages",
		Title:      "VM physical pages not allocated from the preferred NUMA node",
		Units:      "pages",
		Fam:        "vm mem",
		Ctx:        "hyperv.vm_vid_remote_physical_pages",
		Priority:   prioHypervVIDRemotePhysicalPages,
		Dims: module.Dims{
			{ID: "hyperv_vid_%s_remote_physical_pages", Name: "remote_physical"},
		},
	}
)

// HyperV VM storage device
var (
	hypervVMDeviceChartsTemplate = module.Charts{
		hypervVMDeviceIOChartTmpl.Copy(),
		hypervVMDeviceIOPSChartTmpl.Copy(),
		hypervVMDeviceErrorCountChartTmpl.Copy(),
	}
	hypervVMDeviceIOChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_device_%s_bytes_read",
		Title:      "VM storage device IO",
		Units:      "bytes/s",
		Fam:        "vm disk",
		Ctx:        "hyperv.vm_device_bytes",
		Priority:   prioHypervVMDeviceBytes,
		Type:       module.Area,
		Dims: module.Dims{
			{ID: "hyperv_vm_device_%s_bytes_read", Name: "read", Algo: module.Incremental},
			{ID: "hyperv_vm_device_%s_bytes_written", Name: "written", Algo: module.Incremental},
		},
	}
	hypervVMDeviceIOPSChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_device_%s_operation_read",
		Title:      "VM storage device IOPS",
		Units:      "operations/s",
		Fam:        "vm disk",
		Ctx:        "hyperv.vm_device_operations",
		Priority:   prioHypervVMDeviceOperations,
		Dims: module.Dims{
			{ID: "hyperv_vm_device_%s_operations_read", Name: "read", Algo: module.Incremental},
			{ID: "hyperv_vm_device_%s_operations_written", Name: "write", Algo: module.Incremental},
		},
	}
	hypervVMDeviceErrorCountChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_device_%s_error_count",
		Title:      "VM storage device errors",
		Units:      "errors/s",
		Fam:        "vm disk",
		Ctx:        "hyperv.vm_device_errors",
		Priority:   prioHypervVMDeviceErrors,
		Dims: module.Dims{
			{ID: "hyperv_vm_device_%s_error_count", Name: "errors", Algo: module.Incremental},
		},
	}
)

// HyperV VM network interface
var (
	hypervVMInterfaceChartsTemplate = module.Charts{
		hypervVMInterfaceTrafficChartTmpl.Copy(),
		hypervVMInterfacePacketsChartTmpl.Copy(),
		hypervVMInterfacePacketsDroppedChartTmpl.Copy(),
	}

	hypervVMInterfaceTrafficChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_interface_%s_bytes",
		Title:      "VM interface traffic",
		Units:      "bytes/s",
		Fam:        "vm net",
		Ctx:        "hyperv.vm_interface_bytes",
		Priority:   prioHypervVMInterfaceBytes,
		Type:       module.Area,
		Dims: module.Dims{
			{ID: "hyperv_vm_interface_%s_bytes_received", Name: "received", Algo: module.Incremental},
			{ID: "hyperv_vm_interface_%s_bytes_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVMInterfacePacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_interface_%s_packets",
		Title:      "VM interface packets",
		Units:      "packets/s",
		Fam:        "vm net",
		Ctx:        "hyperv.vm_interface_packets",
		Priority:   prioHypervVMInterfacePackets,
		Dims: module.Dims{
			{ID: "hyperv_vm_interface_%s_packets_received", Name: "received", Algo: module.Incremental},
			{ID: "hyperv_vm_interface_%s_packets_sent", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVMInterfacePacketsDroppedChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vm_interface_%s_packets_dropped",
		Title:      "VM interface packets dropped",
		Units:      "drops/s",
		Fam:        "vm net",
		Ctx:        "hyperv.vm_interface_packets_dropped",
		Priority:   prioHypervVMInterfacePacketsDropped,
		Dims: module.Dims{
			{ID: "hyperv_vm_interface_%s_packets_incoming_dropped", Name: "incoming", Algo: module.Incremental},
			{ID: "hyperv_vm_interface_%s_packets_outgoing_dropped", Name: "outgoing", Algo: module.Incremental},
		},
	}
)

// HyperV Virtual Switch
var (
	hypervVswitchChartsTemplate = module.Charts{
		hypervVswitchTrafficChartTmpl.Copy(),
		hypervVswitchPacketsChartTmpl.Copy(),
		hypervVswitchDirectedPacketsChartTmpl.Copy(),
		hypervVswitchBroadcastPacketsChartTmpl.Copy(),
		hypervVswitchMulticastPacketsChartTmpl.Copy(),
		hypervVswitchDroppedPacketsChartTmpl.Copy(),
		hypervVswitchExtensionDroppedPacketsChartTmpl.Copy(),
		hypervVswitchPacketsFloodedTotalChartTmpl.Copy(),
		hypervVswitchLearnedMACAddressChartTmpl.Copy(),
		hypervVswitchPurgedMACAddressChartTmpl.Copy(),
	}

	hypervVswitchTrafficChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_traffic",
		Title:      "Virtual switch traffic",
		Units:      "bytes/s",
		Fam:        "vswitch traffic",
		Ctx:        "hyperv.vswitch_bytes",
		Priority:   prioHypervVswitchTrafficTotal,
		Type:       module.Area,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_bytes_received_total", Name: "received", Algo: module.Incremental},
			{ID: "hyperv_vswitch_%s_bytes_sent_total", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVswitchPacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_packets",
		Title:      "Virtual switch packets",
		Units:      "packets/s",
		Fam:        "vswitch packets",
		Ctx:        "hyperv.vswitch_packets",
		Priority:   prioHypervVswitchPackets,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_packets_received_total", Name: "received", Algo: module.Incremental},
			// FIXME: https://github.com/prometheus-community/windows_exporter/pull/1201
			//{ID: "hyperv_vswitch_%s_packets_sent_total", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVswitchDirectedPacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_directed_packets",
		Title:      "Virtual switch directed packets",
		Units:      "packets/s",
		Fam:        "vswitch packets",
		Ctx:        "hyperv.vswitch_directed_packets",
		Priority:   prioHypervVswitchDirectedPackets,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_directed_packets_received_total", Name: "received", Algo: module.Incremental},
			{ID: "hyperv_vswitch_%s_directed_packets_send_total", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVswitchBroadcastPacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_broadcast_packets",
		Title:      "Virtual switch broadcast packets",
		Units:      "packets/s",
		Fam:        "vswitch packets",
		Ctx:        "hyperv.vswitch_broadcast_packets",
		Priority:   prioHypervVswitchBroadcastPackets,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_broadcast_packets_received_total", Name: "received", Algo: module.Incremental},
			{ID: "hyperv_vswitch_%s_broadcast_packets_sent_total", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVswitchMulticastPacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_multicast_packets",
		Title:      "Virtual switch multicast packets",
		Units:      "packets/s",
		Fam:        "vswitch packets",
		Ctx:        "hyperv.vswitch_multicast_packets",
		Priority:   prioHypervVswitchMulticastPackets,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_multicast_packets_received_total", Name: "received", Algo: module.Incremental},
			{ID: "hyperv_vswitch_%s_multicast_packets_sent_total", Name: "sent", Algo: module.Incremental},
		},
	}
	hypervVswitchDroppedPacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_dropped_packets",
		Title:      "Virtual switch dropped packets",
		Units:      "drops/s",
		Fam:        "vswitch drops",
		Ctx:        "hyperv.vswitch_dropped_packets",
		Priority:   prioHypervVswitchDroppedPackets,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_dropped_packets_incoming_total", Name: "incoming", Algo: module.Incremental},
			{ID: "hyperv_vswitch_%s_dropped_packets_outcoming_total", Name: "outgoing", Algo: module.Incremental},
		},
	}
	hypervVswitchExtensionDroppedPacketsChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_extensions_dropped_packets_incoming",
		Title:      "Virtual switch extensions dropped packets",
		Units:      "drops/s",
		Fam:        "vswitch drops",
		Ctx:        "hyperv.vswitch_extensions_dropped_packets",
		Priority:   prioHypervVswitchExtensionsDroppedPackets,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_extensions_dropped_packets_incoming_total", Name: "incoming", Algo: module.Incremental},
			{ID: "hyperv_vswitch_%s_extensions_dropped_packets_outcoming_total", Name: "outgoing", Algo: module.Incremental},
		},
	}
	hypervVswitchPacketsFloodedTotalChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_packets_flooded",
		Title:      "Virtual switch flooded packets",
		Units:      "packets/s",
		Fam:        "vswitch flood",
		Ctx:        "hyperv.vswitch_packets_flooded",
		Priority:   prioHypervVswitchPacketsFlooded,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_packets_flooded_total", Name: "flooded", Algo: module.Incremental},
		},
	}
	hypervVswitchLearnedMACAddressChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_learned_mac_addresses",
		Title:      "Virtual switch learned MAC addresses",
		Units:      "mac addresses/s",
		Fam:        "vswitch mac addresses",
		Ctx:        "hyperv.vswitch_learned_mac_addresses",
		Priority:   prioHypervVswitchLearnedMACAddresses,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_learned_mac_addresses_total", Name: "learned", Algo: module.Incremental},
		},
	}
	hypervVswitchPurgedMACAddressChartTmpl = module.Chart{
		OverModule: "hyperv",
		ID:         "vswitch_%s_purged_mac_addresses",
		Title:      "Virtual switch purged MAC addresses",
		Units:      "mac addresses/s",
		Fam:        "vswitch mac addresses",
		Ctx:        "hyperv.vswitch_purged_mac_addresses",
		Priority:   prioHypervVswitchPurgeMACAddress,
		Dims: module.Dims{
			{ID: "hyperv_vswitch_%s_purged_mac_addresses_total", Name: "purged", Algo: module.Incremental},
		},
	}
)

// Collectors
var (
	collectorChartsTmpl = module.Charts{
		collectorDurationChartTmpl.Copy(),
		collectorStatusChartTmpl.Copy(),
	}
	collectorDurationChartTmpl = module.Chart{
		ID:       "collector_%s_duration",
		Title:    "Duration of a data collection",
		Units:    "seconds",
		Fam:      "collection",
		Ctx:      "windows.collector_duration",
		Priority: prioCollectorDuration,
		Dims: module.Dims{
			{ID: "collector_%s_duration", Name: "duration", Div: precision},
		},
	}
	collectorStatusChartTmpl = module.Chart{
		ID:       "collector_%s_status",
		Title:    "Status of a data collection",
		Units:    "status",
		Fam:      "collection",
		Ctx:      "windows.collector_status",
		Priority: prioCollectorStatus,
		Dims: module.Dims{
			{ID: "collector_%s_status_success", Name: "success"},
			{ID: "collector_%s_status_fail", Name: "fail"},
		},
	}
)

func (c *Collector) addCPUCharts() {
	charts := cpuCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addCPUCoreCharts(core string) {
	charts := cpuCoreChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, core)
		chart.Labels = []module.Label{
			{Key: "core", Value: core},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, core)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeCPUCoreCharts(core string) {
	px := fmt.Sprintf("cpu_core_%s", core)
	c.removeCharts(px)
}

func (c *Collector) addMemoryCharts() {
	charts := memCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addDiskCharts(disk string) {
	charts := diskChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, disk)
		chart.Labels = []module.Label{
			{Key: "disk", Value: disk},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, disk)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDiskCharts(disk string) {
	px := fmt.Sprintf("logical_disk_%s", disk)
	c.removeCharts(px)
}

func (c *Collector) addNICCharts(nic string) {
	charts := nicChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, nic)
		chart.Labels = []module.Label{
			{Key: "nic", Value: nic},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, nic)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeNICCharts(nic string) {
	px := fmt.Sprintf("nic_%s", nic)
	c.removeCharts(px)
}

func (c *Collector) addTCPCharts() {
	charts := tcpCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addOSCharts() {
	charts := osCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addSystemCharts() {
	charts := systemCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addLogonCharts() {
	charts := logonCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addADFSCharts() {
	charts := adfsCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addExchangeCharts() {
	charts := exchangeCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addExchangeWorkloadCharts(name string) {
	charts := exchangeWorkloadChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "workload", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeExchangeWorkloadCharts(name string) {
	px := fmt.Sprintf("exchange_workload_%s", name)
	c.removeCharts(px)
}

func (c *Collector) addExchangeLDAPCharts(name string) {
	charts := exchangeLDAPChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "ldap_process", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeExchangeLDAPCharts(name string) {
	px := fmt.Sprintf("exchange_ldap_%s", name)
	c.removeCharts(px)
}

func (c *Collector) addExchangeHTTPProxyCharts(name string) {
	charts := exchangeHTTPProxyChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "http_proxy", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeExchangeHTTPProxyCharts(name string) {
	px := fmt.Sprintf("exchange_http_proxy_%s", name)
	c.removeCharts(px)
}

func (c *Collector) addThermalZoneCharts(zone string) {
	charts := thermalzoneChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, zone)
		chart.Labels = []module.Label{
			{Key: "thermalzone", Value: zone},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, zone)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeThermalZoneCharts(zone string) {
	px := fmt.Sprintf("thermalzone_%s", zone)
	c.removeCharts(px)
}

func (c *Collector) addIISWebsiteCharts(website string) {
	charts := iisWebsiteChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, website)
		chart.Labels = []module.Label{
			{Key: "website", Value: website},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, website)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeIIWebsiteSCharts(website string) {
	px := fmt.Sprintf("iis_website_%s", website)
	c.removeCharts(px)
}

func (c *Collector) addMSSQLDBCharts(instance string, dbname string) {
	charts := mssqlDatabaseChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dbname, instance)
		chart.Labels = []module.Label{
			{Key: "mssql_instance", Value: instance},
			{Key: "database", Value: dbname},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dbname, instance)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeMSSQLDBCharts(instance string, dbname string) {
	px := fmt.Sprintf("mssql_db_%s_instance_%s", dbname, instance)
	c.removeCharts(px)
}

func (c *Collector) addMSSQLInstanceCharts(instance string) {
	charts := mssqlInstanceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, instance)
		chart.Labels = []module.Label{
			{Key: "mssql_instance", Value: instance},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, instance)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeMSSQLInstanceCharts(instance string) {
	px := fmt.Sprintf("mssql_instance_%s", instance)
	c.removeCharts(px)
}

func (c *Collector) addProcessesCharts() {
	charts := processesCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addADCharts() {
	charts := adCharts.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addCertificateTemplateCharts(template string) {
	charts := adcsCertTemplateChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, template)
		chart.Labels = []module.Label{
			{Key: "cert_template", Value: template},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, template)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeCertificateTemplateCharts(template string) {
	px := fmt.Sprintf("adcs_cert_template_%s", template)
	c.removeCharts(px)
}

func (c *Collector) addProcessToCharts(procID string) {
	for _, chart := range *c.Charts() {
		var dim *module.Dim
		switch chart.ID {
		case processesCPUUtilizationTotalChart.ID:
			id := fmt.Sprintf("process_%s_cpu_time", procID)
			dim = &module.Dim{ID: id, Name: procID, Algo: module.Incremental, Div: 1000, Mul: 100}
			if procID == "Idle" {
				dim.Hidden = true
			}
		case processesMemoryUsageChart.ID:
			id := fmt.Sprintf("process_%s_working_set_private_bytes", procID)
			dim = &module.Dim{ID: id, Name: procID}
		case processesIOBytesChart.ID:
			id := fmt.Sprintf("process_%s_io_bytes", procID)
			dim = &module.Dim{ID: id, Name: procID, Algo: module.Incremental}
		case processesIOOperationsChart.ID:
			id := fmt.Sprintf("process_%s_io_operations", procID)
			dim = &module.Dim{ID: id, Name: procID, Algo: module.Incremental}
		case processesPageFaultsChart.ID:
			id := fmt.Sprintf("process_%s_page_faults", procID)
			dim = &module.Dim{ID: id, Name: procID, Algo: module.Incremental}
		case processesPageFileBytes.ID:
			id := fmt.Sprintf("process_%s_page_file_bytes", procID)
			dim = &module.Dim{ID: id, Name: procID}
		case processesThreads.ID:
			id := fmt.Sprintf("process_%s_threads", procID)
			dim = &module.Dim{ID: id, Name: procID}
		case processesHandlesChart.ID:
			id := fmt.Sprintf("process_%s_handles", procID)
			dim = &module.Dim{ID: id, Name: procID}
		default:
			continue
		}

		if err := chart.AddDim(dim); err != nil {
			c.Warning(err)
			continue
		}
		chart.MarkNotCreated()
	}
}

func (c *Collector) removeProcessFromCharts(procID string) {
	for _, chart := range *c.Charts() {
		var id string
		switch chart.ID {
		case processesCPUUtilizationTotalChart.ID:
			id = fmt.Sprintf("process_%s_cpu_time", procID)
		case processesMemoryUsageChart.ID:
			id = fmt.Sprintf("process_%s_working_set_private_bytes", procID)
		case processesIOBytesChart.ID:
			id = fmt.Sprintf("process_%s_io_bytes", procID)
		case processesIOOperationsChart.ID:
			id = fmt.Sprintf("process_%s_io_operations", procID)
		case processesPageFaultsChart.ID:
			id = fmt.Sprintf("process_%s_page_faults", procID)
		case processesPageFileBytes.ID:
			id = fmt.Sprintf("process_%s_page_file_bytes", procID)
		case processesThreads.ID:
			id = fmt.Sprintf("process_%s_threads", procID)
		case processesHandlesChart.ID:
			id = fmt.Sprintf("process_%s_handles", procID)
		default:
			continue
		}

		if err := chart.MarkDimRemove(id, false); err != nil {
			c.Warning(err)
			continue
		}
		chart.MarkNotCreated()
	}
}

func (c *Collector) addProcessNetFrameworkExceptionsCharts(procName string) {
	charts := netFrameworkCLRExceptionsChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessFromNetFrameworkExceptionsCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrexception", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkInteropCharts(procName string) {
	charts := netFrameworkCLRInteropChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkInteropCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrinterop", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkJITCharts(procName string) {
	charts := netFrameworkCLRJITChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkJITCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrjit", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkLoadingCharts(procName string) {
	charts := netFrameworkCLRLoadingChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkLoadingCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrloading", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkLocksAndThreadsCharts(procName string) {
	charts := netFrameworkCLRLocksAndThreadsChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkLocksAndThreadsCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrlocksandthreads", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkMemoryCharts(procName string) {
	charts := netFrameworkCLRMemoryChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkMemoryCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrmemory", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkRemotingCharts(procName string) {
	charts := netFrameworkCLRRemotingChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkRemotingCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrremoting", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addProcessNetFrameworkSecurityCharts(procName string) {
	charts := netFrameworkCLRSecurityChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, strings.ToLower(procName))
		chart.Labels = []module.Label{
			{Key: "process", Value: procName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, procName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProcessNetFrameworkSecurityCharts(procName string) {
	px := fmt.Sprintf("netframework_%s_clrsecurity", strings.ToLower(procName))
	c.removeCharts(px)
}

func (c *Collector) addServiceCharts(svc string) {
	charts := serviceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, svc)
		chart.Labels = []module.Label{
			{Key: "service", Value: svc},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, svc)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeServiceCharts(svc string) {
	px := fmt.Sprintf("service_%s", svc)
	c.removeCharts(px)
}

func (c *Collector) addCollectorCharts(name string) {
	charts := collectorChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "collector", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addHypervCharts() {
	charts := hypervChartsTmpl.Copy()

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addHypervVMCharts(vm string) {
	charts := hypervVMChartsTemplate.Copy()
	n := hypervCleanName(vm)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, n)
		chart.Labels = []module.Label{
			{Key: "vm_name", Value: vm},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHypervVMCharts(vm string) {
	px := fmt.Sprintf("vm_%s", hypervCleanName(vm))
	c.removeCharts(px)
}

func (c *Collector) addHypervVMDeviceCharts(device string) {
	charts := hypervVMDeviceChartsTemplate.Copy()
	n := hypervCleanName(device)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, n)
		chart.Labels = []module.Label{
			{Key: "vm_device", Value: device},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHypervVMDeviceCharts(device string) {
	px := fmt.Sprintf("vm_device_%s", hypervCleanName(device))
	c.removeCharts(px)
}

func (c *Collector) addHypervVMInterfaceCharts(iface string) {
	charts := hypervVMInterfaceChartsTemplate.Copy()
	n := hypervCleanName(iface)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, n)
		chart.Labels = []module.Label{
			{Key: "vm_interface", Value: iface},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHypervVMInterfaceCharts(iface string) {
	px := fmt.Sprintf("vm_interface_%s", hypervCleanName(iface))
	c.removeCharts(px)
}

func (c *Collector) addHypervVSwitchCharts(vswitch string) {
	charts := hypervVswitchChartsTemplate.Copy()
	n := hypervCleanName(vswitch)

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, n)
		chart.Labels = []module.Label{
			{Key: "vswitch", Value: vswitch},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeHypervVSwitchCharts(vswitch string) {
	px := fmt.Sprintf("vswitch_%s", hypervCleanName(vswitch))
	c.removeCharts(px)
}

func (c *Collector) removeCollectorCharts(name string) {
	px := fmt.Sprintf("collector_%s", name)
	c.removeCharts(px)
}

func (c *Collector) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
