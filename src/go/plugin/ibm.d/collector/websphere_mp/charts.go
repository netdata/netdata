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
	prioJVMHeap = module.Priority + iota
	prioJVMGC
	prioJVMThreads
	prioJVMClasses
	prioRESTRequests
	prioRESTTiming
	prioMPHealth
	prioMPMetrics
	prioCustomMetrics
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
			ID:       "jvm_memory_heap",
			Title:    "JVM Heap Memory Usage",
			Units:    "bytes",
			Fam:      "jvm memory",
			Ctx:      "websphere_mp.jvm_memory_heap",
			Priority: prioJVMHeap,
			Type:     module.Stacked,
			Dims: module.Dims{
				{ID: "jvm_memory_heap_used", Name: "used"},
				{ID: "jvm_memory_heap_committed", Name: "committed"},
			},
		},
		{
			ID:       "jvm_memory_heap_max",
			Title:    "JVM Heap Memory Maximum",
			Units:    "bytes",
			Fam:      "jvm memory",
			Ctx:      "websphere_mp.jvm_memory_heap_max",
			Priority: prioJVMHeap + 1,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_memory_heap_max", Name: "max"},
			},
		},
		{
			ID:       "jvm_gc_collections",
			Title:    "JVM Garbage Collection",
			Units:    "collections/s",
			Fam:      "jvm gc",
			Ctx:      "websphere_mp.jvm_gc_collections",
			Priority: prioJVMGC,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_gc_collections_total", Name: "collections", Algo: module.Incremental},
			},
		},
		{
			ID:       "jvm_threads",
			Title:    "JVM Threads",
			Units:    "threads",
			Fam:      "jvm threads",
			Ctx:      "websphere_mp.jvm_threads",
			Priority: prioJVMThreads,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_thread_count", Name: "threads"},
				{ID: "jvm_thread_daemon_count", Name: "daemon"},
				{ID: "jvm_thread_max_count", Name: "max"},
			},
		},
		{
			ID:       "jvm_classes",
			Title:    "JVM Classes",
			Units:    "classes",
			Fam:      "jvm classes",
			Ctx:      "websphere_mp.jvm_classes",
			Priority: prioJVMClasses,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: "jvm_classes_loaded", Name: "loaded"},
				{ID: "jvm_classes_unloaded", Name: "unloaded", Algo: module.Incremental},
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
					Fam:      "rest requests",
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
					Fam:      "rest timing",
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

func (w *WebSphereMicroProfile) createMPChart(metricID, originalName string) *module.Charts {
	if strings.Contains(originalName, "health") {
		return &module.Charts{
			{
				ID:       "mp_health_" + metricID,
				Title:    "MicroProfile Health",
				Units:    "status",
				Fam:      "mp health",
				Ctx:      "websphere_mp.mp_health",
				Priority: prioMPHealth,
				Type:     module.Line,
				Dims: module.Dims{
					{ID: metricID, Name: "status"},
				},
			},
		}
	}

	return w.createGenericChart(metricID, originalName, "mp", prioMPMetrics)
}

func (w *WebSphereMicroProfile) createCustomChart(metricID, originalName string) *module.Charts {
	// Extract application name from metric
	appName := "unknown"
	if strings.HasPrefix(originalName, "application_") {
		parts := strings.Split(originalName, "_")
		if len(parts) >= 2 {
			appName = parts[1]
		}
	}

	chartID := fmt.Sprintf("app_%s_%s", appName, strings.ReplaceAll(metricID, "application_", ""))
	title := fmt.Sprintf("Application %s - %s", appName, strings.ReplaceAll(originalName, "_", " "))

	return &module.Charts{
		{
			ID:       chartID,
			Title:    title,
			Units:    "value",
			Fam:      "application metrics",
			Ctx:      "websphere_mp.application",
			Priority: prioCustomMetrics,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "value"},
			},
		},
	}
}

func (w *WebSphereMicroProfile) createMemoryChart(metricID, originalName string) *module.Charts {
	return &module.Charts{
		{
			ID:       "jvm_memory_" + metricID,
			Title:    "JVM Memory - " + strings.ReplaceAll(originalName, "_", " "),
			Units:    "bytes",
			Fam:      "jvm memory",
			Ctx:      "websphere_mp.jvm_memory",
			Priority: prioJVMHeap + 10,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "bytes"},
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
				Fam:      "jvm gc",
				Ctx:      "websphere_mp.jvm_gc_time",
				Priority: prioJVMGC + 10,
				Type:     module.Line,
				Dims: module.Dims{
					{ID: metricID, Name: "time", Div: precision},
				},
			},
		}
	} else {
		return &module.Charts{
			{
				ID:       "jvm_gc_count_" + metricID,
				Title:    "JVM GC Count - " + strings.ReplaceAll(originalName, "_", " "),
				Units:    "collections/s",
				Fam:      "jvm gc",
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
			Fam:      "jvm threads",
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
			Fam:      "jvm classes",
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
			Units:    "value",
			Fam:      family,
			Ctx:      "websphere_mp." + family,
			Priority: priority,
			Type:     module.Line,
			Dims: module.Dims{
				{ID: metricID, Name: "value"},
			},
		},
	}
}
