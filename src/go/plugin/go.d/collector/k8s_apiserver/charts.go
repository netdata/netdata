// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import "github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

type (
	Charts = collectorapi.Charts
	Chart  = collectorapi.Chart
	Dims   = collectorapi.Dims
	Dim    = collectorapi.Dim
)

var baseCharts = Charts{
	// === Overview ===
	requestsTotalChart.Copy(),
	requestsDroppedChart.Copy(),

	// === Requests ===
	requestsByVerbChart.Copy(),
	requestsByCodeChart.Copy(),
	requestsByResourceChart.Copy(),

	// === Latency ===
	requestLatencyChart.Copy(),
	responseSizeChart.Copy(),

	// === Inflight ===
	inflightRequestsChart.Copy(),
	longrunningRequestsChart.Copy(),

	// === REST Client ===
	restClientRequestsByCodeChart.Copy(),
	restClientRequestsByMethodChart.Copy(),
	restClientLatencyChart.Copy(),

	// === Admission ===
	admissionStepLatencyChart.Copy(),

	// === Audit ===
	auditEventsChart.Copy(),

	// === Authentication ===
	authRequestsChart.Copy(),

	// === Process ===
	goroutinesChart.Copy(),
	threadsChart.Copy(),
	processMemoryChart.Copy(),
	heapMemoryChart.Copy(),
	gcDurationChart.Copy(),
	openFDsChart.Copy(),
	cpuUsageChart.Copy(),
}

// === Overview Family ===

var requestsTotalChart = Chart{
	ID:    "requests_total",
	Title: "API Server Request Rate",
	Units: "requests/s",
	Fam:   "requests",
	Ctx:   "k8s_apiserver.requests_total",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "request_total", Name: "requests", Algo: collectorapi.Incremental},
	},
}

var requestsDroppedChart = Chart{
	ID:    "requests_dropped",
	Title: "API Server Dropped Requests",
	Units: "requests/s",
	Fam:   "requests",
	Ctx:   "k8s_apiserver.requests_dropped",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "request_dropped_total", Name: "dropped", Algo: collectorapi.Incremental},
	},
}

// === Requests Family ===

var requestsByVerbChart = Chart{
	ID:    "requests_by_verb",
	Title: "API Server Requests By Verb",
	Units: "requests/s",
	Fam:   "requests",
	Ctx:   "k8s_apiserver.requests_by_verb",
	Type:  collectorapi.Stacked,
	// Dims added dynamically
}

var requestsByCodeChart = Chart{
	ID:    "requests_by_code",
	Title: "API Server Requests By Status Code",
	Units: "requests/s",
	Fam:   "requests",
	Ctx:   "k8s_apiserver.requests_by_code",
	Type:  collectorapi.Stacked,
	// Dims added dynamically
}

var requestsByResourceChart = Chart{
	ID:    "requests_by_resource",
	Title: "API Server Requests By Resource",
	Units: "requests/s",
	Fam:   "requests",
	Ctx:   "k8s_apiserver.requests_by_resource",
	Type:  collectorapi.Stacked,
	// Dims added dynamically
}

// === Latency Family ===

var requestLatencyChart = Chart{
	ID:    "request_latency",
	Title: "API Server Request Latency",
	Units: "milliseconds",
	Fam:   "latency",
	Ctx:   "k8s_apiserver.request_latency",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "request_latency_p50", Name: "p50", Div: 1000},
		{ID: "request_latency_p90", Name: "p90", Div: 1000},
		{ID: "request_latency_p99", Name: "p99", Div: 1000},
	},
}

var responseSizeChart = Chart{
	ID:    "response_size",
	Title: "API Server Response Size",
	Units: "bytes",
	Fam:   "latency",
	Ctx:   "k8s_apiserver.response_size",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "request_response_size_p50", Name: "p50"},
		{ID: "request_response_size_p90", Name: "p90"},
		{ID: "request_response_size_p99", Name: "p99"},
	},
}

// === Inflight Family ===

var inflightRequestsChart = Chart{
	ID:    "inflight_requests",
	Title: "API Server Inflight Requests",
	Units: "requests",
	Fam:   "inflight",
	Ctx:   "k8s_apiserver.inflight_requests",
	Type:  collectorapi.Stacked,
	Dims: Dims{
		{ID: "inflight_mutating", Name: "mutating"},
		{ID: "inflight_readonly", Name: "read_only"},
	},
}

var longrunningRequestsChart = Chart{
	ID:    "longrunning_requests",
	Title: "API Server Long-Running Requests",
	Units: "requests",
	Fam:   "inflight",
	Ctx:   "k8s_apiserver.longrunning_requests",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "inflight_longrunning", Name: "longrunning"},
	},
}

// === REST Client Family ===

var restClientRequestsByCodeChart = Chart{
	ID:    "rest_client_requests_by_code",
	Title: "REST Client Requests By Status Code",
	Units: "requests/s",
	Fam:   "rest client",
	Ctx:   "k8s_apiserver.rest_client_requests_by_code",
	Type:  collectorapi.Stacked,
	// Dims added dynamically
}

var restClientRequestsByMethodChart = Chart{
	ID:    "rest_client_requests_by_method",
	Title: "REST Client Requests By Method",
	Units: "requests/s",
	Fam:   "rest client",
	Ctx:   "k8s_apiserver.rest_client_requests_by_method",
	Type:  collectorapi.Stacked,
	// Dims added dynamically
}

var restClientLatencyChart = Chart{
	ID:    "rest_client_latency",
	Title: "REST Client Request Latency",
	Units: "milliseconds",
	Fam:   "rest client",
	Ctx:   "k8s_apiserver.rest_client_latency",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "rest_client_latency_p50", Name: "p50", Div: 1000},
		{ID: "rest_client_latency_p90", Name: "p90", Div: 1000},
		{ID: "rest_client_latency_p99", Name: "p99", Div: 1000},
	},
}

// === Admission Family ===

var admissionStepLatencyChart = Chart{
	ID:    "admission_step_latency",
	Title: "Admission Step Latency",
	Units: "milliseconds",
	Fam:   "admission",
	Ctx:   "k8s_apiserver.admission_step_latency",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "admission_step_latency_validate", Name: "validate", Div: 1000},
		{ID: "admission_step_latency_admit", Name: "admit", Div: 1000},
	},
}

// Dynamic charts for admission controllers and webhooks are created in collect.go

// === Etcd Family ===

// Dynamic chart for etcd object counts is created in collect.go

// === Audit Family ===

var auditEventsChart = Chart{
	ID:    "audit_events",
	Title: "API Server Audit Events",
	Units: "events/s",
	Fam:   "audit",
	Ctx:   "k8s_apiserver.audit_events",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "audit_events_total", Name: "events", Algo: collectorapi.Incremental},
		{ID: "audit_rejected_total", Name: "rejected", Algo: collectorapi.Incremental},
	},
}

// === Authentication Family ===

var authRequestsChart = Chart{
	ID:    "authentication_requests",
	Title: "API Server Authenticated Requests",
	Units: "requests/s",
	Fam:   "authentication",
	Ctx:   "k8s_apiserver.authentication_requests",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "auth_authenticated_requests_total", Name: "authenticated", Algo: collectorapi.Incremental},
	},
}

// === Process Family ===

var goroutinesChart = Chart{
	ID:    "goroutines",
	Title: "Goroutines",
	Units: "goroutines",
	Fam:   "process",
	Ctx:   "k8s_apiserver.goroutines",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "process_goroutines", Name: "goroutines"},
	},
}

var threadsChart = Chart{
	ID:    "threads",
	Title: "OS Threads",
	Units: "threads",
	Fam:   "process",
	Ctx:   "k8s_apiserver.threads",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "process_threads", Name: "threads"},
	},
}

var processMemoryChart = Chart{
	ID:    "process_memory",
	Title: "Process Memory",
	Units: "bytes",
	Fam:   "process",
	Ctx:   "k8s_apiserver.process_memory",
	Type:  collectorapi.Stacked,
	Dims: Dims{
		{ID: "process_resident_memory_bytes", Name: "resident"},
		{ID: "process_virtual_memory_bytes", Name: "virtual"},
	},
}

var heapMemoryChart = Chart{
	ID:    "heap_memory",
	Title: "Go Heap Memory",
	Units: "bytes",
	Fam:   "process",
	Ctx:   "k8s_apiserver.heap_memory",
	Type:  collectorapi.Stacked,
	Dims: Dims{
		{ID: "process_heap_alloc_bytes", Name: "alloc"},
		{ID: "process_heap_inuse_bytes", Name: "inuse"},
		{ID: "process_stack_inuse_bytes", Name: "stack"},
	},
}

var gcDurationChart = Chart{
	ID:    "gc_duration",
	Title: "GC Duration",
	Units: "seconds",
	Fam:   "process",
	Ctx:   "k8s_apiserver.gc_duration",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "process_gc_duration_min", Name: "min", Div: 1000000},
		{ID: "process_gc_duration_p25", Name: "p25", Div: 1000000},
		{ID: "process_gc_duration_p50", Name: "p50", Div: 1000000},
		{ID: "process_gc_duration_p75", Name: "p75", Div: 1000000},
		{ID: "process_gc_duration_max", Name: "max", Div: 1000000},
	},
}

var openFDsChart = Chart{
	ID:    "open_fds",
	Title: "Open File Descriptors",
	Units: "file descriptors",
	Fam:   "process",
	Ctx:   "k8s_apiserver.open_fds",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "process_open_fds", Name: "open"},
		{ID: "process_max_fds", Name: "max"},
	},
}

var cpuUsageChart = Chart{
	ID:    "cpu_usage",
	Title: "CPU Usage",
	Units: "seconds/s",
	Fam:   "process",
	Ctx:   "k8s_apiserver.cpu_usage",
	Type:  collectorapi.Line,
	Dims: Dims{
		{ID: "process_cpu_seconds_total", Name: "cpu", Algo: collectorapi.Incremental, Div: 1000},
	},
}

// === Dynamic Chart Templates ===

func newWorkqueueDepthChart(name string) *Chart {
	return &Chart{
		ID:    "workqueue_depth_" + name,
		Title: "Work Queue Depth",
		Units: "items",
		Fam:   "workqueue",
		Ctx:   "k8s_apiserver.workqueue_depth",
		Type:  collectorapi.Line,
		Dims: Dims{
			{ID: "workqueue_" + name + "_depth", Name: "depth"},
		},
	}
}

func newWorkqueueLatencyChart(name string) *Chart {
	return &Chart{
		ID:    "workqueue_latency_" + name,
		Title: "Work Queue Latency",
		Units: "microseconds",
		Fam:   "workqueue",
		Ctx:   "k8s_apiserver.workqueue_latency",
		Type:  collectorapi.Line,
		Dims: Dims{
			{ID: "workqueue_" + name + "_latency_p50", Name: "p50"},
			{ID: "workqueue_" + name + "_latency_p90", Name: "p90"},
			{ID: "workqueue_" + name + "_latency_p99", Name: "p99"},
		},
	}
}

func newWorkqueueAddsChart(name string) *Chart {
	return &Chart{
		ID:    "workqueue_adds_" + name,
		Title: "Work Queue Adds",
		Units: "items/s",
		Fam:   "workqueue",
		Ctx:   "k8s_apiserver.workqueue_adds",
		Type:  collectorapi.Line,
		Dims: Dims{
			{ID: "workqueue_" + name + "_adds_total", Name: "adds", Algo: collectorapi.Incremental},
			{ID: "workqueue_" + name + "_retries_total", Name: "retries", Algo: collectorapi.Incremental},
		},
	}
}

func newWorkqueueDurationChart(name string) *Chart {
	return &Chart{
		ID:    "workqueue_duration_" + name,
		Title: "Work Queue Work Duration",
		Units: "microseconds",
		Fam:   "workqueue",
		Ctx:   "k8s_apiserver.workqueue_duration",
		Type:  collectorapi.Line,
		Dims: Dims{
			{ID: "workqueue_" + name + "_duration_p50", Name: "p50"},
			{ID: "workqueue_" + name + "_duration_p90", Name: "p90"},
			{ID: "workqueue_" + name + "_duration_p99", Name: "p99"},
		},
	}
}

func newAdmissionControllerLatencyChart(name string) *Chart {
	// Heatmap chart with histogram bucket dimensions
	// K8s admission controller histogram buckets: 5ms, 25ms, 100ms, 500ms, 1s, 2.5s, +Inf
	return &Chart{
		ID:    "admission_controller_latency_" + name,
		Title: "Admission Controller Latency",
		Units: "events/s",
		Fam:   "admission",
		Ctx:   "k8s_apiserver.admission_controller_latency",
		Type:  collectorapi.Heatmap,
		Dims: Dims{
			{ID: "admission_controller_" + name + "_bucket_5ms", Name: "5ms", Algo: collectorapi.Incremental},
			{ID: "admission_controller_" + name + "_bucket_25ms", Name: "25ms", Algo: collectorapi.Incremental},
			{ID: "admission_controller_" + name + "_bucket_100ms", Name: "100ms", Algo: collectorapi.Incremental},
			{ID: "admission_controller_" + name + "_bucket_500ms", Name: "500ms", Algo: collectorapi.Incremental},
			{ID: "admission_controller_" + name + "_bucket_1s", Name: "1s", Algo: collectorapi.Incremental},
			{ID: "admission_controller_" + name + "_bucket_2500ms", Name: "2.5s", Algo: collectorapi.Incremental},
			{ID: "admission_controller_" + name + "_bucket_inf", Name: "+Inf", Algo: collectorapi.Incremental},
		},
	}
}

func newAdmissionWebhookLatencyChart(name string) *Chart {
	// Heatmap chart with histogram bucket dimensions
	// K8s admission webhook histogram buckets: 5ms, 25ms, 100ms, 500ms, 1s, 2.5s, +Inf
	return &Chart{
		ID:    "admission_webhook_latency_" + name,
		Title: "Admission Webhook Latency",
		Units: "events/s",
		Fam:   "admission",
		Ctx:   "k8s_apiserver.admission_webhook_latency",
		Type:  collectorapi.Heatmap,
		Dims: Dims{
			{ID: "admission_webhook_" + name + "_bucket_5ms", Name: "5ms", Algo: collectorapi.Incremental},
			{ID: "admission_webhook_" + name + "_bucket_25ms", Name: "25ms", Algo: collectorapi.Incremental},
			{ID: "admission_webhook_" + name + "_bucket_100ms", Name: "100ms", Algo: collectorapi.Incremental},
			{ID: "admission_webhook_" + name + "_bucket_500ms", Name: "500ms", Algo: collectorapi.Incremental},
			{ID: "admission_webhook_" + name + "_bucket_1s", Name: "1s", Algo: collectorapi.Incremental},
			{ID: "admission_webhook_" + name + "_bucket_2500ms", Name: "2.5s", Algo: collectorapi.Incremental},
			{ID: "admission_webhook_" + name + "_bucket_inf", Name: "+Inf", Algo: collectorapi.Incremental},
		},
	}
}

func newEtcdObjectCountsChart() *Chart {
	return &Chart{
		ID:    "etcd_object_counts",
		Title: "Objects Stored In Etcd",
		Units: "objects",
		Fam:   "etcd",
		Ctx:   "k8s_apiserver.etcd_object_counts",
		Type:  collectorapi.Stacked,
		// Dims added dynamically per resource type
	}
}
