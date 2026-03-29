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

type parseDriver struct {
	sr promselector.Selector

	familyTypes map[string]model.MetricType
	pending     []pendingSample
	currSeries  labels.Labels
}

type pendingSample struct {
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

func (d *parseDriver) parse(text []byte, ownLabels bool, onHelp func(name, help string), onSample func(Sample) error) error {
	d.reset()

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
			if onHelp != nil {
				name, help := parser.Help()
				onHelp(string(name), sanitizeHelp(string(help)))
			}
		case textparse.EntryType:
			name, typ := parser.Type()
			baseName := string(name)
			d.familyTypes[baseName] = typ
			var err error
			d.pending, err = emitResolvedPending(d.pending, baseName, typ, onSample)
			if err != nil {
				return err
			}
		case textparse.EntrySeries:
			d.currSeries = d.currSeries[:0]
			parser.Metric(&d.currSeries)

			if d.sr != nil && !d.sr.Matches(d.currSeries) {
				continue
			}

			_, _, value := parser.Series()

			sample, baseName, role, ok := d.makeSample(value, ownLabels)
			if !ok {
				continue
			}

			switch sample.Kind {
			case SampleKindSummaryQuantile:
				var err error
				d.pending, err = emitResolvedPending(d.pending, sample.Name, model.MetricTypeSummary, onSample)
				if err != nil {
					return err
				}
			case SampleKindHistogramBucket:
				var err error
				d.pending, err = emitResolvedPending(d.pending, strings.TrimSuffix(sample.Name, bucketSuffix), model.MetricTypeHistogram, onSample)
				if err != nil {
					return err
				}
			}

			if role != pendingNone {
				if !ownLabels {
					sample.Labels = copyLabels(sample.Labels)
				}
				d.pending = append(d.pending, pendingSample{
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

	for _, pendingSample := range d.pending {
		if err := onSample(pendingSample.sample); err != nil {
			return err
		}
	}

	d.pending = d.pending[:0]
	return nil
}

func (d *parseDriver) makeSample(value float64, ownLabels bool) (Sample, string, pendingRole, bool) {
	name, ok := metricNameValue(d.currSeries)
	if !ok {
		return Sample{}, "", pendingNone, false
	}

	var lbs labels.Labels
	if ownLabels {
		lbs = copyLabelsExcept(d.currSeries, labels.MetricName)
	} else {
		lbs, _, _ = removeLabel(d.currSeries, labels.MetricName)
	}

	sample := Sample{
		Name:       name,
		Labels:     lbs,
		Value:      value,
		Kind:       SampleKindScalar,
		FamilyType: d.familyTypes[name],
	}
	if sample.FamilyType == "" {
		sample.FamilyType = model.MetricTypeUnknown
	}

	if sample.Labels.Has(quantileLabel) {
		sample.Kind = SampleKindSummaryQuantile
		sample.FamilyType = model.MetricTypeSummary
		d.familyTypes[name] = model.MetricTypeSummary
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, bucketSuffix) && sample.Labels.Has(bucketLabel) {
		baseName := strings.TrimSuffix(name, bucketSuffix)
		sample.Kind = SampleKindHistogramBucket
		sample.FamilyType = model.MetricTypeHistogram
		d.familyTypes[baseName] = model.MetricTypeHistogram
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, sumSuffix) {
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", pendingNone, true
		}

		baseName := strings.TrimSuffix(name, sumSuffix)
		switch d.familyTypes[baseName] {
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
		switch d.familyTypes[baseName] {
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

func (d *parseDriver) reset() {
	d.currSeries = d.currSeries[:0]

	if d.familyTypes == nil {
		d.familyTypes = make(map[string]model.MetricType)
	}
	for k := range d.familyTypes {
		delete(d.familyTypes, k)
	}

	d.pending = d.pending[:0]
}

func emitResolvedPending(pending []pendingSample, baseName string, typ model.MetricType, onSample func(Sample) error) ([]pendingSample, error) {
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
