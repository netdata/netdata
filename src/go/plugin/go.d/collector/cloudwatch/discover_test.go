// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"

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
	return &cloudwatch.GetMetricDataOutput{}, nil
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
	}
}

func TestDiscoverInstances(t *testing.T) {
	// discoverInstances lists one page and keeps only the metrics whose dimension-NAME
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
				mkMetric("BucketSizeBytes", "BucketName", "b2", "StorageType", "StandardStorage"),
				mkMetric("BucketSizeBytes", "BucketName", "b1"), // wrong cardinality -> rejected
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

			got, err := discoverInstances(context.Background(), client, prof, tc.recentlyActive)
			require.NoError(t, err)

			gotDimValues := make([][]string, len(got))
			for i, inst := range got {
				gotDimValues[i] = inst.DimensionValues
			}
			assert.Equal(t, tc.wantDimValues, gotDimValues)
		})
	}
}

func TestMatchInstanceDimensions_DuplicateNameRejected(t *testing.T) {
	// Same cardinality as the profile but a repeated dimension name must not match.
	dims := []cwtypes.Dimension{
		{Name: aws.String("InstanceId"), Value: aws.String("i-1")},
		{Name: aws.String("InstanceId"), Value: aws.String("i-2")},
	}
	_, ok := matchInstanceDimensions(dims, []string{"InstanceId", "ImageId"})
	assert.False(t, ok)
}

func TestConstantDimensionsHold(t *testing.T) {
	tests := map[string]struct {
		dims   []cwprofiles.InstanceDimension
		values []string
		want   bool
	}{
		"no constant dimensions": {
			dims:   []cwprofiles.InstanceDimension{{Name: "InstanceId", Label: "instance_id"}},
			values: []string{"i-1"},
			want:   true,
		},
		"constant value matches": {
			dims:   []cwprofiles.InstanceDimension{{Name: "DistributionId", Label: "distribution_id"}, {Name: "Region", Constant: aws.String("Global")}},
			values: []string{"E1", "Global"},
			want:   true,
		},
		"constant value mismatch fails closed": {
			dims:   []cwprofiles.InstanceDimension{{Name: "DistributionId", Label: "distribution_id"}, {Name: "Region", Constant: aws.String("Global")}},
			values: []string{"E1", "us-east-1"},
			want:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, constantDimensionsHold(tc.dims, tc.values))
		})
	}
}

func TestDiscoverInstances_ConstantDimensionFailClosed(t *testing.T) {
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

	got, err := discoverInstances(context.Background(), client, prof, false)
	require.NoError(t, err)

	gotDimValues := make([][]string, len(got))
	for i, inst := range got {
		gotDimValues[i] = inst.DimensionValues
	}
	assert.Equal(t, [][]string{{"E1", "Global"}, {"E2", "Global"}}, gotDimValues)
}

func TestDiscoverInstances_Pagination(t *testing.T) {
	client := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{
		page([]cwtypes.Metric{mkMetric("CPUUtilization", "InstanceId", "i-1")}, "next"),
		page([]cwtypes.Metric{mkMetric("CPUUtilization", "InstanceId", "i-2")}, ""),
	}}

	prof := dimProfile("AWS/EC2", 300, "InstanceId")
	got, err := discoverInstances(context.Background(), client, prof, true)
	require.NoError(t, err)

	require.Len(t, got, 2)
	assert.Equal(t, []string{"i-1"}, got[0].DimensionValues)
	assert.Equal(t, []string{"i-2"}, got[1].DimensionValues)

	require.Len(t, client.inputs, 2)
	assert.Nil(t, client.inputs[0].NextToken)
	require.NotNil(t, client.inputs[1].NextToken)
	assert.Equal(t, "next", *client.inputs[1].NextToken)
}

func TestDiscoverInstances_RecentlyActiveAndNamespace(t *testing.T) {
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

			_, err := discoverInstances(context.Background(), client, prof, tc.useRecentlyActive)
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

func TestDiscoverInstances_EmptyAndError(t *testing.T) {
	prof := dimProfile("AWS/EC2", 300, "InstanceId")

	empty := &fakeCloudWatch{pages: []*cloudwatch.ListMetricsOutput{page(nil, "")}}
	got, err := discoverInstances(context.Background(), empty, prof, true)
	require.NoError(t, err)
	assert.Empty(t, got)

	failing := &fakeCloudWatch{err: errors.New("access denied")}
	_, err = discoverInstances(context.Background(), failing, prof, true)
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

func (f *nsCloudWatch) GetMetricData(context.Context, *cloudwatch.GetMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	return &cloudwatch.GetMetricDataOutput{}, nil
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

	results := discoverAll(context.Background(), newClient,
		[]string{"000000000000"}, []cwprofiles.ResolvedProfile{ec2, s3}, []string{"us-east-1", "us-west-2"}, true, 5, 0)
	require.Len(t, results, 4)

	snap, errs := buildDiscoverySnapshot(results, nil, time.Unix(1000, 0), 300)
	require.Empty(t, errs)

	assert.Equal(t, 4, snap.totalInstances())
	assert.Equal(t, [][]string{{"i-1"}}, dimValues(snap.Instances[discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-east-1"}]))
	assert.Equal(t, [][]string{{"i-2"}, {"i-3"}}, dimValues(snap.Instances[discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-west-2"}]))
	assert.Equal(t, [][]string{{"b1", "StandardStorage"}}, dimValues(snap.Instances[discoveryKey{Account: "000000000000", Profile: "s3", Region: "us-east-1"}]))
	assert.NotContains(t, snap.Instances, discoveryKey{Account: "000000000000", Profile: "s3", Region: "us-west-2"}, "empty target must be omitted")

	assert.Equal(t, time.Unix(1000, 0), snap.FetchedAt)
	assert.Equal(t, time.Unix(1300, 0), snap.ExpiresAt)
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

	results := discoverAll(context.Background(), newClient,
		[]string{"000000000000"}, []cwprofiles.ResolvedProfile{ec2}, []string{"us-east-1", "bad-region"}, true, 5, 0)

	snap, errs := buildDiscoverySnapshot(results, nil, time.Unix(1000, 0), 300)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "bad-region")
	assert.Equal(t, 1, snap.totalInstances())
	assert.Contains(t, snap.Instances, discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-east-1"})
}

func TestBuildDiscoverySnapshot_FailSoftCarriesForward(t *testing.T) {
	prev := map[discoveryKey][]discoveredInstance{
		{Account: "000000000000", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
		{Account: "000000000000", Profile: "ec2", Region: "us-west-2"}: {{DimensionValues: []string{"i-9"}}},
	}
	results := []discoveryResult{
		{Key: discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-east-1"}, Instances: []discoveredInstance{{DimensionValues: []string{"i-2"}}}}, // refreshed
		{Key: discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-west-2"}, Err: errors.New("throttled")},                                        // failed
	}

	snap, errs := buildDiscoverySnapshot(results, prev, time.Unix(1000, 0), 300)
	require.Len(t, errs, 1)
	assert.Equal(t, [][]string{{"i-2"}}, dimValues(snap.Instances[discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-east-1"}]), "succeeded target is refreshed")
	assert.Equal(t, [][]string{{"i-9"}}, dimValues(snap.Instances[discoveryKey{Account: "000000000000", Profile: "ec2", Region: "us-west-2"}]), "failed target carries forward last-known instances")
}

func TestDiscoverySnapshot_Expired(t *testing.T) {
	var zero discoverySnapshot
	assert.True(t, zero.expired(time.Unix(1000, 0)), "never-fetched snapshot is expired")

	snap := discoverySnapshot{FetchedAt: time.Unix(1000, 0), ExpiresAt: time.Unix(1300, 0)}
	assert.False(t, snap.expired(time.Unix(1299, 0)))
	assert.True(t, snap.expired(time.Unix(1300, 0)))
	assert.True(t, snap.expired(time.Unix(1500, 0)))
}

// dimValues extracts the dimension-value slices for stable comparison.
func dimValues(insts []discoveredInstance) [][]string {
	out := make([][]string, len(insts))
	for i, ins := range insts {
		out[i] = ins.DimensionValues
	}
	return out
}

func TestCollector_selectProfiles(t *testing.T) {
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	t.Run("auto selects enabled profiles", func(t *testing.T) {
		c := &Collector{Config: Config{Profiles: ProfilesConfig{Mode: profilesModeAuto}}}
		got, err := c.selectProfiles(catalog)
		require.NoError(t, err)
		enabled := 0
		for _, p := range catalog.AllProfiles() {
			if !p.Config.Disabled {
				enabled++
			}
		}
		assert.Len(t, got, enabled)
		assert.Less(t, enabled, len(catalog.AllProfiles()), "expected some profiles disabled by default")
	})

	t.Run("combined selects all profiles", func(t *testing.T) {
		c := &Collector{Config: Config{Profiles: ProfilesConfig{Mode: profilesModeCombined}}}
		got, err := c.selectProfiles(catalog)
		require.NoError(t, err)
		assert.Len(t, got, len(catalog.AllProfiles()))
	})

	t.Run("exact selects matching profiles by basename", func(t *testing.T) {
		c := &Collector{Config: Config{Profiles: ProfilesConfig{
			Mode:      profilesModeExact,
			ModeExact: &ProfilesExactConfig{Entries: []ProfileEntry{{Name: "ec2"}, {Name: "s3"}}},
		}}}
		got, err := c.selectProfiles(catalog)
		require.NoError(t, err)
		require.Len(t, got, 2)
		assert.Equal(t, "ec2", got[0].Name)
		assert.Equal(t, "s3", got[1].Name)
	})

	t.Run("exact with no match errors", func(t *testing.T) {
		c := &Collector{Config: Config{Profiles: ProfilesConfig{
			Mode:      profilesModeExact,
			ModeExact: &ProfilesExactConfig{Entries: []ProfileEntry{{Name: "bogus"}}},
		}}}
		_, err := c.selectProfiles(catalog)
		assert.Error(t, err)
	})

	t.Run("exact selects a default-disabled deep-grain profile by name", func(t *testing.T) {
		c := &Collector{Config: Config{Profiles: ProfilesConfig{
			Mode:      profilesModeExact,
			ModeExact: &ProfilesExactConfig{Entries: []ProfileEntry{{Name: "alb_target"}}},
		}}}
		got, err := c.selectProfiles(catalog)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "alb_target", got[0].Name, "exact selects a disabled profile by basename")
	})

	t.Run("unsupported mode errors", func(t *testing.T) {
		c := &Collector{Config: Config{Profiles: ProfilesConfig{Mode: "weird"}}}
		_, err := c.selectProfiles(catalog)
		assert.Error(t, err)
	})
}

func newDiscoveryTestCollector(regionMetrics map[string]map[string][]cwtypes.Metric) (*Collector, map[string]*nsCloudWatch) {
	c := New()
	c.Config.Regions = regionsOf(regionMetrics)
	c.applyDefaults()

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
	c.profiles = []cwprofiles.ResolvedProfile{resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))}
	c.accounts = []cwAccount{{accountID: "000000000000"}}

	base := time.Unix(1000, 0)
	c.now = func() time.Time { return base }

	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Equal(t, 2, c.discovery.totalInstances())
	callsAfterFirst := fakes["us-east-1"].calls
	assert.Positive(t, callsAfterFirst)

	// Within TTL: no refetch.
	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Equal(t, callsAfterFirst, fakes["us-east-1"].calls, "must not refetch within TTL")

	// After TTL: refetch.
	c.now = func() time.Time { return base.Add(301 * time.Second) }
	require.NoError(t, c.refreshDiscovery(context.Background()))
	assert.Greater(t, fakes["us-east-1"].calls, callsAfterFirst, "must refetch after TTL")
}

func TestCollector_refreshDiscovery_TotalFailureFirstPassErrors(t *testing.T) {
	c := New()
	c.Config.Regions = []string{"us-east-1"}
	c.applyDefaults()
	c.profiles = []cwprofiles.ResolvedProfile{resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))}
	c.accounts = []cwAccount{{accountID: "000000000000"}}
	c.now = func() time.Time { return time.Unix(1000, 0) }
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) {
		return aws.Config{}, errors.New("no credentials")
	}

	err := c.refreshDiscovery(context.Background())
	assert.Error(t, err, "all-target failure on the first pass must surface")
	assert.True(t, c.discovery.FetchedAt.IsZero())
}

func TestCollector_collect_runsDiscovery(t *testing.T) {
	c, _ := newDiscoveryTestCollector(map[string]map[string][]cwtypes.Metric{
		"us-east-1": {"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
	})
	c.profiles = []cwprofiles.ResolvedProfile{resolved("ec2", dimProfile("AWS/EC2", 300, "InstanceId"))}
	c.now = func() time.Time { return time.Unix(1000, 0) }
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: "000000000000"} }

	require.NoError(t, c.collect(context.Background()))
	assert.Equal(t, []string{"000000000000"}, c.accountIDs())
	assert.Equal(t, 1, c.discovery.totalInstances())
}

func regionsOf(m map[string]map[string][]cwtypes.Metric) []string {
	out := make([]string, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	return out
}

func TestProfileUsesRecentlyActive(t *testing.T) {
	ec2 := dimProfile("AWS/EC2", 300, "InstanceId")
	s3 := dimProfile("AWS/S3", 86400, "BucketName")

	assert.True(t, profileUsesRecentlyActive(ec2, true))
	assert.False(t, profileUsesRecentlyActive(ec2, false))
	assert.False(t, profileUsesRecentlyActive(s3, true), "daily period must disable PT3H")

	// A per-metric override beyond 3h also disables PT3H for the whole profile.
	mixed := dimProfile("AWS/Custom", 300, "Id")
	mixed.Metrics = []cwprofiles.Metric{{ID: "m", MetricName: "M", Statistics: []string{"average"}, Period: 86400}}
	assert.False(t, profileUsesRecentlyActive(mixed, true))
}
