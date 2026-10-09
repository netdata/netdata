// SPDX-License-Identifier: GPL-3.0-or-later

// Package ingest normalizes the supported Java metric surface. It is not a
// general OTLP receiver: process admission belongs to the monitor.
package ingest

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

type Result struct{ Accepted, Rejected int }
type Application struct {
	Application, Instance, Runtime string
	LastSeen                       time.Time
	Samples                        []Sample
}
type Sample struct {
	Name                  string
	Labels                map[string]string
	Value                 float64
	Histogram             *metrix.HistogramPoint
	SourceTime, StartTime uint64
}
type entry struct {
	sample Sample
	origin string
}
type appState struct {
	name    string
	runtime string
	points  map[string]entry
}
type Store struct {
	mu   sync.Mutex
	apps map[string]*appState
	// Bounds stay pinned for the receiver lifetime: metrix descriptors can outlive
	// an application's last live sample, including after Remove.
	bounds    []float64
	boundsSet bool
}

func New() *Store { return &Store{apps: make(map[string]*appState)} }
func (s *Store) Admit(instance, application string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if instance == "" || application == "" {
		return
	}
	if a := s.apps[instance]; a != nil && a.name == application {
		return
	}
	s.apps[instance] = &appState{name: application, points: make(map[string]entry)}
}
func (s *Store) Remove(instance string) { s.mu.Lock(); defer s.mu.Unlock(); delete(s.apps, instance) }

// Snapshot returns owned copies, omitting each stale point independently. LastSeen
// is the newest source observation, never the most recent HTTP receipt.
func (s *Store) Snapshot(now time.Time, maxAge time.Duration) []Application {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Application
	for instance, a := range s.apps {
		result := Application{Application: a.name, Instance: instance, Runtime: a.runtime}
		for _, e := range a.points {
			observed := time.Unix(0, int64(e.sample.SourceTime))
			if observed.After(now) || now.Sub(observed) > maxAge {
				continue
			}
			result.Samples = append(result.Samples, clone(e.sample))
			if observed.After(result.LastSeen) {
				result.LastSeen = observed
			}
		}
		if len(result.Samples) == 0 {
			continue
		}
		sort.Slice(result.Samples, func(i, j int) bool { return identity(result.Samples[i]) < identity(result.Samples[j]) })
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Instance < out[j].Instance })
	return out
}

type candidate struct {
	instance, key, origin, runtime string
	sample                         Sample
}

// Ingest rejects unsupported or malformed known points independently. Unknown
// metric families are deliberately ignored. A non-nil error describes partial
// rejection; callers must still return Result.Accepted as successfully consumed.
func (s *Store) Ingest(req *collectorv1.ExportMetricsServiceRequest, now time.Time) (Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result Result
	if req == nil {
		return result, fmt.Errorf("nil metrics request")
	}
	var pending []candidate
	duplicates := make(map[string]int)
	for _, resource := range req.ResourceMetrics {
		attrs, valid := resourceAttributes(resource.GetResource().GetAttributes())
		instance := attrs["service.instance.id"]
		app := s.apps[instance]
		for _, scope := range resource.GetScopeMetrics() {
			scopeBytes, _ := proto.MarshalOptions{Deterministic: true}.Marshal(scope.GetScope())
			for _, metric := range scope.GetMetrics() {
				name, unit, projection := family(metric.GetName())
				if name == "" {
					continue
				}
				points, ok := normalize(metric, name, unit, projection)
				if !ok {
					result.Rejected += max(1, pointCount(metric))
					continue
				}
				for _, p := range points {
					if p == nil || !valid || app == nil || attrs["service.name"] == "" || !validTime(p.sample, now) {
						result.Rejected++
						continue
					}
					p.instance = instance
					p.runtime = attrs["process.runtime.version"]
					p.key = identity(p.sample)
					p.origin = string(scopeBytes) + "\x00" + scope.GetSchemaUrl() + "\x00" + p.origin
					duplicates[instance+"\x00"+p.key]++
					pending = append(pending, *p)
				}
			}
		}
	}
	for _, p := range pending {
		a := s.apps[p.instance]
		if duplicates[p.instance+"\x00"+p.key] > 1 {
			result.Rejected++
			continue
		}
		old, exists := a.points[p.key]
		if exists && (p.origin != old.origin || p.sample.SourceTime <= old.sample.SourceTime || p.sample.StartTime < old.sample.StartTime) {
			result.Rejected++
			continue
		}
		if p.sample.Histogram != nil {
			bounds := make([]float64, len(p.sample.Histogram.Buckets))
			for i, b := range p.sample.Histogram.Buckets {
				bounds[i] = b.UpperBound
			}
			if s.boundsSet && !slices.Equal(bounds, s.bounds) {
				result.Rejected++
				continue
			}
			if exists && p.sample.StartTime == old.sample.StartTime && regresses(*p.sample.Histogram, *old.sample.Histogram) {
				result.Rejected++
				continue
			}
			s.bounds, s.boundsSet = bounds, true
		}
		a.points[p.key] = entry{sample: p.sample, origin: p.origin}
		if p.runtime != "" {
			a.runtime = p.runtime
		}
		result.Accepted++
	}
	if result.Rejected > 0 {
		return result, fmt.Errorf("rejected %d Java metric points (invalid, ambiguous, unadmitted or non-advancing source)", result.Rejected)
	}
	return result, nil
}

func family(name string) (string, string, map[string]string) {
	switch name {
	case "jvm.memory.used":
		return "jvm_memory_used_bytes", "By", map[string]string{"jvm.memory.type": "memory_type", "jvm.memory.pool.name": "memory_pool"}
	case "http.server.request.duration":
		return "http_server_request_duration_seconds", "s", map[string]string{"http.request.method": "method", "http.route": "route", "http.response.status_code": "status"}
	case "netdata.spike.hikari.connections":
		return "hikari_connections", "{connection}", map[string]string{"pool.name": "pool_name", "pool.id": "pool_id", "state": "state"}
	case "netdata.spike.hikari.pending_requests":
		return "hikari_pending_requests", "{request}", map[string]string{"pool.name": "pool_name", "pool.id": "pool_id"}
	case "netdata.spike.hikari.limit":
		return "hikari_limit", "{connection}", map[string]string{"pool.name": "pool_name", "pool.id": "pool_id"}
	}
	return "", "", nil
}
func normalize(m *metricsv1.Metric, name, unit string, projection map[string]string) ([]*candidate, bool) {
	if m.Unit != unit {
		return nil, false
	}
	if name == "http_server_request_duration_seconds" {
		h := m.GetHistogram()
		if h == nil || h.AggregationTemporality != metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
			return nil, false
		}
		out := make([]*candidate, 0, len(h.DataPoints))
		for _, p := range h.DataPoints {
			c := point(name, p.GetAttributes(), projection, p.GetTimeUnixNano(), p.GetStartTimeUnixNano())
			hp, ok := histogram(p)
			if c == nil || !ok {
				out = append(out, nil)
				continue
			}
			c.sample.Histogram = hp
			c.sample.Labels["source_epoch"] = strconv.FormatUint(p.StartTimeUnixNano, 10)
			out = append(out, c)
		}
		return out, true
	}
	var points []*metricsv1.NumberDataPoint
	if g := m.GetGauge(); g != nil {
		points = g.DataPoints
	} else if sum := m.GetSum(); name == "jvm_memory_used_bytes" && sum != nil && !sum.IsMonotonic && sum.AggregationTemporality == metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
		// Pinned Java 2.32.0 emits this non-monotonic cumulative Sum for memory.
		points = sum.DataPoints
	} else {
		return nil, false
	}
	out := make([]*candidate, 0, len(points))
	for _, p := range points {
		c := point(name, p.GetAttributes(), projection, p.GetTimeUnixNano(), p.GetStartTimeUnixNano())
		if c == nil || p.GetFlags() != 0 {
			out = append(out, nil)
			continue
		}
		switch value := p.Value.(type) {
		case *metricsv1.NumberDataPoint_AsInt:
			c.sample.Value = float64(value.AsInt)
		case *metricsv1.NumberDataPoint_AsDouble:
			c.sample.Value = value.AsDouble
		default:
			out = append(out, nil)
			continue
		}
		if !finite(c.sample.Value) || c.sample.Value < 0 {
			out = append(out, nil)
			continue
		}
		out = append(out, c)
	}
	return out, true
}
func point(name string, attrs []*commonv1.KeyValue, projection map[string]string, stamp, start uint64) *candidate {
	values, ok := attributes(attrs)
	if !ok {
		return nil
	}
	labels := make(map[string]string, len(projection))
	for source, target := range projection {
		if values[source] == "" {
			return nil
		}
		labels[target] = values[source]
	}
	origin, _ := json.Marshal(values)
	return &candidate{sample: Sample{Name: name, Labels: labels, SourceTime: stamp, StartTime: start}, origin: string(origin)}
}
func attributes(attrs []*commonv1.KeyValue) (map[string]string, bool) {
	values := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		if kv == nil || kv.Value == nil {
			return nil, false
		}
		if _, exists := values[kv.Key]; exists {
			return nil, false
		}
		switch v := kv.Value.Value.(type) {
		case *commonv1.AnyValue_StringValue:
			values[kv.Key] = v.StringValue
		case *commonv1.AnyValue_IntValue:
			values[kv.Key] = strconv.FormatInt(v.IntValue, 10)
		case *commonv1.AnyValue_BoolValue:
			values[kv.Key] = strconv.FormatBool(v.BoolValue)
		case *commonv1.AnyValue_DoubleValue:
			values[kv.Key] = strconv.FormatFloat(v.DoubleValue, 'g', -1, 64)
		default:
			return nil, false
		}
	}
	return values, true
}
func histogram(p *metricsv1.HistogramDataPoint) (*metrix.HistogramPoint, bool) {
	if p == nil || p.Flags != 0 || p.StartTimeUnixNano == 0 || p.Sum == nil || !finite(*p.Sum) || *p.Sum < 0 || len(p.BucketCounts) != len(p.ExplicitBounds)+1 {
		return nil, false
	}
	result := &metrix.HistogramPoint{Count: float64(p.Count), Sum: *p.Sum}
	var count uint64
	for i, n := range p.BucketCounts {
		if math.MaxUint64-count < n {
			return nil, false
		}
		count += n
		if i == len(p.ExplicitBounds) {
			continue
		}
		bound := p.ExplicitBounds[i]
		if !finite(bound) || bound < 0 || (i > 0 && bound <= p.ExplicitBounds[i-1]) {
			return nil, false
		}
		result.Buckets = append(result.Buckets, metrix.BucketPoint{UpperBound: bound, CumulativeCount: float64(count)})
	}
	if count != p.Count {
		return nil, false
	}
	return result, true
}
func regresses(p, old metrix.HistogramPoint) bool {
	if p.Count < old.Count || p.Sum < old.Sum {
		return true
	}
	previous, oldPrevious := 0.0, 0.0
	for i, b := range p.Buckets {
		if b.CumulativeCount-previous < old.Buckets[i].CumulativeCount-oldPrevious {
			return true
		}
		previous, oldPrevious = b.CumulativeCount, old.Buckets[i].CumulativeCount
	}
	return p.Count-previous < old.Count-oldPrevious
}
func validTime(p Sample, now time.Time) bool {
	return p.SourceTime > 0 && p.SourceTime <= math.MaxInt64 && p.StartTime <= p.SourceTime && !time.Unix(0, int64(p.SourceTime)).After(now)
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func identity(p Sample) string {
	labels := make(map[string]string, len(p.Labels))
	for k, v := range p.Labels {
		if k != "source_epoch" {
			labels[k] = v
		}
	}
	data, _ := json.Marshal(labels)
	return p.Name + string(data)
}
func clone(p Sample) Sample {
	labels := make(map[string]string, len(p.Labels))
	for k, v := range p.Labels {
		labels[k] = v
	}
	p.Labels = labels
	if p.Histogram != nil {
		h := *p.Histogram
		h.Buckets = slices.Clone(h.Buckets)
		p.Histogram = &h
	}
	return p
}
func pointCount(m *metricsv1.Metric) int {
	switch v := m.Data.(type) {
	case *metricsv1.Metric_Gauge:
		return len(v.Gauge.GetDataPoints())
	case *metricsv1.Metric_Sum:
		return len(v.Sum.GetDataPoints())
	case *metricsv1.Metric_Histogram:
		return len(v.Histogram.GetDataPoints())
	case *metricsv1.Metric_ExponentialHistogram:
		return len(v.ExponentialHistogram.GetDataPoints())
	case *metricsv1.Metric_Summary:
		return len(v.Summary.GetDataPoints())
	}
	return 0
}

// Ignore unrelated resource metadata (including process.command_args arrays).
// Only monitor identity and the runtime display value belong to this receiver.
func resourceAttributes(attrs []*commonv1.KeyValue) (map[string]string, bool) {
	var selected []*commonv1.KeyValue
	for _, kv := range attrs {
		if kv == nil {
			continue
		}
		switch kv.Key {
		case "service.instance.id", "service.name", "process.runtime.version":
			if _, ok := kv.GetValue().GetValue().(*commonv1.AnyValue_StringValue); !ok {
				return nil, false
			}
			selected = append(selected, kv)
		}
	}
	return attributes(selected)
}
