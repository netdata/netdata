// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
)

func TestPlannedQueryKey_StableIdentityContract(t *testing.T) {
	newQuery := func() plannedQuery {
		query := testPlannedQuery("", "target", "us-east-1", "AWS/EC2", 300)
		query.seriesName = "ec2.cpu_utilization_average"
		query.labels = []metrix.Label{{Key: "account_id", Value: "000000000000"}, {Key: "instance_id", Value: "i-1"}}
		query.tagLabels = []metrix.Label{{Key: "owner", Value: "old"}}
		query.query.MetricStat.Stat = aws.String("Average")
		query.query.MetricStat.Metric.MetricName = aws.String("CPUUtilization")
		query.query.MetricStat.Metric.Dimensions = []cwtypes.Dimension{{Name: aws.String("InstanceId"), Value: aws.String("i-1")}}
		return query
	}

	base := newQuery()
	baseKey := plannedQueryKey(base)

	changedTag := newQuery()
	changedTag.tagLabels = []metrix.Label{{Key: "owner", Value: "new"}}
	assert.Equal(t, baseKey, plannedQueryKey(changedTag), "mutable tag labels do not change query state identity")

	mutations := map[string]func(*plannedQuery){
		"target":            func(q *plannedQuery) { q.target = "other" },
		"region":            func(q *plannedQuery) { q.region = "us-west-2" },
		"policy period":     func(q *plannedQuery) { q.policy.period += time.Minute },
		"lookback":          func(q *plannedQuery) { q.policy.lookback += 5 * time.Minute },
		"publication delay": func(q *plannedQuery) { q.policy.publicationDelay += time.Minute },
		"series":            func(q *plannedQuery) { q.seriesName = "ec2.other_average" },
		"identity label":    func(q *plannedQuery) { q.labels[1].Value = "i-2" },
		"namespace":         func(q *plannedQuery) { q.query.MetricStat.Metric.Namespace = aws.String("AWS/RDS") },
		"metric name":       func(q *plannedQuery) { q.query.MetricStat.Metric.MetricName = aws.String("Other") },
		"statistic":         func(q *plannedQuery) { q.query.MetricStat.Stat = aws.String("Maximum") },
		"query period":      func(q *plannedQuery) { q.query.MetricStat.Period = aws.Int32(60) },
		"dimension name":    func(q *plannedQuery) { q.query.MetricStat.Metric.Dimensions[0].Name = aws.String("OtherId") },
		"dimension value":   func(q *plannedQuery) { q.query.MetricStat.Metric.Dimensions[0].Value = aws.String("i-2") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			query := newQuery()
			mutate(&query)
			assert.NotEqual(t, baseKey, plannedQueryKey(query))
		})
	}
}
