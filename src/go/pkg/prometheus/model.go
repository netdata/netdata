// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// SampleKind classifies a streamed [Sample] by the role it plays in its typed
// family. The driver assigns it as it parses, so a consumer can re-assemble
// typed families (or relabel) without re-deriving the role from the name.
type SampleKind uint8

const (
	// SampleKindScalar is a plain sample (gauge, counter, untyped, or the base
	// series of a summary/histogram). Interpret it together with FamilyType.
	SampleKindScalar SampleKind = iota
	SampleKindHistogramBucket
	SampleKindHistogramSum
	SampleKindHistogramCount
	SampleKindSummaryQuantile
	SampleKindSummarySum
	SampleKindSummaryCount
)

// Sample is a single scraped series exposed before typed-family assembly.
//
// Name is the __name__ label value (found by lookup, not by position — do not
// assume label index 0). Labels holds every other label, including structural
// labels such as "le" (histogram buckets) and "quantile" (summary quantiles) —
// textparse canonicalizes these to floats (e.g. "1" -> "1.0") only when the family
// type is known from # TYPE, otherwise leaving them raw, so do not assume a fixed
// form when matching. Labels never contains __name__. Value is the sample value.
// Kind and FamilyType carry the classification the driver derived for this sample.
//
// Sample is the unit a Prometheus metric-relabeling step operates on.
type Sample struct {
	Name       string
	Labels     labels.Labels
	Value      float64
	Kind       SampleKind
	FamilyType model.MetricType
}

// SampleTransform transforms or drops a single scraped Sample before typed-family
// assembly. Return (sample, true, nil) to keep it (optionally mutated — rewrite Name
// or mutate Labels in place), (_, false, nil) to drop it, or a non-nil error to abort
// the scrape. Each Sample owns its Labels, so in-place mutation is safe and does not
// affect other samples. It is the hook a Prometheus metric-relabeling step plugs into.
//
// Kind and FamilyType reflect the classification BEFORE the transform runs; rewriting
// Name, le, or quantile does NOT reclassify the sample (matching Prometheus, where
// relabeling cannot retype a series).
type SampleTransform func(Sample) (Sample, bool, error)
