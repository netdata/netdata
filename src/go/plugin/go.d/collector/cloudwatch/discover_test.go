// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCloudWatch returns the configured pages in sequence and records inputs.
type fakeCloudWatch struct {
	pages   []*cloudwatch.ListMetricsOutput
	pageIdx int
	inputs  []*cloudwatch.ListMetricsInput
	err     error

	getMetricData func(context.Context, *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error)
}

type cancelingListMetrics struct {
	cancel context.CancelFunc
	out    *cloudwatch.ListMetricsOutput
}

type staticListMetrics struct {
	out *cloudwatch.ListMetricsOutput
}

func (f cancelingListMetrics) ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.cancel()
	return f.out, nil
}

func (cancelingListMetrics) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return nil, nil
}

func (f staticListMetrics) ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return f.out, nil
}

func (staticListMetrics) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return nil, nil
}

func (f *fakeCloudWatch) ListMetrics(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.inputs = append(f.inputs, in)
	if f.err != nil {
		return nil, f.err
	}
	out := f.pages[f.pageIdx]
	f.pageIdx++
	return out, nil
}

func (f *fakeCloudWatch) GetMetricData(ctx context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	if f.getMetricData != nil {
		return f.getMetricData(ctx, in)
	}
	return completeNoData(in), nil
}

func completeNoData(in *cloudwatch.GetMetricDataInput) *cloudwatch.GetMetricDataOutput {
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, query := range in.MetricDataQueries {
		results = append(results, cwtypes.MetricDataResult{Id: query.Id, StatusCode: cwtypes.StatusCodeComplete})
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}
}

func mkMetric(name string, nameValuePairs ...string) cwtypes.Metric {
	var dims []cwtypes.Dimension
	for i := 0; i+1 < len(nameValuePairs); i += 2 {
		dims = append(dims, cwtypes.Dimension{
			Name:  aws.String(nameValuePairs[i]),
			Value: aws.String(nameValuePairs[i+1]),
		})
	}
	return cwtypes.Metric{MetricName: aws.String(name), Dimensions: dims}
}

func page(metrics []cwtypes.Metric, nextToken string) *cloudwatch.ListMetricsOutput {
	out := &cloudwatch.ListMetricsOutput{Metrics: metrics}
	if nextToken != "" {
		out.NextToken = aws.String(nextToken)
	}
	return out
}

func dimProfile(namespace string, period int, dimNames ...string) cwprofiles.Profile {
	dims := make([]cwprofiles.InstanceDimension, len(dimNames))
	for i, n := range dimNames {
		dims[i] = cwprofiles.InstanceDimension{Name: n, Label: n}
	}
	return cwprofiles.Profile{
		Namespace: namespace,
		Period:    period,
		Instance:  cwprofiles.InstanceSpec{Dimensions: dims},
		Metrics:   []cwprofiles.Metric{{ID: "m", MetricName: "M", Statistics: []string{"average"}}},
	}
}

func discoverOneProfile(ctx context.Context, client cloudwatchClient, profile cwprofiles.Profile, useRecentlyActive bool) ([]discoveredInstance, error) {
	const profileName = "test"
	instances, err := discoverProfileGroup(ctx, client, discoveryGroup{
		Namespace: profile.Namespace, RecentlyActive: useRecentlyActive,
		Profiles: []cwprofiles.ResolvedProfile{{Name: profileName, Config: profile}},
	})
	return instances[profileName], err
}

func TestDiscoverProfileGroup(t *testing.T) {
	// discoverOneProfile lists one page and keeps only the metrics whose dimension-NAME
	// set exactly equals the profile's, collapsing CloudWatch's multi-granularity
	// fan-out to one deduped instance per identity, in list order.
	tests := map[string]struct {
		metrics        []cwtypes.Metric
		namespace      string
		period         int
		dims           []string
		recentlyActive bool
		wantDimValues  [][]string
	}{
		"ALB exact filter collapses multi-granularity + dedups to one per {LoadBalancer}": {
			metrics: []cwtypes.Metric{
				mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa"),
				mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "TargetGroup", "tg/x/1"),
				mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "AvailabilityZone", "us-east-1a"),
				mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "TargetGroup", "tg/x/1", "AvailabilityZone", "us-east-1a"),
				mkMetric("ProcessedBytes", "LoadBalancer", "app/lb1/aaa"), // same instance as first -> dedup
				mkMetric("RequestCount", "LoadBalancer", "app/lb2/bbb"),   // second instance
			},
			namespace:      "AWS/ApplicationELB",
			period:         300,
			dims:           []string{"LoadBalancer"},
			recentlyActive: true,
			wantDimValues:  [][]string{{"app/lb1/aaa"}, {"app/lb2/bbb"}},
		},
		"S3 two-dimension identity keeps exact {BucketName,StorageType}, rejects wrong cardinality": {
			metrics: []cwtypes.Metric{
				mkMetric("BucketSizeBytes", "BucketName", "b1", "StorageType", "StandardStorage"),
				mkMetric("NumberOfObjects", "BucketName", "b1", "StorageType", "AllStorageTypes"),
				mkMetric("BucketSizeBytes", "StorageType", "StandardStorage", "BucketName", "b2"), // AWS order is irrelevant
				mkMetric("BucketSizeBytes", "BucketName", "b1"),                                   // wrong cardinality -> rejected
			},
			namespace:      "AWS/S3",
			period:         86400,
			dims:           []string{"BucketName", "StorageType"},
			recentlyActive: false,
			wantDimValues:  [][]string{{"b1", "StandardStorage"}, {"b1", "AllStorageTypes"}, {"b2", "StandardStorage"}},
		},
		"wrong dimension name rejected (exact name-set, not just cardinality)": {
			metrics: []cwtypes.Metric{
				mkMetric("CPUUtilization", "InstanceId", "i-1"),
				mkMetric("CPUUtilization", "ImageId", "ami-1"), // 1 dim, wrong name -> rejected
			},
			namespace:      "AWS/EC2",
			period:         300,
			dims:           []string{"InstanceId"},
			recentlyActive: true,
			wantDimValues:  [][]string{{"i-1"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(tc.metrics, "")}}
			prof := dimProfile(tc.namespace, tc.period, tc.dims...)

			got, err := discoverOneProfile(context.Background(), client, prof, tc.recentlyActive)
			require.NoError(t, err)

			gotDimValues := make([][]string, len(got))
			for i, inst := range got {
				gotDimValues[i] = inst.DimensionValues
			}
			assert.Equal(t, tc.wantDimValues, gotDimValues)
		})
	}
}

func TestDiscoverProfileGroup_CancellationStopsLocalMatching(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := cancelingListMetrics{
		cancel: cancel,
		out:    &cloudwatch.ListMetricsOutput{Metrics: []cwtypes.Metric{mkMetric("M", "Id", "one")}},
	}
	profile := dimProfile("AWS/Test", 300, "Id")

	_, err := discoverOneProfile(ctx, client, profile, false)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCanonicalMetricDimensions_DuplicateNameRejected(t *testing.T) {
	// Same cardinality as the profile but a repeated dimension name must not match.
	dims := []cwtypes.Dimension{
		{Name: aws.String("InstanceId"), Value: aws.String("i-1")},
		{Name: aws.String("InstanceId"), Value: aws.String("i-2")},
	}
	_, _, ok := canonicalMetricDimensions(dims)
	assert.False(t, ok)
}

func TestDiscoveryMatcher_ConstantDimensionsHold(t *testing.T) {
	tests := map[string]struct {
		constants []discoveryConstant
		values    []string
		want      bool
	}{
		"no constant dimensions": {
			values: []string{"i-1"},
			want:   true,
		},
		"constant value matches": {
			constants: []discoveryConstant{{index: 1, value: "Global"}},
			values:    []string{"E1", "Global"},
			want:      true,
		},
		"constant value mismatch fails closed": {
			constants: []discoveryConstant{{index: 1, value: "Global"}},
			values:    []string{"E1", "us-east-1"},
			want:      false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			matcher := discoveryMatcher{constants: tc.constants}
			assert.Equal(t, tc.want, matcher.constantsHold(tc.values))
		})
	}
}

func TestDiscoverProfileGroup_ConstantDimensionFailClosed(t *testing.T) {
	// A constant dimension is part of the exact NAME-set match, but only metrics
	// whose value equals the pinned constant are kept (fail-closed). The constant
	// value is retained in DimensionValues so it can be sent in the query.
	prof := cwprofiles.Profile{
		Namespace: "AWS/CloudFront",
		Period:    300,
		Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{
			{Name: "DistributionId", Label: "distribution_id"},
			{Name: "Region", Constant: aws.String("Global")},
		}},
	}
	metrics := []cwtypes.Metric{
		mkMetric("Requests", "DistributionId", "E1", "Region", "Global"),    // kept
		mkMetric("Requests", "DistributionId", "E2", "Region", "Global"),    // kept (dedup vs E1 by identity)
		mkMetric("Requests", "DistributionId", "E3", "Region", "us-east-1"), // dropped: constant mismatch
		mkMetric("Requests", "DistributionId", "E4"),                        // dropped: wrong cardinality
	}
	client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(metrics, "")}}

	got, err := discoverOneProfile(context.Background(), client, prof, false)
	require.NoError(t, err)

	gotDimValues := make([][]string, len(got))
	for i, inst := range got {
		gotDimValues[i] = inst.DimensionValues
	}
	assert.Equal(t, [][]string{{"E1", "Global"}, {"E2", "Global"}}, gotDimValues)
}

func TestDiscoverProfileGroup_Pagination(t *testing.T) {
	client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{
		page([]cwtypes.Metric{mkMetric("CPUUtilization", "InstanceId", "i-1")}, "next"),
		page([]cwtypes.Metric{mkMetric("CPUUtilization", "InstanceId", "i-2")}, ""),
	}}

	prof := dimProfile("AWS/EC2", 300, "InstanceId")
	got, err := discoverOneProfile(context.Background(), client, prof, true)
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.Equal(t, []string{"i-1"}, got[0].DimensionValues)
	assert.Equal(t, []string{"i-2"}, got[1].DimensionValues)

	require.Len(t, client.inputs, 2)
	assert.Nil(t, client.inputs[0].NextToken)
	require.NotNil(t, client.inputs[1].NextToken)
	assert.Equal(t, "next", *client.inputs[1].NextToken)
}

func TestDiscoverProfileGroup_RecentlyActiveAndNamespace(t *testing.T) {
	tests := map[string]struct {
		useRecentlyActive  bool
		wantRecentlyActive cwtypes.RecentlyActive
	}{
		"enabled sets PT3H": {useRecentlyActive: true, wantRecentlyActive: cwtypes.RecentlyActivePt3h},
		"disabled omits it": {useRecentlyActive: false, wantRecentlyActive: ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(nil, "")}}
			prof := dimProfile("AWS/EC2", 300, "InstanceId")

			_, err := discoverOneProfile(context.Background(), client, prof, tc.useRecentlyActive)
			require.NoError(t, err)

			require.Len(t, client.inputs, 1)
			in := client.inputs[0]
			require.NotNil(t, in.Namespace)
			assert.Equal(t, "AWS/EC2", *in.Namespace)
			assert.Nil(t, in.MetricName, "MetricName must be omitted (list all)")
			assert.Equal(t, tc.wantRecentlyActive, in.RecentlyActive)
		})
	}
}

func TestDiscoverProfileGroup_EmptyAndError(t *testing.T) {
	prof := dimProfile("AWS/EC2", 300, "InstanceId")

	empty := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(nil, "")}}
	got, err := discoverOneProfile(context.Background(), empty, prof, true)
	require.NoError(t, err)
	assert.Empty(t, got)

	failing := &fakeCloudWatch{err: errors.New("access denied")}
	_, err = discoverOneProfile(context.Background(), failing, prof, true)
	assert.Error(t, err)
}

// nsCloudWatch is a thread-safe fake that returns metrics keyed by the requested
// namespace (one client is shared across a region's profiles in discoverAll).
type nsCloudWatch struct {
	mu    sync.Mutex
	byNS  map[string][]cwtypes.Metric
	calls int
}

func (f *nsCloudWatch) ListMetrics(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return &cloudwatch.ListMetricsOutput{Metrics: f.byNS[aws.ToString(in.Namespace)]}, nil
}

func (f *nsCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return completeNoData(in), nil
}

func resolved(name string, prof cwprofiles.Profile) cwprofiles.ResolvedProfile {
	return cwprofiles.ResolvedProfile{Name: name, Config: prof}
}

func TestDiscoverAll(t *testing.T) {
	ec2 := resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))
	s3 := resolved("s3", dimProfile("AWS/S3", 86400, "BucketName", "StorageType"))

	regionData := map[string]map[string][]cwtypes.Metric{
		"us-east-1": {
			"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
			"AWS/S3":  {mkMetric("BucketSizeBytes", "BucketName", "b1", "StorageType", "StandardStorage")},
		},
		"us-west-2": {
			"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-2"), mkMetric("CPUUtilization", "InstanceId", "i-3")},
			"AWS/S3":  nil, // no buckets in this region
		},
	}

	newClient := func(_, region string) (cloudwatchClient, error) {
		return &nsCloudWatch{byNS: regionData[region]}, nil
	}

	var groups []discoveryGroup
	for _, region := range []string{"us-east-1", "us-west-2"} {
		groups = append(groups,
			discoveryGroup{Target: "base", Region: region, Namespace: "AWS/EC2", RecentlyActive: true, Profiles: []cwprofiles.ResolvedProfile{ec2}},
			discoveryGroup{Target: "base", Region: region, Namespace: "AWS/S3", Profiles: []cwprofiles.ResolvedProfile{s3}},
		)
	}
	results := discoverAll(context.Background(), newClient, groups, 5, 0)
	require.Len(t, results, 4)

	snap, failures := buildDiscoverySnapshot(results, nil, time.Unix(1000, 0), 300)
	require.Zero(t, failures)

	assert.Equal(t, 4, snap.totalInstances())
	assert.Equal(t, [][]string{{"i-1"}}, dimValues(snap.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}]))
	assert.Equal(t, [][]string{{"i-2"}, {"i-3"}}, dimValues(snap.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-west-2"}]))
	assert.Equal(t, [][]string{{"b1", "StandardStorage"}}, dimValues(snap.Instances[discoveryKey{Target: "base", Profile: "s3", Region: "us-east-1"}]))
	assert.NotContains(t, snap.Instances, discoveryKey{Target: "base", Profile: "s3", Region: "us-west-2"}, "empty target must be omitted")

	assert.Equal(t, time.Unix(1000, 0), snap.FetchedAt)
	assert.Equal(t, time.Unix(1300, 0), snap.ExpiresAt)
}

func TestDiscoverAll_UsesCompiledGroupNamespace(t *testing.T) {
	profile := resolved("custom", dimProfile("WRONG/ProfileNamespace", 300, "InstanceId"))
	fake := &nsCloudWatch{byNS: map[string][]cwtypes.Metric{
		"AWS/CompiledNamespace": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
	}}
	results := discoverAll(context.Background(), func(_, _ string) (cloudwatchClient, error) { return fake, nil }, []discoveryGroup{{
		Target: "base", Region: "us-east-1", Namespace: "AWS/CompiledNamespace", Profiles: []cwprofiles.ResolvedProfile{profile},
	}}, 1, 0)

	require.Len(t, results, 1)
	assert.Equal(t, [][]string{{"i-1"}}, dimValues(results[0].Instances["custom"]))
}

func TestDiscoverAll_ClientBuildErrorIsPerTarget(t *testing.T) {
	ec2 := resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))

	newClient := func(_, region string) (cloudwatchClient, error) {
		if region == "bad-region" {
			return nil, errors.New("no credentials for region")
		}
		return &nsCloudWatch{byNS: map[string][]cwtypes.Metric{
			"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
		}}, nil
	}

	results := discoverAll(context.Background(), newClient, []discoveryGroup{
		{Target: "base", Region: "us-east-1", Namespace: "AWS/EC2", RecentlyActive: true, Profiles: []cwprofiles.ResolvedProfile{ec2}},
		{Target: "base", Region: "bad-region", Namespace: "AWS/EC2", RecentlyActive: true, Profiles: []cwprofiles.ResolvedProfile{ec2}},
	}, 5, 0)

	snap, failures := buildDiscoverySnapshot(results, nil, time.Unix(1000, 0), 300)
	require.Equal(t, 1, failures)
	assert.Equal(t, 1, snap.totalInstances())
	assert.Contains(t, snap.Instances, discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"})
}

func TestCollector_DiscoveryGroupsDeduplicateCompatibleNamespaceScans(t *testing.T) {
	falseValue := false
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{
		Defaults: &falseValue,
		Include:  []string{"alb", "alb_target_health"},
	}
	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	c := New()
	c.plan = plan
	resolved := resolvedTarget{target: plan.Targets[0], accountID: "000000000000"}
	c.resolvedByRef = map[string]resolvedTarget{"base": resolved}

	groups := c.discoveryGroups()
	require.Len(t, groups, 1)
	assert.Equal(t, "AWS/ApplicationELB", groups[0].Namespace)
	assert.Equal(t, []string{"alb", "alb_target_health"}, []string{groups[0].Profiles[0].Name, groups[0].Profiles[1].Name})

	fake := &nsCloudWatch{byNS: map[string][]cwtypes.Metric{"AWS/ApplicationELB": {}}}
	results := discoverAll(context.Background(), func(_, _ string) (cloudwatchClient, error) { return fake, nil }, groups, 5, 0)
	require.Len(t, results, 1, "one shared scan produces one group result")
	assert.Len(t, results[0].Instances, 0)
	assert.Equal(t, 1, fake.calls, "compatible profiles share one ListMetrics scan")
}

func TestCollector_DiscoveryGroupsDeduplicateProfileAcrossTagPolicies(t *testing.T) {
	falseValue := false
	filters := func(value string) *RuleFiltersConfig {
		entries := []ResourceTagFilterConfig{{Key: "environment", Values: []string{value}}}
		return &RuleFiltersConfig{ResourceTags: &entries}
	}
	cfg := validBaseConfig()
	cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}}
	cfg.Rules[0].Filters = filters("production")
	cfg.Rules = append(cfg.Rules, RuleConfig{
		Name: "staging", Targets: []string{"base"}, Regions: []string{"us-east-1"},
		Profiles: &ProfileSelectorConfig{Defaults: &falseValue, Include: []string{"ec2"}},
		Filters:  filters("staging"),
	})
	plan, _, err := compileTestConfig(t, cfg)
	require.NoError(t, err)
	require.Len(t, plan.Scopes, 2, "different tag policies remain distinct collection scopes")
	assert.NotNil(t, plan.TagJoins["ec2"], "profile association is validated and compiled once")

	c := New()
	c.plan = plan
	c.resolvedByRef = map[string]resolvedTarget{
		"base": {target: plan.Targets[0], accountID: "000000000000"},
	}

	groups := c.discoveryGroups()
	require.Len(t, groups, 1)
	require.Len(t, groups[0].Profiles, 1, "shared discovery contains one matcher per profile")
	assert.Equal(t, "ec2", groups[0].Profiles[0].Name)
}

func TestCollector_DiscoveryGroupsCoalesceToLeastRestrictiveRecentlyActivePolicy(t *testing.T) {
	c := New()
	fastProfile := dimProfile("AWS/Shared", 300, "Id")
	fastProfile.Metrics = append(fastProfile.Metrics,
		cwprofiles.Metric{ID: "daily", MetricName: "Daily", Statistics: []string{"average"}, Period: 86400})
	fast := resolved("fast", fastProfile)
	slow := resolved("slow", dimProfile("AWS/Shared", 86400, "Id"))
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{fast, slow})
	c.plan.Scopes[0].SelectedSeries = c.plan.Scopes[0].SelectedSeries[:1]

	groups := c.discoveryGroups()
	require.Len(t, groups, 1, "one namespace must produce one ListMetrics scan")
	assert.False(t, groups[0].RecentlyActive, "one long-period profile makes the shared scan unfiltered")
	assert.Equal(t, []string{"fast", "slow"}, []string{groups[0].Profiles[0].Name, groups[0].Profiles[1].Name})

	fake := &nsCloudWatch{byNS: map[string][]cwtypes.Metric{"AWS/Shared": {}}}
	results := discoverAll(context.Background(), func(_, _ string) (cloudwatchClient, error) { return fake, nil }, groups, 5, 0)
	require.Len(t, results, 1)
	assert.Equal(t, 1, fake.calls, "the unfiltered superset scan replaces a redundant filtered sibling scan")
}

func TestCollector_DiscoveryGroupsUseUnionOfSelectedSeriesPolicies(t *testing.T) {
	c := New()
	profile := cwprofiles.ResolvedProfile{Name: "mixed", Config: cwprofiles.Profile{
		Namespace: "AWS/Shared", Period: 300,
		Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "Id", Label: "id"}}},
		Metrics: []cwprofiles.Metric{
			{ID: "fast", MetricName: "Fast", Statistics: []string{"average"}},
			{ID: "slow", MetricName: "Slow", Statistics: []string{"average"}, Period: 86400},
		},
	}}
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{profile})
	all := testCompiledSeries(profile)
	c.plan.Scopes[0].SelectedSeries = all[:1]
	second := c.plan.Scopes[0]
	second.SelectedSeries = all[1:]
	c.plan.Scopes = append(c.plan.Scopes, second)

	groups := c.discoveryGroups()
	require.Len(t, groups, 1, "one profile must not trigger duplicate ListMetrics streams")
	assert.False(t, groups[0].RecentlyActive, "one selected daily series disables PT3H for the shared profile matcher")
}

func TestBuildDiscoverySnapshot_FailSoftCarriesForward(t *testing.T) {
	prev := map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
		{Target: "base", Profile: "ec2", Region: "us-west-2"}: {{DimensionValues: []string{"i-9"}}},
	}
	profile := resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))
	results := []discoveryGroupResult{
		{Group: discoveryGroup{Target: "base", Region: "us-east-1", Profiles: []cwprofiles.ResolvedProfile{profile}}, Instances: map[string][]discoveredInstance{"ec2": {{DimensionValues: []string{"i-2"}}}}},
		{Group: discoveryGroup{Target: "base", Region: "us-west-2", Profiles: []cwprofiles.ResolvedProfile{profile}}, Err: errors.New("throttled")},
	}

	snap, failures := buildDiscoverySnapshot(results, prev, time.Unix(1000, 0), 300)
	require.Equal(t, 1, failures)
	assert.Equal(t, [][]string{{"i-2"}}, dimValues(snap.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}]), "succeeded target is refreshed")
	assert.Equal(t, [][]string{{"i-9"}}, dimValues(snap.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-west-2"}]), "failed target carries forward last-known instances")
}

func TestDiscoverySnapshot_Expired(t *testing.T) {
	var zero discoverySnapshot
	assert.True(t, zero.expired(time.Unix(1000, 0)), "never-fetched snapshot is expired")

	snap := discoverySnapshot{FetchedAt: time.Unix(1000, 0), ExpiresAt: time.Unix(1300, 0)}
	assert.False(t, snap.expired(time.Unix(1299, 0)))
	assert.True(t, snap.expired(time.Unix(1300, 0)))
	assert.True(t, snap.expired(time.Unix(1500, 0)))
}

func BenchmarkDiscoverProfileGroupPolicyCount(b *testing.B) {
	const instances = 1000
	catalog, err := cwprofiles.DefaultCatalog()
	if err != nil {
		b.Fatal(err)
	}
	metrics := make([]cwtypes.Metric, instances)
	for i := range instances {
		metrics[i] = mkMetric("CPUUtilization", "InstanceId", fmt.Sprintf("i-%d", i))
	}
	for _, policies := range []int{1, 4} {
		b.Run(fmt.Sprintf("policies=%d", policies), func(b *testing.B) {
			cfg := resourceTagPolicyConfig(policies)
			cfg.applyDefaults()
			plan, _, err := compileConfig(cfg, catalog)
			if err != nil {
				b.Fatal(err)
			}
			c := New()
			c.plan = plan
			c.resolvedByRef = map[string]resolvedTarget{
				"base": {target: plan.Targets[0], accountID: "000000000000"},
			}
			groups := c.discoveryGroups()
			if len(groups) != 1 || len(groups[0].Profiles) != 1 {
				b.Fatalf("got %d groups with %d profiles, want one shared group/profile", len(groups), len(groups[0].Profiles))
			}
			client := &nsCloudWatch{byNS: map[string][]cwtypes.Metric{"AWS/EC2": metrics}}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				got, err := discoverProfileGroup(context.Background(), client, groups[0])
				if err != nil {
					b.Fatal(err)
				}
				if len(got["ec2"]) != instances {
					b.Fatalf("discovered %d instances, want %d", len(got["ec2"]), instances)
				}
			}
		})
	}
}

// dimValues extracts the dimension-value slices for stable comparison.
func dimValues(insts []discoveredInstance) [][]string {
	out := make([][]string, len(insts))
	for i, ins := range insts {
		out[i] = ins.DimensionValues
	}
	return out
}

func newDiscoveryTestCollector(regionMetrics map[string]map[string][]cwtypes.Metric) (*Collector, map[string]*nsCloudWatch) {
	c := New()
	regions := regionsOf(regionMetrics)
	configureExactRule(c, regions, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", regions, []cwprofiles.ResolvedProfile{resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))})

	fakes := make(map[string]*nsCloudWatch, len(regionMetrics))
	for region, byNS := range regionMetrics {
		fakes[region] = &nsCloudWatch{byNS: byNS}
	}

	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newCloudWatchClient = func(cfg aws.Config) cloudwatchClient { return fakes[cfg.Region] }

	return c, fakes
}

func TestCollector_refreshDiscovery_TTLCaching(t *testing.T) {
	c, fakes := newDiscoveryTestCollector(map[string]map[string][]cwtypes.Metric{
		"us-east-1": {"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		"us-west-2": {"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-2")}},
	})

	base := time.Unix(1000, 0)
	c.now = func() time.Time { return base }

	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Equal(t, 2, c.discovery.totalInstances())
	callsAfterFirst := fakes["us-east-1"].calls
	assert.Positive(t, callsAfterFirst)
	c.tagFetchPlan = []tagFetchGroup{{key: tagFetchKey{target: "topology-sentinel"}}}

	// Within TTL: no refetch.
	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Equal(t, callsAfterFirst, fakes["us-east-1"].calls, "must not refetch within TTL")
	assert.Len(t, c.tagFetchPlan, 1, "cached tag topology follows the discovery lifetime")

	// After TTL: refetch.
	c.now = func() time.Time { return base.Add(301 * time.Second) }
	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Greater(t, fakes["us-east-1"].calls, callsAfterFirst, "must refetch after TTL")
	assert.Nil(t, c.tagFetchPlan, "a new discovery snapshot invalidates tag fetch topology")
}

func TestCollector_refreshDiscovery_TotalFailureFirstPassErrors(t *testing.T) {
	var logs bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&logs)
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))})
	c.now = func() time.Time { return time.Unix(1000, 0) }
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) {
		return aws.Config{}, errors.New("no credentials")
	}

	err := c.refreshDiscovery(context.Background())
	assert.Error(t, err, "all-target failure on the first pass must surface")
	assert.True(t, c.discovery.FetchedAt.IsZero())
	assert.Contains(t, logs.String(), "AWS/EC2")
}

// errListMetrics is a CloudWatch client whose ListMetrics always errors — used to
// make one discovery target fail while others succeed.
type errListMetrics struct{}

func (errListMetrics) ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return nil, errors.New("throttled")
}

func (errListMetrics) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return completeNoData(in), nil
}

func TestCollector_refreshDiscovery_EmptySuccessPlusFailureNotFatalOnFirstPass(t *testing.T) {
	// One target succeeds with zero instances (a resource-free region) while another
	// errors. On the first pass this must NOT be fatal — not every target failed.
	c := New()
	configureExactRule(c, []string{"us-east-1", "us-west-2"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1", "us-west-2"}, []cwprofiles.ResolvedProfile{resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))})
	c.now = func() time.Time { return time.Unix(1000, 0) }
	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newCloudWatchClient = func(cfg aws.Config) cloudwatchClient {
		if cfg.Region == "us-west-2" {
			return errListMetrics{} // this target errors
		}
		return &nsCloudWatch{byNS: map[string][]cwtypes.Metric{}} // empty but successful
	}

	require.NoError(t, c.refreshDiscovery(context.Background()), "empty success + one error must not be fatal")
	assert.Zero(t, c.discovery.totalInstances())
	assert.False(t, c.discovery.FetchedAt.IsZero(), "snapshot is committed despite the empty+error mix")
}

func TestCollector_collect_LateResolvedAccountDiscoveredSameCycle(t *testing.T) {
	// Role A resolves first and populates a fresh discovery snapshot; role B fails
	// once. When B resolves on a later cycle — still within discovery.refresh_every —
	// it must be discovered that same cycle, not after the TTL expires.
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"", "222222222222"}, failAt: map[int]bool{0: true}},
	})
	c.newCloudWatchClient = func(aws.Config) cloudwatchClient {
		return &nsCloudWatch{byNS: map[string][]cwtypes.Metric{
			"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
		}}
	}
	base := time.Unix(1000, 0)
	c.now = func() time.Time { return base }

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Equal(t, []string{"first"}, resolvedTargetNames(c))
	assert.Contains(t, c.discovery.Instances, discoveryKey{Target: "first", Profile: "ec2", Region: "us-east-1"})
	assert.NotContains(t, c.discovery.Instances, discoveryKey{Target: "second", Profile: "ec2", Region: "us-east-1"})

	// Next cycle, still inside the 300s TTL: role B resolves and is discovered now.
	c.now = func() time.Time { return base.Add(60 * time.Second) }
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, resolvedTargetNames(c))
	assert.Contains(t, c.discovery.Instances, discoveryKey{Target: "second", Profile: "ec2", Region: "us-east-1"},
		"a late-resolved account is discovered the same cycle, not after refresh_every")
}

func TestCollector_collect_runsDiscovery(t *testing.T) {
	c, _ := newDiscoveryTestCollector(map[string]map[string][]cwtypes.Metric{
		"us-east-1": {"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
	})
	c.now = func() time.Time { return time.Unix(1000, 0) }

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, []string{"base"}, resolvedTargetNames(c))
	assert.Equal(t, 1, c.discovery.totalInstances())
}

func TestDiscoverProfileGroup_WorkBounds(t *testing.T) {
	group := discoveryGroup{
		Target: "base", Region: "us-east-1", Namespace: "AWS/EC2",
		Profiles: []cwprofiles.ResolvedProfile{{Name: "ec2", Config: cwprofiles.Profile{
			Namespace: "AWS/EC2",
			Instance:  cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "InstanceId", Label: "instance_id"}}},
		}}},
	}

	t.Run("exact page maximum", func(t *testing.T) {
		pages := make([]*cloudwatch.ListMetricsOutput, maxListMetricsPages)
		for i := range pages {
			pages[i] = &cloudwatch.ListMetricsOutput{}
			if i+1 < len(pages) {
				pages[i].NextToken = aws.String(fmt.Sprintf("page-%d", i+1))
			}
		}
		_, err := discoverProfileGroup(context.Background(), &fakeCloudWatch{pages: pages}, group)
		require.NoError(t, err)
	})

	t.Run("page maximum exceeded", func(t *testing.T) {
		pages := make([]*cloudwatch.ListMetricsOutput, maxListMetricsPages)
		for i := range pages {
			pages[i] = &cloudwatch.ListMetricsOutput{NextToken: aws.String(fmt.Sprintf("page-%d", i+1))}
		}
		_, err := discoverProfileGroup(context.Background(), &fakeCloudWatch{pages: pages}, group)
		assert.ErrorContains(t, err, "requires more than 100 pages")
	})

	t.Run("repeated pagination token", func(t *testing.T) {
		pages := []*cloudwatch.ListMetricsOutput{
			{NextToken: aws.String("repeat")},
			{NextToken: aws.String("repeat")},
		}
		_, err := discoverProfileGroup(context.Background(), &fakeCloudWatch{pages: pages}, group)
		assert.ErrorContains(t, err, "repeated pagination token")
	})

	t.Run("scanned metric maximum exceeded", func(t *testing.T) {
		pages := []*cloudwatch.ListMetricsOutput{{Metrics: make([]cwtypes.Metric, maxScannedMetricsPerGroup+1)}}
		_, err := discoverProfileGroup(context.Background(), &fakeCloudWatch{pages: pages}, group)
		assert.ErrorContains(t, err, "scanned more than 50000 metrics")
	})

	t.Run("candidate instance maximum exceeded", func(t *testing.T) {
		metrics := make([]cwtypes.Metric, maxCandidateInstancesPerGroup+1)
		for i := range metrics {
			metrics[i] = mkMetric("CPUUtilization", "InstanceId", fmt.Sprintf("i-%d", i))
		}
		_, err := discoverProfileGroup(context.Background(), &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{{Metrics: metrics}}}, group)
		assert.ErrorContains(t, err, "found more than 20000 candidate instances")
	})

	t.Run("residual matcher evaluation maximum exceeded", func(t *testing.T) {
		const profileCount = 1001
		const metricCount = 1000
		profiles := make([]cwprofiles.ResolvedProfile, profileCount)
		for i := range profiles {
			constant := fmt.Sprintf("kind-%d", i)
			profiles[i] = cwprofiles.ResolvedProfile{
				Name: fmt.Sprintf("profile-%d", i),
				Config: cwprofiles.Profile{Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{
					{Name: "Id", Label: "id"},
					{Name: "Kind", Constant: &constant},
				}}},
			}
		}
		metrics := make([]cwtypes.Metric, metricCount)
		for i := range metrics {
			metrics[i] = mkMetric("M", "Id", fmt.Sprintf("id-%d", i), "Kind", "unmatched")
		}
		group := discoveryGroup{Namespace: "AWS/Test", Profiles: profiles}

		_, err := discoverProfileGroup(context.Background(), staticListMetrics{
			out: &cloudwatch.ListMetricsOutput{Metrics: metrics},
		}, group)
		assert.ErrorContains(t, err, "more than 1000000 residual profile matches")
	})
}

func BenchmarkDiscoverProfileGroup_ResidualMatching(b *testing.B) {
	const profiles = 1000
	const metrics = 1000
	group := discoveryGroup{Namespace: "AWS/Test", Profiles: make([]cwprofiles.ResolvedProfile, profiles)}
	for i := range group.Profiles {
		constant := fmt.Sprintf("kind-%d", i)
		group.Profiles[i] = cwprofiles.ResolvedProfile{
			Name: fmt.Sprintf("profile-%d", i),
			Config: cwprofiles.Profile{Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{
				{Name: "Id", Label: "id"},
				{Name: "Kind", Constant: &constant},
			}}},
		}
	}
	listed := make([]cwtypes.Metric, metrics)
	for i := range listed {
		listed[i] = mkMetric("M", "Kind", "unmatched", "Id", fmt.Sprintf("id-%d", i))
	}
	client := staticListMetrics{out: &cloudwatch.ListMetricsOutput{Metrics: listed}}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		instances, err := discoverProfileGroup(context.Background(), client, group)
		if err != nil {
			b.Fatal(err)
		}
		if len(instances) != 0 {
			b.Fatalf("expected no matches, got %d profiles", len(instances))
		}
	}
}

func regionsOf(m map[string]map[string][]cwtypes.Metric) []string {
	out := make([]string, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	return out
}

func TestSelectedSeriesUseRecentlyActive(t *testing.T) {
	ec2 := dimProfile("AWS/EC2", 300, "InstanceId")
	s3 := dimProfile("AWS/S3", 86400, "BucketName")

	assert.True(t, selectedSeriesUseRecentlyActive(testCompiledSeries(cwprofiles.ResolvedProfile{Name: "ec2", Config: ec2}), true))
	assert.False(t, selectedSeriesUseRecentlyActive(testCompiledSeries(cwprofiles.ResolvedProfile{Name: "ec2", Config: ec2}), false))
	assert.False(t, selectedSeriesUseRecentlyActive(testCompiledSeries(cwprofiles.ResolvedProfile{Name: "s3", Config: s3}), true), "daily period must disable PT3H")

	// A per-metric override beyond 3h also disables PT3H for the whole profile.
	mixed := dimProfile("AWS/Custom", 300, "Id")
	mixed.Metrics = []cwprofiles.Metric{{ID: "m", MetricName: "M", Statistics: []string{"average"}, Period: 86400}}
	assert.False(t, selectedSeriesUseRecentlyActive(testCompiledSeries(cwprofiles.ResolvedProfile{Name: "mixed", Config: mixed}), true))
}
