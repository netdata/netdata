// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Precision for floating point values
const precision = 1000

// Priority constants for chart ordering
const (
	// System metrics (1000-1999) - Most important, shown first
	prioSystemCPU        = 1000
	prioSystemMemory     = 1100
	prioSystemGC         = 1200
	prioSystemUptime     = 1300
	prioSystemJVM        = 1400
	
	// Connection pools (2000-2999) - Critical for app health
	prioJDBCPools        = 2000
	prioJCAPools         = 2200
	prioJMSAdapter       = 2400
	prioTCPChannels      = 2600
	
	// Web components (3000-3999) - User-facing metrics
	prioServlets         = 3100
	prioWebServlets      = 3100  // Alias
	prioSessions         = 3200
	prioWebSessions      = 3200  // Alias
	prioWebContainers    = 3300
	prioWebApps          = 3300  // Alias
	prioPortlets         = 3500
	prioWebPortlets      = 3500  // Alias
	prioURLs             = 3600
	prioEnterpriseApps   = 3700
	prioEJBContainer     = 3800
	prioConnectionMgr    = 3900
	
	// Transactions (4000-4999) - Business operations
	prioTransactionManager = 4000
	
	// Thread management (5000-5999) - Internal resources
	prioThreadPools      = 5000
	prioObjectPools      = 5100
	
	// Caching (5200-5399) - Performance optimization
	prioDynaCache        = 5200
	prioCacheManager     = 5200  // Alias for dynamic cache
	prioObjectCache      = 5300
	
	// ORB/EJB (6000-6099) - Enterprise components
	prioORB              = 6000
	
	// Web Services (6100-6199)
	prioWebServices      = 6100
	
	// Interceptors (6200-6299)
	prioInterceptors     = 6220
	
	// Security (7000-7999)
	prioSecurityAuth     = 7000
	prioSecurityAuthz    = 7100
	
	// High availability (8000-8099)
	prioHAManager        = 8000
	
	// Extensions (8100-8199)
	prioExtensionRegistry = 8100
	prioRegistry         = 8100  // Alias for extension registry
	
	// System data (8200-8299)
	prioSystemData       = 8200
	
	// WLM (8300-8399)
	prioWLM              = 8300
	
	// Connection Manager (8400-8499)
	prioConnectionManager = 8400
	
	// Security (already defined)
	prioSecurity = prioSecurityAuth
	
	// SIB Messaging (8300-8399)
	prioSIBMessaging     = 8300
	
	// Components (10000+) - Application-specific
	prioComponents       = 10000
	
	// Monitoring (catch-all)
	prioMonitoring       = 79000
)

// Base charts - required by framework
var baseCharts = module.Charts{}

// Chart templates based on ACTUAL WebSphere PMI metrics

var transactionChartsTmpl = module.Charts{
	{
		ID:       "transaction_manager_%s",
		Title:    "Transaction Manager",
		Units:    "transactions/s",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager",
		Type:     module.Line,
		Priority: prioTransactionManager,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_global_begun", Name: "global_begun", Algo: module.Incremental},
			{ID: "transaction_manager_%s_global_committed", Name: "global_committed", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_begun", Name: "local_begun", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_committed", Name: "local_committed", Algo: module.Incremental},
			{ID: "transaction_manager_%s_global_rolled_back", Name: "global_rolled_back", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_rolled_back", Name: "local_rolled_back", Algo: module.Incremental},
			{ID: "transaction_manager_%s_global_timeout", Name: "global_timeout", Algo: module.Incremental},
			{ID: "transaction_manager_%s_local_timeout", Name: "local_timeout", Algo: module.Incremental},
			{ID: "transaction_manager_%s_GlobalInvolvedCount", Name: "global_involved", Algo: module.Incremental},
			{ID: "transaction_manager_%s_OptimizationCount", Name: "optimizations", Algo: module.Incremental},
		},
	},
	{
		ID:       "transaction_manager_%s_active",
		Title:    "Transaction Manager Active Transactions",
		Units:    "transactions",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager_active",
		Type:     module.Line,
		Priority: prioTransactionManager + 10,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_ActiveCount", Name: "global_active"},
			{ID: "transaction_manager_%s_LocalActiveCount", Name: "local_active"},
		},
	},
	// DEPRECATED: Old TimeStatistic charts - replaced by smart processor
	// These charts mixed total, count, min, max, and mean in single charts
	// Now replaced by separate rate, current_latency, and lifetime_latency charts
	/*
	{
		ID:       "transaction_manager_%s_global_times",
		Title:    "Transaction Manager Global Transaction Times",
		Units:    "milliseconds/s",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager_global_times",
		Type:     module.Line,
		Priority: prioTransactionManager + 20,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_GlobalTranTime_total", Name: "transaction_time", Algo: module.Incremental},
			{ID: "transaction_manager_%s_GlobalPrepareTime_total", Name: "prepare_time", Algo: module.Incremental},
			{ID: "transaction_manager_%s_GlobalCommitTime_total", Name: "commit_time", Algo: module.Incremental},
			{ID: "transaction_manager_%s_GlobalBeforeCompletionTime_total", Name: "before_completion_time", Algo: module.Incremental},
			// Hidden dimensions for count, min, max
			{ID: "transaction_manager_%s_GlobalTranTime_count", Name: "transaction_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalTranTime_min", Name: "transaction_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalTranTime_max", Name: "transaction_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalPrepareTime_count", Name: "prepare_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalPrepareTime_min", Name: "prepare_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalPrepareTime_max", Name: "prepare_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalCommitTime_count", Name: "commit_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalCommitTime_min", Name: "commit_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalCommitTime_max", Name: "commit_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalBeforeCompletionTime_count", Name: "before_completion_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalBeforeCompletionTime_min", Name: "before_completion_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_GlobalBeforeCompletionTime_max", Name: "before_completion_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "transaction_manager_%s_local_times",
		Title:    "Transaction Manager Local Transaction Times",
		Units:    "milliseconds/s",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager_local_times",
		Type:     module.Line,
		Priority: prioTransactionManager + 30,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_LocalTranTime_total", Name: "transaction_time", Algo: module.Incremental},
			{ID: "transaction_manager_%s_LocalCommitTime_total", Name: "commit_time", Algo: module.Incremental},
			{ID: "transaction_manager_%s_LocalBeforeCompletionTime_total", Name: "before_completion_time", Algo: module.Incremental},
			// Hidden dimensions for count, min, max
			{ID: "transaction_manager_%s_LocalTranTime_count", Name: "transaction_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalTranTime_min", Name: "transaction_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalTranTime_max", Name: "transaction_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalCommitTime_count", Name: "commit_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalCommitTime_min", Name: "commit_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalCommitTime_max", Name: "commit_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalBeforeCompletionTime_count", Name: "before_completion_count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalBeforeCompletionTime_min", Name: "before_completion_min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "transaction_manager_%s_LocalBeforeCompletionTime_max", Name: "before_completion_max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "transaction_manager_%s_global_avg_times",
		Title:    "Transaction Manager Global Average Times",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager_global_avg_times",
		Type:     module.Line,
		Priority: prioTransactionManager + 40,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_GlobalTranTime_mean", Name: "avg_transaction_time", Div: precision},
			{ID: "transaction_manager_%s_GlobalPrepareTime_mean", Name: "avg_prepare_time", Div: precision},
			{ID: "transaction_manager_%s_GlobalCommitTime_mean", Name: "avg_commit_time", Div: precision},
			{ID: "transaction_manager_%s_GlobalBeforeCompletionTime_mean", Name: "avg_before_completion_time", Div: precision},
		},
	},
	{
		ID:       "transaction_manager_%s_local_avg_times",
		Title:    "Transaction Manager Local Average Times",
		Units:    "milliseconds",
		Fam:      "transactions",
		Ctx:      "websphere_pmi.transaction_manager_local_avg_times",
		Type:     module.Line,
		Priority: prioTransactionManager + 50,
		Dims: module.Dims{
			{ID: "transaction_manager_%s_LocalTranTime_mean", Name: "avg_transaction_time", Div: precision},
			{ID: "transaction_manager_%s_LocalCommitTime_mean", Name: "avg_commit_time", Div: precision},
			{ID: "transaction_manager_%s_LocalBeforeCompletionTime_mean", Name: "avg_before_completion_time", Div: precision},
		},
	},
	*/
}

var jvmChartsTmpl = module.Charts{
	{
		ID:       "jvm_memory_%s",
		Title:    "JVM Memory",
		Units:    "bytes",
		Fam:      "system/memory",
		Ctx:      "websphere_pmi.jvm_memory",
		Type:     module.Stacked,
		Priority: prioSystemMemory,
		Dims: module.Dims{
			{ID: "jvm_memory_%s_free", Name: "free"},
			{ID: "jvm_memory_%s_used", Name: "used"},
		},
	},
	{
		ID:       "jvm_uptime_%s",
		Title:    "JVM Uptime",
		Units:    "seconds",
		Fam:      "system/uptime",
		Ctx:      "websphere_pmi.jvm_uptime",
		Type:     module.Line,
		Priority: prioSystemUptime,
		Dims: module.Dims{
			{ID: "jvm_uptime_%s_seconds", Name: "uptime"},
		},
	},
	{
		ID:       "jvm_cpu_%s",
		Title:    "JVM CPU Usage",
		Units:    "percentage",
		Fam:      "system/cpu",
		Ctx:      "websphere_pmi.jvm_cpu",
		Type:     module.Line,
		Priority: prioSystemCPU,
		Dims: module.Dims{
			{ID: "jvm_cpu_%s_usage", Name: "usage", Div: 100},
		},
	},
	{
		ID:       "jvm_heap_size_%s",
		Title:    "JVM Heap Size",
		Units:    "bytes",
		Fam:      "system/memory",
		Ctx:      "websphere_pmi.jvm_heap_size",
		Type:     module.Line,
		Priority: prioSystemMemory + 10,
		Dims: module.Dims{
			{ID: "jvm_runtime_%s_HeapSize_value", Name: "current"},
			{ID: "jvm_runtime_%s_HeapSize_upper_bound", Name: "upper_bound"},
			{ID: "jvm_runtime_%s_HeapSize_lower_bound", Name: "lower_bound"},
			{ID: "jvm_runtime_%s_HeapSize_high_watermark", Name: "high_watermark"},
			{ID: "jvm_runtime_%s_HeapSize_low_watermark", Name: "low_watermark"},
			// Hidden dimensions for mean and integral
			{ID: "jvm_runtime_%s_HeapSize_mean", Name: "mean", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "jvm_runtime_%s_HeapSize_integral", Name: "integral", DimOpts: module.DimOpts{Hidden: true}},
		},
	},
}

var threadPoolChartsTmpl = module.Charts{
	{
		ID:       "thread_pool_%s_threads",
		Title:    "Thread Pool Threads",
		Units:    "threads",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_threads",
		Type:     module.Stacked,
		Priority: prioThreadPools,
		Dims: module.Dims{
			{ID: "thread_pool_%s_active", Name: "active"},
			{ID: "thread_pool_%s_idle", Name: "idle"},
		},
	},
	{
		ID:       "thread_pool_%s_lifecycle",
		Title:    "Thread Pool Lifecycle",
		Units:    "operations/s",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_lifecycle",
		Type:     module.Line,
		Priority: prioThreadPools + 10,
		Dims: module.Dims{
			{ID: "thread_pool_%s_CreateCount", Name: "created", Algo: module.Incremental},
			{ID: "thread_pool_%s_DestroyCount", Name: "destroyed", Algo: module.Incremental},
			{ID: "thread_pool_%s_DeclaredThreadHungCount", Name: "hung_declared", Algo: module.Incremental},
			{ID: "thread_pool_%s_ClearedThreadHangCount", Name: "hang_cleared", Algo: module.Incremental},
		},
	},
	{
		ID:       "thread_pool_%s_concurrent_hung",
		Title:    "Thread Pool Concurrent Hung Threads",
		Units:    "threads",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_concurrent_hung",
		Type:     module.Line,
		Priority: prioThreadPools + 20,
		Dims: module.Dims{
			{ID: "thread_pool_%s_ConcurrentHungThreadCount_current", Name: "current"},
			{ID: "thread_pool_%s_ConcurrentHungThreadCount_high_watermark", Name: "high_watermark"},
			{ID: "thread_pool_%s_ConcurrentHungThreadCount_low_watermark", Name: "low_watermark"},
			{ID: "thread_pool_%s_ConcurrentHungThreadCount_mean", Name: "mean", Div: precision},
			// Hidden dimension for integral
			{ID: "thread_pool_%s_ConcurrentHungThreadCount_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	// DEPRECATED: Old ActiveTime charts - replaced by smart processor
	/*
	{
		ID:       "thread_pool_%s_active_time",
		Title:    "Thread Pool Active Time",
		Units:    "milliseconds/s",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_active_time",
		Type:     module.Line,
		Priority: prioThreadPools + 30,
		Dims: module.Dims{
			{ID: "thread_pool_%s_ActiveTime_total", Name: "active_time", Algo: module.Incremental},
			// Hidden dimensions for count, min, max
			{ID: "thread_pool_%s_ActiveTime_count", Name: "count", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "thread_pool_%s_ActiveTime_min", Name: "min", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "thread_pool_%s_ActiveTime_max", Name: "max", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "thread_pool_%s_avg_active_time",
		Title:    "Thread Pool Average Active Time",
		Units:    "milliseconds",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_avg_active_time",
		Type:     module.Line,
		Priority: prioThreadPools + 40,
		Dims: module.Dims{
			{ID: "thread_pool_%s_ActiveTime_mean", Name: "avg_active_time", Div: precision},
		},
	},
	*/
	{
		ID:       "thread_pool_%s_percent_used",
		Title:    "Thread Pool Percent Used",
		Units:    "percentage",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_percent_used",
		Type:     module.Line,
		Priority: prioThreadPools + 50,
		Dims: module.Dims{
			{ID: "thread_pool_%s_PercentUsed_value", Name: "current"},
			{ID: "thread_pool_%s_PercentUsed_high_watermark", Name: "high_watermark"},
			{ID: "thread_pool_%s_PercentUsed_low_watermark", Name: "low_watermark"},
			{ID: "thread_pool_%s_PercentUsed_mean", Name: "mean", Div: precision},
			// Hidden dimensions for bounds and integral
			{ID: "thread_pool_%s_PercentUsed_upper_bound", Name: "upper_bound", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "thread_pool_%s_PercentUsed_lower_bound", Name: "lower_bound", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "thread_pool_%s_PercentUsed_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "thread_pool_%s_percent_maxed",
		Title:    "Thread Pool Percent Maxed",
		Units:    "percentage",
		Fam:      "thread_pools",
		Ctx:      "websphere_pmi.thread_pool_percent_maxed",
		Type:     module.Line,
		Priority: prioThreadPools + 60,
		Dims: module.Dims{
			{ID: "thread_pool_%s_PercentMaxed_value", Name: "current"},
			{ID: "thread_pool_%s_PercentMaxed_high_watermark", Name: "high_watermark"},
			{ID: "thread_pool_%s_PercentMaxed_low_watermark", Name: "low_watermark"},
			{ID: "thread_pool_%s_PercentMaxed_mean", Name: "mean", Div: precision},
			// Hidden dimensions for bounds and integral
			{ID: "thread_pool_%s_PercentMaxed_upper_bound", Name: "upper_bound", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "thread_pool_%s_PercentMaxed_lower_bound", Name: "lower_bound", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "thread_pool_%s_PercentMaxed_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
}

// Additional ThreadPool charts for v9.0.5+
var threadPoolMaxPoolSizeChartTmpl = module.Chart{
	ID:       "thread_pool_%s_max_pool_size",
	Title:    "Thread Pool Maximum Size",
	Units:    "threads",
	Fam:      "thread_pools",
	Ctx:      "websphere_pmi.thread_pool_max_pool_size",
	Type:     module.Line,
	Priority: prioThreadPools + 70,
	Dims: module.Dims{
		{ID: "thread_pool_%s_MaxPoolSize", Name: "max_pool_size"},
	},
}

var jdbcChartsTmpl = module.Charts{
	{
		ID:       "jdbc_%s_connections",
		Title:    "JDBC Connections",
		Units:    "connections/s",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_connections",
		Type:     module.Line,
		Priority: prioJDBCPools,
		Dims: module.Dims{
			{ID: "jdbc_%s_connections_created", Name: "created", Algo: module.Incremental},
			{ID: "jdbc_%s_connections_closed", Name: "closed", Algo: module.Incremental},
			{ID: "jdbc_%s_connections_allocated", Name: "allocated", Algo: module.Incremental},
			{ID: "jdbc_%s_connections_returned", Name: "returned", Algo: module.Incremental},
		},
	},
	{
		ID:       "jdbc_%s_pool_size",
		Title:    "JDBC Pool Size",
		Units:    "connections",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_pool_size",
		Type:     module.Line,
		Priority: prioJDBCPools + 10,
		Dims: module.Dims{
			{ID: "jdbc_%s_pool_free", Name: "free"},
			{ID: "jdbc_%s_pool_size", Name: "total"},
		},
	},
	{
		ID:       "jdbc_%s_wait_time",
		Title:    "JDBC Wait Time",
		Units:    "milliseconds/s",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_wait_time",
		Type:     module.Line,
		Priority: prioJDBCPools + 20,
		Dims: module.Dims{
			{ID: "jdbc_%s_wait_time_total", Name: "wait_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "jdbc_%s_waiting_threads",
		Title:    "JDBC Waiting Threads",
		Units:    "threads",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_waiting_threads",
		Type:     module.Line,
		Priority: prioJDBCPools + 30,
		Dims: module.Dims{
			{ID: "jdbc_%s_waiting_threads", Name: "waiting_threads"},
		},
	},
	{
		ID:       "jdbc_%s_use_time",
		Title:    "JDBC Use Time",
		Units:    "milliseconds/s",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_use_time",
		Type:     module.Line,
		Priority: prioJDBCPools + 40,
		Dims: module.Dims{
			{ID: "jdbc_%s_use_time_total", Name: "use_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "jdbc_%s_faults",
		Title:    "JDBC Faults",
		Units:    "faults/s",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_faults",
		Type:     module.Line,
		Priority: prioJDBCPools + 50,
		Dims: module.Dims{
			{ID: "jdbc_%s_FaultCount", Name: "faults", Algo: module.Incremental},
		},
	},
	{
		ID:       "jdbc_%s_managed_connections",
		Title:    "JDBC Managed Connections",
		Units:    "connections",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_managed_connections",
		Type:     module.Line,
		Priority: prioJDBCPools + 60,
		Dims: module.Dims{
			{ID: "jdbc_%s_ManagedConnectionCount", Name: "managed"},
			{ID: "jdbc_%s_ConnectionHandleCount", Name: "handles"},
		},
	},
	{
		ID:       "jdbc_%s_percent_used",
		Title:    "JDBC Pool Percent Used",
		Units:    "percentage",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_percent_used",
		Type:     module.Line,
		Priority: prioJDBCPools + 80,
		Dims: module.Dims{
			{ID: "jdbc_%s_PercentUsed_current", Name: "current"},
			{ID: "jdbc_%s_PercentUsed_high_watermark", Name: "high_watermark"},
			{ID: "jdbc_%s_PercentUsed_low_watermark", Name: "low_watermark"},
			{ID: "jdbc_%s_PercentUsed_mean", Name: "mean", Div: precision},
			{ID: "jdbc_%s_PercentUsed_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "jdbc_%s_percent_maxed",
		Title:    "JDBC Pool Percent Maxed",
		Units:    "percentage",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_percent_maxed",
		Type:     module.Line,
		Priority: prioJDBCPools + 90,
		Dims: module.Dims{
			{ID: "jdbc_%s_PercentMaxed_current", Name: "current"},
			{ID: "jdbc_%s_PercentMaxed_high_watermark", Name: "high_watermark"},
			{ID: "jdbc_%s_PercentMaxed_low_watermark", Name: "low_watermark"},
			{ID: "jdbc_%s_PercentMaxed_mean", Name: "mean", Div: precision},
			{ID: "jdbc_%s_PercentMaxed_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "jdbc_%s_prepared_statement_discards",
		Title:    "JDBC Prepared Statement Cache Discards",
		Units:    "discards/s",
		Fam:      "connections/jdbc",
		Ctx:      "websphere_pmi.jdbc_prepared_statement_discards",
		Type:     module.Line,
		Priority: prioJDBCPools + 100,
		Dims: module.Dims{
			{ID: "jdbc_%s_PrepStmtCacheDiscardCount", Name: "discards", Algo: module.Incremental},
		},
	},
}

// Chart templates for specialized parsers

var extensionRegistryChartsTmpl = module.Charts{
	{
		ID:       "extension_registry_%s_metrics",
		Title:    "Extension Registry Metrics",
		Units:    "requests/s",
		Fam:      "registry",
		Ctx:      "websphere_pmi.extension_registry_metrics",
		Type:     module.Line,
		Priority: prioRegistry,
		Dims: module.Dims{
			{ID: "extension_registry_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "extension_registry_%s_hits", Name: "hits", Algo: module.Incremental},
			{ID: "extension_registry_%s_displacements", Name: "displacements", Algo: module.Incremental},
		},
	},
	{
		ID:       "extension_registry_%s_hit_rate",
		Title:    "Extension Registry Hit Rate",
		Units:    "percentage",
		Fam:      "registry",
		Ctx:      "websphere_pmi.extension_registry_hit_rate",
		Type:     module.Line,
		Priority: prioRegistry + 10,
		Dims: module.Dims{
			{ID: "extension_registry_%s_hit_rate", Name: "hit_rate"},
		},
	},
}

var sibJMSAdapterChartsTmpl = module.Charts{
	{
		ID:       "sib_jms_%s_connections",
		Title:    "SIB JMS Adapter Connections",
		Units:    "connections/s",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_connections",
		Type:     module.Line,
		Priority: prioSIBMessaging,
		Dims: module.Dims{
			{ID: "sib_jms_%s_create_count", Name: "created", Algo: module.Incremental},
			{ID: "sib_jms_%s_close_count", Name: "closed", Algo: module.Incremental},
			{ID: "sib_jms_%s_allocate_count", Name: "allocated", Algo: module.Incremental},
			{ID: "sib_jms_%s_freed_count", Name: "freed", Algo: module.Incremental},
		},
	},
	{
		ID:       "sib_jms_%s_faults",
		Title:    "SIB JMS Adapter Faults",
		Units:    "faults/s",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_faults",
		Type:     module.Line,
		Priority: prioSIBMessaging + 10,
		Dims: module.Dims{
			{ID: "sib_jms_%s_fault_count", Name: "faults", Algo: module.Incremental},
		},
	},
	{
		ID:       "sib_jms_%s_managed",
		Title:    "SIB JMS Adapter Managed Resources",
		Units:    "resources",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_managed",
		Type:     module.Line,
		Priority: prioSIBMessaging + 20,
		Dims: module.Dims{
			{ID: "sib_jms_%s_managed_connections", Name: "managed_connections"},
			{ID: "sib_jms_%s_connection_handles", Name: "connection_handles"},
		},
	},
	{
		ID:       "sib_jms_%s_pool_size",
		Title:    "SIB JMS Adapter Pool Sizes",
		Units:    "connections",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_pool_size",
		Type:     module.Line,
		Priority: prioSIBMessaging + 30,
		Dims: module.Dims{
			{ID: "sib_jms_%s_PoolSize_value", Name: "pool_size"},
			{ID: "sib_jms_%s_FreePoolSize_value", Name: "free_pool_size"},
			{ID: "sib_jms_%s_PoolSize_upper_bound", Name: "max_pool_size"},
			{ID: "sib_jms_%s_PoolSize_lower_bound", Name: "min_pool_size"},
			{ID: "sib_jms_%s_FreePoolSize_upper_bound", Name: "max_free_pool_size"},
			{ID: "sib_jms_%s_FreePoolSize_lower_bound", Name: "min_free_pool_size"},
			{ID: "sib_jms_%s_PoolSize_mean", Name: "pool_size_mean", Div: precision},
			{ID: "sib_jms_%s_PoolSize_integral", Name: "pool_size_integral", Div: precision},
			{ID: "sib_jms_%s_FreePoolSize_mean", Name: "free_pool_size_mean", Div: precision},
			{ID: "sib_jms_%s_FreePoolSize_integral", Name: "free_pool_size_integral", Div: precision},
		},
	},
	{
		ID:       "sib_jms_%s_pool_bounds",
		Title:    "SIB JMS Adapter Pool High/Low Water Marks",
		Units:    "connections",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_pool_bounds",
		Type:     module.Line,
		Priority: prioSIBMessaging + 40,
		Dims: module.Dims{
			{ID: "sib_jms_%s_PoolSize_high_watermark", Name: "pool_size_peak"},
			{ID: "sib_jms_%s_PoolSize_low_watermark", Name: "pool_size_min"},
			{ID: "sib_jms_%s_FreePoolSize_high_watermark", Name: "free_pool_peak"},
			{ID: "sib_jms_%s_FreePoolSize_low_watermark", Name: "free_pool_min"},
		},
	},
	{
		ID:       "sib_jms_%s_utilization",
		Title:    "SIB JMS Adapter Pool Utilization",
		Units:    "percentage",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_utilization",
		Type:     module.Line,
		Priority: prioSIBMessaging + 50,
		Dims: module.Dims{
			{ID: "sib_jms_%s_PercentUsed_current", Name: "percent_used"},
			{ID: "sib_jms_%s_PercentMaxed_current", Name: "percent_maxed"},
			{ID: "sib_jms_%s_PercentUsed_mean", Name: "percent_used_mean", Div: precision},
			{ID: "sib_jms_%s_PercentUsed_integral", Name: "percent_used_integral", Div: precision},
			{ID: "sib_jms_%s_PercentMaxed_mean", Name: "percent_maxed_mean", Div: precision},
			{ID: "sib_jms_%s_PercentMaxed_integral", Name: "percent_maxed_integral", Div: precision},
		},
	},
	{
		ID:       "sib_jms_%s_utilization_peaks",
		Title:    "SIB JMS Adapter Utilization Peaks",
		Units:    "percentage",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_utilization_peaks",
		Type:     module.Line,
		Priority: prioSIBMessaging + 60,
		Dims: module.Dims{
			{ID: "sib_jms_%s_PercentUsed_high_watermark", Name: "used_peak"},
			{ID: "sib_jms_%s_PercentUsed_low_watermark", Name: "used_min"},
			{ID: "sib_jms_%s_PercentMaxed_high_watermark", Name: "maxed_peak"},
			{ID: "sib_jms_%s_PercentMaxed_low_watermark", Name: "maxed_min"},
		},
	},
	{
		ID:       "sib_jms_%s_waiting_threads",
		Title:    "SIB JMS Adapter Waiting Threads",
		Units:    "threads",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_waiting_threads",
		Type:     module.Line,
		Priority: prioSIBMessaging + 70,
		Dims: module.Dims{
			{ID: "sib_jms_%s_WaitingThreadCount_current", Name: "waiting_threads"},
			{ID: "sib_jms_%s_WaitingThreadCount_high_watermark", Name: "waiting_peak"},
			{ID: "sib_jms_%s_WaitingThreadCount_low_watermark", Name: "waiting_min"},
			{ID: "sib_jms_%s_WaitingThreadCount_mean", Name: "waiting_mean", Div: precision},
			{ID: "sib_jms_%s_WaitingThreadCount_integral", Name: "waiting_integral", Div: precision},
		},
	},
	{
		ID:       "sib_jms_%s_timing",
		Title:    "SIB JMS Adapter Connection Timing",
		Units:    "milliseconds",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_timing",
		Type:     module.Line,
		Priority: prioSIBMessaging + 80,
		Dims: module.Dims{
			{ID: "sib_jms_%s_UseTime_count", Name: "use_time_count"},
			{ID: "sib_jms_%s_UseTime_total", Name: "use_time_total"},
			{ID: "sib_jms_%s_UseTime_min", Name: "use_time_min"},
			{ID: "sib_jms_%s_UseTime_max", Name: "use_time_max"},
			{ID: "sib_jms_%s_UseTime_mean", Name: "use_time_mean", Div: precision},
		},
	},
	{
		ID:       "sib_jms_%s_wait_timing",
		Title:    "SIB JMS Adapter Wait Timing",
		Units:    "milliseconds",
		Fam:      "connections/jms",
		Ctx:      "websphere_pmi.sib_jms_wait_timing",
		Type:     module.Line,
		Priority: prioSIBMessaging + 90,
		Dims: module.Dims{
			{ID: "sib_jms_%s_WaitTime_count", Name: "wait_time_count"},
			{ID: "sib_jms_%s_WaitTime_total", Name: "wait_time_total"},
			{ID: "sib_jms_%s_WaitTime_min", Name: "wait_time_min"},
			{ID: "sib_jms_%s_WaitTime_max", Name: "wait_time_max"},
			{ID: "sib_jms_%s_WaitTime_mean", Name: "wait_time_mean", Div: precision},
		},
	},
}

var servletsComponentChartsTmpl = module.Charts{
	{
		ID:       "servlets_component_%s_requests",
		Title:    "Servlets Component Requests",
		Units:    "requests/s",
		Fam:      "web/servlets",
		Ctx:      "websphere_pmi.servlets_component_requests",
		Type:     module.Line,
		Priority: prioWebServlets,
		Dims: module.Dims{
			{ID: "servlets_component_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "servlets_component_%s_errors", Name: "errors", Algo: module.Incremental},
			{ID: "servlets_component_%s_URIRequestCount", Name: "uri_requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlets_component_%s_concurrent",
		Title:    "Servlets Component Concurrent Requests",
		Units:    "requests",
		Fam:      "web/servlets",
		Ctx:      "websphere_pmi.servlets_component_concurrent",
		Type:     module.Line,
		Priority: prioWebServlets + 20,
		Dims: module.Dims{
			{ID: "servlets_component_%s_concurrent_requests", Name: "concurrent_requests"},
			{ID: "servlets_component_%s_URIConcurrentRequests_current", Name: "uri_concurrent_current"},
			{ID: "servlets_component_%s_URIConcurrentRequests_high_watermark", Name: "uri_concurrent_peak"},
			{ID: "servlets_component_%s_URIConcurrentRequests_low_watermark", Name: "uri_concurrent_min"},
			{ID: "servlets_component_%s_URIConcurrentRequests_mean", Name: "uri_concurrent_mean", Div: precision},
			{ID: "servlets_component_%s_URIConcurrentRequests_integral", Name: "uri_concurrent_integral", Div: precision},
		},
	},
}

var wimComponentChartsTmpl = module.Charts{
	{
		ID:       "wim_%s_portlet_requests",
		Title:    "WIM Component Portlet Requests",
		Units:    "requests/s",
		Fam:      "components",
		Ctx:      "websphere_pmi.wim_portlet_requests",
		Type:     module.Line,
		Priority: prioComponents,
		Dims: module.Dims{
			{ID: "wim_%s_portlet_requests", Name: "requests", Algo: module.Incremental},
			{ID: "wim_%s_portlet_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "wim_%s_concurrent_requests",
		Title:    "WIM Component Concurrent Portlet Requests",
		Units:    "requests",
		Fam:      "components",
		Ctx:      "websphere_pmi.wim_concurrent_requests",
		Type:     module.Line,
		Priority: prioComponents + 20,
		Dims: module.Dims{
			{ID: "wim_%s_Number_of_concurrent_portlet_requests_current", Name: "concurrent_current"},
			{ID: "wim_%s_Number_of_concurrent_portlet_requests_high_watermark", Name: "concurrent_peak"},
			{ID: "wim_%s_Number_of_concurrent_portlet_requests_low_watermark", Name: "concurrent_min"},
			{ID: "wim_%s_Number_of_concurrent_portlet_requests_mean", Name: "concurrent_mean", Div: precision},
			{ID: "wim_%s_Number_of_concurrent_portlet_requests_integral", Name: "concurrent_integral", Div: precision},
		},
	},
}

var wlmTaggedComponentChartsTmpl = module.Charts{
}

var pmiWebServiceServiceChartsTmpl = module.Charts{
	{
		ID:       "pmi_webservice_%s_requests",
		Title:    "PMI Web Service Service Requests",
		Units:    "requests/s",
		Fam:      "web_services",
		Ctx:      "websphere_pmi.pmi_webservice_requests",
		Type:     module.Line,
		Priority: prioWebServices,
		Dims: module.Dims{
			{ID: "pmi_webservice_%s_requests_received", Name: "received", Algo: module.Incremental},
			{ID: "pmi_webservice_%s_requests_dispatched", Name: "dispatched", Algo: module.Incremental},
			{ID: "pmi_webservice_%s_requests_successful", Name: "successful", Algo: module.Incremental},
		},
	},
	{
		ID:       "pmi_webservice_%s_request_size",
		Title:    "PMI Web Service Request Size",
		Units:    "bytes",
		Fam:      "web_services",
		Ctx:      "websphere_pmi.pmi_webservice_request_size",
		Type:     module.Line,
		Priority: prioWebServices + 40,
		Dims: module.Dims{
			{ID: "pmi_webservice_%s_RequestSizeService_count", Name: "request_count"},
			{ID: "pmi_webservice_%s_RequestSizeService_total", Name: "request_total"},
			{ID: "pmi_webservice_%s_RequestSizeService_min", Name: "request_min"},
			{ID: "pmi_webservice_%s_RequestSizeService_max", Name: "request_max"},
			{ID: "pmi_webservice_%s_RequestSizeService_mean", Name: "request_mean", Div: precision},
			{ID: "pmi_webservice_%s_RequestSizeService_sum_of_squares", Name: "request_sum_squares", Div: precision},
		},
	},
	{
		ID:       "pmi_webservice_%s_reply_size",
		Title:    "PMI Web Service Reply Size",
		Units:    "bytes",
		Fam:      "web_services",
		Ctx:      "websphere_pmi.pmi_webservice_reply_size",
		Type:     module.Line,
		Priority: prioWebServices + 50,
		Dims: module.Dims{
			{ID: "pmi_webservice_%s_ReplySizeService_count", Name: "reply_count"},
			{ID: "pmi_webservice_%s_ReplySizeService_total", Name: "reply_total"},
			{ID: "pmi_webservice_%s_ReplySizeService_min", Name: "reply_min"},
			{ID: "pmi_webservice_%s_ReplySizeService_max", Name: "reply_max"},
			{ID: "pmi_webservice_%s_ReplySizeService_mean", Name: "reply_mean", Div: precision},
			{ID: "pmi_webservice_%s_ReplySizeService_sum_of_squares", Name: "reply_sum_squares", Div: precision},
		},
	},
	{
		ID:       "pmi_webservice_%s_total_size",
		Title:    "PMI Web Service Total Size",
		Units:    "bytes",
		Fam:      "web_services",
		Ctx:      "websphere_pmi.pmi_webservice_total_size",
		Type:     module.Line,
		Priority: prioWebServices + 60,
		Dims: module.Dims{
			{ID: "pmi_webservice_%s_SizeService_count", Name: "size_count"},
			{ID: "pmi_webservice_%s_SizeService_total", Name: "size_total"},
			{ID: "pmi_webservice_%s_SizeService_min", Name: "size_min"},
			{ID: "pmi_webservice_%s_SizeService_max", Name: "size_max"},
			{ID: "pmi_webservice_%s_SizeService_mean", Name: "size_mean", Div: precision},
			{ID: "pmi_webservice_%s_SizeService_sum_of_squares", Name: "size_sum_squares", Div: precision},
		},
	},
}

var tcpChannelDCSChartsTmpl = module.Charts{
	{
		ID:       "tcp_dcs_%s_lifecycle",
		Title:    "TCP Channel DCS Lifecycle",
		Units:    "operations/s",
		Fam:      "connections/tcp",
		Ctx:      "websphere_pmi.tcp_dcs_lifecycle",
		Type:     module.Line,
		Priority: prioTCPChannels,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_create_count", Name: "created", Algo: module.Incremental},
			{ID: "tcp_dcs_%s_destroy_count", Name: "destroyed", Algo: module.Incremental},
		},
	},
	{
		ID:       "tcp_dcs_%s_threads",
		Title:    "TCP Channel DCS Thread Management",
		Units:    "threads/s",
		Fam:      "connections/tcp",
		Ctx:      "websphere_pmi.tcp_dcs_threads",
		Type:     module.Line,
		Priority: prioTCPChannels + 10,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_thread_hung_count", Name: "hung_declared", Algo: module.Incremental},
			{ID: "tcp_dcs_%s_thread_hang_cleared", Name: "hang_cleared", Algo: module.Incremental},
		},
	},
	{
		ID:       "tcp_dcs_%s_pool_status",
		Title:    "TCP Channel DCS Pool Status",
		Units:    "threads",
		Fam:      "connections/tcp",
		Ctx:      "websphere_pmi.tcp_dcs_pool_status",
		Type:     module.Line,
		Priority: prioTCPChannels + 20,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_active_count", Name: "active"},
			{ID: "tcp_dcs_%s_pool_size", Name: "pool_size"},
			{ID: "tcp_dcs_%s_concurrent_hung_threads", Name: "concurrent_hung"},
		},
	},
	{
		ID:       "tcp_dcs_%s_utilization",
		Title:    "TCP Channel DCS Pool Utilization",
		Units:    "percentage",
		Fam:      "connections/tcp",
		Ctx:      "websphere_pmi.tcp_dcs_utilization",
		Type:     module.Line,
		Priority: prioTCPChannels + 30,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_percent_used", Name: "used"},
			{ID: "tcp_dcs_%s_percent_maxed", Name: "maxed"},
		},
	},
	{
		ID:       "tcp_dcs_%s_pool_limits",
		Title:    "TCP Channel DCS Pool Limits",
		Units:    "threads",
		Fam:      "connections/tcp",
		Ctx:      "websphere_pmi.tcp_dcs_pool_limits",
		Type:     module.Line,
		Priority: prioTCPChannels + 50,
		Dims: module.Dims{
			{ID: "tcp_dcs_%s_MaxPoolSize", Name: "max_pool_size"},
		},
	},
}

var detailsComponentChartsTmpl = module.Charts{
	{
		ID:       "details_%s_portlets",
		Title:    "Details Component Loaded Portlets",
		Units:    "portlets",
		Fam:      "components",
		Ctx:      "websphere_pmi.details_portlets",
		Type:     module.Line,
		Priority: prioComponents + 100,
		Dims: module.Dims{
			{ID: "details_%s_loaded_portlets", Name: "loaded_portlets"},
		},
	},
}

var iscProductDetailsChartsTmpl = module.Charts{
	{
		ID:       "isc_product_%s_portlet_requests",
		Title:    "ISC Product Details Portlet Requests",
		Units:    "requests/s",
		Fam:      "components",
		Ctx:      "websphere_pmi.isc_product_portlet_requests",
		Type:     module.Line,
		Priority: prioComponents + 200,
		Dims: module.Dims{
			{ID: "isc_product_%s_portlet_requests", Name: "requests", Algo: module.Incremental},
			{ID: "isc_product_%s_portlet_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "isc_product_%s_concurrent_requests",
		Title:    "ISC Product Details Concurrent Portlet Requests",
		Units:    "requests",
		Fam:      "components",
		Ctx:      "websphere_pmi.isc_product_concurrent_requests",
		Type:     module.Line,
		Priority: prioComponents + 220,
		Dims: module.Dims{
			{ID: "isc_product_%s_Number_of_concurrent_portlet_requests_current", Name: "current"},
			{ID: "isc_product_%s_Number_of_concurrent_portlet_requests_high_watermark", Name: "high_watermark"},
			{ID: "isc_product_%s_Number_of_concurrent_portlet_requests_low_watermark", Name: "low_watermark"},
			{ID: "isc_product_%s_Number_of_concurrent_portlet_requests_mean", Name: "mean", Div: precision},
			{ID: "isc_product_%s_Number_of_concurrent_portlet_requests_integral", Name: "integral", Div: precision},
		},
	},
}

