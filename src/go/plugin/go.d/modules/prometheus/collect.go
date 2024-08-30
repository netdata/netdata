// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

const (
	precision = 1000
)

func (p *Prometheus) collect() (map[string]int64, error) {
	mfs, err := p.prom.Scrape()
	if err != nil {
		return nil, err
	}

	if mfs.Len() == 0 {
		p.Warningf("endpoint '%s' returned 0 metric families", p.URL)
		return nil, nil
	}

	// TODO: shouldn't modify the value from Config
	if p.ExpectedPrefix != "" {
		if !hasPrefix(mfs, p.ExpectedPrefix) {
			return nil, fmt.Errorf("'%s' metrics have no expected prefix (%s)", p.URL, p.ExpectedPrefix)
		}
		p.ExpectedPrefix = ""
	}

	// TODO: shouldn't modify the value from Config
	if p.MaxTS > 0 {
		if n := calcMetrics(mfs); n > p.MaxTS {
			return nil, fmt.Errorf("'%s' num of time series (%d) > limit (%d)", p.URL, n, p.MaxTS)
		}
		p.MaxTS = 0
	}

	mx := make(map[string]int64)

	p.resetCache()
	defer p.removeStaleCharts()

	for _, mf := range mfs {
		if strings.HasSuffix(mf.Name(), "_info") {
			continue
		}
		if p.MaxTSPerMetric > 0 && len(mf.Metrics()) > p.MaxTSPerMetric {
			p.Debugf("metric '%s' num of time series (%d) > limit (%d), skipping it",
				mf.Name(), len(mf.Metrics()), p.MaxTSPerMetric)
			continue
		}

		switch mf.Type() {
		case model.MetricTypeGauge:
			p.collectGauge(mx, mf)
		case model.MetricTypeCounter:
			p.collectCounter(mx, mf)
		case model.MetricTypeSummary:
			p.collectSummary(mx, mf)
		case model.MetricTypeHistogram:
			p.collectHistogram(mx, mf)
		case model.MetricTypeUnknown:
			p.collectUntyped(mx, mf)
		}
	}

	return mx, nil
}

func (p *Prometheus) collectGauge(mx map[string]int64, mf *prometheus.MetricFamily) {
	for _, m := range mf.Metrics() {
		if m.Gauge() == nil || math.IsNaN(m.Gauge().Value()) {
			continue
		}

		id := mf.Name() + p.joinLabels(m.Labels())

		if !p.cache.hasP(id) {
			p.addGaugeChart(id, mf.Name(), mf.Help(), m.Labels())
		}

		mx[id] = int64(m.Gauge().Value() * precision)
	}
}

func (p *Prometheus) collectCounter(mx map[string]int64, mf *prometheus.MetricFamily) {
	for _, m := range mf.Metrics() {
		if m.Counter() == nil || math.IsNaN(m.Counter().Value()) {
			continue
		}

		id := mf.Name() + p.joinLabels(m.Labels())

		if !p.cache.hasP(id) {
			p.addCounterChart(id, mf.Name(), mf.Help(), m.Labels())
		}

		mx[id] = int64(m.Counter().Value() * precision)
	}
}

func (p *Prometheus) collectSummary(mx map[string]int64, mf *prometheus.MetricFamily) {
	for _, m := range mf.Metrics() {
		if m.Summary() == nil || len(m.Summary().Quantiles()) == 0 {
			continue
		}

		id := mf.Name() + p.joinLabels(m.Labels())

		if !p.cache.hasP(id) {
			p.addSummaryCharts(id, mf.Name(), mf.Help(), m.Labels(), m.Summary().Quantiles())
		}

		for _, v := range m.Summary().Quantiles() {
			if !math.IsNaN(v.Value()) {
				dimID := fmt.Sprintf("%s_quantile=%s", id, formatFloat(v.Quantile()))
				mx[dimID] = int64(v.Value() * precision * precision)
			}
		}

		mx[id+"_sum"] = int64(m.Summary().Sum() * precision)
		mx[id+"_count"] = int64(m.Summary().Count())
	}
}

func (p *Prometheus) collectHistogram(mx map[string]int64, mf *prometheus.MetricFamily) {
	for _, m := range mf.Metrics() {
		if m.Histogram() == nil || len(m.Histogram().Buckets()) == 0 {
			continue
		}

		id := mf.Name() + p.joinLabels(m.Labels())

		if !p.cache.hasP(id) {
			p.addHistogramCharts(id, mf.Name(), mf.Help(), m.Labels(), m.Histogram().Buckets())
		}

		for _, v := range m.Histogram().Buckets() {
			if !math.IsNaN(v.CumulativeCount()) {
				dimID := fmt.Sprintf("%s_bucket=%s", id, formatFloat(v.UpperBound()))
				mx[dimID] = int64(v.CumulativeCount())
			}
		}

		mx[id+"_sum"] = int64(m.Histogram().Sum() * precision)
		mx[id+"_count"] = int64(m.Histogram().Count())
	}
}

func (p *Prometheus) collectUntyped(mx map[string]int64, mf *prometheus.MetricFamily) {
	for _, m := range mf.Metrics() {
		if m.Untyped() == nil || math.IsNaN(m.Untyped().Value()) {
			continue
		}

		if p.isFallbackTypeGauge(mf.Name()) {
			id := mf.Name() + p.joinLabels(m.Labels())

			if !p.cache.hasP(id) {
				p.addGaugeChart(id, mf.Name(), mf.Help(), m.Labels())
			}

			mx[id] = int64(m.Untyped().Value() * precision)
		}

		if p.isFallbackTypeCounter(mf.Name()) || strings.HasSuffix(mf.Name(), "_total") {
			id := mf.Name() + p.joinLabels(m.Labels())

			if !p.cache.hasP(id) {
				p.addCounterChart(id, mf.Name(), mf.Help(), m.Labels())
			}

			mx[id] = int64(m.Untyped().Value() * precision)
		}
	}
}

func (p *Prometheus) isFallbackTypeGauge(name string) bool {
	return p.fallbackType.gauge != nil && p.fallbackType.gauge.MatchString(name)
}

func (p *Prometheus) isFallbackTypeCounter(name string) bool {
	return p.fallbackType.counter != nil && p.fallbackType.counter.MatchString(name)
}

func (p *Prometheus) joinLabels(labels labels.Labels) string {
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

func (p *Prometheus) resetCache() {
	for _, v := range p.cache.entries {
		v.seen = false
	}
}

const maxNotSeenTimes = 10

func (p *Prometheus) removeStaleCharts() {
	for k, v := range p.cache.entries {
		if v.seen {
			continue
		}
		if v.notSeenTimes++; v.notSeenTimes >= maxNotSeenTimes {
			for _, chart := range v.charts {
				chart.MarkRemove()
				chart.MarkNotCreated()
			}
			delete(p.cache.entries, k)
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
