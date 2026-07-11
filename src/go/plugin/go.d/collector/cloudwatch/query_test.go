// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryWindow(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)

	tests := map[string]struct {
		period, offset     int
		wantStart, wantEnd int64
	}{
		"5m period, default offset":                  {period: 300, offset: 600, wantStart: 999_999_000, wantEnd: 999_999_300},
		"5m period, offset below period uses period": {period: 300, offset: 120, wantStart: 999_999_300, wantEnd: 999_999_600},
		"daily period reads a settled bucket":        {period: 86400, offset: 600, wantStart: 999_820_800, wantEnd: 999_907_200},
		"offset not a period multiple stays aligned": {period: 300, offset: 601, wantStart: 999_999_000, wantEnd: 999_999_300},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			start, end := queryWindow(now, tc.period, tc.offset)
			assert.Equal(t, tc.wantStart, start.Unix(), "start")
			assert.Equal(t, tc.wantEnd, end.Unix(), "end")
			assert.Equal(t, int64(tc.period), end.Unix()-start.Unix(), "window length == period")
			assert.Zero(t, end.Unix()%int64(tc.period), "end is aligned to a period boundary")
			assert.True(t, end.Before(now), "window ends in the past")
		})
	}
}

func ec2QueryProfile() cwprofiles.Profile {
	return cwprofiles.Profile{
		Namespace: "AWS/EC2",
		Period:    300,
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

	ids := make(map[string]bool)
	byInstance := make(map[string][]plannedQuery)
	for _, pq := range plan {
		ids[pq.id] = true
		byInstance[labelValue(pq.labels, "instance_id")] = append(byInstance[labelValue(pq.labels, "instance_id")], pq)
	}
	assert.Len(t, ids, 6, "query Ids are unique")
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
		Period:    300,
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
	group := first[0].groupKey()
	c.observations.nextQueryAt[group] = time.Unix(1_000_000_300, 0)

	c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}] = append(
		c.discovery.Instances[discoveryKey{Target: "base", Profile: "ec2", Region: "us-east-1"}],
		discoveredInstance{DimensionValues: []string{"i-2"}},
	)
	assert.Len(t, requireCurrentQueryPlan(t, c), len(first), "mutating an input without invalidation does not rebuild")

	c.invalidateQueryPlan()
	assert.Len(t, requireCurrentQueryPlan(t, c), len(first)*2, "input invalidation rebuilds the query blueprint")
	assert.NotContains(t, c.observations.nextQueryAt, group, "adding a query to an existing group makes the expanded group immediately due")
}

func filteredOverlapCollector(t *testing.T) *Collector {
	t.Helper()
	c := multiTargetCollector(t, map[string]stsClient{
		"first":  &seqSTS{accounts: []string{"111111111111"}},
		"second": &seqSTS{accounts: []string{"111111111111"}},
	})
	require.NoError(t, c.ensureTargets(context.Background()))
	require.Len(t, c.plan.Scopes, 2)
	join, err := resolveTagJoinProfile(c.plan.Scopes[0].Profile)
	require.NoError(t, err)
	c.plan.TagJoins["ec2"] = join
	c.plan.Scopes[0].TagFilter = []resourceTagFilter{{key: "environment", values: []string{"production"}}}
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

func TestBuildQueryPlan_OrderedResourceTagFiltering(t *testing.T) {
	t.Run("known non-match falls through to lower rule", func(t *testing.T) {
		c := filteredOverlapCollector(t)
		c.tags = tagSnapshot{
			members: tagMembership{0: {"i-1": {}}},
			unknown: map[int]struct{}{}, fetchedAt: time.Unix(1, 0),
		}

		plan := requireBuildQueryPlan(t, c)
		assert.Equal(t, map[string]string{"i-1": "first", "i-2": "second"}, queryOwnersByInstance(plan))
	})

	t.Run("first failure reserves every candidate from lower rules", func(t *testing.T) {
		c := filteredOverlapCollector(t)
		c.tags = tagSnapshot{members: tagMembership{}, unknown: map[int]struct{}{0: {}}}

		assert.Empty(t, requireBuildQueryPlan(t, c))
	})

	t.Run("later failure queries last-known members and reserves the rest", func(t *testing.T) {
		c := filteredOverlapCollector(t)
		c.tags = tagSnapshot{
			members: tagMembership{0: {"i-1": {}}},
			unknown: map[int]struct{}{0: {}},
		}

		plan := requireBuildQueryPlan(t, c)
		assert.Equal(t, map[string]string{"i-1": "first"}, queryOwnersByInstance(plan))
	})
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
	for _, instances := range []int{100, 1000, 10000} {
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

// gmdCloudWatch is a thread-safe GetMetricData fake: every query gets f.value
// unless its Id is in gaps (or gapAll).
type gmdCloudWatch struct {
	mu     sync.Mutex
	calls  int
	err    error
	value  float64
	gaps   map[string]bool
	gapAll bool
	status cwtypes.StatusCode
}

func (f *gmdCloudWatch) ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return &cloudwatch.ListMetricsOutput{}, nil
}

func (f *gmdCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, q := range in.MetricDataQueries {
		id := aws.ToString(q.Id)
		r := cwtypes.MetricDataResult{Id: aws.String(id), StatusCode: f.status}
		if !f.gapAll && !f.gaps[id] {
			r.Values = []float64{f.value}
			r.Timestamps = []time.Time{time.Unix(1, 0)}
		}
		results = append(results, r)
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

func TestExecuteQueries(t *testing.T) {
	one := map[string][][]string{"us-east-1": {{"i-1"}}}
	two := map[string][][]string{"us-east-1": {{"i-1"}, {"i-2"}}}

	tests := map[string]struct {
		instances   map[string][][]string
		fake        *gmdCloudWatch
		wantSamples int
		wantValue   float64
		wantErr     bool
	}{
		"all queries return a value": {
			instances: two, fake: &gmdCloudWatch{value: 42}, wantSamples: 6, wantValue: 42,
		},
		"missing datapoints are gaps": {
			instances: one, fake: &gmdCloudWatch{gapAll: true}, wantSamples: 0,
		},
		"non-finite datapoints are gaps": {
			instances: one, fake: &gmdCloudWatch{value: math.NaN()}, wantSamples: 0,
		},
		"all GetMetricData calls failing errors": {
			instances: one, fake: &gmdCloudWatch{err: errors.New("throttled")}, wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := ec2QueryCollector([]string{"us-east-1"}, tc.instances)
			useFakeClient(c, tc.fake)

			samples, _, _, err := c.executeQueries(context.Background(), requireBuildQueryPlan(t, c), time.Unix(1_000_000_000, 0))
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, samples, tc.wantSamples)
			for _, s := range samples {
				assert.Equal(t, tc.wantValue, s.value)
				assert.NotEmpty(t, s.seriesName)
				assert.Equal(t, "123456789012", labelValue(s.labels, "account_id"))
			}
		})
	}
}

func TestExecuteQueries_ReportsIndependentChunkFailures(t *testing.T) {
	const sensitive = "SENSITIVE_API_MESSAGE"
	var logs bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&logs)
	c.applyDefaults()
	failing := &gmdCloudWatch{err: errors.New(sensitive)}
	c.clients.clients[clientKey{target: "target-a", region: "us-east-1"}] = failing
	c.clients.clients[clientKey{target: "target-b", region: "eu-west-1"}] = failing
	plan := []plannedQuery{
		{id: "q0", target: "target-a", region: "us-east-1", period: 300, query: cwtypes.MetricDataQuery{Id: aws.String("q0")}},
		{id: "q1", target: "target-b", region: "eu-west-1", period: 600, query: cwtypes.MetricDataQuery{Id: aws.String("q1")}},
	}

	_, _, _, err := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
	require.Error(t, err)
	assert.Contains(t, logs.String(), "target-a")
	assert.Contains(t, logs.String(), "us-east-1")
	assert.Contains(t, logs.String(), "target-b")
	assert.Contains(t, logs.String(), "eu-west-1")
	assert.Contains(t, logs.String(), "period 300s")
	assert.Contains(t, logs.String(), "period 600s")
	assert.NotContains(t, logs.String(), sensitive)
}

func TestExecuteQueries_ReportsIndependentForbiddenResults(t *testing.T) {
	var logs bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&logs)
	c.applyDefaults()
	c.clients.clients[clientKey{target: "target-a", region: "us-east-1"}] = &gmdCloudWatch{status: cwtypes.StatusCodeForbidden}
	c.clients.clients[clientKey{target: "target-b", region: "eu-west-1"}] = &gmdCloudWatch{status: cwtypes.StatusCodeForbidden}
	plan := []plannedQuery{
		forbiddenTestQuery("q0", "target-a", "us-east-1", "AWS/EC2", 300),
		forbiddenTestQuery("q1", "target-b", "eu-west-1", "AWS/RDS", 600),
	}

	samples, _, _, err := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
	require.NoError(t, err)
	assert.Empty(t, samples)
	for _, want := range []string{"target-a", "us-east-1", "AWS/EC2", "300s", "target-b", "eu-west-1", "AWS/RDS", "600s"} {
		assert.Contains(t, logs.String(), want)
	}
}

func forbiddenTestQuery(id, target, region, namespace string, period int) plannedQuery {
	return plannedQuery{
		id: id, target: target, region: region, period: period,
		query: cwtypes.MetricDataQuery{
			Id:         aws.String(id),
			MetricStat: &cwtypes.MetricStat{Metric: &cwtypes.Metric{Namespace: aws.String(namespace)}},
		},
	}
}

// TestBuildChunkJobs verifies query batching into GetMetricData-sized chunks (one
// job per chunk) and that a group whose (target, region) client failed is skipped and
// marked failed. chunkSize is an explicit argument, so production passes the fixed
// metricsPerQuery (the AWS 500/call maximum) while tests exercise the multi-chunk
// split without constructing 500+ queries.
func TestBuildChunkJobs(t *testing.T) {
	key := queryGroupKey{target: "base", region: "us-east-1", period: 300}
	groupOf := func(n int) map[queryGroupKey][]cwtypes.MetricDataQuery {
		qs := make([]cwtypes.MetricDataQuery, n)
		for i := range qs {
			qs[i] = cwtypes.MetricDataQuery{Id: aws.String(fmt.Sprintf("q%d", i))}
		}
		return map[queryGroupKey][]cwtypes.MetricDataQuery{key: qs}
	}
	ok := map[clientKey]cloudwatchClient{{target: "base", region: "us-east-1"}: &gmdCloudWatch{}}

	tests := map[string]struct {
		groups       map[queryGroupKey][]cwtypes.MetricDataQuery
		groupClients map[clientKey]cloudwatchClient
		chunkSize    int
		wantJobs     int
		wantQueries  int
		wantFailed   bool
	}{
		"all queries fit one chunk":           {groups: groupOf(6), groupClients: ok, chunkSize: 500, wantJobs: 1, wantQueries: 6},
		"split into even chunks":              {groups: groupOf(6), groupClients: ok, chunkSize: 2, wantJobs: 3, wantQueries: 6},
		"uneven last chunk":                   {groups: groupOf(5), groupClients: ok, chunkSize: 2, wantJobs: 3, wantQueries: 5},
		"group without a target client fails": {groups: groupOf(6), groupClients: map[clientKey]cloudwatchClient{}, chunkSize: 2, wantJobs: 0, wantFailed: true},
	}

	c := New()
	c.applyDefaults()
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			jobs, failed := c.buildChunkJobs(tc.groups, tc.groupClients, time.Unix(1_000_000_000, 0), tc.chunkSize)
			require.Len(t, jobs, tc.wantJobs)
			total := 0
			for _, j := range jobs {
				assert.LessOrEqual(t, len(j.chunk), tc.chunkSize, "no chunk exceeds chunkSize")
				total += len(j.chunk)
			}
			assert.Equal(t, tc.wantQueries, total, "every query is chunked exactly once")
			assert.Equal(t, tc.wantFailed, failed[key])
		})
	}
}

// pagingGMD returns one page per GetMetricData call (with the matching NextToken).
type pagingGMD struct {
	pages  []map[string]float64
	tokens []string
	calls  int
	reqs   []*cloudwatch.GetMetricDataInput
}

func (f *pagingGMD) ListMetrics(context.Context, *cloudwatch.ListMetricsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	return &cloudwatch.ListMetricsOutput{}, nil
}

func (f *pagingGMD) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.reqs = append(f.reqs, in)
	idx := f.calls
	f.calls++
	out := &cloudwatch.GetMetricDataOutput{}
	for id, v := range f.pages[idx] {
		out.MetricDataResults = append(out.MetricDataResults, cwtypes.MetricDataResult{
			Id:         aws.String(id),
			Values:     []float64{v},
			Timestamps: []time.Time{time.Unix(1, 0)},
		})
	}
	if idx < len(f.tokens) && f.tokens[idx] != "" {
		out.NextToken = aws.String(f.tokens[idx])
	}
	return out, nil
}

func TestExecuteQueries_PaginationAndDedup(t *testing.T) {
	c := ec2QueryCollector([]string{"us-east-1"}, map[string][][]string{"us-east-1": {{"i-1"}}})
	plan := requireBuildQueryPlan(t, c)
	require.Len(t, plan, 3) // cpu_utilization_average, duration_average, duration_p90

	fake := &pagingGMD{
		pages: []map[string]float64{
			{plan[0].id: 10}, // page 1: newest value for q0
			{plan[0].id: 99, plan[1].id: 20, plan[2].id: 30}, // page 2: stale dup q0 (ignored) + q1,q2
		},
		tokens: []string{"page2", ""},
	}
	useFakeClient(c, fake)

	samples, _, _, err := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
	require.NoError(t, err)
	require.Len(t, samples, 3)
	assert.Equal(t, 2, fake.calls, "followed NextToken to the second page")

	byName := map[string]float64{}
	for _, s := range samples {
		byName[s.seriesName] = s.value
	}
	assert.Equal(t, float64(10), byName["ec2.cpu_utilization_average"], "first (newest) value per Id wins across pages")
	assert.Equal(t, float64(20), byName["ec2.duration_average"])
	assert.Equal(t, float64(30), byName["ec2.duration_p90"])

	// Each GetMetricData request scans newest-first over the same aligned window,
	// and the second page carries the NextToken the first page returned.
	require.Len(t, fake.reqs, 2)
	for i, r := range fake.reqs {
		assert.Equal(t, cwtypes.ScanByTimestampDescending, r.ScanBy, "req %d ScanBy", i)
		assert.Equal(t, int64(999_999_000), aws.ToTime(r.StartTime).Unix(), "req %d start", i)
		assert.Equal(t, int64(999_999_300), aws.ToTime(r.EndTime).Unix(), "req %d end", i)
	}
	assert.Nil(t, fake.reqs[0].NextToken, "first page has no NextToken")
	assert.Equal(t, "page2", aws.ToString(fake.reqs[1].NextToken), "second page uses the returned token")
}

func TestExecuteQueries_RegionClientFailures(t *testing.T) {
	tests := map[string]struct {
		regions     []string
		instances   map[string][][]string
		failRegion  func(region string) bool
		wantErr     bool
		wantSamples int
		wantRegion  string
	}{
		"all clients fail errors": {
			regions:    []string{"us-east-1"},
			instances:  map[string][][]string{"us-east-1": {{"i-1"}}},
			failRegion: func(string) bool { return true },
			wantErr:    true,
		},
		"a usable region is not aborted by another region's failure": {
			regions:     []string{"good", "bad"},
			instances:   map[string][][]string{"good": {{"i-1"}}, "bad": {{"i-2"}}},
			failRegion:  func(r string) bool { return r == "bad" },
			wantSamples: 3, // only the good region's instance (1 instance x 3 series)
			wantRegion:  "good",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := ec2QueryCollector(tc.regions, tc.instances)
			fake := &gmdCloudWatch{value: 7}
			c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
				if tc.failRegion(region) {
					return aws.Config{}, errors.New("no credentials for region")
				}
				return aws.Config{Region: region}, nil
			}
			c.newCloudWatchClient = func(aws.Config) cloudwatchClient { return fake }

			samples, _, _, err := c.executeQueries(context.Background(), requireBuildQueryPlan(t, c), time.Unix(1_000_000_000, 0))
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, samples, tc.wantSamples)
			for _, s := range samples {
				assert.Equal(t, tc.wantRegion, labelValue(s.labels, "region"))
			}
		})
	}
}
