// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

type scrapeFastParser struct {
	sr selector.Selector

	familyTypes map[string]model.MetricType
	pending     []fastPendingSample
	currSeries  labels.Labels
}

type fastPendingSample struct {
	sample   promscrapemodel.Sample
	baseName string
	role     fastPendingRole
}

type fastPendingRole uint8

const (
	fastPendingNone fastPendingRole = iota
	fastPendingSum
	fastPendingCount
)

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
			case promscrapemodel.SampleKindSummaryQuantile:
				if err := p.emitResolvedPending(sample.Name, model.MetricTypeSummary, asm); err != nil {
					return err
				}
			case promscrapemodel.SampleKindHistogramBucket:
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

func (p *scrapeFastParser) makeScratchSample(value float64) (promscrapemodel.Sample, string, fastPendingRole, bool) {
	name, ok := metricNameValue(p.currSeries)
	if !ok {
		return promscrapemodel.Sample{}, "", fastPendingNone, false
	}

	lbs, _, _ := removeLabel(p.currSeries, labels.MetricName)
	sample := promscrapemodel.Sample{
		Name:       name,
		Labels:     lbs,
		Value:      value,
		Kind:       promscrapemodel.SampleKindScalar,
		FamilyType: p.familyTypes[name],
	}
	if sample.FamilyType == "" {
		sample.FamilyType = model.MetricTypeUnknown
	}

	if lbs.Has(quantileLabel) {
		sample.Kind = promscrapemodel.SampleKindSummaryQuantile
		sample.FamilyType = model.MetricTypeSummary
		p.familyTypes[name] = model.MetricTypeSummary
		return sample, "", fastPendingNone, true
	}

	if strings.HasSuffix(name, bucketSuffix) {
		baseName := strings.TrimSuffix(name, bucketSuffix)
		if lbs.Has(bucketLabel) {
			sample.Kind = promscrapemodel.SampleKindHistogramBucket
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
			sample.Kind = promscrapemodel.SampleKindSummarySum
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", fastPendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = promscrapemodel.SampleKindHistogramSum
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
			sample.Kind = promscrapemodel.SampleKindSummaryCount
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", fastPendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = promscrapemodel.SampleKindHistogramCount
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
				sample.Kind = promscrapemodel.SampleKindSummarySum
			} else {
				sample.Kind = promscrapemodel.SampleKindSummaryCount
			}
		case model.MetricTypeHistogram:
			if pendingSample.role == fastPendingSum {
				sample.Kind = promscrapemodel.SampleKindHistogramSum
			} else {
				sample.Kind = promscrapemodel.SampleKindHistogramCount
			}
		default:
			sample.Kind = promscrapemodel.SampleKindScalar
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

func sanitizeHelp(help string) string {
	if strings.IndexByte(help, '\n') == -1 {
		return help
	}
	return reSpace.ReplaceAllString(strings.TrimSpace(help), " ")
}
