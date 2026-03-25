// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

type (
	SampleKind uint8

	Sample struct {
		Name       string
		Labels     labels.Labels
		Value      float64
		Kind       SampleKind
		FamilyType model.MetricType
	}

	Samples []Sample
)

const (
	SampleKindScalar SampleKind = iota
	SampleKindHistogramBucket
	SampleKindHistogramSum
	SampleKindHistogramCount
	SampleKindSummaryQuantile
	SampleKindSummarySum
	SampleKindSummaryCount
)

func (s *Samples) Add(sample Sample) {
	*s = append(*s, sample)
}

func (s *Samples) Reset() {
	*s = (*s)[:0]
}

func (s Samples) Len() int {
	return len(s)
}

// FindByName returns all raw samples with the given metric name.
// The returned order matches the original scrape order.
func (s Samples) FindByName(name string) Samples {
	if len(s) == 0 {
		return Samples{}
	}

	out := Samples{}
	for _, sample := range s {
		if sample.Name == name {
			out = append(out, sample)
		}
	}

	return out
}

// FindByNames returns all raw samples whose metric name matches any of names.
// The returned order matches the original scrape order.
func (s Samples) FindByNames(names ...string) Samples {
	switch len(names) {
	case 0:
		return Samples{}
	case 1:
		return s.FindByName(names[0])
	}

	nameSet := make(map[string]struct{}, len(names))
	for _, name := range names {
		nameSet[name] = struct{}{}
	}

	out := Samples{}
	for _, sample := range s {
		if _, ok := nameSet[sample.Name]; ok {
			out = append(out, sample)
		}
	}

	return out
}
