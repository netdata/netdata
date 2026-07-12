// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// CloudWatch bills up to five statistics requested for the same metric in one
// GetMetricData call as one metric request. A billing unit must therefore stay
// whole when point-aware batching narrows a request below 500 queries.
const maxStatisticsPerMetricBillingUnit = 5

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

type queryWorkBudget struct {
	queries         int
	totalDatapoints int
	billingGroups   map[queryBatchKey]map[string]int
}

func newQueryWorkBudget() *queryWorkBudget {
	return &queryWorkBudget{billingGroups: make(map[queryBatchKey]map[string]int)}
}

func (b *queryWorkBudget) reserveQuery(policy queryPolicy) error {
	buckets := policy.bucketCount()
	if buckets <= 0 || buckets > maxQueryBuckets {
		return fmt.Errorf("CloudWatch query plan contains an invalid %d-bucket query", buckets)
	}
	if b.queries >= maxPlannedQueries {
		return fmt.Errorf("CloudWatch query plan would exceed maximum %d queries", maxPlannedQueries)
	}
	if b.totalDatapoints > maxPlannedDatapoints-buckets {
		return fmt.Errorf("CloudWatch query plan requires more than %d datapoints per all-due pass", maxPlannedDatapoints)
	}
	b.queries++
	b.totalDatapoints += buckets
	return nil
}

func (b *queryWorkBudget) addBillingGroup(batchKey queryBatchKey, metricKey string) {
	groups := b.billingGroups[batchKey]
	if groups == nil {
		groups = make(map[string]int)
		b.billingGroups[batchKey] = groups
	}
	groups[metricKey]++
}

func (b *queryWorkBudget) validateBatches() error {
	batches := 0
	for key, groups := range b.billingGroups {
		_, count := packBillingUnitShapes(groups, queryBatchWidth(key.policy))
		batches += count
		if batches > maxPlannedQueryBatches {
			return fmt.Errorf("CloudWatch query plan requires more than %d GetMetricData batches per all-due pass", maxPlannedQueryBatches)
		}
	}
	return nil
}

func validatePlannedQueryWork(plan []plannedQuery) error {
	budget := newQueryWorkBudget()
	for _, query := range plan {
		if err := budget.reserveQuery(query.policy); err != nil {
			return err
		}
		budget.addBillingGroup(query.batchKey(), queryMetricBillingKey(query))
	}
	return budget.validateBatches()
}

type billingUnitShape struct {
	metricKey string
	part      int
	size      int
	batch     int
}

// packBillingUnitShapes applies deterministic first-fit-decreasing packing to
// whole <=5-statistic billing units. Both work validation and execution use the
// returned assignment, so their batch counts cannot drift.
func packBillingUnitShapes(groups map[string]int, width int) ([]billingUnitShape, int) {
	var units []billingUnitShape
	for key, count := range groups {
		for part := 0; count > 0; part++ {
			size := min(count, maxStatisticsPerMetricBillingUnit)
			units = append(units, billingUnitShape{metricKey: key, part: part, size: size})
			count -= size
		}
	}
	slices.SortFunc(units, func(a, b billingUnitShape) int {
		if a.size != b.size {
			return b.size - a.size
		}
		if a.metricKey < b.metricKey {
			return -1
		}
		if a.metricKey > b.metricKey {
			return 1
		}
		return a.part - b.part
	})

	var remaining []int
	for i := range units {
		batch := -1
		for j, capacity := range remaining {
			if capacity >= units[i].size {
				batch = j
				break
			}
		}
		if batch == -1 {
			batch = len(remaining)
			remaining = append(remaining, width)
		}
		remaining[batch] -= units[i].size
		units[i].batch = batch
	}
	return units, len(remaining)
}

func packQueryGroup(queries []plannedQuery, width int) [][]plannedQuery {
	groups := make(map[string][]plannedQuery)
	counts := make(map[string]int)
	for _, query := range queries {
		key := queryMetricBillingKey(query)
		groups[key] = append(groups[key], query)
		counts[key]++
	}
	units, batchCount := packBillingUnitShapes(counts, width)
	batches := make([][]plannedQuery, batchCount)
	for _, unit := range units {
		start := unit.part * maxStatisticsPerMetricBillingUnit
		batches[unit.batch] = append(batches[unit.batch], groups[unit.metricKey][start:start+unit.size]...)
	}
	return batches
}

func queryMetricBillingKey(query plannedQuery) string {
	if query.billingKey != "" {
		return query.billingKey
	}
	if query.query.MetricStat == nil || query.query.MetricStat.Metric == nil {
		return ""
	}
	metric := query.query.MetricStat.Metric
	return metricBillingKey(aws.ToString(metric.Namespace), aws.ToString(metric.MetricName), metric.Dimensions)
}

func metricBillingKey(namespace, metricName string, dimensions []cwtypes.Dimension) string {
	dims := slices.Clone(dimensions)
	slices.SortFunc(dims, func(a, b cwtypes.Dimension) int {
		an, bn := aws.ToString(a.Name), aws.ToString(b.Name)
		if an < bn {
			return -1
		}
		if an > bn {
			return 1
		}
		av, bv := aws.ToString(a.Value), aws.ToString(b.Value)
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
		return 0
	})
	var key strings.Builder
	writeLengthPrefixed(&key, namespace)
	writeLengthPrefixed(&key, metricName)
	for _, dim := range dims {
		writeLengthPrefixed(&key, aws.ToString(dim.Name))
		writeLengthPrefixed(&key, aws.ToString(dim.Value))
	}
	return key.String()
}
