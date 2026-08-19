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

// LabelsWithName returns a detached selector-compatible label set containing
// __name__ followed by the sample labels.
func (s Sample) LabelsWithName() labels.Labels {
	out := make(labels.Labels, 0, len(s.Labels)+1)
	out = append(out, labels.Label{Name: labels.MetricName, Value: s.Name})
	out = append(out, s.Labels...)
	return out
}

// HelpEntry is a family's HELP text, keyed by family name, carried alongside a
// SampleBatch (HELP is parsed per family, before assembly).
type HelpEntry struct {
	Name string
	Help string
}

// SampleBatch is the flat, classified sample stream of one scrape plus the
// per-family HELP, returned by [Prometheus.ScrapeSamples] before typed-family
// assembly. The caller owns the samples (each Sample owns its Labels) and may
// relabel them in place, then fold the result with [Assemble].
type SampleBatch struct {
	Help    []HelpEntry
	Samples []Sample
}

// SampleComponentToken returns the stable physical-component token owned by
// the parser/assembler model. An unknown kind returns an empty token.
func SampleComponentToken(kind SampleKind) string {
	switch kind {
	case SampleKindScalar:
		return "scalar"
	case SampleKindHistogramBucket:
		return "histogram_bucket"
	case SampleKindHistogramSum:
		return "histogram_sum"
	case SampleKindHistogramCount:
		return "histogram_count"
	case SampleKindSummaryQuantile:
		return "summary_quantile"
	case SampleKindSummarySum:
		return "summary_sum"
	case SampleKindSummaryCount:
		return "summary_count"
	default:
		return ""
	}
}

// MetricTypeToken returns the stable source-type token for a parsed sample.
// Prometheus represents exposition without a TYPE declaration as unknown;
// the public token calls that source shape untyped.
func MetricTypeToken(typ model.MetricType) string {
	switch typ {
	case model.MetricTypeGauge:
		return "gauge"
	case model.MetricTypeCounter:
		return "counter"
	case model.MetricTypeHistogram:
		return "histogram"
	case model.MetricTypeSummary:
		return "summary"
	case model.MetricTypeUnknown:
		return "untyped"
	default:
		return ""
	}
}
