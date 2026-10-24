// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/stretchr/testify/assert"
)

func TestPlannedQueryKey_StableIdentityContract(t *testing.T) {
	profileDims := []cwprofiles.InstanceDimension{{Name: "InstanceId", Label: "instance_id"}}
	newQuery := func() (plannedQuery, structuralID) {
		query := testPlannedQuery("", "target", "us-east-1", "AWS/EC2", 300)
		query.seriesName = "ec2.cpu_utilization_average"
		query.labels = []metrix.Label{{Key: "account_id", Value: "000000000000"}, {Key: "instance_id", Value: "i-1"}}
		query.tagLabels = []metrix.Label{{Key: "owner", Value: "old"}}
		query.query.MetricStat.Stat = aws.String("Average")
		query.query.MetricStat.Metric.MetricName = aws.String("CPUUtilization")
		query.query.MetricStat.Metric.Dimensions = []cwtypes.Dimension{{Name: aws.String("InstanceId"), Value: aws.String("i-1")}}
		dimensionID := metricDimensionID("AWS/EC2", query.query.MetricStat.Metric.Dimensions)
		query.billingKey = metricBillingKey("AWS/EC2", "CPUUtilization", dimensionID)
		instanceID := finalInstanceID("ec2", "000000000000", "us-east-1", profileDims, []string{"i-1"})
		return query, instanceID
	}
	refreshBillingKey := func(query *plannedQuery) {
		metric := query.query.MetricStat.Metric
		namespace := aws.ToString(metric.Namespace)
		query.billingKey = metricBillingKey(namespace, aws.ToString(metric.MetricName), metricDimensionID(namespace, metric.Dimensions))
	}

	base, baseInstanceID := newQuery()
	baseKey := plannedQueryKey(base, baseInstanceID)

	changedTag, changedTagInstanceID := newQuery()
	changedTag.tagLabels = []metrix.Label{{Key: "owner", Value: "new"}}
	assert.Equal(t, baseKey, plannedQueryKey(changedTag, changedTagInstanceID), "mutable tag labels do not change query state identity")

	mutations := map[string]func(*plannedQuery, *structuralID){
		"target":            func(q *plannedQuery, _ *structuralID) { q.target = "other" },
		"region":            func(q *plannedQuery, _ *structuralID) { q.region = "us-west-2" },
		"policy period":     func(q *plannedQuery, _ *structuralID) { q.policy.Period += time.Minute },
		"lookback":          func(q *plannedQuery, _ *structuralID) { q.policy.Lookback += 5 * time.Minute },
		"publication delay": func(q *plannedQuery, _ *structuralID) { q.policy.PublicationDelay += time.Minute },
		"series":            func(q *plannedQuery, _ *structuralID) { q.seriesName = "ec2.other_average" },
		"identity": func(_ *plannedQuery, id *structuralID) {
			*id = finalInstanceID("ec2", "000000000000", "us-east-1", profileDims, []string{"i-2"})
		},
		"namespace": func(q *plannedQuery, _ *structuralID) {
			q.query.MetricStat.Metric.Namespace = aws.String("AWS/RDS")
			refreshBillingKey(q)
		},
		"metric name": func(q *plannedQuery, _ *structuralID) {
			q.query.MetricStat.Metric.MetricName = aws.String("Other")
			refreshBillingKey(q)
		},
		"statistic":    func(q *plannedQuery, _ *structuralID) { q.query.MetricStat.Stat = aws.String("Maximum") },
		"query period": func(q *plannedQuery, _ *structuralID) { q.query.MetricStat.Period = aws.Int32(60) },
		"dimension name": func(q *plannedQuery, _ *structuralID) {
			q.query.MetricStat.Metric.Dimensions[0].Name = aws.String("OtherId")
			refreshBillingKey(q)
		},
		"dimension value": func(q *plannedQuery, _ *structuralID) {
			q.query.MetricStat.Metric.Dimensions[0].Value = aws.String("i-2")
			refreshBillingKey(q)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			query, instanceID := newQuery()
			mutate(&query, &instanceID)
			assert.NotEqual(t, baseKey, plannedQueryKey(query, instanceID))
		})
	}
}

func TestFinalInstanceID_IdentityContract(t *testing.T) {
	constant := "fixed"
	dimensions := []cwprofiles.InstanceDimension{
		{Name: "InstanceId", Label: "instance_id"},
		{Name: "Kind", Constant: &constant},
	}
	base := finalInstanceID("profile", "000000000000", "us-east-1", dimensions, []string{"i-1", "fixed"})
	renamed := append([]cwprofiles.InstanceDimension(nil), dimensions...)
	renamed[0].Label = "resource_id"

	tests := map[string]struct {
		profile    string
		account    string
		region     string
		dimensions []cwprofiles.InstanceDimension
		values     []string
		wantEqual  bool
	}{
		"constant dimension excluded": {
			profile: "profile", account: "000000000000", region: "us-east-1",
			dimensions: dimensions, values: []string{"i-1", "other"}, wantEqual: true,
		},
		"profile changes identity": {
			profile: "other", account: "000000000000", region: "us-east-1",
			dimensions: dimensions, values: []string{"i-1", "fixed"},
		},
		"account changes identity": {
			profile: "profile", account: "111111111111", region: "us-east-1",
			dimensions: dimensions, values: []string{"i-1", "fixed"},
		},
		"region changes identity": {
			profile: "profile", account: "000000000000", region: "us-west-2",
			dimensions: dimensions, values: []string{"i-1", "fixed"},
		},
		"dimension value changes identity": {
			profile: "profile", account: "000000000000", region: "us-east-1",
			dimensions: dimensions, values: []string{"i-2", "fixed"},
		},
		"dimension label changes identity": {
			profile: "profile", account: "000000000000", region: "us-east-1",
			dimensions: renamed, values: []string{"i-1", "fixed"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := finalInstanceID(tc.profile, tc.account, tc.region, tc.dimensions, tc.values)
			if tc.wantEqual {
				assert.Equal(t, base, got)
			} else {
				assert.NotEqual(t, base, got)
			}
		})
	}
}
