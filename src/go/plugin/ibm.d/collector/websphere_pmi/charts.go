// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	precision = 1000 // Precision multiplier for floating-point values

	// Priority constants for chart ordering
	prioJVMHeap = module.Priority + iota
	prioJVMHeapUsage
	prioJVMNonHeap
	prioJVMGC
	prioJVMThreads
	prioJVMClasses
	prioJVMProcessCPU
	prioJVMUptime

	prioThreadPoolSize
	prioThreadPoolActive
	prioThreadPoolHungThreads

	prioJDBCPoolSize
	prioJDBCPoolActive
	prioJDBCPoolWaitTime
	prioJDBCPoolUseTime
	prioJDBCPoolCreatedDestroyed
	prioJDBCPoolWaitingThreads

	prioJCAPoolSize
	prioJCAPoolActive
	prioJCAPoolWaitTime
	prioJCAPoolUseTime
	prioJCAPoolCreatedDestroyed
	prioJCAPoolWaitingThreads

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

	// APM priorities
	prioServletRequests
	prioServletResponseTime
	prioServletConcurrent
	prioEJBInvocations
	prioEJBResponseTime
	prioEJBPool
	prioJDBCTimeBreakdown
	prioJDBCStmtCache
	prioJDBCConnectionReuse
)

var baseCharts = module.Charts{
	// JVM Heap Memory Usage - Additive dimensions (used + free = total)
	{
		ID:       "jvm_heap_memory_usage",
		Title:    "JVM Heap Memory Usage",
		Units:    "bytes",
		Fam:      "jvm/memory",
		Ctx:      "websphere_pmi.jvm_heap_memory_usage",
		Priority: prioJVMHeap,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jvm_heap_used", Name: "used"},
			{ID: "jvm_heap_free", Name: "free"},
		},
	},
	// JVM Heap Memory Configuration - Separate chart for non-additive limits
	{
		ID:       "jvm_heap_memory_config",
		Title:    "JVM Heap Memory Configuration",
		Units:    "bytes",
		Fam:      "jvm/memory",
		Ctx:      "websphere_pmi.jvm_heap_memory_config",
		Priority: prioJVMHeap + 1,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "jvm_heap_committed", Name: "committed"},
			{ID: "jvm_heap_max", Name: "max"},
		},
	},
	{
		ID:       "jvm_heap_usage",
		Title:    "JVM Heap Usage",
		Units:    "percentage",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_heap_usage",
		Priority: prioJVMHeapUsage,
		Dims: module.Dims{
			{ID: "jvm_heap_usage_percent", Name: "usage", Div: precision},
		},
	},

	// JVM GC
	{
		ID:       "jvm_gc_count",
		Title:    "JVM Garbage Collection Count",
		Units:    "collections/s",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_gc_count",
		Priority: prioJVMGC,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "jvm_gc_count", Name: "collections", Algo: module.Incremental},
		},
	},
	{
		ID:       "jvm_gc_time",
		Title:    "JVM Garbage Collection Time",
		Units:    "milliseconds",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_gc_time",
		Priority: prioJVMGC + 1,
		Dims: module.Dims{
			{ID: "jvm_gc_time", Name: "time", Algo: module.Incremental},
		},
	},

	// JVM Classes - Current loaded count
	{
		ID:       "jvm_classes_loaded",
		Title:    "JVM Loaded Classes",
		Units:    "classes",
		Fam:      "jvm/classes",
		Ctx:      "websphere_pmi.jvm_classes_loaded",
		Priority: prioJVMClasses,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "jvm_classes_loaded", Name: "loaded"},
		},
	},
	// JVM Classes - Unloading rate
	{
		ID:       "jvm_classes_unloaded_rate",
		Title:    "JVM Classes Unloaded Rate",
		Units:    "classes/s",
		Fam:      "jvm/classes",
		Ctx:      "websphere_pmi.jvm_classes_unloaded_rate",
		Priority: prioJVMClasses + 1,
		Type:     module.Line,
		Dims: module.Dims{
			{ID: "jvm_classes_unloaded", Name: "unloaded", Algo: module.Incremental},
		},
	},

	// JVM Process CPU
	{
		ID:       "jvm_process_cpu",
		Title:    "JVM Process CPU Usage",
		Units:    "percentage",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_process_cpu",
		Priority: prioJVMProcessCPU,
		Dims: module.Dims{
			{ID: "jvm_process_cpu_percent", Name: "cpu", Div: precision},
		},
	},

	// JVM Uptime
	{
		ID:       "jvm_uptime",
		Title:    "JVM Uptime",
		Units:    "seconds",
		Fam:      "jvm",
		Ctx:      "websphere_pmi.jvm_uptime",
		Priority: prioJVMUptime,
		Dims: module.Dims{
			{ID: "jvm_uptime", Name: "uptime"},
		},
	},
}

// Dynamic chart templates
var (
	threadPoolSizeChartTmpl = module.Chart{
		ID:       "threadpool_%s_size",
		Title:    "Thread Pool Size",
		Units:    "threads",
		Fam:      "threadpools",
		Ctx:      "websphere_pmi.threadpool_size",
		Priority: prioThreadPoolSize,
		Dims: module.Dims{
			{ID: "threadpool_%s_size", Name: "size"},
			{ID: "threadpool_%s_max_size", Name: "max_size"},
		},
	}

	threadPoolActiveChartTmpl = module.Chart{
		ID:       "threadpool_%s_active",
		Title:    "Thread Pool Active Threads",
		Units:    "threads",
		Fam:      "threadpools",
		Ctx:      "websphere_pmi.threadpool_active",
		Priority: prioThreadPoolActive,
		Dims: module.Dims{
			{ID: "threadpool_%s_active", Name: "active"},
		},
	}

	threadPoolHungChartTmpl = module.Chart{
		ID:       "threadpool_%s_hung",
		Title:    "Thread Pool Hung Threads",
		Units:    "threads",
		Fam:      "threadpools",
		Ctx:      "websphere_pmi.threadpool_hung",
		Priority: prioThreadPoolHungThreads,
		Dims: module.Dims{
			{ID: "threadpool_%s_hung", Name: "hung"},
		},
	}

	jdbcPoolSizeChartTmpl = module.Chart{
		ID:       "jdbc_%s_pool_size",
		Title:    "JDBC Connection Pool Size",
		Units:    "connections",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_pool_size",
		Priority: prioJDBCPoolSize,
		Dims: module.Dims{
			{ID: "jdbc_%s_size", Name: "size"},
			{ID: "jdbc_%s_max_size", Name: "max_size"},
		},
	}

	jdbcPoolActiveChartTmpl = module.Chart{
		ID:       "jdbc_%s_pool_active",
		Title:    "JDBC Pool Active Connections",
		Units:    "connections",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_pool_active",
		Priority: prioJDBCPoolActive,
		Dims: module.Dims{
			{ID: "jdbc_%s_active", Name: "active"},
			{ID: "jdbc_%s_free", Name: "free"},
		},
	}

	jdbcPoolWaitTimeChartTmpl = module.Chart{
		ID:       "jdbc_%s_wait_time",
		Title:    "JDBC Pool Wait Time",
		Units:    "milliseconds",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_wait_time",
		Priority: prioJDBCPoolWaitTime,
		Dims: module.Dims{
			{ID: "jdbc_%s_wait_time_avg", Name: "average", Div: precision},
			{ID: "jdbc_%s_wait_time_max", Name: "max"},
		},
	}

	jdbcPoolUseTimeChartTmpl = module.Chart{
		ID:       "jdbc_%s_use_time",
		Title:    "JDBC Connection Use Time",
		Units:    "milliseconds",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_use_time",
		Priority: prioJDBCPoolUseTime,
		Dims: module.Dims{
			{ID: "jdbc_%s_use_time_avg", Name: "average", Div: precision},
			{ID: "jdbc_%s_use_time_max", Name: "max"},
		},
	}

	jdbcPoolCreatedDestroyedChartTmpl = module.Chart{
		ID:       "jdbc_%s_created_destroyed",
		Title:    "JDBC Connections Created/Destroyed",
		Units:    "connections/s",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_created_destroyed",
		Priority: prioJDBCPoolCreatedDestroyed,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jdbc_%s_created", Name: "created", Algo: module.Incremental},
			{ID: "jdbc_%s_destroyed", Name: "destroyed", Algo: module.Incremental, Mul: -1},
		},
	}

	jdbcPoolWaitingThreadsChartTmpl = module.Chart{
		ID:       "jdbc_%s_waiting_threads",
		Title:    "JDBC Pool Waiting Threads",
		Units:    "threads",
		Fam:      "jdbc",
		Ctx:      "websphere_pmi.jdbc_waiting_threads",
		Priority: prioJDBCPoolWaitingThreads,
		Dims: module.Dims{
			{ID: "jdbc_%s_waiting_threads", Name: "waiting"},
		},
	}

	jcaPoolSizeChartTmpl = module.Chart{
		ID:       "jca_%s_pool_size",
		Title:    "JCA Connection Pool Size",
		Units:    "connections",
		Fam:      "jca",
		Ctx:      "websphere_pmi.jca_pool_size",
		Priority: prioJCAPoolSize,
		Dims: module.Dims{
			{ID: "jca_%s_size", Name: "size"},
			{ID: "jca_%s_max_size", Name: "max_size"},
		},
	}

	jcaPoolActiveChartTmpl = module.Chart{
		ID:       "jca_%s_pool_active",
		Title:    "JCA Pool Active Connections",
		Units:    "connections",
		Fam:      "jca",
		Ctx:      "websphere_pmi.jca_pool_active",
		Priority: prioJCAPoolActive,
		Dims: module.Dims{
			{ID: "jca_%s_active", Name: "active"},
			{ID: "jca_%s_free", Name: "free"},
		},
	}

	jcaPoolWaitTimeChartTmpl = module.Chart{
		ID:       "jca_%s_wait_time",
		Title:    "JCA Pool Wait Time",
		Units:    "milliseconds",
		Fam:      "jca",
		Ctx:      "websphere_pmi.jca_wait_time",
		Priority: prioJCAPoolWaitTime,
		Dims: module.Dims{
			{ID: "jca_%s_wait_time_avg", Name: "average", Div: precision},
			{ID: "jca_%s_wait_time_max", Name: "max"},
		},
	}

	jcaPoolUseTimeChartTmpl = module.Chart{
		ID:       "jca_%s_use_time",
		Title:    "JCA Connection Use Time",
		Units:    "milliseconds",
		Fam:      "jca",
		Ctx:      "websphere_pmi.jca_use_time",
		Priority: prioJCAPoolUseTime,
		Dims: module.Dims{
			{ID: "jca_%s_use_time_avg", Name: "average", Div: precision},
			{ID: "jca_%s_use_time_max", Name: "max"},
		},
	}

	jcaPoolCreatedDestroyedChartTmpl = module.Chart{
		ID:       "jca_%s_created_destroyed",
		Title:    "JCA Connections Created/Destroyed",
		Units:    "connections/s",
		Fam:      "jca",
		Ctx:      "websphere_pmi.jca_created_destroyed",
		Priority: prioJCAPoolCreatedDestroyed,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jca_%s_created", Name: "created", Algo: module.Incremental},
			{ID: "jca_%s_destroyed", Name: "destroyed", Algo: module.Incremental, Mul: -1},
		},
	}

	jcaPoolWaitingThreadsChartTmpl = module.Chart{
		ID:       "jca_%s_waiting_threads",
		Title:    "JCA Pool Waiting Threads",
		Units:    "threads",
		Fam:      "jca",
		Ctx:      "websphere_pmi.jca_waiting_threads",
		Priority: prioJCAPoolWaitingThreads,
		Dims: module.Dims{
			{ID: "jca_%s_waiting_threads", Name: "waiting"},
		},
	}

	appRequestsChartTmpl = module.Chart{
		ID:       "app_%s_requests",
		Title:    "Application Requests",
		Units:    "requests/s",
		Fam:      "apps",
		Ctx:      "websphere_pmi.app_requests",
		Priority: prioAppRequests,
		Dims: module.Dims{
			{ID: "app_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	}

	appResponseTimeChartTmpl = module.Chart{
		ID:       "app_%s_response_time",
		Title:    "Application Response Time",
		Units:    "milliseconds",
		Fam:      "apps",
		Ctx:      "websphere_pmi.app_response_time",
		Priority: prioAppResponseTime,
		Dims: module.Dims{
			{ID: "app_%s_response_time_avg", Name: "average", Div: precision},
			{ID: "app_%s_response_time_max", Name: "max"},
		},
	}

	appSessionsChartTmpl = module.Chart{
		ID:       "app_%s_sessions",
		Title:    "Application Sessions",
		Units:    "sessions",
		Fam:      "apps",
		Ctx:      "websphere_pmi.app_sessions",
		Priority: prioAppSessions,
		Dims: module.Dims{
			{ID: "app_%s_sessions_active", Name: "active"},
			{ID: "app_%s_sessions_live", Name: "live"},
		},
	}

	servletRequestsChartTmpl = module.Chart{
		ID:       "servlet_%s_requests",
		Title:    "Servlet Requests",
		Units:    "requests/s",
		Fam:      "servlets",
		Ctx:      "websphere_pmi.servlet_requests",
		Priority: prioServletRequests,
		Dims: module.Dims{
			{ID: "servlet_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "servlet_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	}

	servletResponseTimeChartTmpl = module.Chart{
		ID:       "servlet_%s_response_time",
		Title:    "Servlet Response Time",
		Units:    "milliseconds",
		Fam:      "servlets",
		Ctx:      "websphere_pmi.servlet_response_time",
		Priority: prioServletResponseTime,
		Dims: module.Dims{
			{ID: "servlet_%s_response_time_avg", Name: "average", Div: precision},
			{ID: "servlet_%s_response_time_max", Name: "max"},
		},
	}

	ejbInvocationsChartTmpl = module.Chart{
		ID:       "ejb_%s_invocations",
		Title:    "EJB Method Invocations",
		Units:    "invocations/s",
		Fam:      "ejbs",
		Ctx:      "websphere_pmi.ejb_invocations",
		Priority: prioEJBInvocations,
		Dims: module.Dims{
			{ID: "ejb_%s_invocations", Name: "invocations", Algo: module.Incremental},
		},
	}

	ejbResponseTimeChartTmpl = module.Chart{
		ID:       "ejb_%s_response_time",
		Title:    "EJB Method Response Time",
		Units:    "milliseconds",
		Fam:      "ejbs",
		Ctx:      "websphere_pmi.ejb_response_time",
		Priority: prioEJBResponseTime,
		Dims: module.Dims{
			{ID: "ejb_%s_response_time_avg", Name: "average", Div: precision},
			{ID: "ejb_%s_response_time_max", Name: "max"},
		},
	}

	ejbPoolChartTmpl = module.Chart{
		ID:       "ejb_%s_pool",
		Title:    "EJB Bean Pool",
		Units:    "beans",
		Fam:      "ejbs",
		Ctx:      "websphere_pmi.ejb_pool",
		Priority: prioEJBPool,
		Dims: module.Dims{
			{ID: "ejb_%s_pool_size", Name: "size"},
			{ID: "ejb_%s_pool_active", Name: "active"},
		},
	}
)

func (w *WebSpherePMI) addThreadPoolCharts(pool string) {
	poolID := cleanID(pool)

	charts := module.Charts{
		threadPoolSizeChartTmpl.Copy(),
		threadPoolActiveChartTmpl.Copy(),
		threadPoolHungChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, poolID)
		chart.Labels = []module.Label{
			{Key: "pool", Value: pool},
		}
		// Add version labels if available
		if w.wasVersion != "" {
			chart.Labels = append(chart.Labels,
				module.Label{Key: "was_version", Value: w.wasVersion},
				module.Label{Key: "was_edition", Value: w.wasEdition},
			)
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, poolID)
		}
		w.charts.Add(chart)
	}

}

func (w *WebSpherePMI) removeThreadPoolCharts(pool string) {
	poolID := cleanID(pool)

	w.charts.Remove(fmt.Sprintf("threadpool_%s_size", poolID))
	w.charts.Remove(fmt.Sprintf("threadpool_%s_active", poolID))
	w.charts.Remove(fmt.Sprintf("threadpool_%s_hung", poolID))
}

func (w *WebSpherePMI) addJDBCPoolCharts(pool string) {
	poolID := cleanID(pool)

	charts := module.Charts{
		jdbcPoolSizeChartTmpl.Copy(),
		jdbcPoolActiveChartTmpl.Copy(),
		jdbcPoolWaitTimeChartTmpl.Copy(),
		jdbcPoolUseTimeChartTmpl.Copy(),
		jdbcPoolCreatedDestroyedChartTmpl.Copy(),
		jdbcPoolWaitingThreadsChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, poolID)
		chart.Labels = []module.Label{
			{Key: "datasource", Value: pool},
		}
		// Add version labels if available
		if w.wasVersion != "" {
			chart.Labels = append(chart.Labels,
				module.Label{Key: "was_version", Value: w.wasVersion},
				module.Label{Key: "was_edition", Value: w.wasEdition},
			)
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, poolID)
		}
		w.charts.Add(chart)
	}

}

func (w *WebSpherePMI) removeJDBCPoolCharts(pool string) {
	poolID := cleanID(pool)

	w.charts.Remove(fmt.Sprintf("jdbc_%s_pool_size", poolID))
	w.charts.Remove(fmt.Sprintf("jdbc_%s_pool_active", poolID))
	w.charts.Remove(fmt.Sprintf("jdbc_%s_wait_time", poolID))
	w.charts.Remove(fmt.Sprintf("jdbc_%s_use_time", poolID))
	w.charts.Remove(fmt.Sprintf("jdbc_%s_created_destroyed", poolID))
	w.charts.Remove(fmt.Sprintf("jdbc_%s_waiting_threads", poolID))
}

func (w *WebSpherePMI) addJCAPoolCharts(pool string) {
	poolID := cleanID(pool)

	charts := module.Charts{
		jcaPoolSizeChartTmpl.Copy(),
		jcaPoolActiveChartTmpl.Copy(),
		jcaPoolWaitTimeChartTmpl.Copy(),
		jcaPoolUseTimeChartTmpl.Copy(),
		jcaPoolCreatedDestroyedChartTmpl.Copy(),
		jcaPoolWaitingThreadsChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, poolID)
		chart.Labels = []module.Label{
			{Key: "jca_resource", Value: pool},
		}
		// Add version labels if available
		if w.wasVersion != "" {
			chart.Labels = append(chart.Labels,
				module.Label{Key: "was_version", Value: w.wasVersion},
				module.Label{Key: "was_edition", Value: w.wasEdition},
			)
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, poolID)
		}
		w.charts.Add(chart)
	}

}

func (w *WebSpherePMI) removeJCAPoolCharts(pool string) {
	poolID := cleanID(pool)

	w.charts.Remove(fmt.Sprintf("jca_%s_pool_size", poolID))
	w.charts.Remove(fmt.Sprintf("jca_%s_pool_active", poolID))
	w.charts.Remove(fmt.Sprintf("jca_%s_wait_time", poolID))
	w.charts.Remove(fmt.Sprintf("jca_%s_use_time", poolID))
	w.charts.Remove(fmt.Sprintf("jca_%s_created_destroyed", poolID))
	w.charts.Remove(fmt.Sprintf("jca_%s_waiting_threads", poolID))
}

func (w *WebSpherePMI) addAppCharts(app string) {
	appID := cleanID(app)

	charts := module.Charts{
		appRequestsChartTmpl.Copy(),
		appResponseTimeChartTmpl.Copy(),
		appSessionsChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, appID)
		chart.Labels = []module.Label{
			{Key: "application", Value: app},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, appID)
		}
		w.charts.Add(chart)
	}

}

func (w *WebSpherePMI) removeAppCharts(app string) {
	appID := cleanID(app)

	w.charts.Remove(fmt.Sprintf("app_%s_requests", appID))
	w.charts.Remove(fmt.Sprintf("app_%s_response_time", appID))
	w.charts.Remove(fmt.Sprintf("app_%s_sessions", appID))
}

func (w *WebSpherePMI) addServletCharts(servlet string) {
	servletID := cleanID(servlet)

	charts := module.Charts{
		servletRequestsChartTmpl.Copy(),
		servletResponseTimeChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, servletID)
		chart.Labels = []module.Label{
			{Key: "servlet", Value: servlet},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, servletID)
		}
		w.charts.Add(chart)
	}

}

func (w *WebSpherePMI) removeServletCharts(servlet string) {
	servletID := cleanID(servlet)

	w.charts.Remove(fmt.Sprintf("servlet_%s_requests", servletID))
	w.charts.Remove(fmt.Sprintf("servlet_%s_response_time", servletID))
}

func (w *WebSpherePMI) addEJBCharts(ejb string) {
	ejbID := cleanID(ejb)

	charts := module.Charts{
		ejbInvocationsChartTmpl.Copy(),
		ejbResponseTimeChartTmpl.Copy(),
		ejbPoolChartTmpl.Copy(),
	}

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, ejbID)
		chart.Labels = []module.Label{
			{Key: "ejb", Value: ejb},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ejbID)
		}
		w.charts.Add(chart)
	}

}

func (w *WebSpherePMI) removeEJBCharts(ejb string) {
	ejbID := cleanID(ejb)

	w.charts.Remove(fmt.Sprintf("ejb_%s_invocations", ejbID))
	w.charts.Remove(fmt.Sprintf("ejb_%s_response_time", ejbID))
	w.charts.Remove(fmt.Sprintf("ejb_%s_pool", ejbID))
}

func cleanID(name string) string {
	// Replace problematic characters with underscores
	replacer := strings.NewReplacer(
		" ", "_",
		".", "_",
		"/", "_",
		"\\", "_",
		":", "_",
		"=", "_",
		",", "_",
		"(", "_",
		")", "_",
		"[", "_",
		"]", "_",
		"{", "_",
		"}", "_",
		"#", "_",
		"*", "_",
		"?", "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\"", "_",
		"'", "_",
	)
	return replacer.Replace(name)
}
