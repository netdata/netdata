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
		key: key, target: target, region: region,
		policy: queryPolicy{period: time.Duration(period) * time.Second, lookback: time.Duration(period) * time.Second, publicationDelay: defaultPublicationDelay},
		query: cwtypes.MetricDataQuery{
			MetricStat: &cwtypes.MetricStat{Metric: &cwtypes.Metric{Namespace: aws.String(namespace)}, Period: aws.Int32(int32(period))},
		},
	}
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

func TestValidatePlannedQueryWork_Boundaries(t *testing.T) {
	basePolicy := queryPolicy{period: time.Minute, lookback: time.Minute}
	makeQueries := func(count int, policy queryPolicy) []plannedQuery {
		queries := make([]plannedQuery, count)
		for i := range queries {
			queries[i] = plannedQuery{key: fmt.Sprintf("q%d", i), target: "base", region: "us-east-1", policy: policy}
		}
		return queries
	}

	t.Run("exact query and batch maximum", func(t *testing.T) {
		queries := makeQueries(maxPlannedQueries, basePolicy)
		require.NoError(t, validatePlannedQueryWork(queries))
	})

	t.Run("query maximum exceeded", func(t *testing.T) {
		err := validatePlannedQueryWork(makeQueries(maxPlannedQueries+1, basePolicy))
		assert.ErrorContains(t, err, "maximum is 20000")
	})

	t.Run("datapoint maximum exceeded", func(t *testing.T) {
		policy := queryPolicy{period: time.Minute, lookback: maxQueryBuckets * time.Minute}
		err := validatePlannedQueryWork(makeQueries(maxPlannedDatapoints/maxQueryBuckets+2, policy))
		assert.ErrorContains(t, err, "more than 600000 datapoints")
	})

	t.Run("batch maximum exceeded", func(t *testing.T) {
		queries := makeQueries(maxPlannedQueryBatches+1, basePolicy)
		for i := range queries {
			queries[i].target = fmt.Sprintf("target-%d", i)
		}
		err := validatePlannedQueryWork(queries)
		assert.ErrorContains(t, err, "more than 40 GetMetricData batches")
	})
}
