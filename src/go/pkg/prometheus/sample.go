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
