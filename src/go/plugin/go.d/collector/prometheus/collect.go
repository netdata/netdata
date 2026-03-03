// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	precision = 1000
)

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

	if mfs.Len() == 0 {
		c.Warningf("endpoint '%s' returned 0 metric families", c.URL)
		return nil, nil
	}

	if !c.expectedPrefixValidated && c.ExpectedPrefix != "" {
		if !hasPrefix(mfs, c.ExpectedPrefix) {
			return nil, fmt.Errorf("'%s' metrics have no expected prefix (%s)", c.URL, c.ExpectedPrefix)
		}
		c.expectedPrefixValidated = true
	}

	if !c.maxTSValidated && c.MaxTS > 0 {
		if n := calcMetrics(mfs); n > c.MaxTS {
			return nil, fmt.Errorf("'%s' num of time series (%d) > limit (%d)", c.URL, n, c.MaxTS)
		}
		c.maxTSValidated = true
	}

	mx := make(map[string]int64)
	seenDims := make(map[string]map[string]bool)

	c.resetCache()
	defer func() {
		c.removeStaleDimensions(seenDims)
		c.removeStaleCharts()
	}()

	if len(c.selectorGroups) == 0 {
		c.collectMetricFamilies(mx, mfs, seenDims)
		return mx, nil
	}

	filteredByGroup := make([]prometheus.MetricFamilies, 0, len(c.selectorGroups))
	for _, g := range c.selectorGroups {
		filtered := filterMetricFamilies(mfs, g.selector)
		filteredByGroup = append(filteredByGroup, filtered)
	}

	prevLR, prevCR, prevDR, prevPrefix := c.labelRelabelRules, c.contextRules, c.dimensionRules, c.chartIDPrefix
	defer func() {
		c.labelRelabelRules, c.contextRules, c.dimensionRules, c.chartIDPrefix = prevLR, prevCR, prevDR, prevPrefix
	}()

	for i, g := range c.selectorGroups {
		c.labelRelabelRules = g.labelRelabel
		c.contextRules = g.contextRules
		c.dimensionRules = g.dimensionRules
		c.chartIDPrefix = g.chartIDPrefix
		c.collectMetricFamilies(mx, filteredByGroup[i], seenDims)
	}

	return mx, nil
}

func (c *Collector) collectMetricFamilies(mx map[string]int64, mfs prometheus.MetricFamilies, seenDims map[string]map[string]bool) {
	for _, mf := range mfs {
		if strings.HasSuffix(mf.Name(), "_info") {
			continue
		}
		if c.MaxTSPerMetric > 0 && len(mf.Metrics()) > c.MaxTSPerMetric {
			c.Debugf("metric '%s' num of time series (%d) > limit (%d), skipping it",
				mf.Name(), len(mf.Metrics()), c.MaxTSPerMetric)
			continue
		}

		switch mf.Type() {
		case model.MetricTypeGauge:
			c.collectGauge(mx, mf, seenDims)
		case model.MetricTypeCounter:
			c.collectCounter(mx, mf, seenDims)
		case model.MetricTypeSummary:
			c.collectSummary(mx, mf, seenDims)
		case model.MetricTypeHistogram:
			c.collectHistogram(mx, mf, seenDims)
		case model.MetricTypeUnknown:
			c.collectUntyped(mx, mf)
		}
	}
}

func (c *Collector) collectGauge(mx map[string]int64, mf *prometheus.MetricFamily, seenDims map[string]map[string]bool) {
	c.collectScalar(
		mx,
		mf,
		seenDims,
		func(m prometheus.Metric) (float64, bool) {
			if m.Gauge() == nil || math.IsNaN(m.Gauge().Value()) {
				return 0, false
			}
			return m.Gauge().Value(), true
		},
		c.addGaugeChart,
		c.addGaugeChartWithDim,
		collectorapi.Absolute,
	)
}

func (c *Collector) collectCounter(mx map[string]int64, mf *prometheus.MetricFamily, seenDims map[string]map[string]bool) {
	c.collectScalar(
		mx,
		mf,
		seenDims,
		func(m prometheus.Metric) (float64, bool) {
			if m.Counter() == nil || math.IsNaN(m.Counter().Value()) {
				return 0, false
			}
			return m.Counter().Value(), true
		},
		c.addCounterChart,
		c.addCounterChartWithDim,
		collectorapi.Incremental,
	)
}

func (c *Collector) collectScalar(
	mx map[string]int64,
	mf *prometheus.MetricFamily,
	seenDims map[string]map[string]bool,
	sampleValue func(prometheus.Metric) (float64, bool),
	addChart func(id, name, help string, labels labels.Labels),
	addChartWithDim func(id, name, help string, labels labels.Labels, dimID, dimName string),
	dimAlgo collectorapi.DimAlgo,
) {
	rule := c.matchDimensionRule(mf.Name())
	warnedMissingLabel := false
	metricIDBase := c.metricIDBase(mf.Name())

	for _, m := range mf.Metrics() {
		value, ok := sampleValue(m)
		if !ok {
			continue
		}

		lbls := c.applyRelabel(m.Labels())

		if rule == nil {
			id := metricIDBase + c.joinLabels(lbls)
			if !c.cache.hasP(id) {
				addChart(id, mf.Name(), mf.Help(), lbls)
			}
			mx[id] = int64(value * precision)
			continue
		}

		dimName, consumed, ok := rule.render(lbls)
		if !ok {
			c.warnMissingTemplateLabel(mf.Name(), rule.template, &warnedMissingLabel)
			continue
		}
		instanceLabels := splitInstanceLabels(lbls, consumed)
		chartID := metricIDBase + c.joinLabels(instanceLabels)
		dimID := dimensionMetricID(chartID, dimName)

		if !c.cache.hasP(chartID) {
			addChartWithDim(chartID, mf.Name(), mf.Help(), instanceLabels, dimID, dimName)
		} else if chart := c.cache.getFirstChart(chartID); chart != nil && !chart.HasDim(dimID) {
			_ = chart.AddDim(&collectorapi.Dim{ID: dimID, Name: dimName, Algo: dimAlgo, Div: precision})
			chart.MarkNotCreated()
		}

		trackSeenDim(seenDims, chartID, dimID)
		mx[dimID] = int64(value * precision)
	}
}

func (c *Collector) collectSummary(mx map[string]int64, mf *prometheus.MetricFamily, seenDims map[string]map[string]bool) {
	rule := c.matchDimensionRule(mf.Name())
	warnedMissingLabel := false
	metricIDBase := c.metricIDBase(mf.Name())

	for _, m := range mf.Metrics() {
		if m.Summary() == nil || len(m.Summary().Quantiles()) == 0 {
			continue
		}

		lbls := c.applyRelabel(m.Labels())

		if rule == nil {
			id := metricIDBase + c.joinLabels(lbls)

			if !c.cache.hasP(id) {
				c.addSummaryCharts(id, mf.Name(), mf.Help(), lbls, m.Summary().Quantiles())
			}

			for _, v := range m.Summary().Quantiles() {
				if !math.IsNaN(v.Value()) {
					dimID := fmt.Sprintf("%s_quantile=%s", id, formatFloat(v.Quantile()))
					mx[dimID] = int64(v.Value() * precision * precision)
				}
			}

			mx[id+"_sum"] = int64(m.Summary().Sum() * precision)
			mx[id+"_count"] = int64(m.Summary().Count())
			continue
		}

		dimName, consumed, ok := rule.render(lbls)
		if !ok {
			c.warnMissingTemplateLabel(mf.Name(), rule.template, &warnedMissingLabel)
			continue
		}
		instanceLabels := splitInstanceLabels(lbls, consumed)
		chartID := metricIDBase + c.joinLabels(instanceLabels)

		if !c.cache.hasP(chartID) {
			c.addSummaryCharts(chartID, mf.Name(), mf.Help(), instanceLabels, m.Summary().Quantiles())
		}

		for _, v := range m.Summary().Quantiles() {
			if !math.IsNaN(v.Value()) {
				dimID := fmt.Sprintf("%s_quantile=%s-dim=%s", chartID, formatFloat(v.Quantile()), sanitizeMetricIDPart(dimName))
				ensureChartDim(c.Charts().Get(chartID), dimID, dimName+"_quantile_"+formatFloat(v.Quantile()), collectorapi.Absolute, precision*precision)
				trackSeenDim(seenDims, chartID, dimID)
				mx[dimID] = int64(v.Value() * precision * precision)
			}
		}

		sumChartID := chartID + "_sum"
		countChartID := chartID + "_count"
		sumDimID := fmt.Sprintf("%s-dim=%s", sumChartID, sanitizeMetricIDPart(dimName))
		countDimID := fmt.Sprintf("%s-dim=%s", countChartID, sanitizeMetricIDPart(dimName))

		ensureChartDim(c.Charts().Get(sumChartID), sumDimID, dimName, collectorapi.Incremental, precision)
		ensureChartDim(c.Charts().Get(countChartID), countDimID, dimName, collectorapi.Incremental, 1)
		trackSeenDim(seenDims, sumChartID, sumDimID)
		trackSeenDim(seenDims, countChartID, countDimID)

		mx[sumDimID] = int64(m.Summary().Sum() * precision)
		mx[countDimID] = int64(m.Summary().Count())
	}
}

func (c *Collector) collectHistogram(mx map[string]int64, mf *prometheus.MetricFamily, seenDims map[string]map[string]bool) {
	rule := c.matchDimensionRule(mf.Name())
	warnedMissingLabel := false
	metricIDBase := c.metricIDBase(mf.Name())

	for _, m := range mf.Metrics() {
		if m.Histogram() == nil || len(m.Histogram().Buckets()) == 0 {
			continue
		}

		lbls := c.applyRelabel(m.Labels())

		if rule == nil {
			id := metricIDBase + c.joinLabels(lbls)

			if !c.cache.hasP(id) {
				c.addHistogramCharts(id, mf.Name(), mf.Help(), lbls, m.Histogram().Buckets())
			}

			for _, v := range m.Histogram().Buckets() {
				if !math.IsNaN(v.CumulativeCount()) {
					dimID := fmt.Sprintf("%s_bucket=%s", id, formatFloat(v.UpperBound()))
					mx[dimID] = int64(v.CumulativeCount())
				}
			}

			mx[id+"_sum"] = int64(m.Histogram().Sum() * precision)
			mx[id+"_count"] = int64(m.Histogram().Count())
			continue
		}

		dimName, consumed, ok := rule.render(lbls)
		if !ok {
			c.warnMissingTemplateLabel(mf.Name(), rule.template, &warnedMissingLabel)
			continue
		}
		instanceLabels := splitInstanceLabels(lbls, consumed)
		chartID := metricIDBase + c.joinLabels(instanceLabels)

		if !c.cache.hasP(chartID) {
			c.addHistogramCharts(chartID, mf.Name(), mf.Help(), instanceLabels, m.Histogram().Buckets())
		}

		for _, v := range m.Histogram().Buckets() {
			if !math.IsNaN(v.CumulativeCount()) {
				dimID := fmt.Sprintf("%s_bucket=%s-dim=%s", chartID, formatFloat(v.UpperBound()), sanitizeMetricIDPart(dimName))
				ensureChartDim(c.Charts().Get(chartID), dimID, dimName+"_bucket_"+formatFloat(v.UpperBound()), collectorapi.Incremental, 1)
				trackSeenDim(seenDims, chartID, dimID)
				mx[dimID] = int64(v.CumulativeCount())
			}
		}

		sumChartID := chartID + "_sum"
		countChartID := chartID + "_count"
		sumDimID := fmt.Sprintf("%s-dim=%s", sumChartID, sanitizeMetricIDPart(dimName))
		countDimID := fmt.Sprintf("%s-dim=%s", countChartID, sanitizeMetricIDPart(dimName))

		ensureChartDim(c.Charts().Get(sumChartID), sumDimID, dimName, collectorapi.Incremental, precision)
		ensureChartDim(c.Charts().Get(countChartID), countDimID, dimName, collectorapi.Incremental, 1)
		trackSeenDim(seenDims, sumChartID, sumDimID)
		trackSeenDim(seenDims, countChartID, countDimID)

		mx[sumDimID] = int64(m.Histogram().Sum() * precision)
		mx[countDimID] = int64(m.Histogram().Count())
	}
}

func (c *Collector) warnMissingTemplateLabel(metricName, template string, warned *bool) {
	if *warned {
		return
	}
	*warned = true
	c.Warningf("metric '%s': dimension template %q references labels missing from a time series, skipping affected series", metricName, template)
}

func (c *Collector) collectUntyped(mx map[string]int64, mf *prometheus.MetricFamily) {
	metricIDBase := c.metricIDBase(mf.Name())
	for _, m := range mf.Metrics() {
		if m.Untyped() == nil || math.IsNaN(m.Untyped().Value()) {
			continue
		}

		if c.isFallbackTypeGauge(mf.Name()) {
			lbls := c.applyRelabel(m.Labels())
			id := metricIDBase + c.joinLabels(lbls)

			if !c.cache.hasP(id) {
				c.addGaugeChart(id, mf.Name(), mf.Help(), lbls)
			}

			mx[id] = int64(m.Untyped().Value() * precision)
		}

		if c.isFallbackTypeCounter(mf.Name()) || strings.HasSuffix(mf.Name(), "_total") {
			lbls := c.applyRelabel(m.Labels())
			id := metricIDBase + c.joinLabels(lbls)

			if !c.cache.hasP(id) {
				c.addCounterChart(id, mf.Name(), mf.Help(), lbls)
			}

			mx[id] = int64(m.Untyped().Value() * precision)
		}
	}
}

func (c *Collector) metricIDBase(metricName string) string {
	if c.chartIDPrefix == "" {
		return metricName
	}
	return c.chartIDPrefix + "_" + metricName
}

func filterMetricFamilies(mfs prometheus.MetricFamilies, sr selector.Selector) prometheus.MetricFamilies {
	if sr == nil {
		return mfs
	}

	out := make(prometheus.MetricFamilies)
	for name, mf := range mfs {
		filtered := filterMetricFamily(mf, name, sr)
		if filtered != nil && len(filtered.Metrics()) > 0 {
			out[name] = filtered
		}
	}
	return out
}

func filterMetricFamily(mf *prometheus.MetricFamily, metricName string, sr selector.Selector) *prometheus.MetricFamily {
	metrics := make([]prometheus.Metric, 0, len(mf.Metrics()))
	for _, m := range mf.Metrics() {
		lbls := labels.Labels{{Name: labels.MetricName, Value: metricName}}
		lbls = append(lbls, m.Labels()...)
		if !sr.Matches(lbls) {
			continue
		}
		metrics = append(metrics, m)
	}
	if len(metrics) == 0 {
		return nil
	}
	return prometheus.NewMetricFamily(mf.Name(), mf.Help(), mf.Type(), metrics)
}

func dimensionMetricID(chartID, dimName string) string {
	return chartID + "-dim=" + sanitizeMetricIDPart(dimName)
}

func sanitizeMetricIDPart(v string) string {
	if strings.IndexByte(v, ' ') != -1 {
		v = spaceReplacer.Replace(v)
	}
	if strings.IndexByte(v, '\\') != -1 {
		v = backslashReplacer.Replace(v)
	}
	if strings.IndexByte(v, '\'') != -1 {
		v = apostropheReplacer.Replace(v)
	}
	if v == "" {
		return "value"
	}
	return v
}

func ensureChartDim(chart *collectorapi.Chart, dimID, dimName string, algo collectorapi.DimAlgo, div int) {
	if chart == nil || chart.HasDim(dimID) {
		return
	}
	dim := &collectorapi.Dim{
		ID:   dimID,
		Name: dimName,
		Algo: algo,
		Div:  div,
	}
	if err := chart.AddDim(dim); err == nil {
		chart.MarkNotCreated()
	}
}

func trackSeenDim(seenDims map[string]map[string]bool, chartID, dimID string) {
	seen, ok := seenDims[chartID]
	if !ok {
		seen = make(map[string]bool)
		seenDims[chartID] = seen
	}
	seen[dimID] = true
}

func (c *Collector) removeStaleDimensions(seenDims map[string]map[string]bool) {
	for chartID, seen := range seenDims {
		chart := c.Charts().Get(chartID)
		if chart == nil {
			continue
		}
		for _, dim := range chart.Dims {
			if seen[dim.ID] {
				continue
			}
			_ = chart.MarkDimRemove(dim.ID, false)
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) isFallbackTypeGauge(name string) bool {
	return c.fallbackType.gauge != nil && c.fallbackType.gauge.MatchString(name)
}

func (c *Collector) isFallbackTypeCounter(name string) bool {
	return c.fallbackType.counter != nil && c.fallbackType.counter.MatchString(name)
}

func (c *Collector) joinLabels(labels labels.Labels) string {
	var sb strings.Builder
	for _, lbl := range labels {
		name, val := lbl.Name, lbl.Value
		if name == "" || val == "" {
			continue
		}

		if strings.IndexByte(val, ' ') != -1 {
			val = spaceReplacer.Replace(val)
		}
		if strings.IndexByte(val, '\\') != -1 {
			if val = decodeLabelValue(val); strings.IndexByte(val, '\\') != -1 {
				val = backslashReplacer.Replace(val)
			}
		}
		if strings.IndexByte(val, '\'') != -1 {
			val = apostropheReplacer.Replace(val)
		}

		sb.WriteString("-" + name + "=" + val)
	}
	return sb.String()
}

func (c *Collector) resetCache() {
	for _, v := range c.cache.entries {
		v.seen = false
	}
}

const maxNotSeenTimes = 10

func (c *Collector) removeStaleCharts() {
	for k, v := range c.cache.entries {
		if v.seen {
			continue
		}
		if v.notSeenTimes++; v.notSeenTimes >= maxNotSeenTimes {
			for _, chart := range v.charts {
				chart.MarkRemove()
				chart.MarkNotCreated()
			}
			delete(c.cache.entries, k)
		}
	}
}

func decodeLabelValue(value string) string {
	v, err := strconv.Unquote("\"" + value + "\"")
	if err != nil {
		return value
	}
	return v
}

var (
	spaceReplacer      = strings.NewReplacer(" ", "_")
	backslashReplacer  = strings.NewReplacer(`\`, "_")
	apostropheReplacer = strings.NewReplacer("'", "")
)

func hasPrefix(mf map[string]*prometheus.MetricFamily, prefix string) bool {
	for name := range mf {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func calcMetrics(mfs prometheus.MetricFamilies) int {
	var n int
	for _, mf := range mfs {
		n += len(mf.Metrics())
	}
	return n
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
