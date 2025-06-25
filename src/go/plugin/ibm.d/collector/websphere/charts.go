// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioJVMHeap = module.Priority + iota
	prioJVMGC
	prioJVMThreads
	prioJVMClasses
	
	prioThreadPools
	prioThreadPoolSize
	prioThreadPoolActive
	prioThreadPoolHung
	
	prioConnectionPools
	prioConnectionPoolSize
	prioConnectionPoolWait
	
	prioWebContainer
	prioSessions
	prioRequests
	prioResponseTime
	
	prioApplications
	prioAppRequests
	prioAppErrors
)

var baseCharts = module.Charts{
	// JVM Metrics
	{
		ID:       "jvm_heap_usage",
		Title:    "JVM Heap Usage",
		Units:    "MiB",
		Fam:      "jvm",
		Ctx:      "websphere.jvm_heap_usage",
		Priority: prioJVMHeap,
		Type:     module.Stacked,
		Dims: module.Dims{
			{ID: "jvm_heap_used", Name: "used"},
			{ID: "jvm_heap_committed", Name: "committed"},
			{ID: "jvm_heap_max", Name: "max"},
		},
	},
	{
		ID:       "jvm_gc_time",
		Title:    "JVM Garbage Collection Time",
		Units:    "milliseconds",
		Fam:      "jvm",
		Ctx:      "websphere.jvm_gc_time",
		Priority: prioJVMGC,
		Dims: module.Dims{
			{ID: "jvm_gc_time", Name: "gc_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "jvm_gc_count",
		Title:    "JVM Garbage Collection Count",
		Units:    "collections/s",
		Fam:      "jvm",
		Ctx:      "websphere.jvm_gc_count",
		Priority: prioJVMGC,
		Dims: module.Dims{
			{ID: "jvm_gc_count", Name: "collections", Algo: module.Incremental},
		},
	},
	{
		ID:       "jvm_threads",
		Title:    "JVM Threads",
		Units:    "threads",
		Fam:      "jvm",
		Ctx:      "websphere.jvm_threads",
		Priority: prioJVMThreads,
		Dims: module.Dims{
			{ID: "jvm_thread_count", Name: "threads"},
			{ID: "jvm_thread_daemon", Name: "daemon"},
			{ID: "jvm_thread_peak", Name: "peak"},
		},
	},
	{
		ID:       "jvm_classes",
		Title:    "JVM Loaded Classes",
		Units:    "classes",
		Fam:      "jvm",
		Ctx:      "websphere.jvm_classes",
		Priority: prioJVMClasses,
		Dims: module.Dims{
			{ID: "jvm_classes_loaded", Name: "loaded"},
			{ID: "jvm_classes_unloaded", Name: "unloaded"},
		},
	},
	
	// Web Container Metrics
	{
		ID:       "web_sessions",
		Title:    "Web Sessions",
		Units:    "sessions",
		Fam:      "web",
		Ctx:      "websphere.web_sessions",
		Priority: prioSessions,
		Dims: module.Dims{
			{ID: "web_sessions_active", Name: "active"},
			{ID: "web_sessions_live", Name: "live"},
			{ID: "web_sessions_invalidated", Name: "invalidated", Algo: module.Incremental},
		},
	},
	{
		ID:       "web_requests",
		Title:    "Web Requests",
		Units:    "requests/s",
		Fam:      "web",
		Ctx:      "websphere.web_requests",
		Priority: prioRequests,
		Dims: module.Dims{
			{ID: "web_requests_total", Name: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "web_errors",
		Title:    "Web Errors",
		Units:    "errors/s",
		Fam:      "web",
		Ctx:      "websphere.web_errors",
		Priority: prioRequests,
		Dims: module.Dims{
			{ID: "web_errors_400", Name: "4xx", Algo: module.Incremental},
			{ID: "web_errors_500", Name: "5xx", Algo: module.Incremental},
		},
	},
}

// Thread pool chart template
var threadPoolChartsTmpl = module.Charts{
	{
		ID:       "threadpool_%s_size",
		Title:    "Thread Pool Size",
		Units:    "threads",
		Fam:      "threadpools",
		Ctx:      "websphere.threadpool_size",
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
		Fam:      "threadpools",
		Ctx:      "websphere.threadpool_active",
		Priority: prioThreadPoolActive,
		Dims: module.Dims{
			{ID: "threadpool_%s_active", Name: "active"},
		},
	},
	{
		ID:       "threadpool_%s_hung",
		Title:    "Thread Pool Hung Threads",
		Units:    "threads",
		Fam:      "threadpools",
		Ctx:      "websphere.threadpool_hung",
		Priority: prioThreadPoolHung,
		Dims: module.Dims{
			{ID: "threadpool_%s_hung", Name: "hung"},
		},
	},
}

// Connection pool chart template
var connectionPoolChartsTmpl = module.Charts{
	{
		ID:       "connpool_%s_size",
		Title:    "Connection Pool Size",
		Units:    "connections",
		Fam:      "connpools",
		Ctx:      "websphere.connpool_size",
		Priority: prioConnectionPoolSize,
		Dims: module.Dims{
			{ID: "connpool_%s_size", Name: "size"},
			{ID: "connpool_%s_free", Name: "free"},
			{ID: "connpool_%s_max", Name: "max"},
		},
	},
	{
		ID:       "connpool_%s_wait_time",
		Title:    "Connection Pool Wait Time",
		Units:    "milliseconds",
		Fam:      "connpools",
		Ctx:      "websphere.connpool_wait_time",
		Priority: prioConnectionPoolWait,
		Dims: module.Dims{
			{ID: "connpool_%s_wait_time_avg", Name: "avg_wait"},
		},
	},
	{
		ID:       "connpool_%s_timeouts",
		Title:    "Connection Pool Timeouts",
		Units:    "timeouts/s",
		Fam:      "connpools",
		Ctx:      "websphere.connpool_timeouts",
		Priority: prioConnectionPoolWait,
		Dims: module.Dims{
			{ID: "connpool_%s_timeouts", Name: "timeouts", Algo: module.Incremental},
		},
	},
}

// Application chart template
var applicationChartsTmpl = module.Charts{
	{
		ID:       "app_%s_requests",
		Title:    "Application Requests",
		Units:    "requests/s",
		Fam:      "applications",
		Ctx:      "websphere.app_requests",
		Priority: prioAppRequests,
		Dims: module.Dims{
			{ID: "app_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "app_%s_response_time",
		Title:    "Application Response Time",
		Units:    "milliseconds",
		Fam:      "applications",
		Ctx:      "websphere.app_response_time",
		Priority: prioResponseTime,
		Dims: module.Dims{
			{ID: "app_%s_response_time_avg", Name: "avg_response"},
		},
	},
	{
		ID:       "app_%s_errors",
		Title:    "Application Errors",
		Units:    "errors/s",
		Fam:      "applications",
		Ctx:      "websphere.app_errors",
		Priority: prioAppErrors,
		Dims: module.Dims{
			{ID: "app_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	},
}

func (w *WebSphere) initCharts() {
	w.charts = baseCharts.Copy()
}

func newThreadPoolCharts(pool string) *module.Charts {
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
	
	return charts
}

func newConnectionPoolCharts(pool string) *module.Charts {
	charts := connectionPoolChartsTmpl.Copy()
	
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(pool))
		chart.Labels = []module.Label{
			{Key: "connection_pool", Value: pool},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(pool))
		}
	}
	
	return charts
}

func newApplicationCharts(app string) *module.Charts {
	charts := applicationChartsTmpl.Copy()
	
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanName(app))
		chart.Labels = []module.Label{
			{Key: "application", Value: app},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cleanName(app))
		}
	}
	
	return charts
}

func cleanName(name string) string {
	// Replace special characters with underscores for metric names
	r := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
		",", "_",
		"(", "_",
		")", "_",
	)
	return strings.ToLower(r.Replace(name))
}