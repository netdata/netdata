// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

// TestCollect_E2E drives the whole collect pipeline for the REAL stock profiles:
// mock ListMetrics + GetMetricData -> discovery -> dimension filter -> query plan ->
// GetMetricData -> observe -> metrix store, then asserts BOTH the exact produced
// series (name, labels, value) AND chart coverage (every produced series flows into
// a chart, every resolvable chart dimension materializes). Each scenario is scoped
// to one profile via exact mode, so it exercises the shipped profile it names.
//
// Fixtures are built in-code as aws-sdk-go-v2 types. GetMetricData answers are keyed
// on the request's MetricStat (namespace, metric, statistic, dimensions) and echoed
// back under the query's synthetic Id, so fixtures are authored by metric/statistic
// rather than fragile q<N> ids. A key absent from the values map, or a NaN value, is
// dropped by the collector as a gap.
func TestCollect_E2E(t *testing.T) {
	const account = "000000000000"

	scenarios := map[string]e2eScenario{
		"ec2 single dimension, rate sums stored undivided": {
			profiles: []string{"ec2"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/EC2": {
					mkMetric("CPUUtilization", "InstanceId", "i-1"),
					mkMetric("NetworkIn", "InstanceId", "i-1"),
					mkMetric("NetworkOut", "InstanceId", "i-1"),
					mkMetric("DiskReadOps", "InstanceId", "i-1"),
					mkMetric("DiskWriteOps", "InstanceId", "i-1"),
					mkMetric("StatusCheckFailed", "InstanceId", "i-1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"):    3.2,
				e2eKey("AWS/EC2", "NetworkIn", "Sum", "InstanceId", "i-1"):             1500, // raw Sum, undivided in the store
				e2eKey("AWS/EC2", "NetworkOut", "Sum", "InstanceId", "i-1"):            900,
				e2eKey("AWS/EC2", "DiskReadOps", "Sum", "InstanceId", "i-1"):           10,
				e2eKey("AWS/EC2", "DiskWriteOps", "Sum", "InstanceId", "i-1"):          20,
				e2eKey("AWS/EC2", "StatusCheckFailed", "Maximum", "InstanceId", "i-1"): 0,
			},
			wantSeries: map[string]metrix.SampleValue{
				`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:     3.2,
				`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:              1500,
				`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:             900,
				`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:           10,
				`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          20,
				`ec2.status_check_failed_maximum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 0,
			},
		},
		"s3 multi-dimension identity, daily period": {
			profiles: []string{"s3"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/S3": {
					mkMetric("BucketSizeBytes", "BucketName", "b1", "StorageType", "StandardStorage"),
					mkMetric("NumberOfObjects", "BucketName", "b1", "StorageType", "AllStorageTypes"),
				},
			},
			// Each storage type is a distinct instance; the cross-product queries gap.
			gmd: map[string]float64{
				e2eKey("AWS/S3", "BucketSizeBytes", "Average", "BucketName", "b1", "StorageType", "StandardStorage"): 1048576,
				e2eKey("AWS/S3", "NumberOfObjects", "Average", "BucketName", "b1", "StorageType", "AllStorageTypes"): 42,
			},
			wantSeries: map[string]metrix.SampleValue{
				`s3.bucket_size_bytes_average{account_id="000000000000",bucket_name="b1",region="us-east-1",storage_type="StandardStorage"}`: 1048576,
				`s3.number_of_objects_average{account_id="000000000000",bucket_name="b1",region="us-east-1",storage_type="AllStorageTypes"}`: 42,
			},
		},
		"lambda multi-statistic fan-out": {
			profiles: []string{"lambda"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/Lambda": {
					mkMetric("Invocations", "FunctionName", "fn-1"),
					mkMetric("Errors", "FunctionName", "fn-1"),
					mkMetric("Throttles", "FunctionName", "fn-1"),
					mkMetric("Duration", "FunctionName", "fn-1"),
				},
			},
			gmd: map[string]float64{
				e2eKey("AWS/Lambda", "Invocations", "Sum", "FunctionName", "fn-1"):  100,
				e2eKey("AWS/Lambda", "Errors", "Sum", "FunctionName", "fn-1"):       2,
				e2eKey("AWS/Lambda", "Throttles", "Sum", "FunctionName", "fn-1"):    1,
				e2eKey("AWS/Lambda", "Duration", "Average", "FunctionName", "fn-1"): 120.5,
				e2eKey("AWS/Lambda", "Duration", "Maximum", "FunctionName", "fn-1"): 350,
				e2eKey("AWS/Lambda", "Duration", "p90", "FunctionName", "fn-1"):     200,
			},
			wantSeries: map[string]metrix.SampleValue{
				`lambda.invocations_sum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:  100,
				`lambda.errors_sum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:       2,
				`lambda.throttles_sum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:    1,
				`lambda.duration_average{account_id="000000000000",function_name="fn-1",region="us-east-1"}`: 120.5,
				`lambda.duration_maximum{account_id="000000000000",function_name="fn-1",region="us-east-1"}`: 350,
				`lambda.duration_p90{account_id="000000000000",function_name="fn-1",region="us-east-1"}`:     200,
			},
		},
		"alb multi-granularity dimension filter keeps only {LoadBalancer}": {
			profiles: []string{"alb"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/ApplicationELB": {
					mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa"),                                   // keep
					mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "TargetGroup", "tg/x/1"),          // reject (extra dim)
					mkMetric("RequestCount", "LoadBalancer", "app/lb1/aaa", "AvailabilityZone", "us-east-1a"), // reject
					mkMetric("RequestCount", "LoadBalancer", "app/lb2/bbb"),                                   // keep (2nd LB)
				},
			},
			// Only RequestCount is served; the profile's other rate/sum metrics
			// record 0 (no activity) and its gauges gap. The dimension filter is
			// proven by the keys: only {LoadBalancer} instances survive, no
			// target_group / availability_zone fan-out.
			gmd: map[string]float64{
				e2eKey("AWS/ApplicationELB", "RequestCount", "Sum", "LoadBalancer", "app/lb1/aaa"): 50,
				e2eKey("AWS/ApplicationELB", "RequestCount", "Sum", "LoadBalancer", "app/lb2/bbb"): 70,
			},
			wantSeries: map[string]metrix.SampleValue{
				`alb.request_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:             50,
				`alb.active_connection_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:   0,
				`alb.http_code_target_2xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_target_3xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_target_4xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_target_5xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.http_code_elb_3xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:         0,
				`alb.http_code_elb_4xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:         0,
				`alb.http_code_elb_5xx_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:         0,
				`alb.new_connection_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:      0,
				`alb.processed_bytes_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`:           0,
				`alb.rejected_connection_count_sum{account_id="000000000000",load_balancer="app/lb1/aaa",region="us-east-1"}`: 0,
				`alb.request_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:             70,
				`alb.active_connection_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:   0,
				`alb.http_code_target_2xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_target_3xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_target_4xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_target_5xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.http_code_elb_3xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:         0,
				`alb.http_code_elb_4xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:         0,
				`alb.http_code_elb_5xx_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:         0,
				`alb.new_connection_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:      0,
				`alb.processed_bytes_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`:           0,
				`alb.rejected_connection_count_sum{account_id="000000000000",load_balancer="app/lb2/bbb",region="us-east-1"}`: 0,
			},
		},
		"no-data: gauges gap, rate/sum metrics become zero": {
			profiles: []string{"ec2"},
			listMetrics: map[string][]cwtypes.Metric{
				"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
			},
			gmd: map[string]float64{
				e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"):    math.NaN(), // gauge, NaN -> gap
				e2eKey("AWS/EC2", "NetworkOut", "Sum", "InstanceId", "i-1"):            900,
				e2eKey("AWS/EC2", "DiskReadOps", "Sum", "InstanceId", "i-1"):           10,
				e2eKey("AWS/EC2", "DiskWriteOps", "Sum", "InstanceId", "i-1"):          20,
				e2eKey("AWS/EC2", "StatusCheckFailed", "Maximum", "InstanceId", "i-1"): 0,
				// NetworkIn is absent -> no datapoint -> rate/sum metric -> recorded as 0.
			},
			wantSeries: map[string]metrix.SampleValue{
				`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:             900,
				`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:           10,
				`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          20,
				`ec2.status_check_failed_maximum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 0,
				`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:              0,
				// cpu_utilization_average (gauge, NaN) gaps; network_in_sum (rate, no data) records 0.
			},
		},
	}

	for name, tc := range scenarios {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config.Regions = []string{"us-east-1"}
			c.Profiles = ProfilesConfig{
				Mode:      profilesModeExact,
				ModeExact: &ProfilesExactConfig{Entries: profileEntries(tc.profiles)},
			}
			c.applyDefaults()
			c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
			useFakeClient(c, &e2eCloudWatch{list: tc.listMetrics, values: tc.gmd, ts: time.Unix(1, 0)})
			c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

			series, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)

			assert.Equal(t, tc.wantSeries, series)
			collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
			collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
		})
	}
}

type e2eScenario struct {
	profiles    []string
	listMetrics map[string][]cwtypes.Metric
	gmd         map[string]float64 // e2eKey -> value; NaN or absent => gap
	wantSeries  map[string]metrix.SampleValue
}

func profileEntries(names []string) []ProfileEntry {
	out := make([]ProfileEntry, len(names))
	for i, name := range names {
		out[i] = ProfileEntry{Name: name}
	}
	return out
}

// e2eCloudWatch is a fixture-driven fake serving both CloudWatch APIs. ListMetrics
// answers per namespace; GetMetricData answers per (namespace, metric, statistic,
// dimensions), echoing back each query's synthetic Id.
type e2eCloudWatch struct {
	mu      sync.Mutex
	list    map[string][]cwtypes.Metric
	values  map[string]float64
	ts      time.Time
	listErr error // when set, ListMetrics fails (discovery error path)
}

func (f *e2eCloudWatch) ListMetrics(_ context.Context, in *cloudwatch.ListMetricsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &cloudwatch.ListMetricsOutput{Metrics: f.list[aws.ToString(in.Namespace)]}, nil
}

func (f *e2eCloudWatch) GetMetricData(_ context.Context, in *cloudwatch.GetMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	results := make([]cwtypes.MetricDataResult, 0, len(in.MetricDataQueries))
	for _, q := range in.MetricDataQueries {
		r := cwtypes.MetricDataResult{Id: q.Id}
		if q.MetricStat != nil {
			if v, ok := f.values[e2eKeyFromMetricStat(q.MetricStat)]; ok {
				r.Values = []float64{v}
				r.Timestamps = []time.Time{f.ts}
			}
		}
		results = append(results, r)
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

// e2eKey builds the fixture key from (namespace, metric, statistic) plus dimension
// name/value pairs. Dimensions are sorted so authoring order does not matter.
func e2eKey(namespace, metric, stat string, dimNameValue ...string) string {
	pairs := make([]string, 0, len(dimNameValue)/2)
	for i := 0; i+1 < len(dimNameValue); i += 2 {
		pairs = append(pairs, dimNameValue[i]+"="+dimNameValue[i+1])
	}
	sort.Strings(pairs)
	return strings.Join([]string{namespace, metric, stat, strings.Join(pairs, ",")}, "|")
}

func e2eKeyFromMetricStat(ms *cwtypes.MetricStat) string {
	var dimNameValue []string
	if ms.Metric != nil {
		for _, d := range ms.Metric.Dimensions {
			dimNameValue = append(dimNameValue, aws.ToString(d.Name), aws.ToString(d.Value))
		}
		return e2eKey(aws.ToString(ms.Metric.Namespace), aws.ToString(ms.Metric.MetricName), aws.ToString(ms.Stat), dimNameValue...)
	}
	return e2eKey("", "", aws.ToString(ms.Stat))
}

func seriesName(key string) string {
	if before, _, ok := strings.Cut(key, "{"); ok {
		return before
	}
	return key
}

// TestAllStockProfiles_PipelineChartComplete is the full-catalog sweep: for EVERY
// stock profile (combined mode includes the disabled deep-grain ones), it feeds a
// synthetic instance with a datapoint for every (metric, statistic), runs the real
// collect, and asserts (a) every profile's every active series is produced and
// (b) every produced series flows into a chart (AssertChartCoverage). Profiles that
// share a namespace (alb/alb_target, s3/s3_requests, dynamodb/dynamodb_operation)
// get profile-unique dimension values so they never collide.
func TestAllStockProfiles_PipelineChartComplete(t *testing.T) {
	const account = "000000000000"
	const region = "us-east-1"

	cat, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	profiles := cat.AllProfiles()
	require.NotEmpty(t, profiles)

	list := map[string][]cwtypes.Metric{}
	values := map[string]float64{}
	wantNames := map[string]struct{}{}

	for _, rp := range profiles {
		prof := rp.Config
		require.NotEmptyf(t, prof.Metrics, "%s has no metrics", rp.Name)
		require.NotEmptyf(t, prof.Instance.Dimensions, "%s has no instance dimensions", rp.Name)

		var dimPairs []string
		for i, d := range prof.Instance.Dimensions {
			dimPairs = append(dimPairs, d.Name, fmt.Sprintf("%s-%d", rp.Name, i))
		}

		// A metric under the instance's dimensions so discovery finds it.
		list[prof.Namespace] = append(list[prof.Namespace], mkMetric(prof.Metrics[0].MetricName, dimPairs...))

		for _, m := range prof.Metrics {
			for _, stat := range m.Statistics {
				token := cwprofiles.NormalizeStatistic(stat)
				require.NotEmptyf(t, token, "%s.%s has bad statistic %q", rp.Name, m.ID, stat)
				values[e2eKey(prof.Namespace, m.MetricName, cwprofiles.StatString(token), dimPairs...)] = 1
				wantNames[cwprofiles.ExportedSeriesName(rp.Name, m.ID, token)] = struct{}{}
			}
		}
	}

	c := New()
	c.Config.Regions = []string{region}
	c.Profiles = ProfilesConfig{Mode: profilesModeCombined} // include disabled deep-grain profiles
	c.applyDefaults()
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
	useFakeClient(c, &e2eCloudWatch{list: list, values: values, ts: time.Unix(1, 0)})
	c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)

	gotNames := make(map[string]struct{}, len(series))
	for k := range series {
		gotNames[seriesName(k)] = struct{}{}
	}
	assert.Equal(t, wantNames, gotNames, "every stock profile's every active (metric, statistic) must produce a series")
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
}

// TestCollect_MultiRegion verifies that one instance discovered in several regions
// produces one series per region (region is part of the identity).
func TestCollect_MultiRegion(t *testing.T) {
	const account = "000000000000"

	c := New()
	c.Config.Regions = []string{"us-east-1", "eu-west-1"}
	c.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesExactConfig{Entries: profileEntries([]string{"ec2"})}}
	c.applyDefaults()
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
	useFakeClient(c, &e2eCloudWatch{
		list:   map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
		values: map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 5},
		ts:     time.Unix(1, 0),
	})
	c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }

	series, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)

	// cpu is a gauge with data in both regions; the ec2 rate/sum metrics have no
	// data and record 0 per region; status_check_failed (gauge) gaps.
	assert.Equal(t, map[string]metrix.SampleValue{
		`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 5,
		`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`: 5,
		`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          0,
		`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:          0,
		`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:         0,
		`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:         0,
		`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:       0,
		`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:       0,
		`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:      0,
		`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="eu-west-1"}`:      0,
	}, series)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

// TestCollect_DiscoveryFailSoft covers the fail-soft discovery contract end-to-end.
func TestCollect_DiscoveryFailSoft(t *testing.T) {
	const account = "000000000000"

	newBase := func(regions ...string) *Collector {
		c := New()
		c.Config.Regions = regions
		c.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesExactConfig{Entries: profileEntries([]string{"ec2"})}}
		c.applyDefaults()
		c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: account} }
		c.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
		return c
	}

	t.Run("total discovery failure on the first pass errors the collect", func(t *testing.T) {
		c := newBase("us-east-1")
		useFakeClient(c, &e2eCloudWatch{listErr: errors.New("AccessDenied")})
		_, err := collecttest.CollectScalarSeries(c)
		require.Error(t, err)
	})

	t.Run("partial region failure is tolerated", func(t *testing.T) {
		c := newBase("us-east-1", "eu-west-1")
		good := &e2eCloudWatch{
			list:   map[string][]cwtypes.Metric{"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")}},
			values: map[string]float64{e2eKey("AWS/EC2", "CPUUtilization", "Average", "InstanceId", "i-1"): 7},
			ts:     time.Unix(1, 0),
		}
		bad := &e2eCloudWatch{listErr: errors.New("region unavailable")}
		useFakeClient(c, good) // sets the region-aware newAWSConfig
		c.newCloudWatchClient = func(cfg aws.Config) cloudwatchClient {
			if cfg.Region == "eu-west-1" {
				return bad
			}
			return good
		}

		series, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
		assert.Equal(t, map[string]metrix.SampleValue{
			`ec2.cpu_utilization_average{account_id="000000000000",instance_id="i-1",region="us-east-1"}`: 7,
			`ec2.network_in_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:          0,
			`ec2.network_out_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:         0,
			`ec2.disk_read_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:       0,
			`ec2.disk_write_ops_sum{account_id="000000000000",instance_id="i-1",region="us-east-1"}`:      0,
		}, series, "the healthy region still produces its series (rate metrics with no data record 0)")
	})
}
