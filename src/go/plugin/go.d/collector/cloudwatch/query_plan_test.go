// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ec2QueryProfile() cwprofiles.Profile {
	return cwprofiles.Profile{
		Namespace: "AWS/EC2",
		Query:     cwquery.Config{Period: longDuration(5 * time.Minute)},
		Instance:  cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "InstanceId", Label: "instance_id"}}},
		Metrics: []cwprofiles.Metric{
			{ID: "cpu_utilization", MetricName: "CPUUtilization", Statistics: []string{"average"}},
			{ID: "duration", MetricName: "Duration", Statistics: []string{"average", "p90"}},
		},
	}
}

func ec2QueryCollector(regions []string, instancesByRegion map[string][][]string) *Collector {
	c := New()
	configureExactRule(c, regions, []string{"ec2"})
	c.applyDefaults()
	setSingleTargetPlan(c, "123456789012", regions, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})

	insts := make(map[discoveryKey][]discoveredInstance)
	for region, list := range instancesByRegion {
		di := make([]discoveredInstance, 0, len(list))
		for _, vals := range list {
			di = append(di, discoveredInstance{DimensionValues: vals})
		}
		insts[discoveryKey{Target: "base", Profile: "ec2", Region: region}] = di
	}
	c.discovery = discoverySnapshot{Instances: insts}
	return c
}

func labelValue(labels []metrix.Label, key string) string {
	for _, l := range labels {
		if l.Key == key {
			return l.Value
		}
	}
	return ""
}

func requireBuildQueryPlan(t testing.TB, c *Collector) []plannedQuery {
	t.Helper()
	plan, err := c.buildQueryPlan()
	require.NoError(t, err)
	return plan
}

func requireCurrentQueryPlan(t testing.TB, c *Collector) []plannedQuery {
	t.Helper()
	plan, err := c.currentQueryPlan()
	require.NoError(t, err)
	return plan
}

func TestBuildQueryPlan(t *testing.T) {
	c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{
		"us-east-1": {{"i-1"}, {"i-2"}},
	})

	plan := requireBuildQueryPlan(t, c)
	require.Len(t, plan, 6) // 2 instances x (1 + 2 statistics)

	ids := make(map[structuralID]bool)
	byInstance := make(map[string][]plannedQuery)
	for _, pq := range plan {
		ids[pq.key] = true
		byInstance[labelValue(pq.labels, "instance_id")] = append(byInstance[labelValue(pq.labels, "instance_id")], pq)
	}
	assert.Len(t, ids, 6, "stable query keys are unique")
	require.Len(t, byInstance, 2)

	for inst, qs := range byInstance {
		require.Len(t, qs, 3)
		series := make(map[string]plannedQuery)
		for _, q := range qs {
			assert.Equal(t, "123456789012", labelValue(q.labels, "account_id"))
			assert.Equal(t, "us-east-1", labelValue(q.labels, "region"))
			assert.Equal(t, inst, labelValue(q.labels, "instance_id"))
			series[q.seriesName] = q
		}
		require.Contains(t, series, "ec2.cpu_utilization_average")
		require.Contains(t, series, "ec2.duration_average")
		require.Contains(t, series, "ec2.duration_p90")

		cpu := series["ec2.cpu_utilization_average"].query
		assert.Equal(t, "AWS/EC2", aws.ToString(cpu.MetricStat.Metric.Namespace))
		assert.Equal(t, "CPUUtilization", aws.ToString(cpu.MetricStat.Metric.MetricName))
		assert.Equal(t, "Average", aws.ToString(cpu.MetricStat.Stat))
		assert.Equal(t, int32(300), aws.ToInt32(cpu.MetricStat.Period))
		require.Len(t, cpu.MetricStat.Metric.Dimensions, 1)
		assert.Equal(t, "InstanceId", aws.ToString(cpu.MetricStat.Metric.Dimensions[0].Name))
		assert.Equal(t, inst, aws.ToString(cpu.MetricStat.Metric.Dimensions[0].Value))

		assert.Equal(t, "p90", aws.ToString(series["ec2.duration_p90"].query.MetricStat.Stat))
	}
}

func TestBuildQueryPlan_ConstantDimension(t *testing.T) {
	// A constant (match-and-query-only) dimension is sent in the GetMetricData
	// query but is not emitted as an identity label.
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"cloudfront"})
	profiles := []cwprofiles.ResolvedProfile{{Name: "cloudfront", Config: cwprofiles.Profile{
		Namespace: "AWS/CloudFront",
		Query:     cwquery.Config{Period: longDuration(5 * time.Minute)},
		Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{
			{Name: "DistributionId", Label: "distribution_id"},
			{Name: "Region", Constant: aws.String("Global")},
		}},
		Metrics: []cwprofiles.Metric{
			{ID: "requests", MetricName: "Requests", Statistics: []string{"sum"}, Rate: true},
		},
	}}}
	setSingleTargetPlan(c, "123456789012", []string{"us-east-1"}, profiles)
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "cloudfront", Region: "us-east-1"}: {{DimensionValues: []string{"E1", "Global"}}},
	}}

	plan := requireBuildQueryPlan(t, c)
	require.Len(t, plan, 1)
	pq := plan[0]

	// Identity labels are {account_id, region, distribution_id}; the constant
	// Region dimension is NOT a label.
	assert.Equal(t, "E1", labelValue(pq.labels, "distribution_id"))
	assert.Len(t, pq.labels, 3)
	for _, l := range pq.labels {
		assert.NotEqualf(t, "Global", l.Value, "constant dimension value must not appear as label %q", l.Key)
	}

	// The query carries BOTH the identifying and the constant dimension.
	dims := pq.query.MetricStat.Metric.Dimensions
	require.Len(t, dims, 2)
	got := map[string]string{}
	for _, d := range dims {
		got[aws.ToString(d.Name)] = aws.ToString(d.Value)
	}
	assert.Equal(t, map[string]string{"DistributionId": "E1", "Region": "Global"}, got)
}

func TestBuildQueryPlan_MultiRegionAndEmpty(t *testing.T) {
	c := ec2QueryCollector([]string{"us-east-1", "us-west-2"}, map[string][][]string{
		"us-east-1": {{"i-1"}},
		"us-west-2": {{"i-2"}},
	})

	plan := requireBuildQueryPlan(t, c)
	perRegion := map[string]int{}
	for _, pq := range plan {
		assert.Equal(t, pq.region, labelValue(pq.labels, "region"), "region label matches the query's region")
		perRegion[pq.region]++
	}
	assert.Equal(t, 3, perRegion["us-east-1"], "1 instance x 3 series")
	assert.Equal(t, 3, perRegion["us-west-2"])

	empty := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{})
	assert.Empty(t, requireBuildQueryPlan(t, empty), "no discovered instances -> empty plan")
}

func TestCurrentQueryPlan_CachesUntilInputsChange(t *testing.T) {
	c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{
		"us-east-1": {{"i-1"}},
	})

	first := requireCurrentQueryPlan(t, c)
	require.NotEmpty(t, first)
	second := requireCurrentQueryPlan(t, c)
	require.NotEmpty(t, second)
	assert.Same(t, &first[0], &second[0], "unchanged inputs reuse the compiled query blueprint")
	now := time.Unix(1_000_000_000, 0)
	for _, query := range first {
		_, end := queryWindow(now, query.policy)
		c.observations.queries[query.key] = queryState{lastCompletedEnd: end}
	}

	c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}] = append(
		c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}],
		discoveredInstance{DimensionValues: []string{"i-2"}},
	)
	assert.Len(t, requireCurrentQueryPlan(t, c), len(first), "mutating an input without invalidation does not rebuild")

	c.invalidateQueryPlan()
	rebuilt := requireCurrentQueryPlan(t, c)
	assert.Len(t, rebuilt, len(first)*2, "input invalidation rebuilds the query blueprint")
	assert.Len(t, c.observations.dueQueries(rebuilt, now), len(first), "only queries for the new instance become due")
}

func filteredOverlapCollector(t *testing.T) *Collector {
	t.Helper()
	defaults := false
	filters := []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
	cfg := twoTargetConfig()
	cfg.Rules = []RuleConfig{
		{
			Name: "first", Targets: []string{"first"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
			Filters:  &RuleFiltersConfig{ResourceTags: &filters},
		},
		{
			Name: "second", Targets: []string{"second"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
		},
	}
	c := multiTargetCollectorWithConfig(t, cfg, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	require.Len(t, c.plan.Scopes, 2)
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}: {
			{DimensionValues: []string{"i-1"}}, {DimensionValues: []string{"i-2"}},
		},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {
			{DimensionValues: []string{"i-1"}}, {DimensionValues: []string{"i-2"}},
		},
	}}
	return c
}

func queryOwnersByInstance(plan []plannedQuery) map[string]string {
	owners := make(map[string]string)
	for _, query := range plan {
		owners[labelValue(query.labels, "instance_id")] = query.target
	}
	return owners
}

func queryOwnersBySeries(plan []plannedQuery) map[string]string {
	owners := make(map[string]string)
	for _, query := range plan {
		owners[query.seriesName] = query.target
	}
	return owners
}

func selectCompiledSeries(t *testing.T, scope collectionScope, names ...string) []compiledSeries {
	t.Helper()
	wanted := make(map[string]struct{}, len(names))
	for _, name := range names {
		wanted[name] = struct{}{}
	}
	var selected []compiledSeries
	for _, series := range scope.SelectedSeries {
		if _, ok := wanted[series.Name]; ok {
			selected = append(selected, series)
			delete(wanted, series.Name)
		}
	}
	require.Empty(t, wanted, "requested test series must exist in the profile")
	return selected
}

func TestBuildQueryPlan_SeriesOwnershipAcrossTargets(t *testing.T) {
	// filteredOverlapCollector compiles the stock EC2 profile; its network series
	// let these scopes exercise partial overlap rather than a synthetic fixture.
	c := filteredOverlapCollector(t)
	c.plan.Scopes[0].TagFilter = nil
	c.plan.Scopes[0].SelectedSeries = selectCompiledSeries(t, c.plan.Scopes[0], "ec2.cpu_utilization_average", "ec2.network_in_sum")
	c.plan.Scopes[1].SelectedSeries = selectCompiledSeries(t, c.plan.Scopes[1], "ec2.network_in_sum", "ec2.network_out_sum")
	c.discovery.Instances[discoveryKey{Target: "first", Profile: "ec2", Region: "us-east-1"}] = []discoveredInstance{{DimensionValues: []string{"i-1"}}}
	c.discovery.Instances[discoveryKey{Target: "second", Profile: "ec2", Region: "us-east-1"}] = []discoveredInstance{{DimensionValues: []string{"i-1"}}}

	plan := requireBuildQueryPlan(t, c)
	assert.Equal(t, map[string]string{
		"ec2.cpu_utilization_average": "first",
		"ec2.network_in_sum":          "first",
		"ec2.network_out_sum":         "second",
	}, queryOwnersBySeries(plan))
}

func TestSeriesOwnershipClaimsEachOrdinalOnce(t *testing.T) {
	var ownership seriesOwnership
	for _, ordinal := range []int{0, 63, 64, 255} {
		assert.Truef(t, ownership.claim(ordinal), "first claim for ordinal %d", ordinal)
		assert.Falsef(t, ownership.claim(ordinal), "duplicate claim for ordinal %d", ordinal)
	}
}

func TestBuildQueryPlan_UnknownTagMembershipReservesOnlySelectedSeries(t *testing.T) {
	c := filteredOverlapCollector(t)
	first := &c.plan.Scopes[0]
	first.SelectedSeries = selectCompiledSeries(t, *first, "ec2.cpu_utilization_average")
	c.tags = tagSnapshot{members: tagMembership{}, unknown: map[int]struct{}{first.TagMembershipID: {}}}

	plan := requireBuildQueryPlan(t, c)
	owners := queryOwnersBySeries(plan)
	assert.NotContains(t, owners, "ec2.cpu_utilization_average")
	assert.Len(t, owners, len(c.plan.Scopes[1].SelectedSeries)-1)
	for _, owner := range owners {
		assert.Equal(t, "second", owner)
	}
}

func TestBuildQueryPlan_FirstEmittingScopeSuppliesSiblingTagLabels(t *testing.T) {
	c := filteredOverlapCollector(t)
	c.plan.Scopes[0].TagFilter = nil
	c.plan.Scopes[0].SelectedSeries = selectCompiledSeries(t, c.plan.Scopes[0], "ec2.cpu_utilization_average")
	c.plan.Scopes[1].SelectedSeries = selectCompiledSeries(t, c.plan.Scopes[1], "ec2.network_in_sum", "ec2.network_out_sum")
	c.discovery.Instances[discoveryKey{Target: "first", Profile: "ec2", Region: "us-east-1"}] = []discoveredInstance{{DimensionValues: []string{"i-1"}}}
	c.discovery.Instances[discoveryKey{Target: "second", Profile: "ec2", Region: "us-east-1"}] = []discoveredInstance{{DimensionValues: []string{"i-1"}}}
	c.tags.labels = map[tagCacheKey][]metrix.Label{
		{target: "first", account: "111111111111", region: "us-east-1", profile: "ec2", joinKey: "i-1"}:  {{Key: "owner", Value: "first"}},
		{target: "second", account: "111111111111", region: "us-east-1", profile: "ec2", joinKey: "i-1"}: {{Key: "owner", Value: "second"}},
	}

	plan := requireBuildQueryPlan(t, c)
	require.Len(t, plan, 3)
	for _, query := range plan {
		assert.Equal(t, "first", labelValue(query.tagLabels, "owner"), query.seriesName)
	}
}

func TestBuildQueryPlan_OrderedResourceTagFiltering(t *testing.T) {
	tests := map[string]struct {
		tags       tagSnapshot
		wantOwners map[string]string
	}{
		"known non-match falls through to lower rule": {
			tags: tagSnapshot{
				members: tagMembership{0: {"i-1": {}}},
				unknown: map[int]struct{}{}, fetchedAt: time.Unix(1, 0),
			},
			wantOwners: map[string]string{"i-1": "first", "i-2": "second"},
		},
		"first failure reserves every candidate from lower rules": {
			tags:       tagSnapshot{members: tagMembership{}, unknown: map[int]struct{}{0: {}}},
			wantOwners: map[string]string{},
		},
		"later failure queries last-known members and reserves the rest": {
			tags: tagSnapshot{
				members: tagMembership{0: {"i-1": {}}},
				unknown: map[int]struct{}{0: {}},
			},
			wantOwners: map[string]string{"i-1": "first"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := filteredOverlapCollector(t)
			c.tags = tc.tags
			assert.Equal(t, tc.wantOwners, queryOwnersByInstance(requireBuildQueryPlan(t, c)))
		})
	}
}

func TestBuildQueryPlan_MaxInstances(t *testing.T) {
	t.Run("counts final instances before metric expansion", func(t *testing.T) {
		c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{"us-east-1": {{"i-1"}, {"i-2"}}})
		c.Limits.MaxInstances = 1
		plan, err := c.buildQueryPlan()
		assert.Nil(t, plan)
		assert.ErrorContains(t, err, "limits.max_instances=1")
	})

	t.Run("overlapping copies count once", func(t *testing.T) {
		c := filteredOverlapCollector(t)
		c.plan.Scopes[0].TagFilter = nil
		c.discovery.Instances[discoveryKey{Target: "first", Profile: "ec2", Region: "us-east-1"}] = []discoveredInstance{{DimensionValues: []string{"i-1"}}}
		c.discovery.Instances[discoveryKey{Target: "second", Profile: "ec2", Region: "us-east-1"}] = []discoveredInstance{{DimensionValues: []string{"i-1"}}}
		c.Limits.MaxInstances = 1
		assert.NotEmpty(t, requireBuildQueryPlan(t, c))
	})
}

func TestCurrentQueryPlan_OverflowRetainsLastValidPlan(t *testing.T) {
	c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{"us-east-1": {{"i-1"}}})
	c.Limits.MaxInstances = 1
	previous := requireCurrentQueryPlan(t, c)
	require.NotEmpty(t, previous)

	c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}] = append(
		c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}],
		discoveredInstance{DimensionValues: []string{"i-2"}},
	)
	c.invalidateQueryPlan()
	plan, err := c.currentQueryPlan()
	assert.Nil(t, plan)
	assert.ErrorContains(t, err, "limits.max_instances=1")
	assert.Equal(t, previous, c.queryPlan, "a rejected refresh does not replace the last valid plan")
	assert.True(t, c.planDirty, "the next collect retries the rejected refresh")
}

func TestBuildQueryPlan_OversizedWorkStopsBeforeFullQueryAllocation(t *testing.T) {
	const instancesPerRegion = 20000
	const totalInstances = 2 * instancesPerRegion
	values := make([][]string, instancesPerRegion)
	for i := range values {
		values[i] = []string{fmt.Sprintf("i-%d", i)}
	}
	c := ec2QueryCollector([]string{"us-east-1", "us-west-2"}, map[string][][]string{
		"us-east-1": values,
		"us-west-2": values,
	})
	for i := range c.plan.Scopes {
		c.plan.Scopes[i].SelectedSeries = c.plan.Scopes[i].SelectedSeries[:1]
	}
	c.Limits.MaxInstances = totalInstances + 1

	var err error
	allocations := testing.AllocsPerRun(1, func() {
		_, err = c.buildQueryPlan()
	})
	require.ErrorContains(t, err, "maximum 20000")
	t.Logf("oversized plan allocations: %.0f", allocations)
	// The preflight path measures about 300k allocations on this fixture; the old
	// post-build validator exceeded 1.03m. This generous ceiling protects early
	// rejection without turning ordinary allocation tuning into a test contract.
	assert.Less(t, allocations, float64(350000), "query work must be rejected before allocating the oversized planned-query set")
}

func TestBuildQueryPlan_MaximumPayloadUsesCompactInternalIdentities(t *testing.T) {
	const (
		instanceCount  = 10
		metricCount    = 20
		dimensionCount = 30
		valueBytes     = 1024
	)
	c := maximumPayloadQueryPlanCollector(instanceCount, metricCount, dimensionCount, valueBytes)

	plan, err := c.buildQueryPlan()
	require.NoError(t, err)
	require.Len(t, plan, instanceCount*metricCount)
	identityBytes := 0
	for _, query := range plan {
		identityBytes += len(query.key) + len(query.billingKey)
	}
	assert.LessOrEqual(t, identityBytes, len(plan)*2*sha256.Size,
		"internal observation and billing identities must remain fixed-size at maximum dimension payloads")
}

func maximumPayloadQueryPlanCollector(instanceCount, metricCount, dimensionCount, valueBytes int) *Collector {
	dimensions := make([]cwprofiles.InstanceDimension, dimensionCount)
	for i := range dimensions {
		dimensions[i] = cwprofiles.InstanceDimension{Name: fmt.Sprintf("Dimension%d", i), Label: fmt.Sprintf("dimension_%d", i)}
	}
	metrics := make([]cwprofiles.Metric, metricCount)
	for i := range metrics {
		metrics[i] = cwprofiles.Metric{ID: fmt.Sprintf("metric_%d", i), MetricName: fmt.Sprintf("Metric%d", i), Statistics: []string{"average"}}
	}
	profile := cwprofiles.ResolvedProfile{Name: "maximum_payload", Config: cwprofiles.Profile{
		Namespace: "AWS/Test", Query: cwquery.Config{Period: longDuration(5 * time.Minute)},
		Instance: cwprofiles.InstanceSpec{Dimensions: dimensions},
		Metrics:  metrics,
	}}
	c := New()
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{profile})
	c.Limits.MaxInstances = instanceCount + 1
	instances := make([]discoveredInstance, instanceCount)
	for i := range instances {
		values := make([]string, dimensionCount)
		for j := range values {
			prefix := fmt.Sprintf("instance-%03d-dimension-%02d-", i, j)
			values[j] = prefix + strings.Repeat("x", valueBytes-len(prefix))
		}
		instances[i] = discoveredInstance{DimensionValues: values}
	}
	c.discovery.Instances = map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: profile.Name, Region: "us-east-1"}: instances,
	}
	return c
}

func BenchmarkBuildQueryPlanMaximumPayload(b *testing.B) {
	const (
		instanceCount  = 1000
		metricCount    = 20
		dimensionCount = 30
		valueBytes     = 1024
	)
	c := maximumPayloadQueryPlanCollector(instanceCount, metricCount, dimensionCount, valueBytes)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plan, err := c.buildQueryPlan()
		if err != nil {
			b.Fatal(err)
		}
		if len(plan) != instanceCount*metricCount {
			b.Fatalf("query plan length = %d, want %d", len(plan), instanceCount*metricCount)
		}
		goruntime.KeepAlive(plan)
	}
}

func BenchmarkCurrentQueryPlanCached(b *testing.B) {
	instances := make([][]string, 256)
	for i := range instances {
		instances[i] = []string{fmt.Sprintf("i-%d", i)}
	}
	c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{"us-east-1": instances})
	require.NotEmpty(b, requireCurrentQueryPlan(b, c))

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plan, err := c.currentQueryPlan()
		if err != nil {
			b.Fatal(err)
		}
		if len(plan) == 0 {
			b.Fatal("cached plan unexpectedly empty")
		}
	}
}

func BenchmarkBuildQueryPlan(b *testing.B) {
	for _, instances := range queryPlanBenchmarkInstanceCounts() {
		for _, selectedPercent := range []int{100, 10} {
			b.Run(fmt.Sprintf("instances_%d/selected_%d_percent", instances, selectedPercent), func(b *testing.B) {
				values := make([][]string, instances)
				for i := range values {
					values[i] = []string{fmt.Sprintf("i-%d", i)}
				}
				c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{"us-east-1": values})
				c.Limits.MaxInstances = instances + 1
				if selectedPercent < 100 {
					c.plan.Scopes[0].TagFilter = []resourceTagFilter{{key: "environment", values: []string{"production"}}}
					c.tags = tagSnapshot{members: make(tagMembership), unknown: map[int]struct{}{}, fetchedAt: time.Unix(1, 0)}
					for i := 0; i < instances; i += 100 / selectedPercent {
						c.tags.members.add(0, fmt.Sprintf("i-%d", i))
					}
				}

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					plan, err := c.buildQueryPlan()
					if err != nil {
						b.Fatal(err)
					}
					goruntime.KeepAlive(plan)
				}
			})
		}
	}
}

func BenchmarkBuildQueryPlanSeriesSelection(b *testing.B) {
	for _, instances := range queryPlanBenchmarkInstanceCounts() {
		for _, selected := range []string{"all", "one"} {
			for _, overlappingScopes := range []int{1, 4} {
				name := fmt.Sprintf("instances_%d/series_%s/scopes_%d", instances, selected, overlappingScopes)
				b.Run(name, func(b *testing.B) {
					c := seriesSelectionBenchmarkCollector(instances, selected, overlappingScopes)

					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						plan, err := c.buildQueryPlan()
						if err != nil {
							b.Fatal(err)
						}
						goruntime.KeepAlive(plan)
					}
				})
			}
		}
	}
}

func BenchmarkCurrentQueryPlanDirtySeriesSelection(b *testing.B) {
	for _, instances := range queryPlanBenchmarkInstanceCounts() {
		for _, selected := range []string{"all", "one"} {
			for _, overlappingScopes := range []int{1, 4} {
				name := fmt.Sprintf("instances_%d/series_%s/scopes_%d", instances, selected, overlappingScopes)
				b.Run(name, func(b *testing.B) {
					c := seriesSelectionBenchmarkCollector(instances, selected, overlappingScopes)
					initial := requireCurrentQueryPlan(b, c)
					require.NotEmpty(b, initial)
					for _, query := range initial {
						c.observations.queries[query.key] = queryState{lastCompletedEnd: time.Unix(1, 0)}
					}
					expectedQueries := len(initial)

					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						c.invalidateQueryPlan()
						plan, err := c.currentQueryPlan()
						if err != nil {
							b.Fatal(err)
						}
						if len(plan) != expectedQueries {
							b.Fatalf("query plan length = %d, want %d", len(plan), expectedQueries)
						}
						goruntime.KeepAlive(plan)
					}
				})
			}
		}
	}
}

func queryPlanBenchmarkInstanceCounts() []int {
	// The full EC2 fixture exports three series, so 6,000 instances exercise a
	// large valid plan (18,000 queries) below maxPlannedQueries.
	return []int{100, 1000, 6000}
}

func seriesSelectionBenchmarkCollector(instances int, selected string, overlappingScopes int) *Collector {
	values := make([][]string, instances)
	for i := range values {
		values[i] = []string{fmt.Sprintf("i-%d", i)}
	}
	c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{"us-east-1": values})
	c.Logger = logger.NewWithWriter(io.Discard)
	c.Limits.MaxInstances = instances + 1
	if selected == "one" {
		c.plan.Scopes[0].SelectedSeries = c.plan.Scopes[0].SelectedSeries[:1]
	}
	base := c.plan.Scopes[0]
	baseInstances := c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}]
	for i := 1; i < overlappingScopes; i++ {
		target := &collectionTarget{Name: fmt.Sprintf("target-%d", i)}
		scope := base
		scope.Target = target
		c.plan.Scopes = append(c.plan.Scopes, scope)
		c.resolvedByRef[target.Name] = resolvedTarget{target: target, accountID: "123456789012"}
		c.discovery.Instances[discoveryKey{Target: target.Name, Profile: "ec2", Region: "us-east-1"}] = baseInstances
	}
	return c
}
