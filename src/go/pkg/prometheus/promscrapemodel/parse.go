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

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promselector"
)

const (
	quantileLabel = "quantile"
	bucketLabel   = "le"
	countSuffix   = "_count"
	sumSuffix     = "_sum"
	bucketSuffix  = "_bucket"
)

type Parser struct {
	sr promselector.Selector

	familyTypes   map[string]model.MetricType
	streamPending []streamPendingSample
	currSeries    labels.Labels
}

type streamPendingSample struct {
	baseName string
	sample   Sample
	role     pendingRole
}

type pendingRole uint8

const (
	pendingNone pendingRole = iota
	pendingSum
	pendingCount
)

func NewParser(sr promselector.Selector) Parser {
	return Parser{sr: sr}
}

func (p *Parser) ParseStream(text []byte, onSample func(Sample) error) error {
	return p.parseStream(text, nil, onSample)
}

func (p *Parser) ParseStreamWithMeta(text []byte, onHelp func(name, help string), onSample func(Sample) error) error {
	return p.parseStream(text, onHelp, onSample)
}

func (p *Parser) parseStream(text []byte, onHelp func(name, help string), onSample func(Sample) error) error {
	p.reset()

	if onSample == nil {
		return nil
	}

	pending := p.streamPending[:0]

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
			return fmt.Errorf("failed to parse prometheus metrics: %v", err)
		}

		switch entry {
		case textparse.EntryHelp:
			if onHelp == nil {
				continue
			}
			name, help := parser.Help()
			onHelp(string(name), sanitizeHelp(string(help)))
		case textparse.EntryType:
			name, typ := parser.Type()
			baseName := string(name)
			p.familyTypes[baseName] = typ
			var err error
			pending, err = emitResolvedPendingStream(pending, baseName, typ, onSample)
			if err != nil {
				return err
			}
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

			switch sample.Kind {
			case SampleKindSummaryQuantile:
				var err error
				pending, err = emitResolvedPendingStream(pending, sample.Name, model.MetricTypeSummary, onSample)
				if err != nil {
					return err
				}
			case SampleKindHistogramBucket:
				var err error
				pending, err = emitResolvedPendingStream(pending, strings.TrimSuffix(sample.Name, bucketSuffix), model.MetricTypeHistogram, onSample)
				if err != nil {
					return err
				}
			}

			if role != pendingNone {
				pending = append(pending, streamPendingSample{
					baseName: baseName,
					sample:   sample,
					role:     role,
				})
				continue
			}

			if err := onSample(sample); err != nil {
				return err
			}
		}
	}

	for _, pendingSample := range pending {
		if err := onSample(pendingSample.sample); err != nil {
			return err
		}
	}
	p.streamPending = pending[:0]

	return nil
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
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, bucketSuffix) && hasLabel(p.currSeries, bucketLabel) {
		baseName := strings.TrimSuffix(name, bucketSuffix)
		sample.Kind = SampleKindHistogramBucket
		sample.FamilyType = model.MetricTypeHistogram
		p.familyTypes[baseName] = model.MetricTypeHistogram
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, sumSuffix) {
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", pendingNone, true
		}

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
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", pendingNone, true
		}

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

func (p *Parser) reset() {
	p.currSeries = p.currSeries[:0]

	if p.familyTypes == nil {
		p.familyTypes = make(map[string]model.MetricType)
	}
	for k := range p.familyTypes {
		delete(p.familyTypes, k)
	}

	p.streamPending = p.streamPending[:0]
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

func emitResolvedPendingStream(pending []streamPendingSample, baseName string, typ model.MetricType, onSample func(Sample) error) ([]streamPendingSample, error) {
	if len(pending) == 0 {
		return pending, nil
	}

	out := pending[:0]
	for _, pendingSample := range pending {
		if pendingSample.baseName != baseName {
			out = append(out, pendingSample)
			continue
		}

		sample := pendingSample.sample
		sample.FamilyType = typ
		switch typ {
		case model.MetricTypeSummary:
			if pendingSample.role == pendingSum {
				sample.Kind = SampleKindSummarySum
			} else {
				sample.Kind = SampleKindSummaryCount
			}
		case model.MetricTypeHistogram:
			if pendingSample.role == pendingSum {
				sample.Kind = SampleKindHistogramSum
			} else {
				sample.Kind = SampleKindHistogramCount
			}
		default:
			sample.Kind = SampleKindScalar
			sample.FamilyType = model.MetricTypeUnknown
		}

		if err := onSample(sample); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func sanitizeHelp(help string) string {
	if strings.IndexByte(help, '\n') == -1 {
		return help
	}
	return strings.Join(strings.Fields(help), " ")
}
