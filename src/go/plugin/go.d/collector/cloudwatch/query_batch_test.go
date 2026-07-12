// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
