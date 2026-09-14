// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
)

func TestSampleIdentitiesFollowAssemblyBoundaries(t *testing.T) {
	base := labels.FromStrings("instance", "a")
	bucketOne := Sample{
		Name:       "request_duration_bucket",
		Labels:     labels.FromStrings("instance", "a", "le", "1"),
		Kind:       SampleKindHistogramBucket,
		FamilyType: model.MetricTypeHistogram,
	}
	bucketTwo := bucketOne
	bucketTwo.Labels = labels.FromStrings("instance", "a", "le", "2")
	sum := Sample{
		Name:       "request_duration_sum",
		Labels:     base,
		Kind:       SampleKindHistogramSum,
		FamilyType: model.MetricTypeHistogram,
	}
	count := sum
	count.Name = "request_duration_count"
	count.Kind = SampleKindHistogramCount

	assert.Equal(t, IdentifySampleSeries(bucketOne), IdentifySampleSeries(bucketTwo))
	assert.Equal(t, IdentifySampleSeries(bucketOne), IdentifySampleSeries(sum))
	assert.Equal(t, IdentifySampleSeries(sum), IdentifySampleSeries(count))
	assert.NotEqual(t, IdentifyRawSample(bucketOne.Name, bucketOne.Labels), IdentifyRawSample(bucketTwo.Name, bucketTwo.Labels))
	assert.NotEqual(t, IdentifyRawSample(sum.Name, sum.Labels), IdentifyRawSample(count.Name, count.Labels))
}

func TestIdentifyRawSampleAcceptsParserAndSampleLabelShapes(t *testing.T) {
	withName := labels.FromStrings("__name__", "app_value", "instance", "a")
	withoutName := labels.FromStrings("instance", "a")

	assert.Equal(t, IdentifyRawSample("app_value", withName), IdentifyRawSample("app_value", withoutName))
	assert.NotEqual(t, IdentifyRawSample("app_value", withoutName), IdentifyRawSample("app_other", withoutName))
	assert.NotEqual(t,
		IdentifyRawSample("app_value", labels.FromStrings("instance", "a")),
		IdentifyRawSample("app_value", labels.FromStrings("instance", "b")),
	)
}
