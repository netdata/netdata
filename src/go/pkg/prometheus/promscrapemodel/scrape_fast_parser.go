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

type scrapeFastParser struct {
	sr promselector.Selector

	familyTypes map[string]model.MetricType
	pending     []fastPendingSample
	currSeries  labels.Labels
}

type fastPendingSample struct {
	sample   Sample
	baseName string
	role     fastPendingRole
}

type fastPendingRole uint8

const (
	fastPendingNone fastPendingRole = iota
	fastPendingSum
	fastPendingCount
)

type FastParser struct {
	parser scrapeFastParser
}

func NewFastParser(sr promselector.Selector) FastParser {
	return FastParser{parser: scrapeFastParser{sr: sr}}
}

func (p *FastParser) ParseToAssembler(text []byte, asm *Assembler) error {
	return p.parser.parseToAssembler(text, asm)
}

func (p *scrapeFastParser) parseToAssembler(text []byte, asm *Assembler) error {
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
			return fmt.Errorf("failed to parse prometheus metrics: %v", err)
		}

		switch entry {
		case textparse.EntryHelp:
			name, help := parser.Help()
			asm.applyHelp(string(name), sanitizeHelp(string(help)))
		case textparse.EntryType:
			name, typ := parser.Type()
			baseName := string(name)
			p.familyTypes[baseName] = typ
			if err := p.emitResolvedPending(baseName, typ, asm); err != nil {
				return err
			}
		case textparse.EntrySeries:
			p.currSeries = p.currSeries[:0]
			parser.Metric(&p.currSeries)

			if p.sr != nil && !p.sr.Matches(p.currSeries) {
				continue
			}

			_, _, value := parser.Series()

			sample, baseName, role, ok := p.makeScratchSample(value)
			if !ok {
				continue
			}

			switch sample.Kind {
			case SampleKindSummaryQuantile:
				if err := p.emitResolvedPending(sample.Name, model.MetricTypeSummary, asm); err != nil {
					return err
				}
			case SampleKindHistogramBucket:
				if err := p.emitResolvedPending(strings.TrimSuffix(sample.Name, bucketSuffix), model.MetricTypeHistogram, asm); err != nil {
					return err
				}
			}

			if role != fastPendingNone {
				sample.Labels = copyLabels(sample.Labels)
				p.pending = append(p.pending, fastPendingSample{
					sample:   sample,
					baseName: baseName,
					role:     role,
				})
				continue
			}

			if err := asm.ApplySample(sample); err != nil {
				return err
			}
		}
	}

	for _, pendingSample := range p.pending {
		if err := asm.ApplySample(pendingSample.sample); err != nil {
			return err
		}
	}

	return nil
}

func (p *scrapeFastParser) makeScratchSample(value float64) (Sample, string, fastPendingRole, bool) {
	name, ok := metricNameValue(p.currSeries)
	if !ok {
		return Sample{}, "", fastPendingNone, false
	}

	lbs, _, _ := removeLabel(p.currSeries, labels.MetricName)
	sample := Sample{
		Name:       name,
		Labels:     lbs,
		Value:      value,
		Kind:       SampleKindScalar,
		FamilyType: p.familyTypes[name],
	}
	if sample.FamilyType == "" {
		sample.FamilyType = model.MetricTypeUnknown
	}

	if lbs.Has(quantileLabel) {
		sample.Kind = SampleKindSummaryQuantile
		sample.FamilyType = model.MetricTypeSummary
		p.familyTypes[name] = model.MetricTypeSummary
		return sample, "", fastPendingNone, true
	}

	if strings.HasSuffix(name, bucketSuffix) {
		baseName := strings.TrimSuffix(name, bucketSuffix)
		if lbs.Has(bucketLabel) {
			sample.Kind = SampleKindHistogramBucket
			sample.FamilyType = model.MetricTypeHistogram
			p.familyTypes[baseName] = model.MetricTypeHistogram
			return sample, "", fastPendingNone, true
		}
	}

	if strings.HasSuffix(name, sumSuffix) {
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", fastPendingNone, true
		}

		baseName := strings.TrimSuffix(name, sumSuffix)
		switch p.familyTypes[baseName] {
		case model.MetricTypeSummary:
			sample.Kind = SampleKindSummarySum
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", fastPendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = SampleKindHistogramSum
			sample.FamilyType = model.MetricTypeHistogram
			return sample, "", fastPendingNone, true
		default:
			return sample, baseName, fastPendingSum, true
		}
	}

	if strings.HasSuffix(name, countSuffix) {
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", fastPendingNone, true
		}

		baseName := strings.TrimSuffix(name, countSuffix)
		switch p.familyTypes[baseName] {
		case model.MetricTypeSummary:
			sample.Kind = SampleKindSummaryCount
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", fastPendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = SampleKindHistogramCount
			sample.FamilyType = model.MetricTypeHistogram
			return sample, "", fastPendingNone, true
		default:
			return sample, baseName, fastPendingCount, true
		}
	}

	return sample, "", fastPendingNone, true
}

func (p *scrapeFastParser) emitResolvedPending(baseName string, typ model.MetricType, asm *Assembler) error {
	if len(p.pending) == 0 {
		return nil
	}

	out := p.pending[:0]
	for _, pendingSample := range p.pending {
		if pendingSample.baseName != baseName {
			out = append(out, pendingSample)
			continue
		}

		sample := pendingSample.sample
		sample.FamilyType = typ
		switch typ {
		case model.MetricTypeSummary:
			if pendingSample.role == fastPendingSum {
				sample.Kind = SampleKindSummarySum
			} else {
				sample.Kind = SampleKindSummaryCount
			}
		case model.MetricTypeHistogram:
			if pendingSample.role == fastPendingSum {
				sample.Kind = SampleKindHistogramSum
			} else {
				sample.Kind = SampleKindHistogramCount
			}
		default:
			sample.Kind = SampleKindScalar
			sample.FamilyType = model.MetricTypeUnknown
		}

		if err := asm.ApplySample(sample); err != nil {
			return err
		}
	}

	p.pending = out
	return nil
}

func (p *scrapeFastParser) reset() {
	p.currSeries = p.currSeries[:0]

	if p.familyTypes == nil {
		p.familyTypes = make(map[string]model.MetricType)
	}
	for k := range p.familyTypes {
		delete(p.familyTypes, k)
	}

	p.pending = p.pending[:0]
}
