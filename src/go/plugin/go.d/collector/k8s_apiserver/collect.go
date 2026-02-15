// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	mtx "github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

const (
	precision = 1000 // for counters (e.g., CPU seconds)

	// For latency: multiply seconds by 1e6 to get microseconds, then chart Div: 1000 gives milliseconds
	latencyPrecision = 1000000

	// Default cardinality limits to prevent unbounded memory growth
	// Once these limits are reached, new dimensions are silently ignored
	defaultMaxResources   = 500
	defaultMaxWorkqueues  = 100
	defaultMaxAdmCtrl     = 100
	defaultMaxAdmWebhooks = 50

	// Cleanup: dimensions not seen for this many cycles are removed
	staleThresholdCycles = 300 // ~5 minutes at 1s update interval
)

// K8s admission histogram bucket bounds (seconds)
// Ref: https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apiserver/pkg/endpoints/metrics/metrics.go
var admissionBucketBounds = []float64{0.005, 0.025, 0.1, 0.5, 1.0, 2.5}

// idReplacer sanitizes label values for use in chart/dimension IDs
// Replaces characters that could cause issues in Netdata with underscores
var idReplacer = strings.NewReplacer(
	".", "_",
	" ", "_",
	"/", "_",
	":", "_",
)

// cleanID sanitizes a string for use in chart or dimension IDs
func cleanID(s string) string {
	return strings.ToLower(idReplacer.Replace(s))
}

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

	c.collectCycle++

	mx := newMetrics()

	c.collectRequests(mfs, mx)
	c.collectInflight(mfs, mx)
	c.collectRESTClient(mfs, mx)
	c.collectAdmission(mfs, mx)
	c.collectEtcd(mfs, mx)
	c.collectWorkqueues(mfs, mx)
	c.collectProcess(mfs, mx)
	c.collectAudit(mfs, mx)
	c.collectAuth(mfs, mx)

	// Periodically cleanup stale dimensions (every 100 cycles to avoid overhead)
	if c.collectCycle%100 == 0 {
		c.cleanupStaleDimensions()
	}

	return stm.ToMap(mx), nil
}

// collectRequests collects apiserver_request_total, apiserver_request_duration_seconds, etc.
func (c *Collector) collectRequests(mfs prometheus.MetricFamilies, mx *metrics) {
	// Total requests and by verb/code/resource
	// Prefer apiserver_request_total (newer), fall back to apiserver_request_count (legacy)
	mf := mfs.Get("apiserver_request_total")
	if mf == nil {
		mf = mfs.Get("apiserver_request_count")
	}

	if mf != nil {
		for _, m := range mf.Metrics() {
			value := metricValue(mf, m)
			if math.IsNaN(value) {
				continue
			}

			verb := m.Labels().Get("verb")
			code := m.Labels().Get("code")
			resource := m.Labels().Get("resource")

			mx.Request.Total.Add(value)

			// By verb
			if verb != "" {
				c.addVerbDimension(verb)
				verbID := cleanID(verb)
				mx.Request.ByVerb[verbID] = mtx.Gauge(mx.Request.ByVerb[verbID].Value() + value)
			}

			// By code
			if code != "" {
				c.addCodeDimension(code)
				codeID := cleanID(code)
				mx.Request.ByCode[codeID] = mtx.Gauge(mx.Request.ByCode[codeID].Value() + value)
			}

			// By resource (with cardinality limit)
			if resource != "" {
				resourceID := cleanID(resource)
				_, seen := c.collectedResources[resourceID]
				if seen || len(c.collectedResources) < defaultMaxResources {
					c.addResourceDimension(resource)
					c.collectedResources[resourceID] = c.collectCycle
					mx.Request.ByResource[resourceID] = mtx.Gauge(mx.Request.ByResource[resourceID].Value() + value)
				}
			}
		}
	}

	// Dropped/rejected requests - sum both legacy and APF (flow control) metrics
	// Both can be present and meaningful on different K8s versions
	var dropped float64
	if mf := mfs.Get("apiserver_dropped_requests_total"); mf != nil {
		for _, m := range mf.Metrics() {
			if v := metricValue(mf, m); !math.IsNaN(v) {
				dropped += v
			}
		}
	}
	if mf := mfs.Get("apiserver_flowcontrol_rejected_requests_total"); mf != nil {
		for _, m := range mf.Metrics() {
			if v := metricValue(mf, m); !math.IsNaN(v) {
				dropped += v
			}
		}
	}
	mx.Request.Dropped.Set(dropped)

	// Request latency and response size (histograms)
	c.collectRequestLatency(mfs, mx)
	c.collectResponseSize(mfs, mx)
}

func (c *Collector) collectRequestLatency(mfs prometheus.MetricFamilies, mx *metrics) {
	mf := mfs.Get("apiserver_request_duration_seconds")
	if mf == nil || mf.Type() != model.MetricTypeHistogram {
		return
	}

	hd := collectHistogramBucketsFromMF(mf, nil)
	if len(hd.buckets) > 0 {
		if p50 := histogramPercentile(hd, 0.5); !math.IsNaN(p50) {
			mx.Request.Latency.P50.Set(p50 * latencyPrecision)
		}
		if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
			mx.Request.Latency.P90.Set(p90 * latencyPrecision)
		}
		if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
			mx.Request.Latency.P99.Set(p99 * latencyPrecision)
		}
	}
}

func (c *Collector) collectResponseSize(mfs prometheus.MetricFamilies, mx *metrics) {
	mf := mfs.Get("apiserver_response_sizes")
	if mf == nil || mf.Type() != model.MetricTypeHistogram {
		return
	}

	hd := collectHistogramBucketsFromMF(mf, nil)
	if len(hd.buckets) > 0 {
		if p50 := histogramPercentile(hd, 0.5); !math.IsNaN(p50) {
			mx.Request.ResponseSize.P50.Set(p50)
		}
		if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
			mx.Request.ResponseSize.P90.Set(p90)
		}
		if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
			mx.Request.ResponseSize.P99.Set(p99)
		}
	}
}

// collectInflight collects apiserver_current_inflight_requests and apiserver_longrunning_requests
func (c *Collector) collectInflight(mfs prometheus.MetricFamilies, mx *metrics) {
	if mf := mfs.Get("apiserver_current_inflight_requests"); mf != nil {
		for _, m := range mf.Metrics() {
			value := metricValue(mf, m)
			if math.IsNaN(value) {
				continue
			}
			kind := m.Labels().Get("request_kind")
			switch kind {
			case "mutating":
				mx.Inflight.Mutating.Set(value)
			case "readOnly":
				mx.Inflight.ReadOnly.Set(value)
			}
		}
	}

	// Sum all long-running requests across all label combinations
	if mf := mfs.Get("apiserver_longrunning_requests"); mf != nil {
		var total float64
		for _, m := range mf.Metrics() {
			if v := metricValue(mf, m); !math.IsNaN(v) {
				total += v
			}
		}
		mx.Inflight.Longrunning.Set(total)
	}
}

// collectRESTClient collects rest_client_requests_total and rest_client_request_duration_seconds
func (c *Collector) collectRESTClient(mfs prometheus.MetricFamilies, mx *metrics) {
	codeChart := c.charts.Get("rest_client_requests_by_code")
	methodChart := c.charts.Get("rest_client_requests_by_method")

	if mf := mfs.Get("rest_client_requests_total"); mf != nil {
		for _, m := range mf.Metrics() {
			value := metricValue(mf, m)
			if math.IsNaN(value) {
				continue
			}

			code := m.Labels().Get("code")
			method := m.Labels().Get("method")

			// By code (track for cleanup)
			if code != "" {
				codeID := cleanID(code)
				_, seen := c.collectedRESTCodes[codeID]
				c.collectedRESTCodes[codeID] = c.collectCycle
				if !seen {
					dimID := "rest_client_by_code_" + codeID
					if codeChart != nil && !codeChart.HasDim(dimID) {
						if err := codeChart.AddDim(&Dim{ID: dimID, Name: code, Algo: module.Incremental}); err != nil {
							c.Warningf("failed to add REST client code dimension %s: %v", code, err)
						} else {
							codeChart.MarkNotCreated()
						}
					}
				}
				mx.RESTClient.ByCode[codeID] = mtx.Gauge(mx.RESTClient.ByCode[codeID].Value() + value)
			}

			// By method (track for cleanup)
			if method != "" {
				methodID := cleanID(method)
				_, seen := c.collectedRESTMethods[methodID]
				c.collectedRESTMethods[methodID] = c.collectCycle
				if !seen {
					dimID := "rest_client_by_method_" + methodID
					if methodChart != nil && !methodChart.HasDim(dimID) {
						if err := methodChart.AddDim(&Dim{ID: dimID, Name: method, Algo: module.Incremental}); err != nil {
							c.Warningf("failed to add REST client method dimension %s: %v", method, err)
						} else {
							methodChart.MarkNotCreated()
						}
					}
				}
				mx.RESTClient.ByMethod[methodID] = mtx.Gauge(mx.RESTClient.ByMethod[methodID].Value() + value)
			}
		}
	}

	// REST client latency
	if mf := mfs.Get("rest_client_request_duration_seconds"); mf != nil && mf.Type() == model.MetricTypeHistogram {
		hd := collectHistogramBucketsFromMF(mf, nil)
		if len(hd.buckets) > 0 {
			if p50 := histogramPercentile(hd, 0.5); !math.IsNaN(p50) {
				mx.RESTClient.Latency.P50.Set(p50 * latencyPrecision)
			}
			if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
				mx.RESTClient.Latency.P90.Set(p90 * latencyPrecision)
			}
			if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
				mx.RESTClient.Latency.P99.Set(p99 * latencyPrecision)
			}
		}
	}
}

// collectAdmission collects admission controller and webhook metrics
func (c *Collector) collectAdmission(mfs prometheus.MetricFamilies, mx *metrics) {
	// Admission step latency - aggregate buckets across all operation/rejected label combinations
	if mf := mfs.Get("apiserver_admission_step_admission_duration_seconds"); mf != nil && mf.Type() == model.MetricTypeHistogram {
		// Use map[type][le] -> count for proper aggregation
		stepBuckets := make(map[string]map[float64]float64)
		stepTotals := make(map[string]float64)

		for _, m := range mf.Metrics() {
			if m.Histogram() == nil {
				continue
			}
			stepType := m.Labels().Get("type")
			if stepType == "" {
				continue
			}

			if stepBuckets[stepType] == nil {
				stepBuckets[stepType] = make(map[float64]float64)
			}

			for _, b := range m.Histogram().Buckets() {
				if math.IsInf(b.UpperBound(), 0) {
					stepTotals[stepType] += b.CumulativeCount()
					continue
				}
				// Aggregate by summing counts for same le across all label combinations
				stepBuckets[stepType][b.UpperBound()] += b.CumulativeCount()
			}
		}

		// Convert aggregated buckets to histogramData for percentile calculation
		for stepType, bucketMap := range stepBuckets {
			hd := histogramData{total: stepTotals[stepType]}
			for le, count := range bucketMap {
				hd.buckets = append(hd.buckets, histogramBucket{le: le, count: count})
			}
			sortBuckets(hd.buckets)

			if p50 := histogramPercentile(hd, 0.5); !math.IsNaN(p50) {
				switch stepType {
				case "validate":
					mx.Admission.StepLatency.Validate.Set(p50 * latencyPrecision)
				case "admit":
					mx.Admission.StepLatency.Admit.Set(p50 * latencyPrecision)
				}
			}
		}
	}

	// Admission controller latency (dynamic charts)
	c.collectAdmissionControllerLatency(mfs, mx)
	c.collectAdmissionWebhookLatency(mfs, mx)
}

func (c *Collector) collectAdmissionControllerLatency(mfs prometheus.MetricFamilies, mx *metrics) {
	mf := mfs.Get("apiserver_admission_controller_admission_duration_seconds")
	if mf == nil || mf.Type() != model.MetricTypeHistogram {
		return
	}

	// Collect as heatmap with non-cumulative bucket counts
	// Must aggregate by controller name, then by le bucket
	controllerBucketMaps := make(map[string]map[float64]float64) // name -> le -> cumulative_count

	for _, m := range mf.Metrics() {
		if m.Histogram() == nil {
			continue
		}
		name := m.Labels().Get("name")
		if name == "" {
			continue
		}

		if controllerBucketMaps[name] == nil {
			controllerBucketMaps[name] = make(map[float64]float64)
		}

		for _, b := range m.Histogram().Buckets() {
			controllerBucketMaps[name][b.UpperBound()] += float64(b.CumulativeCount())
		}
	}

	// Collect names into slice first to avoid modifying maps during iteration
	controllerNames := make([]string, 0, len(controllerBucketMaps))
	for name := range controllerBucketMaps {
		controllerNames = append(controllerNames, name)
	}

	for _, name := range controllerNames {
		bucketMap := controllerBucketMaps[name]

		// Sanitize name for use in chart/dimension IDs
		cleanName := cleanID(name)

		// Cardinality limit: only accept new items if under limit
		_, seen := c.collectedAdmissionCtrl[cleanName]
		if !seen && len(c.collectedAdmissionCtrl) >= defaultMaxAdmCtrl {
			continue
		}

		// Track last-seen cycle
		c.collectedAdmissionCtrl[cleanName] = c.collectCycle

		if !seen {
			if err := c.charts.Add(newAdmissionControllerLatencyChart(cleanName)); err != nil {
				c.Warningf("failed to add admission controller chart %s: %v", name, err)
			}
		}

		if mx.Admission.Controllers[cleanName] == nil {
			mx.Admission.Controllers[cleanName] = &admissionControllerMetrics{}
		}

		// Use cumulative bucket values directly - chart uses Incremental algorithm
		// This avoids negative values when Prometheus counters reset
		c.setAdmissionBucketMetrics(bucketMap, mx.Admission.Controllers[cleanName])
	}
}

func (c *Collector) collectAdmissionWebhookLatency(mfs prometheus.MetricFamilies, mx *metrics) {
	mf := mfs.Get("apiserver_admission_webhook_admission_duration_seconds")
	if mf == nil || mf.Type() != model.MetricTypeHistogram {
		return
	}

	// Collect as heatmap with non-cumulative bucket counts
	webhookBucketMaps := make(map[string]map[float64]float64) // name -> le -> cumulative_count

	for _, m := range mf.Metrics() {
		if m.Histogram() == nil {
			continue
		}
		name := m.Labels().Get("name")
		if name == "" {
			continue
		}

		if webhookBucketMaps[name] == nil {
			webhookBucketMaps[name] = make(map[float64]float64)
		}

		for _, b := range m.Histogram().Buckets() {
			webhookBucketMaps[name][b.UpperBound()] += float64(b.CumulativeCount())
		}
	}

	// Collect names into slice first to avoid modifying maps during iteration
	webhookNames := make([]string, 0, len(webhookBucketMaps))
	for name := range webhookBucketMaps {
		webhookNames = append(webhookNames, name)
	}

	for _, name := range webhookNames {
		bucketMap := webhookBucketMaps[name]

		// Sanitize name for use in chart/dimension IDs
		cleanName := cleanID(name)

		// Cardinality limit: only accept new items if under limit
		_, seen := c.collectedAdmissionWH[cleanName]
		if !seen && len(c.collectedAdmissionWH) >= defaultMaxAdmWebhooks {
			continue
		}

		// Track last-seen cycle
		c.collectedAdmissionWH[cleanName] = c.collectCycle

		if !seen {
			if err := c.charts.Add(newAdmissionWebhookLatencyChart(cleanName)); err != nil {
				c.Warningf("failed to add admission webhook chart %s: %v", name, err)
			}
		}

		if mx.Admission.Webhooks[cleanName] == nil {
			mx.Admission.Webhooks[cleanName] = &admissionWebhookMetrics{}
		}

		// Use cumulative bucket values directly - chart uses Incremental algorithm
		// This avoids negative values when Prometheus counters reset
		c.setAdmissionBucketMetrics(bucketMap, mx.Admission.Webhooks[cleanName])
	}
}

// collectEtcd collects etcd/storage object count metrics
func (c *Collector) collectEtcd(mfs prometheus.MetricFamilies, mx *metrics) {
	chart := c.charts.Get("etcd_object_counts")
	if chart == nil {
		chart = newEtcdObjectCountsChart()
		if err := c.charts.Add(chart); err != nil {
			c.Warningf("failed to add etcd object counts chart: %v", err)
		}
	}

	// Try apiserver_storage_objects first (newer k8s), then etcd_object_counts (older)
	mf := mfs.Get("apiserver_storage_objects")
	if mf == nil {
		mf = mfs.Get("etcd_object_counts")
	}
	if mf == nil {
		return
	}

	for _, m := range mf.Metrics() {
		value := metricValue(mf, m)
		if math.IsNaN(value) {
			continue
		}

		resource := m.Labels().Get("resource")
		if resource == "" {
			continue
		}

		resourceName := simplifyResourceName(resource)
		resourceID := cleanID(resourceName)
		dimID := "etcd_objects_" + resourceID

		if chart != nil && !chart.HasDim(dimID) {
			if err := chart.AddDim(&Dim{ID: dimID, Name: resourceName}); err != nil {
				c.Debugf("failed to add etcd object dimension %s: %v", resourceName, err)
			} else {
				chart.MarkNotCreated()
			}
		}
		mx.Etcd.ObjectCounts[resourceID] = mtx.Gauge(value)
	}
}

// collectWorkqueues collects controller work queue metrics
func (c *Collector) collectWorkqueues(mfs prometheus.MetricFamilies, mx *metrics) {
	// Pre-index metrics by queue name
	depthByName := make(map[string]float64)
	addsByName := make(map[string]float64)
	retriesByName := make(map[string]float64)

	if mf := mfs.Get("workqueue_depth"); mf != nil {
		for _, m := range mf.Metrics() {
			if name := m.Labels().Get("name"); name != "" {
				if v := metricValue(mf, m); !math.IsNaN(v) {
					depthByName[name] = v
				}
			}
		}
	}
	if mf := mfs.Get("workqueue_adds_total"); mf != nil {
		for _, m := range mf.Metrics() {
			if name := m.Labels().Get("name"); name != "" {
				if v := metricValue(mf, m); !math.IsNaN(v) {
					addsByName[name] = v
				}
			}
		}
	}
	if mf := mfs.Get("workqueue_retries_total"); mf != nil {
		for _, m := range mf.Metrics() {
			if name := m.Labels().Get("name"); name != "" {
				if v := metricValue(mf, m); !math.IsNaN(v) {
					retriesByName[name] = v
				}
			}
		}
	}

	queueLatencyMF := mfs.Get("workqueue_queue_duration_seconds")
	workDurationMF := mfs.Get("workqueue_work_duration_seconds")

	// Merge all queue names from depth, adds, and retries to avoid missing workqueues
	// that may not have depth metric but have other metrics
	allQueueNames := make(map[string]struct{})
	for name := range depthByName {
		allQueueNames[name] = struct{}{}
	}
	for name := range addsByName {
		allQueueNames[name] = struct{}{}
	}
	for name := range retriesByName {
		allQueueNames[name] = struct{}{}
	}

	// Collect names into slice to avoid modifying maps during iteration
	queueNames := make([]string, 0, len(allQueueNames))
	for name := range allQueueNames {
		queueNames = append(queueNames, name)
	}

	for _, queueName := range queueNames {
		// Sanitize name for use in chart/dimension IDs
		cleanName := cleanID(queueName)

		// Cardinality limit: only accept new items if under limit
		_, seen := c.collectedWorkqueues[cleanName]
		if !seen && len(c.collectedWorkqueues) >= defaultMaxWorkqueues {
			continue
		}

		// Track last-seen cycle
		c.collectedWorkqueues[cleanName] = c.collectCycle

		if !seen {
			if err := c.charts.Add(newWorkqueueDepthChart(cleanName)); err != nil {
				c.Warningf("failed to add workqueue depth chart %s: %v", queueName, err)
			}
			if err := c.charts.Add(newWorkqueueLatencyChart(cleanName)); err != nil {
				c.Warningf("failed to add workqueue latency chart %s: %v", queueName, err)
			}
			if err := c.charts.Add(newWorkqueueAddsChart(cleanName)); err != nil {
				c.Warningf("failed to add workqueue adds chart %s: %v", queueName, err)
			}
			if err := c.charts.Add(newWorkqueueDurationChart(cleanName)); err != nil {
				c.Warningf("failed to add workqueue duration chart %s: %v", queueName, err)
			}
		}

		if mx.Workqueue.Controllers[cleanName] == nil {
			mx.Workqueue.Controllers[cleanName] = &workqueueMetrics{}
		}

		wq := mx.Workqueue.Controllers[cleanName]
		wq.Depth.Set(depthByName[queueName])
		wq.Adds.Set(addsByName[queueName])
		wq.Retries.Set(retriesByName[queueName])

		// Queue latency (histogram)
		if queueLatencyMF != nil && queueLatencyMF.Type() == model.MetricTypeHistogram {
			hd := collectHistogramBucketsFromMF(queueLatencyMF, func(m prometheus.Metric) bool {
				return m.Labels().Get("name") == queueName
			})
			if len(hd.buckets) > 0 {
				if p50 := histogramPercentile(hd, 0.5); !math.IsNaN(p50) {
					wq.LatencyP50.Set(p50 * latencyPrecision)
				}
				if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
					wq.LatencyP90.Set(p90 * latencyPrecision)
				}
				if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
					wq.LatencyP99.Set(p99 * latencyPrecision)
				}
			}
		}

		// Work duration (histogram)
		if workDurationMF != nil && workDurationMF.Type() == model.MetricTypeHistogram {
			hd := collectHistogramBucketsFromMF(workDurationMF, func(m prometheus.Metric) bool {
				return m.Labels().Get("name") == queueName
			})
			if len(hd.buckets) > 0 {
				if p50 := histogramPercentile(hd, 0.5); !math.IsNaN(p50) {
					wq.DurationP50.Set(p50 * latencyPrecision)
				}
				if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
					wq.DurationP90.Set(p90 * latencyPrecision)
				}
				if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
					wq.DurationP99.Set(p99 * latencyPrecision)
				}
			}
		}
	}
}

// collectProcess collects Go runtime and process metrics
func (c *Collector) collectProcess(mfs prometheus.MetricFamilies, mx *metrics) {
	mx.Process.Goroutines.Set(getMaxValue(mfs, "go_goroutines"))
	mx.Process.Threads.Set(getMaxValue(mfs, "go_threads"))
	mx.Process.CPUSeconds.Set(getMaxValue(mfs, "process_cpu_seconds_total") * precision)
	mx.Process.ResidentMemory.Set(getMaxValue(mfs, "process_resident_memory_bytes"))
	mx.Process.VirtualMemory.Set(getMaxValue(mfs, "process_virtual_memory_bytes"))
	mx.Process.OpenFDs.Set(getMaxValue(mfs, "process_open_fds"))
	mx.Process.MaxFDs.Set(getMaxValue(mfs, "process_max_fds"))
	mx.Process.HeapAlloc.Set(getMaxValue(mfs, "go_memstats_heap_alloc_bytes"))
	mx.Process.HeapInuse.Set(getMaxValue(mfs, "go_memstats_heap_inuse_bytes"))
	mx.Process.StackInuse.Set(getMaxValue(mfs, "go_memstats_stack_inuse_bytes"))

	// GC duration (summary with quantile labels)
	if mf := mfs.Get("go_gc_duration_seconds"); mf != nil && mf.Type() == model.MetricTypeSummary {
		for _, m := range mf.Metrics() {
			if m.Summary() == nil {
				continue
			}
			for _, q := range m.Summary().Quantiles() {
				if math.IsNaN(q.Value()) {
					continue
				}
				switch q.Quantile() {
				case 0:
					mx.Process.GCDurationMin.Set(q.Value() * latencyPrecision)
				case 0.25:
					mx.Process.GCDurationP25.Set(q.Value() * latencyPrecision)
				case 0.5:
					mx.Process.GCDurationP50.Set(q.Value() * latencyPrecision)
				case 0.75:
					mx.Process.GCDurationP75.Set(q.Value() * latencyPrecision)
				case 1:
					mx.Process.GCDurationMax.Set(q.Value() * latencyPrecision)
				}
			}
		}
	}
}

// collectAudit collects audit event metrics
func (c *Collector) collectAudit(mfs prometheus.MetricFamilies, mx *metrics) {
	mx.Audit.EventsTotal.Set(getMaxValue(mfs, "apiserver_audit_event_total"))
	mx.Audit.RejectedTotal.Set(getMaxValue(mfs, "apiserver_audit_requests_rejected_total"))
}

// collectAuth collects authentication metrics
func (c *Collector) collectAuth(mfs prometheus.MetricFamilies, mx *metrics) {
	// Sum across all usernames
	if mf := mfs.Get("authenticated_user_requests"); mf != nil {
		for _, m := range mf.Metrics() {
			if v := metricValue(mf, m); !math.IsNaN(v) {
				mx.Auth.AuthenticatedRequests.Add(v)
			}
		}
	}

	// Client certificate expiration - histogram, track count of certs expiring within 24h
	if mf := mfs.Get("apiserver_client_certificate_expiration_seconds"); mf != nil && mf.Type() == model.MetricTypeHistogram {
		for _, m := range mf.Metrics() {
			if m.Histogram() == nil {
				continue
			}
			for _, b := range m.Histogram().Buckets() {
				if b.UpperBound() == 86400 { // 1 day bucket
					mx.Auth.CertExpirationSeconds.Set(float64(b.CumulativeCount()))
					break
				}
			}
		}
	}
}

// Helper functions for dynamic dimension creation

func (c *Collector) addVerbDimension(verb string) {
	_, seen := c.collectedVerbs[verb]
	c.collectedVerbs[verb] = c.collectCycle

	if seen {
		return
	}

	chart := c.charts.Get("requests_by_verb")
	if chart == nil {
		c.Warningf("chart 'requests_by_verb' not found, cannot add dimension for verb: %s", verb)
		return
	}
	dimID := "request_by_verb_" + cleanID(verb)
	if !chart.HasDim(dimID) {
		if err := chart.AddDim(&Dim{ID: dimID, Name: verb, Algo: module.Incremental}); err != nil {
			c.Warningf("failed to add verb dimension %s: %v", verb, err)
		} else {
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) addCodeDimension(code string) {
	_, seen := c.collectedCodes[code]
	c.collectedCodes[code] = c.collectCycle

	if seen {
		return
	}

	chart := c.charts.Get("requests_by_code")
	if chart == nil {
		c.Warningf("chart 'requests_by_code' not found, cannot add dimension for code: %s", code)
		return
	}
	dimID := "request_by_code_" + cleanID(code)
	if !chart.HasDim(dimID) {
		if err := chart.AddDim(&Dim{ID: dimID, Name: code, Algo: module.Incremental}); err != nil {
			c.Warningf("failed to add code dimension %s: %v", code, err)
		} else {
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) addResourceDimension(resource string) {
	_, seen := c.collectedResources[resource]
	// Note: cycle tracking for resources is done in collectRequests due to cardinality limit check
	if seen {
		return
	}

	chart := c.charts.Get("requests_by_resource")
	if chart == nil {
		c.Warningf("chart 'requests_by_resource' not found, cannot add dimension for resource: %s", resource)
		return
	}
	dimID := "request_by_resource_" + cleanID(resource)
	if !chart.HasDim(dimID) {
		if err := chart.AddDim(&Dim{ID: dimID, Name: resource, Algo: module.Incremental}); err != nil {
			c.Warningf("failed to add resource dimension %s: %v", resource, err)
		} else {
			chart.MarkNotCreated()
		}
	}
}

// simplifyResourceName removes API group suffix from resource names
func simplifyResourceName(resource string) string {
	parts := strings.SplitN(resource, ".", 2)
	return parts[0]
}

// metricValue extracts the value from a metric based on its type
func metricValue(mf *prometheus.MetricFamily, m prometheus.Metric) float64 {
	switch mf.Type() {
	case model.MetricTypeGauge:
		if m.Gauge() != nil {
			return m.Gauge().Value()
		}
	case model.MetricTypeCounter:
		if m.Counter() != nil {
			return m.Counter().Value()
		}
	case model.MetricTypeUnknown:
		if m.Gauge() != nil {
			return m.Gauge().Value()
		}
		if m.Counter() != nil {
			return m.Counter().Value()
		}
	}
	return math.NaN()
}

// getMaxValue gets the maximum value across all metrics in a family
func getMaxValue(mfs prometheus.MetricFamilies, name string) float64 {
	mf := mfs.Get(name)
	if mf == nil {
		return 0
	}

	var maxVal float64
	for _, m := range mf.Metrics() {
		if v := metricValue(mf, m); !math.IsNaN(v) && v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// Histogram percentile calculation utilities

type histogramBucket struct {
	le    float64
	count float64
}

type histogramData struct {
	buckets []histogramBucket
	total   float64 // from +Inf bucket
}

// collectHistogramBucketsFromMF collects histogram buckets from a metric family
func collectHistogramBucketsFromMF(mf *prometheus.MetricFamily, filter func(prometheus.Metric) bool) histogramData {
	bucketMap := make(map[float64]float64)
	var total float64

	for _, m := range mf.Metrics() {
		if filter != nil && !filter(m) {
			continue
		}
		if m.Histogram() == nil {
			continue
		}

		for _, b := range m.Histogram().Buckets() {
			if math.IsInf(b.UpperBound(), 0) {
				total += b.CumulativeCount()
				continue
			}
			bucketMap[b.UpperBound()] += b.CumulativeCount()
		}
	}

	buckets := make([]histogramBucket, 0, len(bucketMap))
	for le, count := range bucketMap {
		buckets = append(buckets, histogramBucket{le: le, count: count})
	}

	sortBuckets(buckets)
	return histogramData{buckets: buckets, total: total}
}

func sortBuckets(buckets []histogramBucket) {
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].le < buckets[j].le
	})
}

// histogramPercentile estimates the percentile value from histogram buckets
// total should be from the +Inf bucket which contains the true total count
func histogramPercentile(hd histogramData, percentile float64) float64 {
	if len(hd.buckets) == 0 || hd.total == 0 {
		return math.NaN()
	}

	target := percentile * hd.total

	var prevBound, prevCount float64
	for _, b := range hd.buckets {
		if b.count >= target {
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

	return hd.buckets[len(hd.buckets)-1].le
}

// bucketSetter is implemented by admission controller and webhook metrics
type bucketSetter interface {
	setBuckets(b5ms, b25ms, b100ms, b500ms, b1s, b2500ms, bInf float64)
}

func (m *admissionControllerMetrics) setBuckets(b5ms, b25ms, b100ms, b500ms, b1s, b2500ms, bInf float64) {
	m.Bucket5ms.Set(b5ms)
	m.Bucket25ms.Set(b25ms)
	m.Bucket100ms.Set(b100ms)
	m.Bucket500ms.Set(b500ms)
	m.Bucket1s.Set(b1s)
	m.Bucket2500ms.Set(b2500ms)
	m.BucketInf.Set(bInf)
}

func (m *admissionWebhookMetrics) setBuckets(b5ms, b25ms, b100ms, b500ms, b1s, b2500ms, bInf float64) {
	m.Bucket5ms.Set(b5ms)
	m.Bucket25ms.Set(b25ms)
	m.Bucket100ms.Set(b100ms)
	m.Bucket500ms.Set(b500ms)
	m.Bucket1s.Set(b1s)
	m.Bucket2500ms.Set(b2500ms)
	m.BucketInf.Set(bInf)
}

// setAdmissionBucketMetrics extracts bucket values and sets them on the metrics
// Converts Prometheus cumulative buckets to non-cumulative for heatmap display
// Uses max(0, diff) to protect against negative values from counter resets
func (c *Collector) setAdmissionBucketMetrics(bucketMap map[float64]float64, m bucketSetter) {
	// Validate bucket presence and log warnings for missing buckets
	var missingBuckets []string
	for _, bound := range admissionBucketBounds {
		if _, ok := bucketMap[bound]; !ok {
			missingBuckets = append(missingBuckets, formatBucketBound(bound))
		}
	}
	if _, ok := bucketMap[math.Inf(1)]; !ok {
		missingBuckets = append(missingBuckets, "+Inf")
	}
	if len(missingBuckets) > 0 {
		c.Debugf("missing histogram buckets: %v", missingBuckets)
	}

	// Extract cumulative bucket values (0 if missing)
	b5ms := bucketMap[admissionBucketBounds[0]]
	b25ms := bucketMap[admissionBucketBounds[1]]
	b100ms := bucketMap[admissionBucketBounds[2]]
	b500ms := bucketMap[admissionBucketBounds[3]]
	b1s := bucketMap[admissionBucketBounds[4]]
	b2500ms := bucketMap[admissionBucketBounds[5]]
	bInf := bucketMap[math.Inf(1)]

	// Convert cumulative to non-cumulative (differential) bucket counts
	// Use max(0, diff) to handle Prometheus counter resets gracefully
	// When a counter resets, the cumulative value decreases, causing negative diffs
	m.setBuckets(
		b5ms,
		math.Max(0, b25ms-b5ms),
		math.Max(0, b100ms-b25ms),
		math.Max(0, b500ms-b100ms),
		math.Max(0, b1s-b500ms),
		math.Max(0, b2500ms-b1s),
		math.Max(0, bInf-b2500ms),
	)
}

// formatBucketBound formats a bucket bound for logging
func formatBucketBound(bound float64) string {
	if bound < 1 {
		return fmt.Sprintf("%.0fms", bound*1000)
	}
	return fmt.Sprintf("%.1fs", bound)
}

// cleanupStaleDimensions removes dimensions that haven't been seen for staleThresholdCycles
func (c *Collector) cleanupStaleDimensions() {
	threshold := c.collectCycle - staleThresholdCycles

	// Cleanup resources
	for name, lastSeen := range c.collectedResources {
		if lastSeen < threshold {
			delete(c.collectedResources, name)
			if chart := c.charts.Get("requests_by_resource"); chart != nil {
				dimID := "request_by_resource_" + cleanID(name)
				_ = chart.RemoveDim(dimID)
				chart.MarkNotCreated()
			}
			c.Debugf("removed stale resource dimension: %s", name)
		}
	}

	// Cleanup verbs
	for name, lastSeen := range c.collectedVerbs {
		if lastSeen < threshold {
			delete(c.collectedVerbs, name)
			if chart := c.charts.Get("requests_by_verb"); chart != nil {
				dimID := "request_by_verb_" + cleanID(name)
				_ = chart.RemoveDim(dimID)
				chart.MarkNotCreated()
			}
			c.Debugf("removed stale verb dimension: %s", name)
		}
	}

	// Cleanup codes
	for name, lastSeen := range c.collectedCodes {
		if lastSeen < threshold {
			delete(c.collectedCodes, name)
			if chart := c.charts.Get("requests_by_code"); chart != nil {
				dimID := "request_by_code_" + cleanID(name)
				_ = chart.RemoveDim(dimID)
				chart.MarkNotCreated()
			}
			c.Debugf("removed stale code dimension: %s", name)
		}
	}

	// Cleanup REST client codes
	for name, lastSeen := range c.collectedRESTCodes {
		if lastSeen < threshold {
			delete(c.collectedRESTCodes, name)
			if chart := c.charts.Get("rest_client_requests_by_code"); chart != nil {
				dimID := "rest_client_by_code_" + cleanID(name)
				_ = chart.RemoveDim(dimID)
				chart.MarkNotCreated()
			}
			c.Debugf("removed stale REST client code dimension: %s", name)
		}
	}

	// Cleanup REST client methods
	for name, lastSeen := range c.collectedRESTMethods {
		if lastSeen < threshold {
			delete(c.collectedRESTMethods, name)
			if chart := c.charts.Get("rest_client_requests_by_method"); chart != nil {
				dimID := "rest_client_by_method_" + cleanID(name)
				_ = chart.RemoveDim(dimID)
				chart.MarkNotCreated()
			}
			c.Debugf("removed stale REST client method dimension: %s", name)
		}
	}

	// Cleanup workqueues (remove charts)
	for name, lastSeen := range c.collectedWorkqueues {
		if lastSeen < threshold {
			delete(c.collectedWorkqueues, name)
			_ = c.charts.Remove("workqueue_depth_" + cleanID(name))
			_ = c.charts.Remove("workqueue_latency_" + cleanID(name))
			_ = c.charts.Remove("workqueue_adds_" + cleanID(name))
			_ = c.charts.Remove("workqueue_duration_" + cleanID(name))
			c.Debugf("removed stale workqueue charts: %s", name)
		}
	}

	// Cleanup admission controllers (remove charts)
	for name, lastSeen := range c.collectedAdmissionCtrl {
		if lastSeen < threshold {
			delete(c.collectedAdmissionCtrl, name)
			_ = c.charts.Remove("admission_controller_latency_" + cleanID(name))
			c.Debugf("removed stale admission controller chart: %s", name)
		}
	}

	// Cleanup admission webhooks (remove charts)
	for name, lastSeen := range c.collectedAdmissionWH {
		if lastSeen < threshold {
			delete(c.collectedAdmissionWH, name)
			_ = c.charts.Remove("admission_webhook_latency_" + cleanID(name))
			c.Debugf("removed stale admission webhook chart: %s", name)
		}
	}
}
