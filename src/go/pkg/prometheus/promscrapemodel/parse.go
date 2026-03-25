// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

const (
	quantileLabel = "quantile"
	bucketLabel   = "le"
	countSuffix   = "_count"
	sumSuffix     = "_sum"
	bucketSuffix  = "_bucket"
)

type Parser struct {
	samples Samples
	sr      selector.Selector

	familyTypes map[string]model.MetricType
	pending     map[string][]pendingRef
	currSeries  labels.Labels
}

type pendingRole uint8

const (
	pendingNone pendingRole = iota
	pendingSum
	pendingCount
)

type pendingRef struct {
	index int
	role  pendingRole
}

func NewParser(sr selector.Selector) Parser {
	return Parser{sr: sr}
}

func (p *Parser) Parse(text []byte) (Samples, error) {
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
		case textparse.EntryType:
			name, typ := parser.Type()
			p.familyTypes[string(name)] = typ
			p.resolvePending(string(name), typ)
		case textparse.EntrySeries:
			p.currSeries = p.currSeries[:0]
			parser.Metric(&p.currSeries)

			if p.sr != nil && !p.sr.Matches(p.currSeries) {
				continue
			}

			_, _, value := parser.Series()

			sample, baseName, role, ok := p.makeSample(value)
			if !ok {
				continue
			}

			p.samples.Add(sample)
			if role != pendingNone {
				p.pending[baseName] = append(p.pending[baseName], pendingRef{
					index: len(p.samples) - 1,
					role:  role,
				})
			}
		}
	}

	return p.samples, nil
}

func (p *Parser) makeSample(value float64) (Sample, string, pendingRole, bool) {
	name, ok := metricNameValue(p.currSeries)
	if !ok {
		return Sample{}, "", pendingNone, false
	}

	sample := Sample{
		Name:       name,
		Labels:     copyLabelsExcept(p.currSeries, labels.MetricName),
		Value:      value,
		Kind:       SampleKindScalar,
		FamilyType: p.familyTypes[name],
	}
	if sample.FamilyType == "" {
		sample.FamilyType = model.MetricTypeUnknown
	}

	if hasLabel(p.currSeries, quantileLabel) {
		sample.Kind = SampleKindSummaryQuantile
		sample.FamilyType = model.MetricTypeSummary
		p.familyTypes[name] = model.MetricTypeSummary
		p.resolvePending(name, model.MetricTypeSummary)
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, bucketSuffix) && hasLabel(p.currSeries, bucketLabel) {
		baseName := strings.TrimSuffix(name, bucketSuffix)
		sample.Kind = SampleKindHistogramBucket
		sample.FamilyType = model.MetricTypeHistogram
		p.familyTypes[baseName] = model.MetricTypeHistogram
		p.resolvePending(baseName, model.MetricTypeHistogram)
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, sumSuffix) {
		baseName := strings.TrimSuffix(name, sumSuffix)
		switch p.familyTypes[baseName] {
		case model.MetricTypeSummary:
			sample.Kind = SampleKindSummarySum
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", pendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = SampleKindHistogramSum
			sample.FamilyType = model.MetricTypeHistogram
			return sample, "", pendingNone, true
		default:
			return sample, baseName, pendingSum, true
		}
	}

	if strings.HasSuffix(name, countSuffix) {
		baseName := strings.TrimSuffix(name, countSuffix)
		switch p.familyTypes[baseName] {
		case model.MetricTypeSummary:
			sample.Kind = SampleKindSummaryCount
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", pendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = SampleKindHistogramCount
			sample.FamilyType = model.MetricTypeHistogram
			return sample, "", pendingNone, true
		default:
			return sample, baseName, pendingCount, true
		}
	}

	return sample, "", pendingNone, true
}

func (p *Parser) resolvePending(baseName string, typ model.MetricType) {
	if typ != model.MetricTypeSummary && typ != model.MetricTypeHistogram {
		return
	}

	refs, ok := p.pending[baseName]
	if !ok {
		return
	}

	for _, ref := range refs {
		if ref.index < 0 || ref.index >= len(p.samples) {
			continue
		}
		sample := &p.samples[ref.index]
		sample.FamilyType = typ
		switch typ {
		case model.MetricTypeSummary:
			if ref.role == pendingSum {
				sample.Kind = SampleKindSummarySum
			} else if ref.role == pendingCount {
				sample.Kind = SampleKindSummaryCount
			}
		case model.MetricTypeHistogram:
			if ref.role == pendingSum {
				sample.Kind = SampleKindHistogramSum
			} else if ref.role == pendingCount {
				sample.Kind = SampleKindHistogramCount
			}
		}
	}

	delete(p.pending, baseName)
}

func (p *Parser) reset() {
	p.samples.Reset()
	p.currSeries = p.currSeries[:0]

	if p.familyTypes == nil {
		p.familyTypes = make(map[string]model.MetricType)
	}
	for k := range p.familyTypes {
		delete(p.familyTypes, k)
	}

	if p.pending == nil {
		p.pending = make(map[string][]pendingRef)
	}
	for k := range p.pending {
		delete(p.pending, k)
	}
}

func copyLabelsExcept(lbs labels.Labels, skip string) []labels.Label {
	out := make([]labels.Label, 0, len(lbs))
	for _, lb := range lbs {
		if lb.Name == skip {
			continue
		}
		out = append(out, lb)
	}
	return out
}

func metricNameValue(lbs labels.Labels) (string, bool) {
	for _, v := range lbs {
		if v.Name == labels.MetricName {
			return v.Value, true
		}
	}
	return "", false
}

func hasLabel(lbs labels.Labels, name string) bool {
	for _, v := range lbs {
		if v.Name == name {
			return true
		}
	}
	return false
}
