// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import "fmt"

type queryBatchKey struct {
	target, region string
	policy         queryPolicy
}

func (q plannedQuery) batchKey() queryBatchKey {
	return queryBatchKey{target: q.target, region: q.region, policy: q.policy}
}

func queryBatchWidth(policy queryPolicy) int {
	return min(metricsPerQuery, maxDatapointsPerRequest/policy.bucketCount())
}

func validatePlannedQueryWork(plan []plannedQuery) error {
	if len(plan) > maxPlannedQueries {
		return fmt.Errorf("CloudWatch query plan contains %d queries; maximum is %d", len(plan), maxPlannedQueries)
	}

	groups := make(map[queryBatchKey]int)
	totalDatapoints := 0
	for _, query := range plan {
		buckets := query.policy.bucketCount()
		if buckets <= 0 || buckets > maxQueryBuckets {
			return fmt.Errorf("CloudWatch query plan contains an invalid %d-bucket query", buckets)
		}
		if totalDatapoints > maxPlannedDatapoints-buckets {
			return fmt.Errorf("CloudWatch query plan requires more than %d datapoints per all-due pass", maxPlannedDatapoints)
		}
		totalDatapoints += buckets
		groups[query.batchKey()]++
	}

	batches := 0
	for key, count := range groups {
		width := queryBatchWidth(key.policy)
		batches += (count + width - 1) / width
		if batches > maxPlannedQueryBatches {
			return fmt.Errorf("CloudWatch query plan requires more than %d GetMetricData batches per all-due pass", maxPlannedQueryBatches)
		}
	}
	return nil
}
