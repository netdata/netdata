// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	// Priority constants for chart ordering
	prioJVMHeap = module.Priority + iota
	prioJVMHeapUsage
	prioJVMNonHeap
	prioJVMGC
	prioJVMThreads
	prioJVMClasses

	prioThreadPoolSize
	prioThreadPoolActive

	prioJDBCPoolSize
	prioJDBCPoolActive
	prioJDBCPoolWaitTime
	prioJDBCPoolUseTime
	prioJDBCPoolCreatedDestroyed

	prioJMSMessages
	prioJMSConsumers

	prioAppRequests
	prioAppResponseTime
	prioAppSessions
	prioAppTransactions

	prioClusterState
	prioClusterMembers
	prioHAManager
	prioDynamicCluster
	prioReplication
	prioConnectionHealth
)

var baseCharts = module.Charts{
	// JVM Heap Memory
	{
		ID:       "jvm_heap_memory",
		Title:    "JVM Heap Memory",
		Units:    "bytes",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_heap_memory",
		Priority: prioJVMHeap,
		Dims: module.Dims{
			{ID: "jvm_heap_used", Name: "used"},
			{ID: "jvm_heap_committed", Name: "committed"},
			{ID: "jvm_heap_max", Name: "max"},
		},
	},
	{
		ID:       "jvm_heap_usage",
		Title:    "JVM Heap Usage",
		Units:    "percentage",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_heap_usage",
		Priority: prioJVMHeapUsage,
		Dims: module.Dims{
			{ID: "jvm_heap_usage_percent", Name: "usage", Div: precision},
		},
	},

	// JVM Non-Heap Memory
	{
		ID:       "jvm_nonheap_memory",
		Title:    "JVM Non-Heap Memory",
		Units:    "bytes",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_nonheap_memory",
		Priority: prioJVMNonHeap,
		Dims: module.Dims{
			{ID: "jvm_nonheap_used", Name: "used"},
			{ID: "jvm_nonheap_committed", Name: "committed"},
		},
	},

	// JVM GC
	{
		ID:       "jvm_gc_count",
		Title:    "JVM Garbage Collection Count",
		Units:    "collections",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_gc_count",
		Priority: prioJVMGC,
		Dims: module.Dims{
			{ID: "jvm_gc_count", Name: "collections"},
		},
	},
	{
		ID:       "jvm_gc_time",
		Title:    "JVM Garbage Collection Time",
		Units:    "milliseconds",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_gc_time",
		Priority: prioJVMGC + 1,
		Dims: module.Dims{
			{ID: "jvm_gc_time", Name: "time"},
		},
	},

	// JVM Threads
	{
		ID:       "jvm_threads",
		Title:    "JVM Threads",
		Units:    "threads",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_threads",
		Priority: prioJVMThreads,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jvm_thread_count", Name: "total"},
			{ID: "jvm_thread_daemon", Name: "daemon"},
		},
	},
	{
		ID:       "jvm_thread_states",
		Title:    "JVM Thread States",
		Units:    "threads",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_thread_states",
		Priority: prioJVMThreads + 1,
		Dims: module.Dims{
			{ID: "jvm_thread_peak", Name: "peak"},
			{ID: "jvm_thread_started", Name: "started"},
		},
	},

	// JVM Classes
	{
		ID:       "jvm_classes",
		Title:    "JVM Classes",
		Units:    "classes",
		Fam:      "jvm",
		Ctx:      "websphere_jmx.jvm_classes",
		Priority: prioJVMClasses,
		Dims: module.Dims{
			{ID: "jvm_classes_loaded", Name: "loaded"},
			{ID: "jvm_classes_unloaded", Name: "unloaded"},
		},
	},

	// Connection health and resilience
	{
		ID:       "connection_health",
		Title:    "Connection Health Status",
		Units:    "status",
		Fam:      "resilience",
		Ctx:      "websphere_jmx.connection_health",
		Priority: prioConnectionHealth,
		Dims: module.Dims{
			{ID: "connection_health", Name: "health"},
			{ID: "circuit_breaker_state", Name: "circuit_breaker"},
		},
	},
	{
		ID:       "connection_staleness",
		Title:    "Connection Staleness",
		Units:    "seconds",
		Fam:      "resilience",
		Ctx:      "websphere_jmx.connection_staleness",
		Priority: prioConnectionHealth + 1,
		Dims: module.Dims{
			{ID: "seconds_since_last_success", Name: "age"},
		},
	},
}

// Thread Pool Charts Template
var threadPoolChartsTmpl = module.Charts{
	{
		ID:       "threadpool_%s_size",
		Title:    "Thread Pool Size",
		Units:    "threads",
		Fam:      "thread pools",
		Ctx:      "websphere_jmx.threadpool_size",
		Priority: prioThreadPoolSize,
		Dims: module.Dims{
			{ID: "threadpool_%s_size", Name: "size"},
			{ID: "threadpool_%s_max", Name: "max"},
		},
	},
	{
		ID:       "threadpool_%s_active",
		Title:    "Thread Pool Active Threads",
		Units:    "threads",
		Fam:      "thread pools",
		Ctx:      "websphere_jmx.threadpool_active",
		Priority: prioThreadPoolActive,
		Dims: module.Dims{
			{ID: "threadpool_%s_active", Name: "active"},
		},
	},
}

// JDBC Pool Charts Template
var jdbcPoolChartsTmpl = module.Charts{
	{
		ID:       "jdbc_%s_size",
		Title:    "JDBC Pool Size",
		Units:    "connections",
		Fam:      "jdbc pools",
		Ctx:      "websphere_jmx.jdbc_pool_size",
		Priority: prioJDBCPoolSize,
		Dims: module.Dims{
			{ID: "jdbc_%s_size", Name: "size"},
		},
	},
	{
		ID:       "jdbc_%s_active",
		Title:    "JDBC Pool Active Connections",
		Units:    "connections",
		Fam:      "jdbc pools",
		Ctx:      "websphere_jmx.jdbc_pool_active",
		Priority: prioJDBCPoolActive,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jdbc_%s_active", Name: "active"},
			{ID: "jdbc_%s_free", Name: "free"},
		},
	},
	{
		ID:       "jdbc_%s_wait_time",
		Title:    "JDBC Pool Average Wait Time",
		Units:    "milliseconds",
		Fam:      "jdbc pools",
		Ctx:      "websphere_jmx.jdbc_pool_wait_time",
		Priority: prioJDBCPoolWaitTime,
		Dims: module.Dims{
			{ID: "jdbc_%s_wait_time", Name: "wait_time", Div: precision},
		},
	},
	{
		ID:       "jdbc_%s_use_time",
		Title:    "JDBC Pool Average Use Time",
		Units:    "milliseconds",
		Fam:      "jdbc pools",
		Ctx:      "websphere_jmx.jdbc_pool_use_time",
		Priority: prioJDBCPoolUseTime,
		Dims: module.Dims{
			{ID: "jdbc_%s_use_time", Name: "use_time", Div: precision},
		},
	},
	{
		ID:       "jdbc_%s_connections",
		Title:    "JDBC Pool Connections Created/Destroyed",
		Units:    "connections",
		Fam:      "jdbc pools",
		Ctx:      "websphere_jmx.jdbc_pool_connections",
		Priority: prioJDBCPoolCreatedDestroyed,
		Dims: module.Dims{
			{ID: "jdbc_%s_total_created", Name: "created"},
			{ID: "jdbc_%s_total_destroyed", Name: "destroyed"},
		},
	},
}

// JMS Destination Charts Template
var jmsDestinationChartsTmpl = module.Charts{
	{
		ID:       "jms_%s_messages",
		Title:    "JMS %s Messages",
		Units:    "messages",
		Fam:      "jms",
		Ctx:      "websphere_jmx.jms_messages",
		Priority: prioJMSMessages,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jms_%s_messages_current", Name: "current"},
			{ID: "jms_%s_messages_pending", Name: "pending"},
		},
	},
	{
		ID:       "jms_%s_messages_total",
		Title:    "JMS %s Total Messages",
		Units:    "messages",
		Fam:      "jms",
		Ctx:      "websphere_jmx.jms_messages_total",
		Priority: prioJMSMessages + 1,
		Dims: module.Dims{
			{ID: "jms_%s_messages_total", Name: "total"},
		},
	},
	{
		ID:       "jms_%s_consumers",
		Title:    "JMS %s Consumers",
		Units:    "consumers",
		Fam:      "jms",
		Ctx:      "websphere_jmx.jms_consumers",
		Priority: prioJMSConsumers,
		Dims: module.Dims{
			{ID: "jms_%s_consumers", Name: "consumers"},
		},
	},
}

// Application Charts Template
var applicationChartsTmpl = module.Charts{
	{
		ID:       "app_%s_requests",
		Title:    "Application Requests",
		Units:    "requests",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_requests",
		Priority: prioAppRequests,
		Dims: module.Dims{
			{ID: "app_%s_requests", Name: "requests"},
		},
	},
	{
		ID:       "app_%s_errors",
		Title:    "Application Errors",
		Units:    "errors",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_errors",
		Priority: prioAppRequests + 1,
		Dims: module.Dims{
			{ID: "app_%s_errors", Name: "errors"},
		},
	},
	{
		ID:       "app_%s_response_time",
		Title:    "Application Response Time",
		Units:    "milliseconds",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_response_time",
		Priority: prioAppResponseTime,
		Dims: module.Dims{
			{ID: "app_%s_response_time", Name: "avg", Div: precision},
			{ID: "app_%s_max_response_time", Name: "max", Div: precision},
		},
	},
}

// Session Charts Template (conditional)
var sessionChartsTmpl = module.Charts{
	{
		ID:       "app_%s_sessions",
		Title:    "Application Sessions",
		Units:    "sessions",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_sessions",
		Priority: prioAppSessions,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "app_%s_sessions_active", Name: "active"},
			{ID: "app_%s_sessions_live", Name: "live"},
		},
	},
	{
		ID:       "app_%s_sessions_invalidated",
		Title:    "Application Sessions Invalidated",
		Units:    "sessions",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_sessions_invalidated",
		Priority: prioAppSessions + 1,
		Dims: module.Dims{
			{ID: "app_%s_sessions_invalidated", Name: "invalidated"},
		},
	},
}

// Transaction Charts Template (conditional)
var transactionChartsTmpl = module.Charts{
	{
		ID:       "app_%s_transactions_active",
		Title:    "Application Active Transactions",
		Units:    "transactions",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_transactions_active",
		Priority: prioAppTransactions,
		Dims: module.Dims{
			{ID: "app_%s_transactions_active", Name: "active"},
		},
	},
	{
		ID:       "app_%s_transactions_total",
		Title:    "Application Transaction Outcomes",
		Units:    "transactions",
		Fam:      "applications",
		Ctx:      "websphere_jmx.app_transactions_total",
		Priority: prioAppTransactions + 1,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "app_%s_transactions_committed", Name: "committed"},
			{ID: "app_%s_transactions_rolledback", Name: "rolledback"},
			{ID: "app_%s_transactions_timeout", Name: "timeout"},
		},
	},
}

// Cluster charts (only for deployment managers)
var clusterCharts = module.Charts{
	{
		ID:       "cluster_state",
		Title:    "Cluster State",
		Units:    "state",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.cluster_state",
		Priority: prioClusterState,
		Dims: module.Dims{
			{ID: "cluster_state", Name: "state"},
		},
	},
	{
		ID:       "cluster_members",
		Title:    "Cluster Members",
		Units:    "members",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.cluster_members",
		Priority: prioClusterMembers,
		Dims: module.Dims{
			{ID: "cluster_running_members", Name: "running"},
			{ID: "cluster_target_members", Name: "target"},
		},
	},
	{
		ID:       "cluster_workload",
		Title:    "Cluster Workload Management",
		Units:    "status",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.cluster_workload",
		Priority: prioClusterMembers + 1,
		Dims: module.Dims{
			{ID: "cluster_wlm_enabled", Name: "wlm_enabled"},
			{ID: "cluster_session_affinity", Name: "session_affinity"},
		},
	},
	// HAManager
	{
		ID:       "ha_manager",
		Title:    "High Availability Manager",
		Units:    "status",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.ha_manager",
		Priority: prioHAManager,
		Dims: module.Dims{
			{ID: "ha_coordinator", Name: "is_coordinator"},
			{ID: "ha_core_group_size", Name: "core_group_size"},
		},
	},
	{
		ID:       "ha_bulletins",
		Title:    "HA Manager Bulletins",
		Units:    "bulletins",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.ha_bulletins",
		Priority: prioHAManager + 1,
		Dims: module.Dims{
			{ID: "ha_bulletins_sent", Name: "sent"},
		},
	},
	// Dynamic cluster
	{
		ID:       "dynamic_cluster_instances",
		Title:    "Dynamic Cluster Instances",
		Units:    "instances",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.dynamic_cluster_instances",
		Priority: prioDynamicCluster,
		Dims: module.Dims{
			{ID: "dynamic_cluster_min_instances", Name: "min"},
			{ID: "dynamic_cluster_max_instances", Name: "max"},
			{ID: "dynamic_cluster_target_instances", Name: "target"},
		},
	},
	// Replication
	{
		ID:       "replication_traffic",
		Title:    "Replication Traffic",
		Units:    "bytes/s",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.replication_traffic",
		Priority: prioReplication,
		Dims: module.Dims{
			{ID: "replication_bytes_sent", Name: "sent", Algo: module.Incremental},
			{ID: "replication_bytes_received", Name: "received", Algo: module.Incremental},
		},
	},
	{
		ID:       "replication_health",
		Title:    "Replication Health",
		Units:    "operations",
		Fam:      "cluster",
		Ctx:      "websphere_jmx.replication_health",
		Priority: prioReplication + 1,
		Dims: module.Dims{
			{ID: "replication_sync_failures", Name: "sync_failures"},
			{ID: "replication_async_queue_depth", Name: "async_queue_depth"},
		},
	},
}

func (w *WebSphereJMX) initCharts() {
	w.charts = baseCharts.Copy()

	// Add cluster charts if this is a deployment manager
	if w.ServerType == "dmgr" && w.CollectClusterMetrics {
		w.charts.Add(*clusterCharts.Copy()...)
	}

	// Add cluster labels to all charts
	w.addClusterLabels(w.charts)
}

func (w *WebSphereJMX) addClusterLabels(charts *module.Charts) {
	for _, chart := range *charts {
		// Add standard cluster labels
		if w.ClusterName != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "cluster", Value: w.ClusterName})
		}
		if w.CellName != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "cell", Value: w.CellName})
		}
		if w.NodeName != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "node", Value: w.NodeName})
		}
		if w.ServerType != "" {
			chart.Labels = append(chart.Labels, module.Label{Key: "server_type", Value: w.ServerType})
		}

		// Add custom labels
		for k, v := range w.CustomLabels {
			chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
		}
	}
}

func (w *WebSphereJMX) newThreadPoolCharts(pool string) *module.Charts {
	charts := threadPoolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(pool))
		chart.Labels = []module.Label{
			{Key: "thread_pool", Value: pool},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(pool))
		}
	}

	// Add cluster labels
	w.addClusterLabels(charts)

	return charts
}

func (w *WebSphereJMX) newJDBCPoolCharts(pool string) *module.Charts {
	charts := jdbcPoolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(pool))
		chart.Labels = []module.Label{
			{Key: "jdbc_pool", Value: pool},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(pool))
		}
	}

	// Add cluster labels
	w.addClusterLabels(charts)

	return charts
}

func (w *WebSphereJMX) newJMSDestinationCharts(dest string, destType string) *module.Charts {
	charts := jmsDestinationChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(dest))
		chart.Title = fmt.Sprintf(chart.Title, destType)
		chart.Labels = []module.Label{
			{Key: "jms_destination", Value: dest},
			{Key: "jms_type", Value: destType},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(dest))
		}
	}

	// Add cluster labels
	w.addClusterLabels(charts)

	return charts
}

func (w *WebSphereJMX) newApplicationCharts(app string, includeSessions, includeTransactions bool) *module.Charts {
	charts := applicationChartsTmpl.Copy()

	// Add session charts if enabled
	if includeSessions {
		charts.Add(*sessionChartsTmpl.Copy()...)
	}

	// Add transaction charts if enabled
	if includeTransactions {
		charts.Add(*transactionChartsTmpl.Copy()...)
	}

	// Update all charts with application-specific information
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(app))
		chart.Labels = []module.Label{
			{Key: "application", Value: app},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(app))
		}
	}

	// Add cluster labels
	w.addClusterLabels(charts)

	return charts
}
