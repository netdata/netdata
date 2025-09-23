//go:build cgo
// +build cgo

package mp

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/common"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/websphere/mp/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/openmetrics"
)

// Collector implements the WebSphere MicroProfile module using the ibm.d framework.
type Collector struct {
	framework.Collector

	Config `yaml:",inline" json:",inline"`

	once sync.Once

	client *openmetrics.Client

	identity common.Identity

	restSelector matcher.Matcher

	mpMetricsVersion string
	serverType       string
}

func (c *Collector) initOnce() {
	c.once.Do(func() {
	})
}

// CollectOnce performs a single scrape.
func (c *Collector) CollectOnce() error {
	c.initOnce()
	if c.client == nil {
		return errors.New("openmetrics client not initialised")
	}

	timeout := c.Config.ClientConfig.Timeout.Duration()
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	series, err := c.client.FetchSeries(ctx, nil)
	if err != nil {
		return err
	}

	agg := make(map[string]int64)
	restData := make(map[restKey]*restMetrics)

	c.mpMetricsVersion = detectMetricsVersion(series)
	if c.serverType == "" {
		c.serverType = "Liberty MicroProfile"
	}

	for _, sample := range series {
		name := sample.Labels.Get("__name__")
		if name == "" {
			continue
		}

		scope := sample.Labels.Get("mp_scope")
		normalized := normalizeMetricName(name, scope)

		method := sample.Labels.Get("method")
		endpoint := sample.Labels.Get("endpoint")

		if method != "" && endpoint != "" {
			if !c.CollectRESTMetrics {
				continue
			}
			c.collectRESTMetric(restData, method, endpoint, normalized, sample.Value)
			continue
		}

		if !c.CollectJVMMetrics && isCoreMetric(normalized) {
			continue
		}

		c.collectCoreMetric(agg, normalized, sample.Value)
	}

	c.exportCoreMetrics(agg)
	c.exportRESTMetrics(restData)

	labels := c.identity.Labels()
	if c.mpMetricsVersion != "" && c.mpMetricsVersion != "unknown" {
		labels["mp_metrics_version"] = c.mpMetricsVersion
	}
	if c.serverType != "" {
		labels["server_type"] = c.serverType
	}
	c.SetGlobalLabels(labels)

	return nil
}

var _ framework.CollectorImpl = (*Collector)(nil)

type restKey struct {
	Method   string
	Endpoint string
}

type restMetrics struct {
	Requests     int64
	ResponseTime int64
	hasRequests  bool
	hasResponse  bool
}

func normalizeMetricName(name, scope string) string {
	if scope != "" && !strings.HasPrefix(name, scope+"_") {
		return scope + "_" + name
	}
	return name
}

func detectMetricsVersion(series prometheus.Series) string {
	hasLabelScope := false
	hasPrefix := false
	hasVendor := false

	for _, s := range series {
		name := s.Labels.Get("__name__")
		scope := s.Labels.Get("mp_scope")
		if scope != "" {
			hasLabelScope = true
			if scope == "vendor" {
				hasVendor = true
			}
		}
		if strings.HasPrefix(name, "base_") || strings.HasPrefix(name, "vendor_") {
			hasPrefix = true
		}
		if strings.HasPrefix(name, "vendor_") {
			hasVendor = true
		}
	}

	if hasLabelScope {
		return "5.1"
	}
	if hasPrefix {
		if hasVendor {
			return "3.0"
		}
		return "4.0"
	}
	return "unknown"
}

func (c *Collector) collectCoreMetric(agg map[string]int64, name string, value float64) {
	switch name {
	case "base_memory_usedHeap_bytes":
		agg["heap_used"] = int64(value)
	case "base_memory_committedHeap_bytes":
		agg["heap_committed"] = int64(value)
	case "base_memory_maxHeap_bytes":
		agg["heap_max"] = int64(value)
	case "vendor_memory_heapUtilization_percent":
		agg["heap_util"] = common.FormatPercent(value)
	case "base_gc_total":
		agg["gc_total"] = int64(value)
	case "base_gc_time_seconds":
		agg["gc_time_ms"] = secondsToMillis(value)
	case "vendor_gc_time_per_cycle_seconds":
		agg["gc_time_cycle_ms"] = secondsToMillis(value)
	case "base_thread_count":
		agg["thread_total"] = int64(value)
	case "base_thread_daemon_count":
		agg["thread_daemon"] = int64(value)
	case "base_thread_max_count":
		agg["thread_peak"] = int64(value)
	case "base_cpu_processCpuLoad_percent":
		agg["cpu_process"] = common.FormatPercent(value)
	case "vendor_cpu_processCpuUtilization_percent":
		agg["cpu_util"] = common.FormatPercent(value)
	case "base_cpu_processCpuTime_seconds":
		agg["cpu_time_ms"] = secondsToMillis(value)
	case "threadpool_activeThreads":
		agg["threadpool_active"] = int64(value)
	case "threadpool_size":
		agg["threadpool_size"] = int64(value)
	}
}

func secondsToMillis(v float64) int64 {
	return int64(math.Round(v * 1000))
}

func (c *Collector) collectRESTMetric(rest map[restKey]*restMetrics, method, endpoint, name string, value float64) {
	target := method + " " + endpoint
	if c.restSelector != nil && !c.restSelector.MatchString(target) {
		return
	}

	key := restKey{Method: method, Endpoint: endpoint}
	metrics := rest[key]
	if metrics == nil {
		if c.MaxRESTEndpoints > 0 && len(rest) >= c.MaxRESTEndpoints {
			return
		}
		metrics = &restMetrics{}
		rest[key] = metrics
	}

	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "request") && (strings.Contains(lower, "total") || strings.Contains(lower, "count")):
		metrics.Requests = int64(value)
		metrics.hasRequests = true
	case strings.Contains(lower, "time") || strings.Contains(lower, "duration"):
		metrics.ResponseTime = secondsToMillis(value)
		metrics.hasResponse = true
	}
}

func (c *Collector) exportCoreMetrics(agg map[string]int64) {
	labels := contexts.EmptyLabels{}

	if used, ok := agg["heap_used"]; ok {
		if committed, okc := agg["heap_committed"]; okc {
			free := committed - used
			if free < 0 {
				free = 0
			}
			contexts.JVM.HeapUsage.Set(c.State, labels, contexts.JVMHeapUsageValues{
				Used: used,
				Free: free,
			})
		}
	}

	if committed, ok := agg["heap_committed"]; ok {
		contexts.JVM.HeapCommitted.Set(c.State, labels, contexts.JVMHeapCommittedValues{Committed: committed})
	}

	if max, ok := agg["heap_max"]; ok {
		contexts.JVM.HeapMax.Set(c.State, labels, contexts.JVMHeapMaxValues{Limit: max})
	}

	if util, ok := agg["heap_util"]; ok {
		contexts.JVM.HeapUtilization.Set(c.State, labels, contexts.JVMHeapUtilizationValues{Utilization: util})
	}

	if rate, ok := agg["gc_total"]; ok {
		contexts.JVM.GCCollections.Set(c.State, labels, contexts.JVMGCCollectionsValues{Rate: rate})
	}

	if total, ok := agg["gc_time_ms"]; ok {
		perCycle := int64(0)
		if cycle, okc := agg["gc_time_cycle_ms"]; okc {
			perCycle = cycle
		}
		contexts.JVM.GCTime.Set(c.State, labels, contexts.JVMGCTimeValues{
			Total:     total,
			Per_cycle: perCycle,
		})
	}

	if total, ok := agg["thread_total"]; ok {
		daemon := agg["thread_daemon"]
		other := total - daemon
		if other < 0 {
			other = 0
		}
		contexts.JVM.ThreadsCurrent.Set(c.State, labels, contexts.JVMThreadsCurrentValues{
			Daemon: daemon,
			Other:  other,
		})
	}

	if peak, ok := agg["thread_peak"]; ok {
		contexts.JVM.ThreadsPeak.Set(c.State, labels, contexts.JVMThreadsPeakValues{Peak: peak})
	}

	if proc, ok := agg["cpu_process"]; ok {
		contexts.CPU.Usage.Set(c.State, contexts.EmptyLabels{}, contexts.CPUUsageValues{
			Process:     proc,
			Utilization: agg["cpu_util"],
		})
	}

	if timeVal, ok := agg["cpu_time_ms"]; ok {
		contexts.CPU.Time.Set(c.State, contexts.EmptyLabels{}, contexts.CPUTimeValues{Total: timeVal})
	}

	active, hasActive := agg["threadpool_active"]
	size, hasSize := agg["threadpool_size"]
	if hasActive || hasSize {
		idle := int64(0)
		if hasActive && hasSize {
			idle = size - active
			if idle < 0 {
				idle = 0
			}
		} else if hasSize {
			idle = size
		}
		contexts.Vendor.ThreadPoolUsage.Set(c.State, labels, contexts.VendorThreadPoolUsageValues{
			Active: active,
			Idle:   idle,
		})
		if hasSize {
			contexts.Vendor.ThreadPoolSize.Set(c.State, labels, contexts.VendorThreadPoolSizeValues{Size: size})
		}
	}
}

func (c *Collector) exportRESTMetrics(rest map[restKey]*restMetrics) {
	for key, metrics := range rest {
		labels := contexts.RESTEndpointLabels{Method: key.Method, Endpoint: key.Endpoint}
		if metrics.hasRequests {
			contexts.RESTEndpoint.Requests.Set(c.State, labels, contexts.RESTEndpointRequestsValues{Requests: metrics.Requests})
		}
		if metrics.hasResponse {
			contexts.RESTEndpoint.ResponseTime.Set(c.State, labels, contexts.RESTEndpointResponseTimeValues{Average: metrics.ResponseTime})
		}
	}
}

func isCoreMetric(name string) bool {
	if strings.HasPrefix(name, "base_") || strings.HasPrefix(name, "vendor_") {
		return true
	}
	if strings.HasPrefix(name, "threadpool_") {
		return true
	}
	return false
}
