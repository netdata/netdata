// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
)

func TestPlannedQueryKey_StableIdentityContract(t *testing.T) {
	base := testPlannedQuery("", "target", "us-east-1", "AWS/EC2", 300)
	base.seriesName = "ec2.cpu_utilization_average"
	base.labels = []metrix.Label{{Key: "account_id", Value: "000000000000"}, {Key: "instance_id", Value: "i-1"}}
	base.tagLabels = []metrix.Label{{Key: "owner", Value: "old"}}
	base.query.MetricStat.Stat = aws.String("Average")
	base.query.MetricStat.Metric.MetricName = aws.String("CPUUtilization")
	baseKey := plannedQueryKey(base)

	changedTag := base
	changedTag.tagLabels = []metrix.Label{{Key: "owner", Value: "new"}}
	assert.Equal(t, baseKey, plannedQueryKey(changedTag), "mutable tag labels do not change query state identity")

	mutations := map[string]func(*plannedQuery){
		"target":         func(q *plannedQuery) { q.target = "other" },
		"region":         func(q *plannedQuery) { q.region = "us-west-2" },
		"policy":         func(q *plannedQuery) { q.policy.lookback += 5 * time.Minute },
		"series":         func(q *plannedQuery) { q.seriesName = "ec2.other_average" },
		"identity label": func(q *plannedQuery) { q.labels[1].Value = "i-2" },
		"statistic":      func(q *plannedQuery) { q.query.MetricStat.Stat = aws.String("Maximum") },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			query := base
			query.labels = append([]metrix.Label(nil), base.labels...)
			mutate(&query)
			assert.NotEqual(t, baseKey, plannedQueryKey(query))
		})
	}
}
