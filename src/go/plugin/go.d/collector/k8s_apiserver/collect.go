// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const (
	precision = 1000 // for counters (e.g., CPU seconds)

	// For latency: multiply seconds by 1e6 to get microseconds, then chart Div: 1000 gives milliseconds
	latencyPrecision = 1000000

	// Default cardinality limits to prevent unbounded memory growth
	defaultMaxResources   = 500
	defaultMaxWorkqueues  = 100
	defaultMaxAdmCtrl     = 100
	defaultMaxAdmWebhooks = 50
)

func (c *Collector) collect() (map[string]int64, error) {
	raw, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	mx := newMetrics()

	c.collectRequests(raw, mx)
	c.collectInflight(raw, mx)
	c.collectRESTClient(raw, mx)
	c.collectAdmission(raw, mx)
	c.collectEtcd(raw, mx)
	c.collectWorkqueues(raw, mx)
	c.collectProcess(raw, mx)
	c.collectAudit(raw, mx)
	c.collectAuth(raw, mx)

	return stm.ToMap(mx), nil
}

// collectRequests collects apiserver_request_total, apiserver_request_duration_seconds, etc.
func (c *Collector) collectRequests(raw prometheus.Series, mx *metrics) {
	// Total requests and by verb/code/resource
	for _, metric := range raw.FindByNames("apiserver_request_total", "apiserver_request_count") {
		verb := metric.Labels.Get("verb")
		code := metric.Labels.Get("code")
		resource := metric.Labels.Get("resource")

		mx.Request.Total.Add(metric.Value)

		// By verb
		if verb != "" {
			c.addVerbDimension(verb)
			mx.Request.ByVerb[verb] = mtx.Gauge(mx.Request.ByVerb[verb].Value() + metric.Value)
		}

		// By code
		if code != "" {
			c.addCodeDimension(code)
			mx.Request.ByCode[code] = mtx.Gauge(mx.Request.ByCode[code].Value() + metric.Value)
		}

		// By resource (with cardinality limit)
		// Allow updates for already-tracked resources even after cap is reached
		if resource != "" {
			if c.collectedResources[resource] || len(c.collectedResources) < defaultMaxResources {
				c.addResourceDimension(resource)
				mx.Request.ByResource[resource] = mtx.Gauge(mx.Request.ByResource[resource].Value() + metric.Value)
			}
		}
	}

	// Dropped/rejected requests - support both legacy and APF metrics
	// Legacy: apiserver_dropped_requests_total (K8s < 1.20, APF disabled)
	// Modern: apiserver_flowcontrol_rejected_requests_total (K8s >= 1.20, APF enabled)
	dropped := raw.FindByName("apiserver_dropped_requests_total").Max()
	if dropped == 0 {
		// Sum across all flow schemas and priority levels
		for _, metric := range raw.FindByName("apiserver_flowcontrol_rejected_requests_total") {
			dropped += metric.Value
		}
	}
	mx.Request.Dropped.Set(dropped)

	// Request latency and response size (histograms)
	c.collectRequestLatency(raw, mx)
	c.collectResponseSize(raw, mx)
}

func (c *Collector) collectRequestLatency(raw prometheus.Series, mx *metrics) {
	// apiserver_request_duration_seconds is a histogram - compute percentiles from buckets
	buckets := collectHistogramBuckets(raw, "apiserver_request_duration_seconds")
	if len(buckets) > 0 {
		if p50 := histogramPercentile(buckets, 0.5); !math.IsNaN(p50) {
			mx.Request.Latency.P50.Set(p50 * latencyPrecision) // seconds to microseconds, chart Div:1000 gives ms
		}
		if p90 := histogramPercentile(buckets, 0.9); !math.IsNaN(p90) {
			mx.Request.Latency.P90.Set(p90 * latencyPrecision)
		}
		if p99 := histogramPercentile(buckets, 0.99); !math.IsNaN(p99) {
			mx.Request.Latency.P99.Set(p99 * latencyPrecision)
		}
	}
}

func (c *Collector) collectResponseSize(raw prometheus.Series, mx *metrics) {
	// apiserver_response_sizes is a histogram - compute percentiles from buckets
	buckets := collectHistogramBuckets(raw, "apiserver_response_sizes")
	if len(buckets) > 0 {
		if p50 := histogramPercentile(buckets, 0.5); !math.IsNaN(p50) {
			mx.Request.ResponseSize.P50.Set(p50)
		}
		if p90 := histogramPercentile(buckets, 0.9); !math.IsNaN(p90) {
			mx.Request.ResponseSize.P90.Set(p90)
		}
		if p99 := histogramPercentile(buckets, 0.99); !math.IsNaN(p99) {
			mx.Request.ResponseSize.P99.Set(p99)
		}
	}
}

// collectInflight collects apiserver_current_inflight_requests and apiserver_longrunning_requests
func (c *Collector) collectInflight(raw prometheus.Series, mx *metrics) {
	for _, metric := range raw.FindByName("apiserver_current_inflight_requests") {
		// FIXED: Label is "request_kind" not "requestKind"
		kind := metric.Labels.Get("request_kind")
		switch kind {
		case "mutating":
			mx.Inflight.Mutating.Set(metric.Value)
		case "readOnly":
			mx.Inflight.ReadOnly.Set(metric.Value)
		}
	}

	// Use apiserver_longrunning_requests (the actual metric name, not apiserver_longrunning_gauge)
	value := raw.FindByName("apiserver_longrunning_requests").Max()
	mx.Inflight.Longrunning.Set(value)
}

// collectRESTClient collects rest_client_requests_total and rest_client_request_duration_seconds
func (c *Collector) collectRESTClient(raw prometheus.Series, mx *metrics) {
	codeChart := c.charts.Get("rest_client_requests_by_code")
	methodChart := c.charts.Get("rest_client_requests_by_method")

	// Single iteration for both code and method dimensions
	for _, metric := range raw.FindByName("rest_client_requests_total") {
		code := metric.Labels.Get("code")
		method := metric.Labels.Get("method")

		// By code
		if code != "" {
			dimID := "rest_client_by_code_" + code
			if codeChart != nil && !codeChart.HasDim(dimID) {
				if err := codeChart.AddDim(&Dim{ID: dimID, Name: code, Algo: module.Incremental}); err != nil {
					c.Warningf("failed to add REST client code dimension %s: %v", code, err)
				} else {
					codeChart.MarkNotCreated()
				}
			}
			mx.RESTClient.ByCode[code] = mtx.Gauge(mx.RESTClient.ByCode[code].Value() + metric.Value)
		}

		// By method
		if method != "" {
			dimID := "rest_client_by_method_" + method
			if methodChart != nil && !methodChart.HasDim(dimID) {
				if err := methodChart.AddDim(&Dim{ID: dimID, Name: method, Algo: module.Incremental}); err != nil {
					c.Warningf("failed to add REST client method dimension %s: %v", method, err)
				} else {
					methodChart.MarkNotCreated()
				}
			}
			mx.RESTClient.ByMethod[method] = mtx.Gauge(mx.RESTClient.ByMethod[method].Value() + metric.Value)
		}
	}

	// REST client latency - metric is rest_client_request_duration_seconds (histogram)
	buckets := collectHistogramBuckets(raw, "rest_client_request_duration_seconds")
	if len(buckets) > 0 {
		if p50 := histogramPercentile(buckets, 0.5); !math.IsNaN(p50) {
			mx.RESTClient.Latency.P50.Set(p50 * latencyPrecision)
		}
		if p90 := histogramPercentile(buckets, 0.9); !math.IsNaN(p90) {
			mx.RESTClient.Latency.P90.Set(p90 * latencyPrecision)
		}
		if p99 := histogramPercentile(buckets, 0.99); !math.IsNaN(p99) {
			mx.RESTClient.Latency.P99.Set(p99 * latencyPrecision)
		}
	}
}

// collectAdmission collects admission controller and webhook metrics
func (c *Collector) collectAdmission(raw prometheus.Series, mx *metrics) {
	// Admission step latency - try histogram first, then summary fallback
	stepBuckets := make(map[string][]histogramBucket)
	for _, metric := range raw.FindByName("apiserver_admission_step_admission_duration_seconds_bucket") {
		if math.IsNaN(metric.Value) {
			continue
		}
		stepType := metric.Labels.Get("type")
		le := metric.Labels.Get("le")
		if stepType == "" || le == "" {
			continue
		}
		bound, err := strconv.ParseFloat(le, 64)
		if err != nil {
			continue
		}
		stepBuckets[stepType] = append(stepBuckets[stepType], histogramBucket{le: bound, count: metric.Value})
	}

	for stepType, buckets := range stepBuckets {
		if p50 := histogramPercentile(buckets, 0.5); !math.IsNaN(p50) {
			switch stepType {
			case "validate":
				mx.Admission.StepLatency.Validate.Set(p50 * latencyPrecision)
			case "admit":
				mx.Admission.StepLatency.Admit.Set(p50 * latencyPrecision)
			}
		}
	}

	// Admission controller latency (dynamic charts)
	c.collectAdmissionControllerLatency(raw, mx)
	c.collectAdmissionWebhookLatency(raw, mx)
}

func (c *Collector) collectAdmissionControllerLatency(raw prometheus.Series, mx *metrics) {
	// Metric is apiserver_admission_controller_admission_duration_seconds (histogram)
	// Collect as heatmap with non-cumulative bucket counts
	// Must aggregate by controller name, then by le bucket (summing across operation/type/rejected labels)
	controllerBucketMaps := make(map[string]map[string]float64) // name -> le_string -> cumulative_count

	for _, metric := range raw.FindByName("apiserver_admission_controller_admission_duration_seconds_bucket") {
		if math.IsNaN(metric.Value) {
			continue
		}
		name := metric.Labels.Get("name")
		le := metric.Labels.Get("le")
		if name == "" || le == "" {
			continue
		}
		if controllerBucketMaps[name] == nil {
			controllerBucketMaps[name] = make(map[string]float64)
		}
		controllerBucketMaps[name][le] += metric.Value
	}

	for name, bucketMap := range controllerBucketMaps {
		// Cardinality limit
		if !c.collectedAdmissionCtrl[name] && len(c.collectedAdmissionCtrl) >= defaultMaxAdmCtrl {
			continue
		}

		if !c.collectedAdmissionCtrl[name] {
			c.collectedAdmissionCtrl[name] = true
			if err := c.charts.Add(newAdmissionControllerLatencyChart(name)); err != nil {
				c.Warningf("failed to add admission controller chart %s: %v", name, err)
			}
		}

		if mx.Admission.Controllers[name] == nil {
			mx.Admission.Controllers[name] = &admissionControllerMetrics{}
		}

		// K8s admission controller histogram buckets: 0.005, 0.025, 0.1, 0.5, 1, 2.5, +Inf
		// Convert cumulative counts to non-cumulative for heatmap
		// Each bucket value = cumulative[bucket] - cumulative[previous_bucket]
		// Note: le labels may be "1" or "1.0" depending on prometheus library parsing
		b5ms := bucketMap["0.005"]
		b25ms := bucketMap["0.025"]
		b100ms := bucketMap["0.1"]
		b500ms := bucketMap["0.5"]
		b1s := bucketMap["1"]
		if b1s == 0 {
			b1s = bucketMap["1.0"]
		}
		b2500ms := bucketMap["2.5"]
		bInf := bucketMap["+Inf"]

		mx.Admission.Controllers[name].Bucket5ms.Set(b5ms)
		mx.Admission.Controllers[name].Bucket25ms.Set(b25ms - b5ms)
		mx.Admission.Controllers[name].Bucket100ms.Set(b100ms - b25ms)
		mx.Admission.Controllers[name].Bucket500ms.Set(b500ms - b100ms)
		mx.Admission.Controllers[name].Bucket1s.Set(b1s - b500ms)
		mx.Admission.Controllers[name].Bucket2500ms.Set(b2500ms - b1s)
		mx.Admission.Controllers[name].BucketInf.Set(bInf - b2500ms)
	}
}

func (c *Collector) collectAdmissionWebhookLatency(raw prometheus.Series, mx *metrics) {
	// Metric is apiserver_admission_webhook_admission_duration_seconds (histogram)
	// Collect as heatmap with non-cumulative bucket counts
	// Must aggregate by webhook name, then by le bucket (summing across operation/type/rejected labels)
	webhookBucketMaps := make(map[string]map[string]float64) // name -> le_string -> cumulative_count

	for _, metric := range raw.FindByName("apiserver_admission_webhook_admission_duration_seconds_bucket") {
		if math.IsNaN(metric.Value) {
			continue
		}
		name := metric.Labels.Get("name")
		le := metric.Labels.Get("le")
		if name == "" || le == "" {
			continue
		}
		if webhookBucketMaps[name] == nil {
			webhookBucketMaps[name] = make(map[string]float64)
		}
		webhookBucketMaps[name][le] += metric.Value
	}

	for name, bucketMap := range webhookBucketMaps {
		// Cardinality limit
		if !c.collectedAdmissionWH[name] && len(c.collectedAdmissionWH) >= defaultMaxAdmWebhooks {
			continue
		}

		if !c.collectedAdmissionWH[name] {
			c.collectedAdmissionWH[name] = true
			if err := c.charts.Add(newAdmissionWebhookLatencyChart(name)); err != nil {
				c.Warningf("failed to add admission webhook chart %s: %v", name, err)
			}
		}

		if mx.Admission.Webhooks[name] == nil {
			mx.Admission.Webhooks[name] = &admissionWebhookMetrics{}
		}

		// K8s admission webhook histogram buckets: 0.005, 0.025, 0.1, 0.5, 1, 2.5, +Inf
		// Convert cumulative counts to non-cumulative for heatmap
		// Note: le labels may be "1" or "1.0" depending on prometheus library parsing
		b5ms := bucketMap["0.005"]
		b25ms := bucketMap["0.025"]
		b100ms := bucketMap["0.1"]
		b500ms := bucketMap["0.5"]
		b1s := bucketMap["1"]
		if b1s == 0 {
			b1s = bucketMap["1.0"]
		}
		b2500ms := bucketMap["2.5"]
		bInf := bucketMap["+Inf"]

		mx.Admission.Webhooks[name].Bucket5ms.Set(b5ms)
		mx.Admission.Webhooks[name].Bucket25ms.Set(b25ms - b5ms)
		mx.Admission.Webhooks[name].Bucket100ms.Set(b100ms - b25ms)
		mx.Admission.Webhooks[name].Bucket500ms.Set(b500ms - b100ms)
		mx.Admission.Webhooks[name].Bucket1s.Set(b1s - b500ms)
		mx.Admission.Webhooks[name].Bucket2500ms.Set(b2500ms - b1s)
		mx.Admission.Webhooks[name].BucketInf.Set(bInf - b2500ms)
	}
}

// collectEtcd collects etcd/storage object count metrics
func (c *Collector) collectEtcd(raw prometheus.Series, mx *metrics) {
	// Object counts per resource (dynamic chart)
	// FIXED: Use apiserver_storage_objects (newer) or etcd_object_counts (older)
	chart := c.charts.Get("etcd_object_counts")
	if chart == nil {
		chart = newEtcdObjectCountsChart()
		if err := c.charts.Add(chart); err != nil {
			c.Warningf("failed to add etcd object counts chart: %v", err)
		}
	}

	// Try apiserver_storage_objects first (newer k8s), then etcd_object_counts (older)
	objectMetrics := raw.FindByName("apiserver_storage_objects")
	if len(objectMetrics) == 0 {
		objectMetrics = raw.FindByName("etcd_object_counts")
	}

	for _, metric := range objectMetrics {
		resource := metric.Labels.Get("resource")
		if resource == "" {
			continue
		}
		// Simplify resource name (remove api group suffix)
		resourceName := simplifyResourceName(resource)
		dimID := "etcd_objects_" + resourceName

		if chart != nil && !chart.HasDim(dimID) {
			if err := chart.AddDim(&Dim{ID: dimID, Name: resourceName}); err != nil {
				c.Debugf("failed to add etcd object dimension %s: %v", resourceName, err)
			} else {
				chart.MarkNotCreated()
			}
		}
		mx.Etcd.ObjectCounts[resourceName] = mtx.Gauge(metric.Value)
	}
}

// collectWorkqueues collects controller work queue metrics
func (c *Collector) collectWorkqueues(raw prometheus.Series, mx *metrics) {
	// Pre-index metrics by queue name for O(1) lookup instead of O(n) per queue
	depthByName := make(map[string]float64)
	addsByName := make(map[string]float64)
	retriesByName := make(map[string]float64)

	for _, metric := range raw.FindByName("workqueue_depth") {
		if name := metric.Labels.Get("name"); name != "" {
			depthByName[name] = metric.Value
		}
	}
	for _, metric := range raw.FindByName("workqueue_adds_total") {
		if name := metric.Labels.Get("name"); name != "" {
			addsByName[name] = metric.Value
		}
	}
	for _, metric := range raw.FindByName("workqueue_retries_total") {
		if name := metric.Labels.Get("name"); name != "" {
			retriesByName[name] = metric.Value
		}
	}

	for queueName := range depthByName {
		// Cardinality limit
		if !c.collectedWorkqueues[queueName] && len(c.collectedWorkqueues) >= defaultMaxWorkqueues {
			continue
		}

		if !c.collectedWorkqueues[queueName] {
			c.collectedWorkqueues[queueName] = true
			if err := c.charts.Add(newWorkqueueDepthChart(queueName)); err != nil {
				c.Warningf("failed to add workqueue depth chart %s: %v", queueName, err)
			}
			if err := c.charts.Add(newWorkqueueLatencyChart(queueName)); err != nil {
				c.Warningf("failed to add workqueue latency chart %s: %v", queueName, err)
			}
			if err := c.charts.Add(newWorkqueueAddsChart(queueName)); err != nil {
				c.Warningf("failed to add workqueue adds chart %s: %v", queueName, err)
			}
		}

		if mx.Workqueue.Controllers[queueName] == nil {
			mx.Workqueue.Controllers[queueName] = &workqueueMetrics{}
		}

		wq := mx.Workqueue.Controllers[queueName]

		// Use pre-indexed values
		wq.Depth.Set(depthByName[queueName])
		wq.Adds.Set(addsByName[queueName])
		wq.Retries.Set(retriesByName[queueName])

		// Queue latency (histogram)
		buckets := collectHistogramBucketsWithLabel(raw, "workqueue_queue_duration_seconds_bucket", "name", queueName)
		if len(buckets) > 0 {
			if p50 := histogramPercentile(buckets, 0.5); !math.IsNaN(p50) {
				wq.LatencyP50.Set(p50 * 1000000) // seconds to microseconds
			}
			if p90 := histogramPercentile(buckets, 0.9); !math.IsNaN(p90) {
				wq.LatencyP90.Set(p90 * 1000000)
			}
			if p99 := histogramPercentile(buckets, 0.99); !math.IsNaN(p99) {
				wq.LatencyP99.Set(p99 * 1000000)
			}
		}

		// Work duration (histogram)
		buckets = collectHistogramBucketsWithLabel(raw, "workqueue_work_duration_seconds_bucket", "name", queueName)
		if len(buckets) > 0 {
			if p50 := histogramPercentile(buckets, 0.5); !math.IsNaN(p50) {
				wq.DurationP50.Set(p50 * 1000000) // seconds to microseconds
			}
			if p90 := histogramPercentile(buckets, 0.9); !math.IsNaN(p90) {
				wq.DurationP90.Set(p90 * 1000000)
			}
			if p99 := histogramPercentile(buckets, 0.99); !math.IsNaN(p99) {
				wq.DurationP99.Set(p99 * 1000000)
			}
		}
	}
}

// collectProcess collects Go runtime and process metrics
func (c *Collector) collectProcess(raw prometheus.Series, mx *metrics) {
	mx.Process.Goroutines.Set(raw.FindByName("go_goroutines").Max())
	mx.Process.Threads.Set(raw.FindByName("go_threads").Max())
	mx.Process.CPUSeconds.Set(raw.FindByName("process_cpu_seconds_total").Max() * precision)
	mx.Process.ResidentMemory.Set(raw.FindByName("process_resident_memory_bytes").Max())
	mx.Process.VirtualMemory.Set(raw.FindByName("process_virtual_memory_bytes").Max())
	mx.Process.OpenFDs.Set(raw.FindByName("process_open_fds").Max())
	mx.Process.MaxFDs.Set(raw.FindByName("process_max_fds").Max())
	mx.Process.HeapAlloc.Set(raw.FindByName("go_memstats_heap_alloc_bytes").Max())
	mx.Process.HeapInuse.Set(raw.FindByName("go_memstats_heap_inuse_bytes").Max())
	mx.Process.StackInuse.Set(raw.FindByName("go_memstats_stack_inuse_bytes").Max())

	// GC duration (summary with quantile labels)
	// K8s exports quantiles: "0", "0.25", "0.5", "0.75", "1"
	for _, metric := range raw.FindByName("go_gc_duration_seconds") {
		if math.IsNaN(metric.Value) {
			continue
		}
		quantile := metric.Labels.Get("quantile")
		switch quantile {
		case "0":
			mx.Process.GCDurationMin.Set(metric.Value * 1000000) // seconds to microseconds
		case "0.25":
			mx.Process.GCDurationP25.Set(metric.Value * 1000000)
		case "0.5":
			mx.Process.GCDurationP50.Set(metric.Value * 1000000)
		case "0.75":
			mx.Process.GCDurationP75.Set(metric.Value * 1000000)
		case "1":
			mx.Process.GCDurationMax.Set(metric.Value * 1000000)
		}
	}
}

// collectAudit collects audit event metrics
func (c *Collector) collectAudit(raw prometheus.Series, mx *metrics) {
	mx.Audit.EventsTotal.Set(raw.FindByName("apiserver_audit_event_total").Max())
	mx.Audit.RejectedTotal.Set(raw.FindByName("apiserver_audit_requests_rejected_total").Max())
}

// collectAuth collects authentication metrics
func (c *Collector) collectAuth(raw prometheus.Series, mx *metrics) {
	// FIXED: Sum across all usernames instead of taking max
	for _, metric := range raw.FindByName("authenticated_user_requests") {
		mx.Auth.AuthenticatedRequests.Add(metric.Value)
	}

	// Client certificate expiration - histogram, track count of certs expiring within 24h
	for _, metric := range raw.FindByName("apiserver_client_certificate_expiration_seconds_bucket") {
		le := metric.Labels.Get("le")
		if le == "86400" { // 1 day bucket
			mx.Auth.CertExpirationSeconds.Set(metric.Value)
			break
		}
	}
}

// Helper functions for dynamic dimension creation

func (c *Collector) addVerbDimension(verb string) {
	if c.collectedVerbs[verb] {
		return
	}
	c.collectedVerbs[verb] = true

	chart := c.charts.Get("requests_by_verb")
	if chart == nil {
		c.Warningf("chart 'requests_by_verb' not found, cannot add dimension for verb: %s", verb)
		return
	}
	dimID := "request_by_verb_" + verb
	if !chart.HasDim(dimID) {
		if err := chart.AddDim(&Dim{ID: dimID, Name: verb, Algo: module.Incremental}); err != nil {
			c.Warningf("failed to add verb dimension %s: %v", verb, err)
		} else {
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) addCodeDimension(code string) {
	if c.collectedCodes[code] {
		return
	}
	c.collectedCodes[code] = true

	chart := c.charts.Get("requests_by_code")
	if chart == nil {
		c.Warningf("chart 'requests_by_code' not found, cannot add dimension for code: %s", code)
		return
	}
	dimID := "request_by_code_" + code
	if !chart.HasDim(dimID) {
		if err := chart.AddDim(&Dim{ID: dimID, Name: code, Algo: module.Incremental}); err != nil {
			c.Warningf("failed to add code dimension %s: %v", code, err)
		} else {
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) addResourceDimension(resource string) {
	if c.collectedResources[resource] {
		return
	}
	c.collectedResources[resource] = true

	chart := c.charts.Get("requests_by_resource")
	if chart == nil {
		c.Warningf("chart 'requests_by_resource' not found, cannot add dimension for resource: %s", resource)
		return
	}
	dimID := "request_by_resource_" + resource
	if !chart.HasDim(dimID) {
		if err := chart.AddDim(&Dim{ID: dimID, Name: resource, Algo: module.Incremental}); err != nil {
			c.Warningf("failed to add resource dimension %s: %v", resource, err)
		} else {
			chart.MarkNotCreated()
		}
	}
}

// simplifyResourceName removes API group suffix from resource names
// e.g., "deployments.apps" -> "deployments"
func simplifyResourceName(resource string) string {
	parts := strings.SplitN(resource, ".", 2)
	return parts[0]
}

// Histogram percentile calculation utilities

type histogramBucket struct {
	le    float64
	count float64
}

// collectHistogramBuckets aggregates histogram buckets across all label combinations
func collectHistogramBuckets(raw prometheus.Series, metricName string) []histogramBucket {
	bucketMap := make(map[float64]float64)

	for _, metric := range raw.FindByName(metricName + "_bucket") {
		if math.IsNaN(metric.Value) {
			continue
		}
		le := metric.Labels.Get("le")
		if le == "" || le == "+Inf" {
			continue
		}
		bound, err := strconv.ParseFloat(le, 64)
		if err != nil {
			continue
		}
		bucketMap[bound] += metric.Value
	}

	buckets := make([]histogramBucket, 0, len(bucketMap))
	for le, count := range bucketMap {
		buckets = append(buckets, histogramBucket{le: le, count: count})
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].le < buckets[j].le
	})

	return buckets
}

// collectHistogramBucketsWithLabel collects buckets for a specific label value
func collectHistogramBucketsWithLabel(raw prometheus.Series, metricName, labelName, labelValue string) []histogramBucket {
	bucketMap := make(map[float64]float64)

	for _, metric := range raw.FindByName(metricName) {
		if math.IsNaN(metric.Value) {
			continue
		}
		if metric.Labels.Get(labelName) != labelValue {
			continue
		}
		le := metric.Labels.Get("le")
		if le == "" || le == "+Inf" {
			continue
		}
		bound, err := strconv.ParseFloat(le, 64)
		if err != nil {
			continue
		}
		bucketMap[bound] += metric.Value
	}

	buckets := make([]histogramBucket, 0, len(bucketMap))
	for le, count := range bucketMap {
		buckets = append(buckets, histogramBucket{le: le, count: count})
	}

	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].le < buckets[j].le
	})

	return buckets
}

// histogramPercentile estimates the percentile value from histogram buckets
// Uses linear interpolation within the bucket containing the percentile
func histogramPercentile(buckets []histogramBucket, percentile float64) float64 {
	if len(buckets) == 0 {
		return math.NaN()
	}

	// Get total count from the last bucket
	total := buckets[len(buckets)-1].count
	if total == 0 {
		return math.NaN()
	}

	target := percentile * total

	// Find the bucket containing the target count
	var prevBound, prevCount float64
	for _, b := range buckets {
		if b.count >= target {
			// Linear interpolation within this bucket
			bucketWidth := b.le - prevBound
			bucketCount := b.count - prevCount
			if bucketCount == 0 {
				return b.le
			}
			fraction := (target - prevCount) / bucketCount
			return prevBound + fraction*bucketWidth
		}
		prevBound = b.le
		prevCount = b.count
	}

	// If we get here, return the last bucket bound
	return buckets[len(buckets)-1].le
}
