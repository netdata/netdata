// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testPlannedQuery(key, target, region, namespace string, period int) plannedQuery {
	return plannedQuery{
		key: testStructuralID(key), target: target, region: region,
		policy: queryPolicy{period: time.Duration(period) * time.Second, lookback: time.Duration(period) * time.Second, publicationDelay: defaultPublicationDelay},
		query: cwtypes.MetricDataQuery{
			MetricStat: &cwtypes.MetricStat{Metric: &cwtypes.Metric{Namespace: aws.String(namespace)}, Period: aws.Int32(int32(period))},
		},
	}
}

func testStructuralID(value string) structuralID {
	return structuralIDFromStrings("test", value)
}

func TestBuildQueryBatches_PointAwareWidth(t *testing.T) {
	policy := queryPolicy{period: time.Minute, lookback: 24 * time.Hour, publicationDelay: 0}
	width := queryBatchWidth(policy)
	assert.Equal(t, 20, width)
	queries := make([]plannedQuery, 45)
	for i := range queries {
		queries[i] = testPlannedQuery(fmt.Sprintf("q%d", i), "base", "us-east-1", "AWS/EC2", 60)
		queries[i].policy = policy
	}
	clients := map[clientKey]cloudwatchClient{{target: "base", region: "us-east-1"}: &gmdCloudWatch{}}
	batches := buildQueryBatches(queries, clients, time.Unix(1_000_000_000, 0))
	require.Len(t, batches, 3)
	assert.Len(t, batches[0].queries, 20)
	assert.Len(t, batches[1].queries, 20)
	assert.Len(t, batches[2].queries, 5)
}

func TestBuildQueryBatches_DoesNotSplitMetricBillingGroup(t *testing.T) {
	policy := queryPolicy{period: time.Minute, lookback: 24 * time.Hour}
	queries := make([]plannedQuery, 0, 22)
	for i := range 19 {
		query := testPlannedQuery(fmt.Sprintf("single-%d", i), "base", "us-east-1", "AWS/Test", 60)
		query.policy = policy
		query.query.MetricStat.Metric.MetricName = aws.String(fmt.Sprintf("Single%d", i))
		query.query.MetricStat.Stat = aws.String("Average")
		queries = append(queries, query)
	}
	for i, statistic := range []string{"Average", "Maximum", "p90"} {
		query := testPlannedQuery(fmt.Sprintf("multi-%d", i), "base", "us-east-1", "AWS/Test", 60)
		query.policy = policy
		query.query.MetricStat.Metric.MetricName = aws.String("Multi")
		query.query.MetricStat.Stat = aws.String(statistic)
		queries = append(queries, query)
	}
	clients := map[clientKey]cloudwatchClient{{target: "base", region: "us-east-1"}: &gmdCloudWatch{}}

	batches := buildQueryBatches(queries, clients, time.Unix(1_000_000_000, 0))
	require.Len(t, batches, 2)
	multiBatch := -1
	multiCount := 0
	for i, batch := range batches {
		for _, query := range batch.queries {
			if aws.ToString(query.query.MetricStat.Metric.MetricName) != "Multi" {
				continue
			}
			if multiBatch == -1 {
				multiBatch = i
			}
			assert.Equal(t, multiBatch, i, "one AWS billing group must stay in one request")
			multiCount++
		}
	}
	assert.Equal(t, 3, multiCount)
}

func TestPackBillingUnitShapes(t *testing.T) {
	t.Run("fragmentation is included in batch count", func(t *testing.T) {
		groups := make(map[structuralID]int)
		for i := range 13 {
			groups[testStructuralID(fmt.Sprintf("metric-%d", i))] = 3
		}
		units, batches := packBillingUnitShapes(groups, 20)
		assert.Len(t, units, 13)
		assert.Equal(t, 3, batches, "thirteen three-statistic groups cannot fit in two twenty-query requests")
	})

	t.Run("more than five statistics becomes multiple whole billing units", func(t *testing.T) {
		units, batches := packBillingUnitShapes(map[structuralID]int{testStructuralID("metric"): 6}, 20)
		require.Len(t, units, 2)
		assert.ElementsMatch(t, []int{5, 1}, []int{units[0].size, units[1].size})
		assert.Equal(t, 1, batches)
	})
}

func TestMetricBillingKey_DimensionOrderIndependent(t *testing.T) {
	first := []cwtypes.Dimension{
		{Name: aws.String("FunctionName"), Value: aws.String("fn-1")},
		{Name: aws.String("Resource"), Value: aws.String("fn-1:alias")},
	}
	second := []cwtypes.Dimension{first[1], first[0]}
	firstID := metricDimensionID("AWS/Lambda", first)
	secondID := metricDimensionID("AWS/Lambda", second)
	assert.Equal(t, firstID, secondID)
	assert.Equal(t, metricBillingKey("AWS/Lambda", "Invocations", firstID), metricBillingKey("AWS/Lambda", "Invocations", secondID))
	assert.NotEqual(t, metricBillingKey("AWS/Lambda", "Invocations", firstID), metricBillingKey("AWS/Lambda", "Errors", firstID))
}

func TestValidatePlannedQueryWork_Boundaries(t *testing.T) {
	basePolicy := queryPolicy{period: time.Minute, lookback: time.Minute}
	makeQueries := func(count int, policy queryPolicy) []plannedQuery {
		queries := make([]plannedQuery, count)
		for i := range queries {
			queries[i] = plannedQuery{key: testStructuralID(fmt.Sprintf("q%d", i)), target: "base", region: "us-east-1", policy: policy}
		}
		return queries
	}

	tests := map[string]struct {
		count   int
		policy  queryPolicy
		mutate  func([]plannedQuery)
		wantErr string
	}{
		"exact query and batch maximum": {count: maxPlannedQueries, policy: basePolicy},
		"query maximum exceeded":        {count: maxPlannedQueries + 1, policy: basePolicy, wantErr: "maximum 20000"},
		"datapoint maximum exceeded": {
			count:   maxPlannedDatapoints/maxQueryBuckets + 2,
			policy:  queryPolicy{period: time.Minute, lookback: maxQueryBuckets * time.Minute},
			wantErr: "more than 600000 datapoints",
		},
		"batch maximum exceeded": {
			count: maxPlannedQueryBatches + 1, policy: basePolicy,
			mutate: func(queries []plannedQuery) {
				for i := range queries {
					queries[i].target = fmt.Sprintf("target-%d", i)
				}
			},
			wantErr: "more than 40 GetMetricData batches",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			queries := makeQueries(tc.count, tc.policy)
			if tc.mutate != nil {
				tc.mutate(queries)
			}
			err := validatePlannedQueryWork(queries)
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}
