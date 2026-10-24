// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
)

// RawSampleIdentity is an opaque collision-resistant identity for one physical
// exposition series before typed-family assembly.
type RawSampleIdentity [32]byte

// LabelSetIdentity is an opaque collision-resistant identity for a label set.
type LabelSetIdentity [32]byte

// SampleSeriesIdentity identifies one logical series at an assembly or writer
// boundary. Typed-family components share the base family and base-label
// identity; bucket and quantile discriminator labels are excluded.
type SampleSeriesIdentity struct {
	Family string
	Labels LabelSetIdentity
}

var emptyLabelSetIdentity = LabelSetIdentity(sha256.Sum256(nil))

// IdentifyRawSample returns the physical exposition identity for metricName and
// its labels. An embedded __name__ label is ignored because metricName is the
// authoritative name argument.
func IdentifyRawSample(metricName string, lbs labels.Labels) RawSampleIdentity {
	digest := sha256.New()
	writeIdentityPart(digest, metricName)
	for _, label := range lbs {
		if label.Name == labels.MetricName {
			continue
		}
		writeIdentityPart(digest, label.Name)
		writeIdentityPart(digest, label.Value)
	}
	var identity RawSampleIdentity
	_ = digest.Sum(identity[:0])
	return identity
}

// IdentifySampleSeries returns the logical assembled-series identity for a
// classified physical sample.
func IdentifySampleSeries(sample Sample) SampleSeriesIdentity {
	excluded := ""
	switch sample.Kind {
	case SampleKindHistogramBucket:
		excluded = bucketLabel
	case SampleKindSummaryQuantile:
		excluded = quantileLabel
	}
	return IdentifySeries(SampleFamilyName(sample), sample.Labels, excluded)
}

// IdentifySeries returns a logical series identity for family and labels,
// excluding the named structural label when non-empty.
func IdentifySeries(family string, lbs labels.Labels, excludedLabel string) SampleSeriesIdentity {
	var digest hash.Hash
	for _, label := range lbs {
		if label.Name == labels.MetricName || label.Name == excludedLabel {
			continue
		}
		if digest == nil {
			digest = sha256.New()
		}
		writeIdentityPart(digest, label.Name)
		writeIdentityPart(digest, label.Value)
	}
	if digest == nil {
		return SampleSeriesIdentity{Family: family, Labels: emptyLabelSetIdentity}
	}
	var identity LabelSetIdentity
	_ = digest.Sum(identity[:0])
	return SampleSeriesIdentity{Family: family, Labels: identity}
}

// SampleFamilyName returns the logical family base name used by Assemble.
func SampleFamilyName(sample Sample) string {
	switch sample.Kind {
	case SampleKindHistogramBucket:
		return strings.TrimSuffix(sample.Name, bucketSuffix)
	case SampleKindHistogramCount, SampleKindSummaryCount:
		return strings.TrimSuffix(sample.Name, countSuffix)
	case SampleKindHistogramSum, SampleKindSummarySum:
		return strings.TrimSuffix(sample.Name, sumSuffix)
	default:
		return sample.Name
	}
}

// SampleStructuralLabelName returns the bucket or quantile discriminator label
// for a classified sample, or an empty string for other component roles.
func SampleStructuralLabelName(kind SampleKind) string {
	switch kind {
	case SampleKindHistogramBucket:
		return bucketLabel
	case SampleKindSummaryQuantile:
		return quantileLabel
	default:
		return ""
	}
}

// SampleSeriesHash returns the base-label hash used by the assembler's typed-
// family grouping. It is an internal-model compatibility helper, not a
// collision-resistant evidence identity; diagnostics should use
// [IdentifySampleSeries].
func SampleSeriesHash(sample Sample) uint64 {
	if excluded := SampleStructuralLabelName(sample.Kind); excluded != "" {
		hash, _ := sample.Labels.HashWithoutLabels(nil, excluded)
		return hash
	}
	return sample.Labels.Hash()
}

func writeIdentityPart(digest hash.Hash, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = digest.Write(size[:])
	_, _ = digest.Write([]byte(value))
}
