// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"fmt"
	"hash/fnv"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const precision = 1000.0

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}
	if mfs.Len() == 0 {
		c.counterSamples = nil
		c.cache.reset()
		c.removeStaleChartsAndDims()
		return nil, nil
	}
	if c.checkMetrics && !hasDCGMMetricFamilies(mfs) {
		return nil, fmt.Errorf("'%s' metrics have no DCGM prefix", c.URL)
	}
	c.checkMetrics = false
	if c.MaxTS > 0 {
		if n := calcDCGMMetricSeries(mfs); n > c.MaxTS {
			return nil, fmt.Errorf("'%s' num of time series (%d) > limit (%d)", c.URL, n, c.MaxTS)
		}
	}
	selected := make(map[metricKey]collectedValue)
	nextCounters := make(map[metricKey]counterSample)
	now := c.now()
	for _, mf := range mfs {
		if !isDCGMMetricName(mf.Name()) {
			continue
		}
		if c.MaxTSPerMetric > 0 && len(mf.Metrics()) > c.MaxTSPerMetric {
			continue
		}
		typ := metricFamilyKind(mf)
		if typ == sampleUnsupported {
			continue
		}
		for _, metric := range mf.Metrics() {
			value, ok := metricValue(metric, typ)
			if !ok || isInvalidMetricValue(value) {
				continue
			}
			def := fieldCatalog[strings.ToUpper(mf.Name())]
			instance, parent := metricInstances(metric.Labels(), def)
			spec := classifyMetric(instance.entity, mf.Name(), mf.Help(), typ)
			spec.DimName = dimensionName(spec, metric.Labels())
			if spec.Raw {
				labels := instance.chartLabels[:0]
				for _, label := range instance.chartLabels {
					if isIdentityOrMetadataLabel(label.Key) {
						labels = append(labels, label)
					}
				}
				instance.chartLabels = labels
			}
			key := metricKey{
				name:      mf.Name(),
				instance:  instance.key,
				dimension: spec.DimName,
			}
			value, ok = c.normalizeValue(value, spec, key, now, nextCounters)
			if !ok {
				continue
			}
			selectValue(selected, collectedValue{
				instance: instance,
				parent:   parent,
				spec:     spec,
				value:    value,
				source:   mf.Name(),
			})
		}
	}
	c.counterSamples = nextCounters
	c.cache.reset()
	mx := make(map[string]int64)
	display := make(map[metricKey]collectedValue)
	for _, v := range selected {
		if v.spec.RateSource == "" {
			c.emitValue(mx, v)
			continue
		}
		v.spec.RateSource = ""
		selectValue(display, v)
	}
	for _, v := range display {
		if v.spec.Transport != "" && v.spec.Direction == "total" {
			if v.instance.entity != entityNVLink {
				continue
			}
			v.spec.Context = contextCatalog["dcgm.nvlink.interconnect.combined.throughput"]
		}
		c.emitValue(mx, v)
	}
	c.emitInterconnectTotals(mx, selected)
	c.removeStaleChartsAndDims()
	if len(mx) == 0 {
		return nil, nil
	}
	return mx, nil
}

func metricFamilyKind(mf *prometheus.MetricFamily) sampleKind {
	switch mf.Type() {
	case model.MetricTypeCounter:
		return sampleCounter
	case model.MetricTypeGauge:
		return sampleGauge
	case model.MetricTypeHistogram, model.MetricTypeSummary:
		return sampleUnsupported
	default:
		if strings.HasSuffix(strings.ToLower(mf.Name()), "_total") {
			return sampleCounter
		}
		return sampleGauge
	}
}

func metricValue(metric prometheus.Metric, typ sampleKind) (float64, bool) {
	if typ == sampleCounter {
		if c := metric.Counter(); c != nil {
			return c.Value(), true
		}
		if u := metric.Untyped(); u != nil {
			return u.Value(), true
		}
		if g := metric.Gauge(); g != nil {
			return g.Value(), true
		}
		return 0, false
	}

	if g := metric.Gauge(); g != nil {
		return g.Value(), true
	}
	if u := metric.Untyped(); u != nil {
		return u.Value(), true
	}
	if c := metric.Counter(); c != nil {
		return c.Value(), true
	}

	return 0, false
}

func hasDCGMMetricFamilies(mfs prometheus.MetricFamilies) bool {
	for name := range mfs {
		if isDCGMMetricName(name) {
			return true
		}
	}
	return false
}

func isDCGMMetricName(name string) bool {
	return strings.HasPrefix(name, "DCGM_") || strings.HasPrefix(strings.ToLower(name), "dcgm_")
}

func calcDCGMMetricSeries(mfs prometheus.MetricFamilies) int {
	var total int
	for name, mf := range mfs {
		if !isDCGMMetricName(name) {
			continue
		}
		total += len(mf.Metrics())
	}
	return total
}

func isInvalidMetricValue(v float64) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return true
	}
	// DCGM often uses large sentinel values for unsupported fields.
	if math.Abs(v) >= 9e18 {
		return true
	}
	return false
}

type entityInstance struct {
	entity      metricEntity
	key         string
	chartLabels []collectorapi.Label
}

func resolveEntityInstance(lbls promlabels.Labels) entityInstance {
	idx := make(map[string]string, len(lbls))
	for _, lbl := range lbls {
		if lbl.Name == "" || lbl.Value == "" {
			continue
		}
		idx[strings.ToLower(lbl.Name)] = lbl.Value
	}

	entity := detectEntity(idx)
	identityKeys := identityKeysForEntity(entity)
	parts := make([]string, 0, len(identityKeys))

	for _, key := range identityKeys {
		v, ok := idx[key]
		if !ok || v == "" {
			continue
		}
		parts = append(parts, key+"="+url.QueryEscape(v))
	}

	if len(parts) == 0 {
		parts = append(parts, "global")
	}

	chartLabels := buildChartLabels(idx)

	return entityInstance{
		entity:      entity,
		key:         strings.Join(parts, "|"),
		chartLabels: chartLabels,
	}
}

func detectEntity(idx map[string]string) metricEntity {
	switch {
	case hasLabel(idx, "nvlink"):
		return entityNVLink
	case hasLabel(idx, "gpu_i_id") || hasLabel(idx, "gpu_instance_id"):
		return entityMIG
	case hasLabel(idx, "nvswitch"):
		return entityNVSwitch
	case hasLabel(idx, "cpucore"):
		return entityCPUCore
	case hasLabel(idx, "cpu"):
		return entityCPU
	case hasLabel(idx, "gpu") || hasLabel(idx, "uuid") || hasLabel(idx, "gpu_uuid"):
		return entityGPU
	default:
		return entityExporter
	}
}

func hasLabel(idx map[string]string, key string) bool {
	v, ok := idx[key]
	return ok && v != ""
}

func identityKeysForEntity(entity metricEntity) []string {
	workload := []string{"namespace", "pod", "container", "job", "hpc_job", "hpc_job_id"}
	switch entity {
	case entityGPU:
		return append([]string{"gpu", "uuid", "gpu_uuid"}, workload...)
	case entityMIG:
		return append([]string{"gpu", "uuid", "gpu_uuid", "gpu_i_id", "gpu_instance_id", "gpu_i_profile", "gpu_instance_profile"}, workload...)
	case entityNVLink:
		return append([]string{"nvswitch", "gpu", "uuid", "gpu_uuid", "gpu_i_id", "gpu_instance_id", "gpu_i_profile", "gpu_instance_profile", "nvlink"}, workload...)
	case entityNVSwitch:
		return append([]string{"nvswitch"}, workload...)
	case entityCPU:
		return append([]string{"cpu"}, workload...)
	case entityCPUCore:
		return append([]string{"cpu", "cpucore"}, workload...)
	default:
		return append([]string{"hostname"}, workload...)
	}
}

func normalizeLabelKey(s string) string {
	return sanitizeID(strings.ToLower(s))
}

func buildChartLabels(idx map[string]string) []collectorapi.Label {
	ignore := map[string]bool{
		"hostname": true, // host identity is already part of Netdata host model
		"err_code": true, // a value of the last-error field, not chart identity
		"err_msg":  true,
		"le":       true, // histogram bucket label
		"quantile": true, // summary label
		"__name__": true, // metric family name label, if present
	}

	keys := make([]string, 0, len(idx))
	for key, value := range idx {
		if value == "" || ignore[key] || isSeriesMetadataLabel(key) {
			continue
		}
		keys = append(keys, key)
	}

	sort.Strings(keys)
	labels := make([]collectorapi.Label, 0, len(keys))
	for _, key := range keys {
		labels = append(labels, collectorapi.Label{
			Key:   normalizeLabelKey(key),
			Value: idx[key],
		})
	}
	return labels
}

// Display normalization is lossy, so identity uses the framed original parts.
func makeID(parts ...string) string {
	h := fnv.New64a()
	var length [20]byte
	for _, part := range parts {
		_, _ = h.Write(strconv.AppendInt(length[:0], int64(len(part)), 10))
		_, _ = h.Write([]byte{':'})
		_, _ = h.Write([]byte(part))
	}
	id := sanitizeID(strings.Join(parts, "_"))
	if len(id) > 140 {
		id = id[:140]
	}
	return id + "_" + strconv.FormatUint(h.Sum64(), 36)
}

func sanitizeID(s string) string {
	if s == "" {
		return "unknown"
	}

	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false

	for _, r := range s {
		isAlphaNum := (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}

		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}

	id := strings.Trim(b.String(), "_")
	if id == "" {
		id = "unknown"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "n_" + id
	}
	return id
}

func isSeriesMetadataLabel(key string) bool {
	switch key {
	case "health_watch", "health_error_code", "health_error_severity", "health_error_category", "clock_event", "xid", "peer_gpu", "link_status":
		return true
	}
	return false
}
