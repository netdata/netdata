// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"
)

// SampleDuplicate identifies a later sample whose physical typed-family
// component is identical to an earlier sample in the same batch.
type SampleDuplicate struct {
	FirstIndex     int
	DuplicateIndex int
}

// FindSampleDuplicates returns duplicate physical components in exposition
// order. Identity follows the classified stream consumed by Assemble: scalar
// samples use name plus labels; sums and counts use the base family plus base
// labels; quantiles and buckets additionally include their structural label
// value. Every duplicate points to the first occurrence of that identity.
func FindSampleDuplicates(batch SampleBatch) []SampleDuplicate {
	type seenComponent struct {
		first      int
		collisions []int
	}

	seen := make(map[sampleComponentKey]seenComponent, len(batch.Samples))
	var duplicates []SampleDuplicate
	var hashBuffer []byte

	for i, sample := range batch.Samples {
		key, buffer := sampleComponentIdentity(sample, hashBuffer[:0])
		hashBuffer = buffer
		entry, ok := seen[key]
		if !ok {
			seen[key] = seenComponent{first: i}
			continue
		}

		first := entry.first
		if !sameSampleComponentBaseLabels(batch.Samples[first], sample) {
			first = -1
			for _, candidate := range entry.collisions {
				if sameSampleComponentBaseLabels(batch.Samples[candidate], sample) {
					first = candidate
					break
				}
			}
		}
		if first >= 0 {
			duplicates = append(duplicates, SampleDuplicate{FirstIndex: first, DuplicateIndex: i})
			continue
		}

		entry.collisions = append(entry.collisions, i)
		seen[key] = entry
	}

	return duplicates
}

type sampleComponentKey struct {
	family         string
	kind           SampleKind
	baseLabelsHash uint64
	discriminator  string
}

func sampleComponentIdentity(sample Sample, hashBuffer []byte) (sampleComponentKey, []byte) {
	key := sampleComponentKey{family: sample.Name, kind: sample.Kind}

	switch sample.Kind {
	case SampleKindHistogramBucket:
		key.family = strings.TrimSuffix(sample.Name, bucketSuffix)
		key.discriminator = sample.Labels.Get(bucketLabel)
		key.baseLabelsHash, hashBuffer = sample.Labels.HashWithoutLabels(hashBuffer, bucketLabel)
	case SampleKindHistogramSum, SampleKindSummarySum:
		key.family = strings.TrimSuffix(sample.Name, sumSuffix)
		key.baseLabelsHash = sample.Labels.Hash()
	case SampleKindHistogramCount, SampleKindSummaryCount:
		key.family = strings.TrimSuffix(sample.Name, countSuffix)
		key.baseLabelsHash = sample.Labels.Hash()
	case SampleKindSummaryQuantile:
		key.discriminator = sample.Labels.Get(quantileLabel)
		key.baseLabelsHash, hashBuffer = sample.Labels.HashWithoutLabels(hashBuffer, quantileLabel)
	default:
		key.baseLabelsHash = sample.Labels.Hash()
	}

	return key, hashBuffer
}

func sameSampleComponentBaseLabels(a, b Sample) bool {
	switch a.Kind {
	case SampleKindHistogramBucket:
		return labelsEqualWithout(a.Labels, b.Labels, bucketLabel)
	case SampleKindSummaryQuantile:
		return labelsEqualWithout(a.Labels, b.Labels, quantileLabel)
	default:
		return labels.Equal(a.Labels, b.Labels)
	}
}

func labelsEqualWithout(a, b labels.Labels, excluded string) bool {
	ai, bi := 0, 0
	for {
		for ai < len(a) && a[ai].Name == excluded {
			ai++
		}
		for bi < len(b) && b[bi].Name == excluded {
			bi++
		}
		if ai == len(a) || bi == len(b) {
			return ai == len(a) && bi == len(b)
		}
		if a[ai] != b[bi] {
			return false
		}
		ai++
		bi++
	}
}
