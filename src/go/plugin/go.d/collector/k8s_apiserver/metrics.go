// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

func newMetrics() *metrics {
	var mx metrics
	mx.Request.ByVerb = make(map[string]mtx.Gauge)
	mx.Request.ByCode = make(map[string]mtx.Gauge)
	mx.Request.ByResource = make(map[string]mtx.Gauge)
	mx.RESTClient.ByCode = make(map[string]mtx.Gauge)
	mx.RESTClient.ByMethod = make(map[string]mtx.Gauge)
	mx.Workqueue.Controllers = make(map[string]*workqueueMetrics)
	mx.Etcd.ObjectCounts = make(map[string]mtx.Gauge)
	mx.Admission.Controllers = make(map[string]*admissionControllerMetrics)
	mx.Admission.Webhooks = make(map[string]*admissionWebhookMetrics)
	return &mx
}

type metrics struct {
	Request    requestMetrics    `stm:"request"`
	Inflight   inflightMetrics   `stm:"inflight"`
	RESTClient restClientMetrics `stm:"rest_client"`
	Admission  admissionMetrics  `stm:"admission"`
	Etcd       etcdMetrics       `stm:"etcd"`
	Workqueue  workqueueGroup    `stm:"workqueue"`
	Process    processMetrics    `stm:"process"`
	Audit      auditMetrics      `stm:"audit"`
	Auth       authMetrics       `stm:"auth"`
}

// Request metrics: apiserver_request_total, apiserver_request_duration_seconds
type requestMetrics struct {
	Total      mtx.Gauge            `stm:"total"`
	ByVerb     map[string]mtx.Gauge `stm:"by_verb"`
	ByCode     map[string]mtx.Gauge `stm:"by_code"`
	ByResource map[string]mtx.Gauge `stm:"by_resource"`
	Dropped    mtx.Gauge            `stm:"dropped_total"`
	// Latency percentiles (from histogram)
	Latency struct {
		P50 mtx.Gauge `stm:"p50"`
		P90 mtx.Gauge `stm:"p90"`
		P99 mtx.Gauge `stm:"p99"`
	} `stm:"latency"`
	// Response sizes (from histogram)
	ResponseSize struct {
		P50 mtx.Gauge `stm:"p50"`
		P90 mtx.Gauge `stm:"p90"`
		P99 mtx.Gauge `stm:"p99"`
	} `stm:"response_size"`
}

// Inflight metrics: apiserver_current_inflight_requests, apiserver_longrunning_gauge
type inflightMetrics struct {
	Mutating    mtx.Gauge `stm:"mutating"`
	ReadOnly    mtx.Gauge `stm:"readonly"`
	Longrunning mtx.Gauge `stm:"longrunning"`
}

// REST client metrics: rest_client_requests_total, rest_client_request_duration_seconds
type restClientMetrics struct {
	ByCode   map[string]mtx.Gauge `stm:"by_code"`
	ByMethod map[string]mtx.Gauge `stm:"by_method"`
	Latency  struct {
		P50 mtx.Gauge `stm:"p50"`
		P90 mtx.Gauge `stm:"p90"`
		P99 mtx.Gauge `stm:"p99"`
	} `stm:"latency"`
}

// Admission metrics
type admissionMetrics struct {
	Controllers map[string]*admissionControllerMetrics `stm:"controller"`
	Webhooks    map[string]*admissionWebhookMetrics    `stm:"webhook"`
	StepLatency struct {
		Validate mtx.Gauge `stm:"validate"`
		Admit    mtx.Gauge `stm:"admit"`
	} `stm:"step_latency"`
}

type admissionControllerMetrics struct {
	// Histogram bucket counts (non-cumulative) for heatmap visualization
	Bucket5ms    mtx.Gauge `stm:"bucket_5ms"`
	Bucket25ms   mtx.Gauge `stm:"bucket_25ms"`
	Bucket100ms  mtx.Gauge `stm:"bucket_100ms"`
	Bucket500ms  mtx.Gauge `stm:"bucket_500ms"`
	Bucket1s     mtx.Gauge `stm:"bucket_1s"`
	Bucket2500ms mtx.Gauge `stm:"bucket_2500ms"`
	BucketInf    mtx.Gauge `stm:"bucket_inf"`
}

type admissionWebhookMetrics struct {
	// Histogram bucket counts (non-cumulative) for heatmap visualization
	Bucket5ms    mtx.Gauge `stm:"bucket_5ms"`
	Bucket25ms   mtx.Gauge `stm:"bucket_25ms"`
	Bucket100ms  mtx.Gauge `stm:"bucket_100ms"`
	Bucket500ms  mtx.Gauge `stm:"bucket_500ms"`
	Bucket1s     mtx.Gauge `stm:"bucket_1s"`
	Bucket2500ms mtx.Gauge `stm:"bucket_2500ms"`
	BucketInf    mtx.Gauge `stm:"bucket_inf"`
}

// Etcd metrics
type etcdMetrics struct {
	ObjectCounts map[string]mtx.Gauge `stm:"objects"`
}

// Workqueue metrics (for each controller)
type workqueueGroup struct {
	Controllers map[string]*workqueueMetrics `stm:""`
}

type workqueueMetrics struct {
	Depth       mtx.Gauge `stm:"depth"`
	Adds        mtx.Gauge `stm:"adds_total"`
	Retries     mtx.Gauge `stm:"retries_total"`
	LatencyP50  mtx.Gauge `stm:"latency_p50"`
	LatencyP90  mtx.Gauge `stm:"latency_p90"`
	LatencyP99  mtx.Gauge `stm:"latency_p99"`
	DurationP50 mtx.Gauge `stm:"duration_p50"`
	DurationP90 mtx.Gauge `stm:"duration_p90"`
	DurationP99 mtx.Gauge `stm:"duration_p99"`
}

// Process metrics: go runtime and process
type processMetrics struct {
	Goroutines     mtx.Gauge `stm:"goroutines"`
	Threads        mtx.Gauge `stm:"threads"`
	CPUSeconds     mtx.Gauge `stm:"cpu_seconds_total"`
	ResidentMemory mtx.Gauge `stm:"resident_memory_bytes"`
	VirtualMemory  mtx.Gauge `stm:"virtual_memory_bytes"`
	OpenFDs        mtx.Gauge `stm:"open_fds"`
	MaxFDs         mtx.Gauge `stm:"max_fds"`
	HeapAlloc      mtx.Gauge `stm:"heap_alloc_bytes"`
	HeapInuse      mtx.Gauge `stm:"heap_inuse_bytes"`
	StackInuse     mtx.Gauge `stm:"stack_inuse_bytes"`
	GCDurationMin  mtx.Gauge `stm:"gc_duration_min"`
	GCDurationP25  mtx.Gauge `stm:"gc_duration_p25"`
	GCDurationP50  mtx.Gauge `stm:"gc_duration_p50"`
	GCDurationP75  mtx.Gauge `stm:"gc_duration_p75"`
	GCDurationMax  mtx.Gauge `stm:"gc_duration_max"`
}

// Audit metrics
type auditMetrics struct {
	EventsTotal   mtx.Gauge `stm:"events_total"`
	RejectedTotal mtx.Gauge `stm:"rejected_total"`
}

// Auth metrics
type authMetrics struct {
	AuthenticatedRequests mtx.Gauge `stm:"authenticated_requests_total"`
	CertExpirationSeconds mtx.Gauge `stm:"client_cert_expiration_seconds"`
}
