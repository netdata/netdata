// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	"math"
	"sort"
	"strings"

	"github.com/prometheus/common/model"

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
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

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

	return stm.ToMap(mx), nil
}

// collectRequests collects apiserver_request_total, apiserver_request_duration_seconds, etc.
func (c *Collector) collectRequests(mfs prometheus.MetricFamilies, mx *metrics) {
	// Total requests and by verb/code/resource
	for _, mf := range mfs {
		name := mf.Name()
		if name != "apiserver_request_total" && name != "apiserver_request_count" {
			continue
		}

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
				mx.Request.ByVerb[verb] = mtx.Gauge(mx.Request.ByVerb[verb].Value() + value)
			}

			// By code
			if code != "" {
				c.addCodeDimension(code)
				mx.Request.ByCode[code] = mtx.Gauge(mx.Request.ByCode[code].Value() + value)
			}

			// By resource (with cardinality limit)
			if resource != "" {
				if c.collectedResources[resource] || len(c.collectedResources) < defaultMaxResources {
					c.addResourceDimension(resource)
					mx.Request.ByResource[resource] = mtx.Gauge(mx.Request.ByResource[resource].Value() + value)
				}
			}
		}
	}

	// Dropped/rejected requests - support both legacy and APF metrics
	var dropped float64
	if mf := mfs.Get("apiserver_dropped_requests_total"); mf != nil {
		for _, m := range mf.Metrics() {
			if v := metricValue(mf, m); !math.IsNaN(v) {
				dropped += v
			}
		}
	}
	if dropped == 0 {
		if mf := mfs.Get("apiserver_flowcontrol_rejected_requests_total"); mf != nil {
			for _, m := range mf.Metrics() {
				if v := metricValue(mf, m); !math.IsNaN(v) {
					dropped += v
				}
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

	if mf := mfs.Get("apiserver_longrunning_requests"); mf != nil {
		var maxVal float64
		for _, m := range mf.Metrics() {
			if v := metricValue(mf, m); !math.IsNaN(v) && v > maxVal {
				maxVal = v
			}
		}
		mx.Inflight.Longrunning.Set(maxVal)
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
				mx.RESTClient.ByCode[code] = mtx.Gauge(mx.RESTClient.ByCode[code].Value() + value)
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
				mx.RESTClient.ByMethod[method] = mtx.Gauge(mx.RESTClient.ByMethod[method].Value() + value)
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
	// Admission step latency
	if mf := mfs.Get("apiserver_admission_step_admission_duration_seconds"); mf != nil && mf.Type() == model.MetricTypeHistogram {
		stepData := make(map[string]*histogramData)

		for _, m := range mf.Metrics() {
			if m.Histogram() == nil {
				continue
			}
			stepType := m.Labels().Get("type")
			if stepType == "" {
				continue
			}

			if stepData[stepType] == nil {
				stepData[stepType] = &histogramData{}
			}

			for _, b := range m.Histogram().Buckets() {
				if math.IsInf(b.UpperBound(), 0) {
					stepData[stepType].total += b.CumulativeCount()
					continue
				}
				stepData[stepType].buckets = append(stepData[stepType].buckets, histogramBucket{
					le:    b.UpperBound(),
					count: b.CumulativeCount(),
				})
			}
		}

		for stepType, hd := range stepData {
			sortBuckets(hd.buckets)
			if p50 := histogramPercentile(*hd, 0.5); !math.IsNaN(p50) {
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
		b5ms := bucketMap[0.005]
		b25ms := bucketMap[0.025]
		b100ms := bucketMap[0.1]
		b500ms := bucketMap[0.5]
		b1s := bucketMap[1.0]
		b2500ms := bucketMap[2.5]
		bInf := bucketMap[math.Inf(1)]

		mx.Admission.Controllers[name].Bucket5ms.Set(b5ms)
		mx.Admission.Controllers[name].Bucket25ms.Set(b25ms - b5ms)
		mx.Admission.Controllers[name].Bucket100ms.Set(b100ms - b25ms)
		mx.Admission.Controllers[name].Bucket500ms.Set(b500ms - b100ms)
		mx.Admission.Controllers[name].Bucket1s.Set(b1s - b500ms)
		mx.Admission.Controllers[name].Bucket2500ms.Set(b2500ms - b1s)
		mx.Admission.Controllers[name].BucketInf.Set(bInf - b2500ms)
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
		b5ms := bucketMap[0.005]
		b25ms := bucketMap[0.025]
		b100ms := bucketMap[0.1]
		b500ms := bucketMap[0.5]
		b1s := bucketMap[1.0]
		b2500ms := bucketMap[2.5]
		bInf := bucketMap[math.Inf(1)]

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
		dimID := "etcd_objects_" + resourceName

		if chart != nil && !chart.HasDim(dimID) {
			if err := chart.AddDim(&Dim{ID: dimID, Name: resourceName}); err != nil {
				c.Debugf("failed to add etcd object dimension %s: %v", resourceName, err)
			} else {
				chart.MarkNotCreated()
			}
		}
		mx.Etcd.ObjectCounts[resourceName] = mtx.Gauge(value)
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
					wq.LatencyP50.Set(p50 * 1000000)
				}
				if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
					wq.LatencyP90.Set(p90 * 1000000)
				}
				if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
					wq.LatencyP99.Set(p99 * 1000000)
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
					wq.DurationP50.Set(p50 * 1000000)
				}
				if p90 := histogramPercentile(hd, 0.9); !math.IsNaN(p90) {
					wq.DurationP90.Set(p90 * 1000000)
				}
				if p99 := histogramPercentile(hd, 0.99); !math.IsNaN(p99) {
					wq.DurationP99.Set(p99 * 1000000)
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
					mx.Process.GCDurationMin.Set(q.Value() * 1000000)
				case 0.25:
					mx.Process.GCDurationP25.Set(q.Value() * 1000000)
				case 0.5:
					mx.Process.GCDurationP50.Set(q.Value() * 1000000)
				case 0.75:
					mx.Process.GCDurationP75.Set(q.Value() * 1000000)
				case 1:
					mx.Process.GCDurationMax.Set(q.Value() * 1000000)
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
