// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSamples_FindByName(t *testing.T) {
	tests := map[string]struct {
		samples Samples
		name    string
		want    Samples
	}{
		"returns matching samples in scrape order": {
			samples: Samples{
				{Name: "alpha", Value: 1, FamilyType: model.MetricTypeGauge},
				{Name: "beta", Value: 2, FamilyType: model.MetricTypeCounter},
				{Name: "alpha", Value: 3, FamilyType: model.MetricTypeGauge},
			},
			name: "alpha",
			want: Samples{
				{Name: "alpha", Value: 1, FamilyType: model.MetricTypeGauge},
				{Name: "alpha", Value: 3, FamilyType: model.MetricTypeGauge},
			},
		},
		"returns empty when not found": {
			samples: Samples{
				{Name: "alpha", Value: 1},
			},
			name: "gamma",
			want: Samples{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := test.samples.FindByName(test.name)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSamples_FindByNames(t *testing.T) {
	tests := map[string]struct {
		samples Samples
		names   []string
		want    Samples
	}{
		"returns all matching samples in scrape order": {
			samples: Samples{
				{Name: "alpha", Value: 1},
				{Name: "beta", Value: 2},
				{Name: "gamma", Value: 3},
				{Name: "alpha", Value: 4},
			},
			names: []string{"gamma", "alpha"},
			want: Samples{
				{Name: "alpha", Value: 1},
				{Name: "gamma", Value: 3},
				{Name: "alpha", Value: 4},
			},
		},
		"deduplicates requested names": {
			samples: Samples{
				{Name: "alpha", Value: 1},
				{Name: "beta", Value: 2},
			},
			names: []string{"alpha", "alpha"},
			want: Samples{
				{Name: "alpha", Value: 1},
			},
		},
		"returns empty for empty names": {
			samples: Samples{
				{Name: "alpha", Value: 1},
			},
			names: []string{},
			want:  Samples{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := test.samples.FindByNames(test.names...)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestSamples_Len(t *testing.T) {
	samples := Samples{
		{
			Name:       "test_metric",
			Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
			Value:      1,
			Kind:       SampleKindScalar,
			FamilyType: model.MetricTypeGauge,
		},
	}

	require.Equal(t, 1, samples.Len())
}
