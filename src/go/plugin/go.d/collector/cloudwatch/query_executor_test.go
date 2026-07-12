// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		status := f.status
		if status == "" {
			status = cwtypes.StatusCodeComplete
		}
		r := cwtypes.MetricDataResult{Id: aws.String(id), StatusCode: status}
		if !f.gapAll && !f.gaps[id] {
			r.Values = []float64{f.value}
			r.Timestamps = []time.Time{aws.ToTime(in.EndTime).Add(-time.Duration(aws.ToInt32(q.MetricStat.Period)) * time.Second)}
		}
		results = append(results, r)
	}
	return &cloudwatch.GetMetricDataOutput{MetricDataResults: results}, nil
}

func TestExecuteQueries(t *testing.T) {
	one := map[string][][]string{"us-east-1": {{"i-1"}}}
	two := map[string][][]string{"us-east-1": {{"i-1"}, {"i-2"}}}

	tests := map[string]struct {
		instances     map[string][][]string
		fake          *gmdCloudWatch
		wantOutcomes  int
		wantTerminal  int
		wantTransient int
		wantValue     float64
	}{
		"all queries return a value": {
			instances: two, fake: &gmdCloudWatch{value: 42}, wantOutcomes: 6, wantTerminal: 6, wantValue: 42,
		},
		"missing datapoints are gaps": {
			instances: one, fake: &gmdCloudWatch{gapAll: true}, wantOutcomes: 3, wantTerminal: 3,
		},
		"non-finite datapoints are gaps": {
			instances: one, fake: &gmdCloudWatch{value: math.NaN()}, wantOutcomes: 3, wantTerminal: 3,
		},
		"all GetMetricData calls failing are transient": {
			instances: one, fake: &gmdCloudWatch{err: errors.New("throttled")}, wantOutcomes: 3, wantTransient: 3,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := ec2QueryCollector([]string{"us-east-1"}, tc.instances)
			useFakeClient(c, tc.fake)

			plan := requireBuildQueryPlan(t, c)
			execution := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
			assert.Len(t, execution.outcomes, tc.wantOutcomes)
			assert.Equal(t, tc.wantTerminal, execution.terminal)
			assert.Equal(t, tc.wantTransient, execution.transient)
			for _, outcome := range execution.outcomes {
				if outcome.hasDatapoint {
					assert.Equal(t, tc.wantValue, outcome.value)
				}
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
		testPlannedQuery("a", "target-a", "us-east-1", "AWS/EC2", 300),
		testPlannedQuery("b", "target-b", "eu-west-1", "AWS/RDS", 600),
	}

	execution := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
	assert.Equal(t, 2, execution.transient)
	assert.Contains(t, logs.String(), "target-a")
	assert.Contains(t, logs.String(), "us-east-1")
	assert.Contains(t, logs.String(), "target-b")
	assert.Contains(t, logs.String(), "eu-west-1")
	assert.Contains(t, logs.String(), "period 5m0s")
	assert.Contains(t, logs.String(), "period 10m0s")
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
		testPlannedQuery("a", "target-a", "us-east-1", "AWS/EC2", 300),
		testPlannedQuery("b", "target-b", "eu-west-1", "AWS/RDS", 600),
	}

	execution := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
	assert.Equal(t, 2, execution.terminal)
	for _, outcome := range execution.outcomes {
		assert.Equal(t, queryOutcomeForbidden, outcome.kind)
	}
	for _, want := range []string{"target-a", "us-east-1", "AWS/EC2", "300s", "target-b", "eu-west-1", "AWS/RDS", "600s"} {
		assert.Contains(t, logs.String(), want)
	}
}

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
			Timestamps: []time.Time{aws.ToTime(in.EndTime).Add(-5 * time.Minute)},
			StatusCode: cwtypes.StatusCodeComplete,
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
			{"q0": 10},
			{"q0": 99, "q1": 20, "q2": 30},
		},
		tokens: []string{"page2", ""},
	}
	useFakeClient(c, fake)

	execution := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
	require.Len(t, execution.outcomes, 3)
	assert.Equal(t, 2, fake.calls, "followed NextToken to the second page")

	byName := map[string]float64{}
	for _, query := range plan {
		byName[query.seriesName] = execution.outcomes[query.key].value
	}
	var values []float64
	for _, value := range byName {
		values = append(values, value)
	}
	assert.ElementsMatch(t, []float64{10, 20, 30}, values)
	assert.NotContains(t, values, float64(99), "the duplicate q0 value on the later page must not replace the first candidate")

	// Each GetMetricData request scans newest-first over the same aligned window,
	// and the second page carries the NextToken the first page returned.
	require.Len(t, fake.reqs, 2)
	for i, r := range fake.reqs {
		assert.Equal(t, cwtypes.ScanByTimestampDescending, r.ScanBy, "req %d ScanBy", i)
		assert.Equal(t, int64(999_999_000), aws.ToTime(r.StartTime).Unix(), "req %d start", i)
		assert.Equal(t, int64(999_999_300), aws.ToTime(r.EndTime).Unix(), "req %d end", i)
		assert.Equal(t, int32(3), aws.ToInt32(r.MaxDatapoints), "req %d exact MaxDatapoints", i)
	}
	assert.Nil(t, fake.reqs[0].NextToken, "first page has no NextToken")
	assert.Equal(t, "page2", aws.ToString(fake.reqs[1].NextToken), "second page uses the returned token")
}

func TestExecuteQueries_RegionClientFailures(t *testing.T) {
	tests := map[string]struct {
		regions       []string
		instances     map[string][][]string
		failRegion    func(region string) bool
		wantTerminal  int
		wantTransient int
	}{
		"all clients fail errors": {
			regions:       []string{"us-east-1"},
			instances:     map[string][][]string{"us-east-1": {{"i-1"}}},
			failRegion:    func(string) bool { return true },
			wantTransient: 3,
		},
		"a usable region is not aborted by another region's failure": {
			regions:       []string{"good", "bad"},
			instances:     map[string][][]string{"good": {{"i-1"}}, "bad": {{"i-2"}}},
			failRegion:    func(r string) bool { return r == "bad" },
			wantTerminal:  3, // only the good region's instance (1 instance x 3 series)
			wantTransient: 3,
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

			plan := requireBuildQueryPlan(t, c)
			execution := c.executeQueries(context.Background(), plan, time.Unix(1_000_000_000, 0))
			assert.Equal(t, tc.wantTerminal, execution.terminal)
			assert.Equal(t, tc.wantTransient, execution.transient)
			for _, query := range plan {
				outcome, ok := execution.outcomes[query.key]
				require.True(t, ok)
				if tc.failRegion(labelValue(query.labels, "region")) {
					assert.Equal(t, queryOutcomeTransient, outcome.kind)
				} else {
					assert.Equal(t, queryOutcomeComplete, outcome.kind)
				}
			}
		})
	}
}
