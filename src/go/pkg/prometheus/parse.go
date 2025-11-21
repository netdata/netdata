package prometheus

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

const (
	quantileLabel = "quantile"
	bucketLabel   = "le"
)

const (
	countSuffix  = "_count"
	sumSuffix    = "_sum"
	bucketSuffix = "_bucket"
)

type promTextParser struct {
	metrics MetricFamilies
	series  Series

	sr selector.Selector

	currMF     *MetricFamily
	currSeries labels.Labels

	summaries  map[uint64]*Summary
	histograms map[uint64]*Histogram

	isCount    bool
	isSum      bool
	isQuantile bool
	isBucket   bool

	currQuantile float64
	currBucket   float64
}

func (p *promTextParser) parseToSeries(text []byte) (Series, error) {
	p.series.Reset()

	parser := textparse.NewPromParser(text, labels.NewSymbolTable())
	for {
		entry, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if entry == textparse.EntryInvalid && strings.HasPrefix(err.Error(), "invalid metric type") {
				continue
			}
			return nil, fmt.Errorf("failed to parse prometheus metrics: %v", err)
		}

		switch entry {
		case textparse.EntrySeries:
			p.currSeries = p.currSeries[:0]

			parser.Metric(&p.currSeries)

			if p.sr != nil && !p.sr.Matches(p.currSeries) {
				continue
			}

			_, _, val := parser.Series()
			p.series.Add(SeriesSample{Labels: copyLabels(p.currSeries), Value: val})
		}
	}

	p.series.Sort()

	return p.series, nil
}

var reSpace = regexp.MustCompile(`\s+`)

func (p *promTextParser) parseToMetricFamilies(text []byte) (MetricFamilies, error) {
	p.reset()

	parser := textparse.NewPromParser(text, labels.NewSymbolTable())
	for {
		entry, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if entry == textparse.EntryInvalid && strings.HasPrefix(err.Error(), "invalid metric type") {
				continue
			}
			return nil, fmt.Errorf("failed to parse prometheus metrics: %v", err)
		}

		switch entry {
		case textparse.EntryHelp:
			name, help := parser.Help()
			p.setMetricFamilyByName(string(name))
			p.currMF.help = string(help)
			if strings.IndexByte(p.currMF.help, '\n') != -1 {
				// convert multiline to one line because HELP is used as the chart title.
				p.currMF.help = reSpace.ReplaceAllString(strings.TrimSpace(p.currMF.help), " ")
			}
		case textparse.EntryType:
			name, typ := parser.Type()
			p.setMetricFamilyByName(string(name))
			p.currMF.typ = typ
		case textparse.EntrySeries:
			p.currSeries = p.currSeries[:0]

			parser.Metric(&p.currSeries)

			if p.sr != nil && !p.sr.Matches(p.currSeries) {
				continue
			}

			p.setMetricFamilyBySeries()

			_, _, value := parser.Series()

			switch p.currMF.typ {
			case model.MetricTypeGauge:
				p.addGauge(value)
			case model.MetricTypeCounter:
				p.addCounter(value)
			case model.MetricTypeSummary:
				p.addSummary(value)
			case model.MetricTypeHistogram:
				p.addHistogram(value)
			case model.MetricTypeUnknown:
				p.addUnknown(value)
			}
		}
	}

	for k, v := range p.metrics {
		if len(v.Metrics()) == 0 {
			delete(p.metrics, k)
		}
	}

	return p.metrics, nil
}

func (p *promTextParser) setMetricFamilyByName(name string) {
	mf, ok := p.metrics[name]
	if !ok {
		mf = &MetricFamily{name: name, typ: model.MetricTypeUnknown}
		p.metrics[name] = mf
	}
	p.currMF = mf
}

func (p *promTextParser) setMetricFamilyBySeries() {
	p.isSum, p.isCount, p.isQuantile, p.isBucket = false, false, false, false
	p.currQuantile, p.currBucket = 0, 0

	name := p.currSeries[0].Value

	if p.currMF != nil && p.currMF.name == name {
		if p.currMF.typ == model.MetricTypeSummary {
			p.setQuantile()
		}
		return
	}

	typ := model.MetricTypeUnknown

	switch {
	case strings.HasSuffix(name, sumSuffix):
		n := strings.TrimSuffix(name, sumSuffix)
		if mf, ok := p.metrics[n]; ok && isSummaryOrHistogram(mf.typ) {
			p.isSum = true
			p.currSeries[0].Value = n
			p.currMF = mf
			return
		}
	case strings.HasSuffix(name, countSuffix):
		n := strings.TrimSuffix(name, countSuffix)
		if mf, ok := p.metrics[n]; ok && isSummaryOrHistogram(mf.typ) {
			p.isCount = true
			p.currSeries[0].Value = n
			p.currMF = mf
			return
		}
	case strings.HasSuffix(name, bucketSuffix):
		n := strings.TrimSuffix(name, bucketSuffix)
		if mf, ok := p.metrics[n]; ok && isSummaryOrHistogram(mf.typ) {
			p.currSeries[0].Value = n
			p.setBucket()
			p.currMF = mf
			return
		}
		if p.currSeries.Has(bucketLabel) {
			p.currSeries[0].Value = n
			p.setBucket()
			name = n
			typ = model.MetricTypeHistogram
		}
	case p.currSeries.Has(quantileLabel):
		typ = model.MetricTypeSummary
		p.setQuantile()
	}

	p.setMetricFamilyByName(name)
	if p.currMF.typ == "" || p.currMF.typ == model.MetricTypeUnknown {
		p.currMF.typ = typ
	}
}

func (p *promTextParser) setQuantile() {
	if lbs, v, ok := removeLabel(p.currSeries, quantileLabel); ok {
		p.isQuantile = true
		p.currSeries = lbs
		p.currQuantile, _ = strconv.ParseFloat(v, 64)
	}
}

func (p *promTextParser) setBucket() {
	if lbs, v, ok := removeLabel(p.currSeries, bucketLabel); ok {
		p.isBucket = true
		p.currSeries = lbs
		p.currBucket, _ = strconv.ParseFloat(v, 64)
	}
}

func (p *promTextParser) addGauge(value float64) {
	p.currSeries = p.currSeries[1:] // remove "__name__"

	if v := len(p.currMF.metrics); v == cap(p.currMF.metrics) {
		p.currMF.metrics = append(p.currMF.metrics, Metric{
			labels: copyLabels(p.currSeries),
			gauge:  &Gauge{value: value},
		})
	} else {
		p.currMF.metrics = p.currMF.metrics[:v+1]
		if p.currMF.metrics[v].gauge == nil {
			p.currMF.metrics[v].gauge = &Gauge{}
		}
		p.currMF.metrics[v].gauge.value = value
		p.currMF.metrics[v].labels = p.currMF.metrics[v].labels[:0]
		p.currMF.metrics[v].labels = append(p.currMF.metrics[v].labels, p.currSeries...)
	}
}

func (p *promTextParser) addCounter(value float64) {
	p.currSeries = p.currSeries[1:] // remove "__name__"

	if v := len(p.currMF.metrics); v == cap(p.currMF.metrics) {
		p.currMF.metrics = append(p.currMF.metrics, Metric{
			labels:  copyLabels(p.currSeries),
			counter: &Counter{value: value},
		})
	} else {
		p.currMF.metrics = p.currMF.metrics[:v+1]
		if p.currMF.metrics[v].counter == nil {
			p.currMF.metrics[v].counter = &Counter{}
		}
		p.currMF.metrics[v].counter.value = value
		p.currMF.metrics[v].labels = p.currMF.metrics[v].labels[:0]
		p.currMF.metrics[v].labels = append(p.currMF.metrics[v].labels, p.currSeries...)
	}
}

func (p *promTextParser) addUnknown(value float64) {
	p.currSeries = p.currSeries[1:] // remove "__name__"

	if v := len(p.currMF.metrics); v == cap(p.currMF.metrics) {
		p.currMF.metrics = append(p.currMF.metrics, Metric{
			labels:  copyLabels(p.currSeries),
			untyped: &Untyped{value: value},
		})
	} else {
		p.currMF.metrics = p.currMF.metrics[:v+1]
		if p.currMF.metrics[v].untyped == nil {
			p.currMF.metrics[v].untyped = &Untyped{}
		}
		p.currMF.metrics[v].untyped.value = value
		p.currMF.metrics[v].labels = p.currMF.metrics[v].labels[:0]
		p.currMF.metrics[v].labels = append(p.currMF.metrics[v].labels, p.currSeries...)
	}
}

func (p *promTextParser) addSummary(value float64) {
	hash := p.currSeries.Hash()

	p.currSeries = p.currSeries[1:] // remove "__name__"

	s, ok := p.summaries[hash]
	if !ok {
		if v := len(p.currMF.metrics); v == cap(p.currMF.metrics) {
			s = &Summary{}
			p.currMF.metrics = append(p.currMF.metrics, Metric{
				labels:  copyLabels(p.currSeries),
				summary: s,
			})
		} else {
			p.currMF.metrics = p.currMF.metrics[:v+1]
			if p.currMF.metrics[v].summary == nil {
				p.currMF.metrics[v].summary = &Summary{}
			}
			p.currMF.metrics[v].summary.sum = 0
			p.currMF.metrics[v].summary.count = 0
			p.currMF.metrics[v].summary.quantiles = p.currMF.metrics[v].summary.quantiles[:0]
			p.currMF.metrics[v].labels = p.currMF.metrics[v].labels[:0]
			p.currMF.metrics[v].labels = append(p.currMF.metrics[v].labels, p.currSeries...)
			s = p.currMF.metrics[v].summary
		}

		p.summaries[hash] = s
	}

	switch {
	case p.isQuantile:
		s.quantiles = append(s.quantiles, Quantile{quantile: p.currQuantile, value: value})
	case p.isSum:
		s.sum = value
	case p.isCount:
		s.count = value
	}
}

func (p *promTextParser) addHistogram(value float64) {
	hash := p.currSeries.Hash()

	p.currSeries = p.currSeries[1:] // remove "__name__"

	h, ok := p.histograms[hash]
	if !ok {
		if v := len(p.currMF.metrics); v == cap(p.currMF.metrics) {
			h = &Histogram{}
			p.currMF.metrics = append(p.currMF.metrics, Metric{
				labels:    copyLabels(p.currSeries),
				histogram: h,
			})
		} else {
			p.currMF.metrics = p.currMF.metrics[:v+1]
			if p.currMF.metrics[v].histogram == nil {
				p.currMF.metrics[v].histogram = &Histogram{}
			}
			p.currMF.metrics[v].histogram.sum = 0
			p.currMF.metrics[v].histogram.count = 0
			p.currMF.metrics[v].histogram.buckets = p.currMF.metrics[v].histogram.buckets[:0]
			p.currMF.metrics[v].labels = p.currMF.metrics[v].labels[:0]
			p.currMF.metrics[v].labels = append(p.currMF.metrics[v].labels, p.currSeries...)
			h = p.currMF.metrics[v].histogram
		}

		p.histograms[hash] = h
	}

	switch {
	case p.isBucket:
		h.buckets = append(h.buckets, Bucket{upperBound: p.currBucket, cumulativeCount: value})
	case p.isSum:
		h.sum = value
	case p.isCount:
		h.count = value
	}
}

func (p *promTextParser) reset() {
	p.currMF = nil
	p.currSeries = p.currSeries[:0]

	if p.metrics == nil {
		p.metrics = make(MetricFamilies)
	}
	for _, mf := range p.metrics {
		mf.help = ""
		mf.typ = ""
		mf.metrics = mf.metrics[:0]
	}

	if p.summaries == nil {
		p.summaries = make(map[uint64]*Summary)
	}
	for k := range p.summaries {
		delete(p.summaries, k)
	}

	if p.histograms == nil {
		p.histograms = make(map[uint64]*Histogram)
	}
	for k := range p.histograms {
		delete(p.histograms, k)
	}
}

func copyLabels(lbs []labels.Label) []labels.Label {
	return append([]labels.Label(nil), lbs...)
}

func removeLabel(lbs labels.Labels, name string) (labels.Labels, string, bool) {
	for i, v := range lbs {
		if v.Name == name {
			return append(lbs[:i], lbs[i+1:]...), v.Value, true
		}
	}
	return lbs, "", false
}

func isSummaryOrHistogram(typ model.MetricType) bool {
	return typ == model.MetricTypeSummary || typ == model.MetricTypeHistogram
}
