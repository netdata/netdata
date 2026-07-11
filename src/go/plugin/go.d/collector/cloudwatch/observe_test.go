// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fullCloudWatch serves both ListMetrics (discovery) and GetMetricData (query).
type fullCloudWatch struct {
	mu        sync.Mutex
	list      map[string][]cwtypes.Metric
	gmdValue  float64
	gap       bool
	status    cwtypes.StatusCode // when set, every GetMetricData result carries this status
	omit      bool               // when true, GetMetricData returns no results at all
	listCalls int
	gmdCalls  int
}

func (f *fullCloudWatch) ListMetrics(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return &cloudwatch.ListMetricsOutput{Metrics: f.list[aws.ToString(in.Namespace)]}, nil
}

func (f *fullCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gmdCalls++
	if f.omit {
		return &cloudwatch.GetMetricDataOutput{}, nil // no results for any query
	}
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, q := range in.MetricDataQueries {
		r := cwtypes.MetricDataResult{Id: q.Id}
		if f.status != "" {
			r.StatusCode = f.status
		}
		if !f.gap {
			r.Values = []float64{f.gmdValue}
			r.Timestamps = []time.Time{time.Unix(1, 0)}
		}
		results = append(results, r)
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

func (f *fullCloudWatch) setValue(v float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gmdValue = v
}

func (f *fullCloudWatch) setGap(gap bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gap = gap
}

func (f *fullCloudWatch) getMetricDataCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gmdCalls
}

func endToEndCollector(gmdValue float64) (*Collector, *fullCloudWatch) {
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})

	fake := &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		gmdValue: gmdValue,
	}
	useFakeClient(c, fake)
	return c, fake
}

func seriesValue(t *testing.T, series map[string]metrix.SampleValue, prefix string) metrix.SampleValue {
	t.Helper()
	var found string
	n := 0
	for k := range series {
		if strings.HasPrefix(k, prefix) {
			found = k
			n++
		}
	}
	require.Equalf(t, 1, n, "exactly one series with prefix %q in %v", prefix, seriesKeys(series))
	return series[found]
}

func seriesKeys(series map[string]metrix.SampleValue) []string {
	keys := make([]string, 0, len(series))
	for k := range series {
		keys = append(keys, k)
	}
	return keys
}

func TestCheck_PopulatesChartTemplateBeforeCollect(t *testing.T) {
	// The framework validates ChartTemplateYAML() in postCheck — after Check and
	// before the first Collect — so Check must leave a non-empty, valid template.
	c := New()
	c.Config = validConfig()
	c.applyDefaults()
	c.newCatalog = cwprofiles.LoadFromDefaultDirs
	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: "000000000000"} }

	require.NoError(t, c.Check(context.Background()))
	require.NotEmpty(t, c.ChartTemplateYAML(), "Check must build the chart template (postCheck reads it before Collect)")
	collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
}

func TestObserve_RetentionAndScheduling(t *testing.T) {
	c, fake := endToEndCollector(10)
	base := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return base }

	// Cycle 1: due -> queried, value 10.
	s1, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Len(t, s1, 3)
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s1, `ec2.cpu_utilization_average{`))
	callsAfterCycle1 := fake.getMetricDataCalls()

	// Cycle 2 at +60s: period 300 not due, discovery TTL not expired -> re-emit cached value,
	// no new query even though the source value changed.
	fake.setValue(20)
	c.now = func() time.Time { return base.Add(60 * time.Second) }
	s2, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Len(t, s2, 3, "not-due series stay visible via re-emit")
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s2, `ec2.cpu_utilization_average{`))
	assert.Equal(t, callsAfterCycle1, fake.getMetricDataCalls(), "no GetMetricData when nothing is due")

	// Cycle 3 at +300s: period 300 due again -> re-query, new value 20.
	c.now = func() time.Time { return base.Add(300 * time.Second) }
	s3, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(20), seriesValue(t, s3, `ec2.cpu_utilization_average{`))
	assert.Greater(t, fake.getMetricDataCalls(), callsAfterCycle1, "due series re-queries")
}

// flakeyCloudWatch fails GetMetricData while gmdCalls <= failGMDUntil, then succeeds.
type flakeyCloudWatch struct {
	mu           sync.Mutex
	list         map[string][]cwtypes.Metric
	value        float64
	failGMDUntil int
	gmdCalls     int
}

func (f *flakeyCloudWatch) ListMetrics(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &cloudwatch.ListMetricsOutput{Metrics: f.list[aws.ToString(in.Namespace)]}, nil
}

func (f *flakeyCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gmdCalls++
	if f.gmdCalls <= f.failGMDUntil {
		return nil, errors.New("throttled")
	}
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, q := range in.MetricDataQueries {
		results = append(results, cwtypes.MetricDataResult{Id: q.Id, Values: []float64{f.value}, Timestamps: []time.Time{time.Unix(1, 0)}})
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

func (f *flakeyCloudWatch) getMetricDataCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gmdCalls
}

func TestObserve_PerRegionScheduleIsolation(t *testing.T) {
	ec2 := cwprofiles.Profile{
		Namespace: "AWS/EC2", Period: 300,
		Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "InstanceId", Label: "instance_id"}}},
		Metrics:  []cwprofiles.Metric{{ID: "cpu_utilization", MetricName: "CPUUtilization", Statistics: []string{"average"}}},
	}
	good := &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-good")}},
		gmdValue: 10,
	}
	bad := &flakeyCloudWatch{
		list:         map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-bad")}},
		failGMDUntil: 1 << 30, // always fail GetMetricData
	}
	c := New()
	configureExactRule(c, []string{"us-east-1", "us-west-2"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1", "us-west-2"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2}})
	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newCloudWatchClient = func(cfg aws.Config) cloudwatchClient {
		if cfg.Region == "us-west-2" {
			return bad
		}
		return good
	}
	base := time.Unix(1_000_000_000, 0)

	// Cycle 1: both regions due; good succeeds, bad's query fails (partial pass).
	c.now = func() time.Time { return base }
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err, "a healthy region keeps the cycle from aborting")
	require.Equal(t, 1, good.getMetricDataCalls())
	require.Equal(t, 1, bad.getMetricDataCalls())

	// Cycle 2 at +60s: good's (good,300) advanced -> not due; bad's stayed due.
	// Only the failed region is retried; the healthy one is NOT re-queried.
	c.now = func() time.Time { return base.Add(60 * time.Second) }
	_, err = collecttest.CollectScalarSeries(c) // only bad is due and still failing -> cycle errors
	require.Error(t, err, "the still-failing region makes the cycle error")
	assert.Equal(t, 1, good.getMetricDataCalls(), "healthy region not re-queried because a sibling region failed")
	assert.Greater(t, bad.getMetricDataCalls(), 1, "failed region stays due and is retried next cycle")
}

func TestCollect_QueryFailureRetriesNextCycle(t *testing.T) {
	fake := &flakeyCloudWatch{
		list:         map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		value:        5,
		failGMDUntil: 1, // the first GetMetricData fails
	}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	useFakeClient(c, fake)

	base := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return base }

	// Cycle 1: query fails -> collect errors, the 300s schedule must NOT advance.
	_, err := collecttest.CollectScalarSeries(c)
	require.Error(t, err)

	// Cycle 2 at +60s: without the fix the period would be skipped until +300s; the
	// retry must happen now and succeed.
	c.now = func() time.Time { return base.Add(60 * time.Second) }
	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.NotEmpty(t, series, "a failed period is retried the next cycle, not skipped for a full period")
}

func TestWithTimeout(t *testing.T) {
	ctx, cancel := withTimeout(context.Background(), 0)
	defer cancel()
	_, ok := ctx.Deadline()
	assert.False(t, ok, "non-positive timeout leaves the context unbounded")

	ctx2, cancel2 := withTimeout(context.Background(), time.Second)
	defer cancel2()
	_, ok = ctx2.Deadline()
	assert.True(t, ok, "positive timeout sets a deadline")
}

func TestObserve_DailySeriesSurvivesEvictionWindow(t *testing.T) {
	// A daily (86400s) series is queried once, then re-emitted every cycle so it
	// survives the metrix 10-unseen-cycle eviction (spec retention acceptance).
	dailyProfile := cwprofiles.Profile{
		Namespace: "AWS/S3",
		Period:    86400,
		Instance:  cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "BucketName", Label: "bucket_name"}}},
		Metrics:   []cwprofiles.Metric{{ID: "bucket_size_bytes", MetricName: "BucketSizeBytes", Statistics: []string{"average"}}},
	}
	fake := &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/S3": {mkMetric("BucketSizeBytes", "BucketName", "b1")}},
		gmdValue: 123,
	}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"s3"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "s3", Config: dailyProfile}})
	useFakeClient(c, fake)

	base := time.Unix(1_000_000_000, 0)
	var series map[string]metrix.SampleValue
	for i := range 15 { // > the 10-cycle eviction window
		offset := time.Duration(i*60) * time.Second
		c.now = func() time.Time { return base.Add(offset) }
		var err error
		series, err = collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
	}

	assert.Equal(t, metrix.SampleValue(123), seriesValue(t, series, `s3.bucket_size_bytes_average{`),
		"daily series stays visible across >10 cycles via re-emit")
	assert.Equal(t, 1, fake.getMetricDataCalls(), "daily metric is queried only once across 15 sub-daily cycles")
}

func TestObserve_MultiPeriodScheduling(t *testing.T) {
	// One job with a 300s profile and an 86400s profile: when only the 300s
	// period is due, it re-queries while the daily one re-emits its cached value.
	ec2 := cwprofiles.Profile{
		Namespace: "AWS/EC2", Period: 300,
		Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "InstanceId", Label: "instance_id"}}},
		Metrics:  []cwprofiles.Metric{{ID: "cpu_utilization", MetricName: "CPUUtilization", Statistics: []string{"average"}}},
	}
	s3 := cwprofiles.Profile{
		Namespace: "AWS/S3", Period: 86400,
		Instance: cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "BucketName", Label: "bucket_name"}}},
		Metrics:  []cwprofiles.Metric{{ID: "bucket_size", MetricName: "BucketSizeBytes", Statistics: []string{"average"}}},
	}
	fake := &fullCloudWatch{
		list: map[string][]cwtypes.Metric{
			"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
			"AWS/S3":  {mkMetric("BucketSizeBytes", "BucketName", "b1")},
		},
		gmdValue: 10,
	}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2", "s3"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2}, {Name: "s3", Config: s3}})
	useFakeClient(c, fake)

	base := time.Unix(1_000_000_000, 0)

	// Cycle 1: both periods due -> both queried at value 10.
	c.now = func() time.Time { return base }
	s1, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s1, `ec2.cpu_utilization_average{`))
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s1, `s3.bucket_size_average{`))

	// Cycle 3 at +300s: only the 300s period is due. ec2 re-queries the new value;
	// s3 (not due) re-emits its cached value.
	fake.setValue(20)
	c.now = func() time.Time { return base.Add(300 * time.Second) }
	s3out, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(20), seriesValue(t, s3out, `ec2.cpu_utilization_average{`), "300s period re-queries")
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s3out, `s3.bucket_size_average{`), "daily period re-emits cached value, not re-queried")
}

func TestObserve_GapInQueriedPeriodNotReEmitted(t *testing.T) {
	c, fake := endToEndCollector(10)
	base := time.Unix(1_000_000_000, 0)

	// Cycle 1: queried, value 10.
	c.now = func() time.Time { return base }
	s1, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s1, `ec2.cpu_utilization_average{`))

	// Cycle 2 at +300s: the 300s period is due again but the query returns a gap.
	// A gap in a successfully-queried period is genuine and must NOT be re-emitted.
	fake.setGap(true)
	c.now = func() time.Time { return base.Add(300 * time.Second) }
	s2, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Empty(t, s2, "due+gap series are genuine gaps, not re-emitted")

	// Cycle 3 at +360s: not due. The due gap cleared the cached value (cpu is a
	// gauge = gap policy), so the stale value is NOT resurrected on a not-due
	// cycle — the series stays gapped until fresh data.
	c.now = func() time.Time { return base.Add(360 * time.Second) }
	s3, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Empty(t, s3, "after a due gap, a gap-policy series is not re-emitted from stale cache")

	// Cycle 4 at +600s: the 300s period is due again and returns data -> recovers.
	fake.setGap(false)
	c.now = func() time.Time { return base.Add(600 * time.Second) }
	s4, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(10), seriesValue(t, s4, `ec2.cpu_utilization_average{`),
		"the series recovers when a later query returns data")
}

func TestObserve_RateMetricNoDataZeroFilled(t *testing.T) {
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"lambda"})
	profiles := []cwprofiles.ResolvedProfile{{Name: "lambda", Config: cwprofiles.Profile{
		Namespace: "AWS/Lambda",
		Period:    300,
		Instance:  cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "FunctionName", Label: "function_name"}}},
		Metrics:   []cwprofiles.Metric{{ID: "errors", MetricName: "Errors", Statistics: []string{"sum"}, Rate: true}},
	}}}
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, profiles)

	fake := &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/Lambda": {mkMetric("Errors", "FunctionName", "fn-1")}},
		gmdValue: 5,
	}
	useFakeClient(c, fake)
	base := time.Unix(1_000_000_000, 0)

	// Cycle 1: queried, value 5.
	c.now = func() time.Time { return base }
	s1, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(5), seriesValue(t, s1, `lambda.errors_sum{`))

	// Cycle 2 at +300s: due, no datapoint. A rate/count metric defaults to
	// nil-as-zero, so "no activity" is recorded as 0 (not a gap, not the stale 5).
	fake.setGap(true)
	c.now = func() time.Time { return base.Add(300 * time.Second) }
	s2, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(0), seriesValue(t, s2, `lambda.errors_sum{`), "rate metric with no data records 0")

	// Cycle 3 at +360s: not due -> 0 is held (re-emitted), not the stale 5.
	c.now = func() time.Time { return base.Add(360 * time.Second) }
	s3, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(0), seriesValue(t, s3, `lambda.errors_sum{`), "zero is re-emitted between queries")
}

func TestObserve_RateMetricNoUsableResultGaps(t *testing.T) {
	// A nil-as-zero (rate) metric must GAP, not record a false 0, when the query
	// yields no USABLE datapoint — a per-result error or an absent result. Zero-fill
	// is reserved for a clean result that simply had no datapoint.
	setups := map[string]func(*fullCloudWatch){
		"InternalError": func(f *fullCloudWatch) { f.status = cwtypes.StatusCodeInternalError },
		"Forbidden":     func(f *fullCloudWatch) { f.status = cwtypes.StatusCodeForbidden },
		"absent result": func(f *fullCloudWatch) { f.omit = true },
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			c := New()
			configureExactRule(c, []string{"us-east-1"}, []string{"lambda"})
			profiles := []cwprofiles.ResolvedProfile{{Name: "lambda", Config: cwprofiles.Profile{
				Namespace: "AWS/Lambda",
				Period:    300,
				Instance:  cwprofiles.InstanceSpec{Dimensions: []cwprofiles.InstanceDimension{{Name: "FunctionName", Label: "function_name"}}},
				Metrics:   []cwprofiles.Metric{{ID: "errors", MetricName: "Errors", Statistics: []string{"sum"}, Rate: true}},
			}}}
			setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, profiles)

			fake := &fullCloudWatch{
				list:     map[string][]cwtypes.Metric{"AWS/Lambda": {mkMetric("Errors", "FunctionName", "fn-1")}},
				gmdValue: 5,
			}
			setup(fake)
			useFakeClient(c, fake)
			c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

			series, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			assert.Empty(t, series, "no usable result -> gap, not a false 0")
		})
	}
}

func TestCleanup_ResetsRuntimeState(t *testing.T) {
	c, _ := endToEndCollector(10)
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.NotEmpty(t, c.resolvedByRef)
	require.NotEmpty(t, c.observations.lastObserved)
	require.NotEmpty(t, c.observations.nextQueryAt)

	c.Cleanup(context.Background())

	assert.Empty(t, c.resolvedByRef)
	assert.Nil(t, c.plan)
	assert.Empty(t, c.observations.lastObserved)
	assert.Empty(t, c.observations.nextQueryAt)
	assert.Empty(t, c.discovery.Instances)
}

func TestPruneObserved(t *testing.T) {
	ecLabels := func(instance string) []metrix.Label {
		return []metrix.Label{
			{Key: "account_id", Value: "000000000000"},
			{Key: "region", Value: "us-east-1"},
			{Key: "instance_id", Value: instance},
		}
	}
	groupKey := queryGroupKey{target: "base", region: "us-east-1", period: 300}
	keep := observedSeries{seriesName: "ec2.cpu_utilization_average", labels: ecLabels("i-1"), value: 1, groupKey: groupKey}
	drop := observedSeries{seriesName: "ec2.cpu_utilization_average", labels: ecLabels("i-2"), value: 2, groupKey: groupKey}

	c := New()
	c.observations.lastObserved = map[string]observedSeries{
		observedKey(keep.seriesName, keep.labels): keep,
		observedKey(drop.seriesName, drop.labels): drop,
	}

	// Plan reflects discovery with only i-1 remaining.
	c.observations.pruneObserved([]plannedQuery{{seriesName: keep.seriesName, labels: keep.labels}})

	require.Len(t, c.observations.lastObserved, 1)
	assert.Contains(t, c.observations.lastObserved, observedKey(keep.seriesName, keep.labels))
	assert.NotContains(t, c.observations.lastObserved, observedKey(drop.seriesName, drop.labels))
}

func TestPruneObserved_DropsStaleScheduleForVanishedGroup(t *testing.T) {
	// A group that leaves the plan must lose its schedule entry, so a later
	// reappearance is unscheduled (immediately due) rather than blocked until a stale
	// nextQueryAt expires. The two groups here differ ONLY by target, which pins that
	// target is part of the (target, region, period) schedule key — a regression that
	// dropped target from the key would keep the vanished group.
	c := New()
	inPlan := queryGroupKey{target: "first", region: "us-east-1", period: 300}
	vanished := queryGroupKey{target: "second", region: "us-east-1", period: 300}
	c.observations.nextQueryAt = map[queryGroupKey]time.Time{
		inPlan:   time.Unix(1_000_000_300, 0),
		vanished: time.Unix(1_000_000_300, 0),
	}
	labels := []metrix.Label{
		{Key: "account_id", Value: "111111111111"},
		{Key: "region", Value: "us-east-1"},
		{Key: "instance_id", Value: "i-1"},
	}

	// Plan retains only the first target's group; the second target is gone.
	c.observations.pruneObserved([]plannedQuery{
		{seriesName: "ec2.cpu_utilization_average", labels: labels, target: "first", region: "us-east-1", period: 300},
	})

	assert.Contains(t, c.observations.nextQueryAt, inPlan, "a group still in the plan keeps its schedule")
	assert.NotContains(t, c.observations.nextQueryAt, vanished, "a group that left the plan (differing only by target) loses its schedule")
}
