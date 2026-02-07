// SPDX-License-Identifier: GPL-3.0-or-later

package dcgm

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const precision = 1000.0

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

	if mfs.Len() == 0 {
		c.Warningf("endpoint '%s' returned 0 metric families", c.URL)
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

	mx := make(map[string]int64)
	c.cache.reset()

	for _, mf := range mfs {
		if !isDCGMMetricName(mf.Name()) {
			continue
		}

		if c.MaxTSPerMetric > 0 && len(mf.Metrics()) > c.MaxTSPerMetric {
			c.Debugf(
				"metric '%s' num of time series (%d) > limit (%d), skipping it",
				mf.Name(),
				len(mf.Metrics()),
				c.MaxTSPerMetric,
			)
			continue
		}

		typ := metricFamilyKind(mf)
		if typ == sampleUnsupported {
			c.Debugf("metric '%s' has unsupported Prometheus type '%s', skipping it", mf.Name(), mf.Type())
			continue
		}
		for _, metric := range mf.Metrics() {
			value, ok := metricValue(metric, typ)
			if !ok || isInvalidMetricValue(value) {
				continue
			}

			instance := resolveEntityInstance(metric.Labels())
			spec := classifyMetric(instance.entity, mf.Name(), mf.Help(), typ)

			chartKey, chart := c.ensureChart(instance, spec.Context)
			dimID := c.ensureDim(chartKey, chart, spec, metric.Labels(), typ)

			mx[dimID] += int64(value * spec.Scale * precision)
		}
	}

	c.removeStaleChartsAndDims()

	if len(mx) == 0 {
		return nil, nil
	}

	return mx, nil
}

func (c *Collector) ensureChart(instance entityInstance, spec contextSpec) (string, *module.Chart) {
	chartKey := spec.ID + "|" + instance.key
	if ch, ok := c.cache.getChart(chartKey); ok {
		return chartKey, ch.chart
	}

	chart := &module.Chart{
		ID:       makeID(spec.ID, instance.key),
		Title:    spec.Title,
		Units:    spec.Units,
		Fam:      spec.Family,
		Ctx:      spec.ID,
		Type:     spec.Type,
		Priority: spec.Priority,
		Labels:   append([]module.Label(nil), instance.chartLabels...),
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}

	ch := c.cache.putChart(chartKey, chart)
	return chartKey, ch.chart
}

func (c *Collector) ensureDim(
	chartKey string,
	chart *module.Chart,
	spec metricSpec,
	lbls promlabels.Labels,
	typ sampleKind,
) string {
	extra := ""
	if !strings.HasSuffix(spec.Context.ID, ".reliability.xid") {
		extra = semanticDimSuffix(lbls)
	}
	dimName := spec.DimName
	if extra != "" {
		dimName = dimName + "_" + extra
	}

	dimID := makeID(chart.ID, dimName)

	ch, ok := c.cache.charts[chartKey]
	if !ok {
		return dimID
	}

	if exists := ch.touchDim(dimID); !exists {
		dim := &module.Dim{ID: dimID, Name: dimName, Div: int(precision)}
		switch typ {
		case sampleCounter:
			dim.Algo = module.Incremental
		default:
			dim.Algo = module.Absolute
		}

		if err := chart.AddDim(dim); err != nil {
			c.Warning(err)
		} else {
			chart.MarkNotCreated()
		}
	}

	return dimID
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
	chartLabels []module.Label
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
		parts = append(parts, key+"="+v)
	}

	if len(parts) == 0 {
		parts = append(parts, "global")
	}

	stableLabelKeys := []string{
		"gpu",
		"uuid",
		"gpu_uuid",
		"gpu_i_id",
		"gpu_instance_id",
		"gpu_i_profile",
		"gpu_instance_profile",
		"device",
		"modelname",
		"model_name",
		"pci_bus_id",
		"nvswitch",
		"nvlink",
		"cpu",
		"cpucore",
		"namespace",
		"pod",
		"container",
		"job",
		"hpc_job",
		"hpc_job_id",
	}

	chartLabels := make([]module.Label, 0, len(stableLabelKeys))
	for _, key := range stableLabelKeys {
		if v, ok := idx[key]; ok && v != "" {
			chartLabels = append(chartLabels, module.Label{Key: normalizeLabelKey(key), Value: v})
		}
	}

	return entityInstance{
		entity:      entity,
		key:         strings.Join(parts, "|"),
		chartLabels: chartLabels,
	}
}

func detectEntity(idx map[string]string) metricEntity {
	switch {
	case hasLabel(idx, "gpu_i_id") || hasLabel(idx, "gpu_instance_id"):
		return entityMIG
	case hasLabel(idx, "nvlink"):
		return entityNVLink
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
		return append([]string{"nvswitch", "gpu", "gpu_uuid", "nvlink"}, workload...)
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

func semanticDimSuffix(lbls promlabels.Labels) string {
	if len(lbls) == 0 {
		return ""
	}

	// Only keep dynamic, semantically meaningful labels that define distinct series
	// for a single DCGM field; avoid static identity/metadata labels in dim names.
	allowed := map[string]bool{
		"err_code": true,
	}

	tokens := make([]string, 0, len(lbls))
	for _, lbl := range lbls {
		k := strings.ToLower(lbl.Name)
		if lbl.Value == "" || !allowed[k] {
			continue
		}
		tokens = append(tokens, normalizeLabelKey(k)+"_"+sanitizeID(strings.ToLower(lbl.Value)))
	}

	if len(tokens) == 0 {
		return ""
	}

	sort.Strings(tokens)
	return strings.Join(tokens, "__")
}

func normalizeLabelKey(s string) string {
	s = strings.ToLower(s)
	return sanitizeID(s)
}

func makeID(parts ...string) string {
	raw := strings.Join(parts, "_")
	id := sanitizeID(raw)
	if len(id) <= 180 {
		return id
	}

	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	checksum := strconv.FormatUint(h.Sum64(), 36)
	return id[:140] + "_" + checksum
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
