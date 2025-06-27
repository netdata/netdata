// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_mp

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	// Critical JVM metrics
	prioJVMHeap      = module.Priority + 100  // Memory is most critical
	prioCPU          = module.Priority + 200  // CPU usage is second most important
	prioJVMGC        = module.Priority + 300  // GC affects performance
	prioJVMThreads   = module.Priority + 400  // Thread issues cause hangs
	
	// Application performance metrics
	prioServlet      = module.Priority + 500  // Request handling performance
	prioSession      = module.Priority + 600  // Session management
	prioThreadPool   = module.Priority + 700  // Thread pool saturation
	
	// Less critical metrics
	prioJVMClasses   = module.Priority + 800  // Class loading issues are rare
	prioRESTRequests = module.Priority + 900  // REST endpoint specific
	prioRESTTiming   = module.Priority + 1000 // REST timing details
	
	// Informational metrics
	prioMPHealth     = module.Priority + 1100 // Health status
	prioCustomMetrics = module.Priority + 1200 // Custom application metrics
	prioOtherMetrics = module.Priority + 1300 // Everything else
)

func (w *WebSphereMicroProfile) initCharts() {
	// Initialize with basic JVM charts
	charts := module.Charts{}

	if w.CollectJVMMetrics {
		charts = append(charts, *newBaseJVMCharts()...)
	}

	w.charts = &charts
}

func newBaseJVMCharts() *module.Charts {
	return &module.Charts{
		{
			ID:       "jvm_memory_heap_usage",
			Title:    "JVM Heap Memory Usage",
			Units:    "bytes",
			Fam:      "jvm/memory",
			Ctx:      "websphere_mp.jvm_memory_heap_usage",
			Priority: prioJVMHeap,
			Type:     module.Stacked,
			Dims: module.Dims{
				{ID: "jvm_memory_heap_used", Name: "used"},
				{ID: "jvm_memory_heap_free", Name: "free"},
			},
		},
		{
			ID:       "jvm_memory_heap_committed",
			Title:    "JVM Heap Memory Committed",
			Units:    "bytes",
			Fam:      "jvm/memory",
			Ctx:      "websphere_mp.jvm_memory_heap_committed",
			Priority: prioJVMHeap + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_memory_heap_committed", Name: "committed"},
			},
		},
		{
			ID:       "jvm_memory_heap_max",
			Title:    "JVM Heap Memory Maximum",
			Units:    "bytes",
			Fam:      "jvm/memory",
			Ctx:      "websphere_mp.jvm_memory_heap_max",
			Priority: prioJVMHeap + 2,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_memory_heap_max", Name: "limit"},
			},
		},
		{
			ID:       "jvm_gc_collections",
			Title:    "JVM Garbage Collection",
			Units:    "collections/s",
			Fam:      "jvm/gc",
			Ctx:      "websphere_mp.jvm_gc_collections",
			Priority: prioJVMGC,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_gc_collections_total", Name: "rate", Algo: module.Incremental},
			},
		},
		{
			ID:       "jvm_gc_time",
			Title:    "JVM GC Time",
			Units:    "milliseconds",
			Fam:      "jvm/gc",
			Ctx:      "websphere_mp.jvm_gc_time",
			Priority: prioJVMGC + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "gc_time_seconds", Name: "total", Mul: 1000 * precision},
				{ID: "gc_time_per_cycle_seconds", Name: "per_cycle", Mul: 1000 * precision},
			},
		},
		{
			ID:       "jvm_heap_utilization",
			Title:    "JVM Heap Utilization",
			Units:    "percentage",
			Fam:      "jvm/memory",
			Ctx:      "websphere_mp.jvm_heap_utilization",
			Priority: prioJVMHeap + 3,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "memory_heapUtilization_percent", Name: "utilization", Mul: precision},
			},
		},
		{
			ID:       "jvm_threads_current",
			Title:    "JVM Current Threads",
			Units:    "threads",
			Fam:      "jvm/threads",
			Ctx:      "websphere_mp.jvm_threads_current",
			Priority: prioJVMThreads,
			Type:     module.Stacked,
			Dims: module.Dims{
				{ID: "jvm_thread_daemon_count", Name: "daemon"},
				{ID: "jvm_thread_other_count", Name: "other"},
			},
		},
		{
			ID:       "jvm_threads_peak",
			Title:    "JVM Peak Threads",
			Units:    "threads",
			Fam:      "jvm/threads",
			Ctx:      "websphere_mp.jvm_threads_peak",
			Priority: prioJVMThreads + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_thread_max_count", Name: "peak"},
			},
		},
		{
			ID:       "jvm_classes_loaded",
			Title:    "JVM Classes Loaded",
			Units:    "classes",
			Fam:      "jvm/classes",
			Ctx:      "websphere_mp.jvm_classes_loaded",
			Priority: prioJVMClasses,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_classes_loaded", Name: "loaded"},
			},
		},
		{
			ID:       "jvm_classes_unloaded_rate",
			Title:    "JVM Classes Unloaded Rate",
			Units:    "classes/s",
			Fam:      "jvm/classes",
			Ctx:      "websphere_mp.jvm_classes_unloaded_rate",
			Priority: prioJVMClasses + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_classes_unloaded", Name: "unloaded", Algo: module.Incremental},
			},
		},
		// CPU metrics
		{
			ID:       "cpu_usage",
			Title:    "JVM CPU Usage",
			Units:    "percentage",
			Fam:      "cpu",
			Ctx:      "websphere_mp.cpu_usage",
			Priority: prioCPU,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "cpu_processCpuLoad_percent", Name: "process", Div: 100, Mul: precision},
				{ID: "cpu_processCpuUtilization_percent", Name: "utilization", Mul: precision},
			},
		},
		{
			ID:       "cpu_time",
			Title:    "JVM CPU Time",
			Units:    "seconds",
			Fam:      "cpu",
			Ctx:      "websphere_mp.cpu_time",
			Priority: prioCPU + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "cpu_processCpuTime_seconds", Name: "total", Mul: precision},
			},
		},
		{
			ID:       "cpu_processors",
			Title:    "Available Processors",
			Units:    "processors",
			Fam:      "cpu",
			Ctx:      "websphere_mp.cpu_processors",
			Priority: prioCPU + 2,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "cpu_availableProcessors", Name: "available"},
			},
		},
		{
			ID:       "system_load",
			Title:    "System Load Average",
			Units:    "load",
			Fam:      "cpu",
			Ctx:      "websphere_mp.system_load",
			Priority: prioCPU + 3,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "cpu_systemLoadAverage", Name: "1min", Mul: precision},
			},
		},
		// Thread pool metrics
		{
			ID:       "threadpool_usage",
			Title:    "Thread Pool Usage",
			Units:    "threads",
			Fam:      "threadpool",
			Ctx:      "websphere_mp.threadpool_usage",
			Priority: prioThreadPool,
			Type:     module.Stacked,
			Dims: module.Dims{
				{ID: "threadpool_activeThreads", Name: "active"},
				{ID: "threadpool_idle", Name: "idle"},
			},
		},
		{
			ID:       "threadpool_size",
			Title:    "Thread Pool Size",
			Units:    "threads",
			Fam:      "threadpool",
			Ctx:      "websphere_mp.threadpool_size",
			Priority: prioThreadPool + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "threadpool_size", Name: "size"},
			},
		},
		// Servlet metrics
		{
			ID:       "servlet_requests",
			Title:    "Servlet Requests",
			Units:    "requests/s",
			Fam:      "servlet",
			Ctx:      "websphere_mp.servlet_requests",
			Priority: prioServlet,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "servlet_request_total", Name: "requests", Algo: module.Incremental},
			},
		},
		{
			ID:       "servlet_response_time",
			Title:    "Servlet Response Time",
			Units:    "milliseconds",
			Fam:      "servlet",
			Ctx:      "websphere_mp.servlet_response_time",
			Priority: prioServlet + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "servlet_request_elapsedTime_per_request_seconds", Name: "avg_response_time", Mul: 1000 * precision},
			},
		},
		// Session metrics
		{
			ID:       "session_active",
			Title:    "Active Sessions",
			Units:    "sessions",
			Fam:      "session",
			Ctx:      "websphere_mp.session_active",
			Priority: prioSession,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "session_activeSessions", Name: "active"},
				{ID: "session_liveSessions", Name: "live"},
			},
		},
		{
			ID:       "session_lifecycle",
			Title:    "Session Lifecycle",
			Units:    "sessions/s",
			Fam:      "session",
			Ctx:      "websphere_mp.session_lifecycle",
			Priority: prioSession + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "session_create_total", Name: "created", Algo: module.Incremental},
				{ID: "session_invalidated_total", Name: "invalidated", Algo: module.Incremental},
				{ID: "session_invalidatedbyTimeout_total", Name: "timed_out", Algo: module.Incremental},
			},
		},
	}
}

func (w *WebSphereMicroProfile) createJVMChart(metricID, originalName string) *module.Charts {
	// Determine chart type based on metric name
	if strings.Contains(originalName, "memory") && strings.Contains(originalName, "heap") {
		return w.createMemoryChart(metricID, originalName)
	} else if strings.Contains(originalName, "gc") {
		return w.createGCChart(metricID, originalName)
	} else if strings.Contains(originalName, "thread") {
		return w.createThreadChart(metricID, originalName)
	} else if strings.Contains(originalName, "class") {
		return w.createClassChart(metricID, originalName)
	}

	return w.createGenericChart(metricID, originalName, "jvm", prioJVMHeap+100)
}

func (w *WebSphereMicroProfile) createRESTChart(metricID, originalName string) *module.Charts {
	// Extract endpoint information from metric name
	parts := strings.Split(metricID, "_")
	if len(parts) >= 3 {
		method := parts[len(parts)-2]
		endpoint := parts[len(parts)-1]

		chartID := fmt.Sprintf("rest_%s_%s", method, endpoint)
		title := fmt.Sprintf("REST %s %s", strings.ToUpper(method), endpoint)

		if strings.Contains(originalName, "count") || strings.Contains(originalName, "total") {
			return &module.Charts{
				{
					ID:       chartID + "_requests",
					Title:    title + " Requests",
					Units:    "requests/s",
					Fam:      "rest api/requests",
					Ctx:      "websphere_mp.rest_requests",
					Priority: prioRESTRequests,
					Type:     module.Line,
					Dims: module.Dims{
						{ID: metricID, Name: "requests", Algo: module.Incremental},
					},
				},
			}
		} else if strings.Contains(originalName, "time") {
			return &module.Charts{
				{
					ID:       chartID + "_timing",
					Title:    title + " Response Time",
					Units:    "milliseconds",
					Fam:      "rest api/timing",
					Ctx:      "websphere_mp.rest_timing",
					Priority: prioRESTTiming,
					Type:     module.Line,
					Dims: module.Dims{
						{ID: metricID, Name: "response_time", Div: precision},
					},
				},
			}
		}
	}

	return w.createGenericChart(metricID, originalName, "rest", prioRESTRequests+100)
}



func (w *WebSphereMicroProfile) createMemoryChart(metricID, originalName string) *module.Charts {
	return &module.Charts{
		{
			ID:       "jvm_memory_" + metricID,
			Title:    "JVM Memory - " + strings.ReplaceAll(originalName, "_", " "),
			Units:    "bytes",
			Fam:      "jvm/memory",
			Ctx:      "websphere_mp.jvm_memory",
			Priority: prioJVMHeap + 10,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "size"},
			},
		},
	}
}

func (w *WebSphereMicroProfile) createGCChart(metricID, originalName string) *module.Charts {
	if strings.Contains(originalName, "time") {
		return &module.Charts{
			{
				ID:       "jvm_gc_time_" + metricID,
				Title:    "JVM GC Time - " + strings.ReplaceAll(originalName, "_", " "),
				Units:    "milliseconds",
				Fam:      "jvm/gc",
				Ctx:      "websphere_mp.jvm_gc_time",
				Priority: prioJVMGC + 10,
				Type:     module.Line,
				Dims: module.Dims{
					{ID: metricID, Name: "duration", Div: precision},
				},
			},
		}
	} else {
		return &module.Charts{
			{
				ID:       "jvm_gc_count_" + metricID,
				Title:    "JVM GC Count - " + strings.ReplaceAll(originalName, "_", " "),
				Units:    "collections/s",
				Fam:      "jvm/gc",
				Ctx:      "websphere_mp.jvm_gc_count",
				Priority: prioJVMGC + 20,
				Type:     module.Line,
				Dims: module.Dims{
					{ID: metricID, Name: "collections", Algo: module.Incremental},
				},
			},
		}
	}
}

func (w *WebSphereMicroProfile) createThreadChart(metricID, originalName string) *module.Charts {
	return &module.Charts{
		{
			ID:       "jvm_threads_" + metricID,
			Title:    "JVM Threads - " + strings.ReplaceAll(originalName, "_", " "),
			Units:    "threads",
			Fam:      "jvm/threads",
			Ctx:      "websphere_mp.jvm_threads",
			Priority: prioJVMThreads + 10,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "threads"},
			},
		},
	}
}

func (w *WebSphereMicroProfile) createClassChart(metricID, originalName string) *module.Charts {
	return &module.Charts{
		{
			ID:       "jvm_classes_" + metricID,
			Title:    "JVM Classes - " + strings.ReplaceAll(originalName, "_", " "),
			Units:    "classes",
			Fam:      "jvm/classes",
			Ctx:      "websphere_mp.jvm_classes",
			Priority: prioJVMClasses + 10,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "classes"},
			},
		},
	}
}

func (w *WebSphereMicroProfile) createGenericChart(metricID, originalName, family string, priority int) *module.Charts {
	return &module.Charts{
		{
			ID:       family + "_" + metricID,
			Title:    strings.Title(family) + " - " + strings.ReplaceAll(originalName, "_", " "),
			Units:    "num",
			Fam:      family,
			Ctx:      "websphere_mp." + family,
			Priority: priority,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "count"},
			},
		},
	}
}

func (w *WebSphereMicroProfile) createOtherChart(metricID, originalName string) *module.Charts {
	// Determine appropriate units based on metric name
	units := "num"
	if strings.Contains(originalName, "_bytes") {
		units = "bytes"
	} else if strings.Contains(originalName, "_seconds") {
		units = "seconds"
	} else if strings.Contains(originalName, "_percent") {
		units = "percentage"
	} else if strings.Contains(originalName, "_count") || strings.Contains(originalName, "_total") {
		units = "count"
	}
	
	return &module.Charts{
		{
			ID:       "other_" + metricID,
			Title:    "Other - " + strings.ReplaceAll(originalName, "_", " "),
			Units:    units,
			Fam:      "other",
			Ctx:      "websphere_mp.other",
			Priority: prioOtherMetrics,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "value"},
			},
		},
	}
}
