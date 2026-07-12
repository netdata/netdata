// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scriptedGetMetricData struct {
	pages []*cloudwatch.GetMetricDataOutput
	errs  map[int]error
	calls int
}

func (f *scriptedGetMetricData) ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return &cloudwatch.ListMetricsOutput{}, nil
}

func (f *scriptedGetMetricData) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	index := f.calls
	f.calls++
	if err := f.errs[index]; err != nil {
		return nil, err
	}
	return f.pages[index], nil
}

func responseTestBatch(client cloudwatchClient, queries ...plannedQuery) queryBatch {
	policy := queryPolicy{period: 5 * time.Minute, lookback: 15 * time.Minute}
	for i := range queries {
		queries[i].policy = policy
	}
	return queryBatch{
		key:    queryBatchKey{target: "base", region: "us-east-1", policy: policy},
		client: client, queries: queries,
		start: time.Unix(0, 0), end: time.Unix(900, 0),
	}
}

func metricResult(id string, status cwtypes.StatusCode, values []float64, timestamps []time.Time) cwtypes.MetricDataResult {
	return cwtypes.MetricDataResult{Id: aws.String(id), StatusCode: status, Values: values, Timestamps: timestamps}
}

func TestRunGetMetricData_SelectsNewestEligibleFinitePair(t *testing.T) {
	fake := &scriptedGetMetricData{pages: []*cloudwatch.GetMetricDataOutput{{MetricDataResults: []cwtypes.MetricDataResult{
		metricResult("q0", cwtypes.StatusCodeComplete,
			[]float64{1, 3, 2, math.NaN()},
			[]time.Time{time.Unix(0, 0), time.Unix(600, 0), time.Unix(300, 0), time.Unix(600, 0)}),
	}}}}
	query := testPlannedQuery("stable", "base", "us-east-1", "AWS/EC2", 300)
	outcomes, _, err := runGetMetricData(context.Background(), responseTestBatch(fake, query))
	require.NoError(t, err)
	require.Contains(t, outcomes, query.key)
	assert.Equal(t, float64(3), outcomes[query.key].value)
	assert.Equal(t, time.Unix(600, 0), outcomes[query.key].datapointAt)
}

func TestRunGetMetricData_PartialRequiresLaterComplete(t *testing.T) {
	query := testPlannedQuery("stable", "base", "us-east-1", "AWS/EC2", 300)

	t.Run("later complete applies accumulated candidate", func(t *testing.T) {
		fake := &scriptedGetMetricData{pages: []*cloudwatch.GetMetricDataOutput{
			{MetricDataResults: []cwtypes.MetricDataResult{metricResult("q0", cwtypes.StatusCodePartialData, []float64{1}, []time.Time{time.Unix(300, 0)})}, NextToken: aws.String("next")},
			{MetricDataResults: []cwtypes.MetricDataResult{metricResult("q0", cwtypes.StatusCodeComplete, []float64{2}, []time.Time{time.Unix(600, 0)})}},
		}}
		outcomes, _, err := runGetMetricData(context.Background(), responseTestBatch(fake, query))
		require.NoError(t, err)
		assert.Equal(t, float64(2), outcomes[query.key].value)
	})

	t.Run("unresolved partial remains transient", func(t *testing.T) {
		fake := &scriptedGetMetricData{pages: []*cloudwatch.GetMetricDataOutput{{MetricDataResults: []cwtypes.MetricDataResult{
			metricResult("q0", cwtypes.StatusCodePartialData, []float64{1}, []time.Time{time.Unix(300, 0)}),
		}}}}
		outcomes, issues, err := runGetMetricData(context.Background(), responseTestBatch(fake, query))
		require.NoError(t, err)
		assert.Empty(t, outcomes)
		require.Len(t, issues, 1)
		assert.Equal(t, queryIssuePartialData, issues[0].kind)
	})
}

func TestRunGetMetricData_InternalErrorCandidateIsNotAcceptedByLaterComplete(t *testing.T) {
	query := testPlannedQuery("stable", "base", "us-east-1", "AWS/EC2", 300)
	fake := &scriptedGetMetricData{pages: []*cloudwatch.GetMetricDataOutput{
		{MetricDataResults: []cwtypes.MetricDataResult{
			metricResult("q0", cwtypes.StatusCodeInternalError, []float64{99}, []time.Time{time.Unix(600, 0)}),
		}, NextToken: aws.String("next")},
		{MetricDataResults: []cwtypes.MetricDataResult{
			metricResult("q0", cwtypes.StatusCodeComplete, nil, nil),
		}},
	}}

	outcomes, issues, err := runGetMetricData(context.Background(), responseTestBatch(fake, query))
	require.NoError(t, err)
	require.Contains(t, outcomes, query.key)
	assert.False(t, outcomes[query.key].hasDatapoint)
	assert.Empty(t, issues)
}

func TestRunGetMetricData_PageFailurePreservesCompletedSiblings(t *testing.T) {
	first := testPlannedQuery("first", "base", "us-east-1", "AWS/EC2", 300)
	second := testPlannedQuery("second", "base", "us-east-1", "AWS/EC2", 300)
	fake := &scriptedGetMetricData{
		pages: []*cloudwatch.GetMetricDataOutput{{
			MetricDataResults: []cwtypes.MetricDataResult{
				metricResult("q0", cwtypes.StatusCodeComplete, []float64{1}, []time.Time{time.Unix(300, 0)}),
				metricResult("q1", cwtypes.StatusCodePartialData, nil, nil),
			},
			NextToken: aws.String("next"),
		}},
		errs: map[int]error{1: errors.New("page failed")},
	}
	outcomes, _, err := runGetMetricData(context.Background(), responseTestBatch(fake, first, second))
	require.Error(t, err)
	assert.Contains(t, outcomes, first.key)
	assert.NotContains(t, outcomes, second.key)
}

func TestRunGetMetricData_BoundsPaginationAtTwoCalls(t *testing.T) {
	query := testPlannedQuery("stable", "base", "us-east-1", "AWS/EC2", 300)
	fake := &scriptedGetMetricData{pages: []*cloudwatch.GetMetricDataOutput{
		{MetricDataResults: []cwtypes.MetricDataResult{metricResult("q0", cwtypes.StatusCodePartialData, nil, nil)}, NextToken: aws.String("second")},
		{MetricDataResults: []cwtypes.MetricDataResult{metricResult("q0", cwtypes.StatusCodePartialData, nil, nil)}, NextToken: aws.String("third")},
	}}
	outcomes, issues, err := runGetMetricData(context.Background(), responseTestBatch(fake, query))
	require.NoError(t, err)
	assert.Empty(t, outcomes)
	assert.Equal(t, maxGetMetricDataPages, fake.calls)
	require.Len(t, issues, 1)
	assert.Equal(t, queryIssuePaginationLimit, issues[0].kind)
}

func TestRunGetMetricData_ReportsUnresolvedResultKinds(t *testing.T) {
	query := testPlannedQuery("stable", "base", "us-east-1", "AWS/EC2", 300)
	tests := map[string]struct {
		results []cwtypes.MetricDataResult
		want    queryResultIssueKind
	}{
		"missing":        {want: queryIssueMissingResult},
		"internal error": {results: []cwtypes.MetricDataResult{metricResult("q0", cwtypes.StatusCodeInternalError, nil, nil)}, want: queryIssueInternalError},
		"unknown status": {results: []cwtypes.MetricDataResult{metricResult("q0", cwtypes.StatusCode("Unknown"), nil, nil)}, want: queryIssueUnknownStatus},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &scriptedGetMetricData{pages: []*cloudwatch.GetMetricDataOutput{{MetricDataResults: tc.results}}}
			outcomes, issues, err := runGetMetricData(context.Background(), responseTestBatch(fake, query))
			require.NoError(t, err)
			assert.Empty(t, outcomes)
			require.Len(t, issues, 1)
			assert.Equal(t, tc.want, issues[0].kind)
		})
	}
}
