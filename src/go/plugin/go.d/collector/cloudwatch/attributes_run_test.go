// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/aws/smithy-go"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollect_ExplicitTagPreservedWhenRDSPermissionMissing(t *testing.T) {
	cfg := rdsProfileConfig("base")
	cfg.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "engine"}, {Key: "owner"}}
	cw := &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")},
		gmdValue: 1,
	}
	c := rdsAttributeCollector(
		t,
		cfg,
		cw,
		map[string]string{"base": "000000000000"},
		map[string]rdsClient{"base": &fakeRDS{
			err: &smithy.GenericAPIError{
				Code: "AccessDenied",
			},
		}},
	)
	c.newRGTAClient = func(aws.Config) rgtaClient {
		return &fakeRGTA{
			resources: []rgtatypes.ResourceTagMapping{
				rgtaResource("arn:aws:rds:us-east-1:000000000000:db:db-a", "engine", "configured-value", "owner", "dba"),
			},
		}
	}
	_, err := collectWithAttributes(t, c)
	require.NoError(t, err)
	got := labelsByInstance(c, rdsCPUSeries)["000000000000/db-a"]
	require.Equal(t, "dba", got["owner"])
	assert.Equal(
		t,
		"configured-value",
		got["engine"],
		"an optional lookup must not remove a previously configured label",
	)
}

func startAttributeRun(t *testing.T, c *Collector) {
	t.Helper()
	// t.Context is canceled before cleanup runs, which stops the runner.
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(t.Context(), func() { close(ready) }) }()
	t.Cleanup(func() {
		select {
		case err := <-done:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("attribute runner did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("attribute runner did not become ready")
	}
}

func waitAttributeResult(t *testing.T, c *Collector) {
	t.Helper()
	require.Eventually(t, func() bool { return len(c.attributeResults) > 0 }, 5*time.Second, time.Millisecond,
		"attribute worker did not publish its result")
}

// Drive complete public collection cycles until the fake's background lookup has
// been consumed. Tests of nonblocking collection call CollectScalarSeries directly.
func collectWithAttributes(t *testing.T, c *Collector) (map[string]metrix.SampleValue, error) {
	t.Helper()
	values, err := collecttest.CollectScalarSeries(c)
	for err == nil && c.attributePending {
		waitAttributeResult(t, c)
		values, err = collecttest.CollectScalarSeries(c)
	}
	return values, err
}

func refreshAttributesForTest(t *testing.T, c *Collector) {
	t.Helper()
	c.refreshAttributes(context.Background())
	for c.attributePending {
		waitAttributeResult(t, c)
		c.refreshAttributes(context.Background())
	}
}

// A call holds its own response channel, so a test can change discovery while a
// specific metadata snapshot is in flight without mutating collector state.
type heldRDSCall struct {
	response chan []rdstypes.DBInstance
	canceled chan struct{}
}

type heldRDS struct{ calls chan heldRDSCall }

func (f *heldRDS) DescribeDBInstances(
	ctx context.Context,
	_ *rds.DescribeDBInstancesInput,
	_ ...func(*rds.Options),
) (*rds.DescribeDBInstancesOutput, error) {
	call := heldRDSCall{
		response: make(chan []rdstypes.DBInstance, 1),
		canceled: make(chan struct{}),
	}
	select {
	case f.calls <- call:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case instances := <-call.response:
		return &rds.DescribeDBInstancesOutput{
			DBInstances: instances,
		}, nil
	case <-ctx.Done():
		close(call.canceled)
		return nil, ctx.Err()
	}
}

func nextHeldRDSCall(t *testing.T, f *heldRDS) heldRDSCall {
	t.Helper()
	select {
	case call := <-f.calls:
		return call
	case <-time.After(5 * time.Second):
		t.Fatal("RDS lookup did not start")
		return heldRDSCall{}
	}
}

func TestCollect_RDSLookupDoesNotBlockMetricsOrQueueDuplicateBatches(t *testing.T) {
	cfg := rdsProfileConfig("base")
	cfg.Rules[0].Profiles.Include = []string{"rds", "ec2"}
	cw := &fullCloudWatch{
		list: map[string][]cwtypes.Metric{
			"AWS/RDS": rdsMetrics("db-a"),
			"AWS/EC2": {mkMetric("CPUUtilization", "InstanceId", "i-1")},
		},
		gmdValue: 1,
	}
	fake := &heldRDS{calls: make(chan heldRDSCall, 2)}
	c := rdsAttributeCollector(t, cfg, cw,
		map[string]string{"base": "000000000000"}, map[string]rdsClient{"base": fake})
	type result struct {
		values map[string]metrix.SampleValue
		err    error
	}
	done := make(chan result, 1)
	go func() { values, err := collecttest.CollectScalarSeries(c); done <- result{values, err} }()
	call := nextHeldRDSCall(t, fake)
	select {
	case result := <-done:
		require.NoError(t, result.err)
		require.Equal(t, metrix.SampleValue(1), seriesValue(t, result.values, "ec2.cpu_utilization_average{"))
	case <-time.After(time.Second):
		call.response <- nil
		<-done
		t.Fatal("numeric collection waited for optional RDS metadata")
	}
	require.Positive(t, cw.getMetricDataCalls())
	for range 3 {
		_, err := collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
	}
	assert.Empty(t, fake.calls, "one outstanding batch regardless of collection ticks")
	call.response <- []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8", "db.t3.micro")}
	_, err := collectWithAttributes(t, c)
	require.NoError(t, err)
	assert.Equal(t, "mysql", labelsByInstance(c, rdsCPUSeries)["000000000000/db-a"]["engine"])
}

func TestCollect_RDSResultUsesCurrentDiscovery(t *testing.T) {
	fake := &heldRDS{calls: make(chan heldRDSCall, 2)}
	c, cw := singleRDSTargetCollector(t, fake, "db-old")
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	oldCall := nextHeldRDSCall(t, fake)

	now = now.Add(6 * time.Minute)
	cw.mu.Lock()
	cw.list["AWS/RDS"] = rdsMetrics("db-new")
	cw.mu.Unlock()
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.NotContains(t, labelsByInstance(c, rdsCPUSeries), "000000000000/db-old")
	oldCall.response <- []rdstypes.DBInstance{rdsInstance("db-old", "mysql", "8", "db.t3.micro")}
	waitAttributeResult(t, c)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	for _, group := range c.attributes {
		assert.NotContains(t, group.labels, "db-old")
	}
	next := nextHeldRDSCall(t, fake)
	next.response <- []rdstypes.DBInstance{rdsInstance("db-new", "postgres", "16", "db.t3.micro")}
	_, err = collectWithAttributes(t, c)
	require.NoError(t, err)
	assert.Equal(t, "postgres", labelsByInstance(c, rdsCPUSeries)["000000000000/db-new"]["engine"])
}

func TestCollect_RDSSlowResultWithUnchangedDiscoveryWaitsForNextRefresh(t *testing.T) {
	fake := &heldRDS{calls: make(chan heldRDSCall, 2)}
	c, _ := singleRDSTargetCollector(t, fake, "db-a")
	now := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return now }
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	call := nextHeldRDSCall(t, fake)

	// Discovery runs again while the metadata request is still outstanding.
	now = now.Add(6 * time.Minute)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	call.response <- []rdstypes.DBInstance{rdsInstance("db-a", "mysql", "8", "db.t3.micro")}
	waitAttributeResult(t, c)
	for range 3 {
		_, err = collecttest.CollectScalarSeries(c)
		require.NoError(t, err)
	}
	assert.Equal(t, "mysql", labelsByInstance(c, rdsCPUSeries)["000000000000/db-a"]["engine"])
	assert.False(t, c.attributePending, "unchanged discovery must not cause an immediate retry")
	assert.Empty(t, fake.calls)

	now = now.Add(6 * time.Minute)
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	nextHeldRDSCall(t, fake).response <- nil
}

func TestRDSAttributes_ManagedRunnerCancellation(t *testing.T) {
	cfg := rdsProfileConfig("base")
	cw := &fullCloudWatch{
		list:     map[string][]cwtypes.Metric{"AWS/RDS": rdsMetrics("db-a")},
		gmdValue: 1,
	}
	fake := &heldRDS{
		calls: make(chan heldRDSCall, 1),
	}
	c := New()
	c.Config = cfg
	c.newCatalog = cwprofiles.DefaultCatalog
	useFakeClient(c, cw)
	c.newSTSClient = func(aws.Config) stsClient {
		return &fakeSTS{
			account: "000000000000",
		}
	}
	c.newRDSClient = func(aws.Config) rdsClient { return fake }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "go.d",
		Name:        "test",
		ModuleName:  "cloudwatch",
		FullName:    "cloudwatch_test",
		Module:      c,
		Out:         io.Discard,
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		run.Stop(nil)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("managed job did not stop")
		}
	})
	select {
	case <-run.StartupDone():
	case <-time.After(5 * time.Second):
		t.Fatal("runner readiness waited for metadata")
	}
	require.NoError(t, run.StartupErr())
	require.Eventually(t, func() bool { job.Tick(1); return len(fake.calls) > 0 }, 5*time.Second, time.Millisecond)
	call := nextHeldRDSCall(t, fake)
	run.Stop(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("managed job did not join the metadata worker")
	}
	select {
	case <-call.canceled:
	default:
		t.Fatal("runner stop did not cancel the RDS request")
	}
	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	assert.Empty(t, c.attributes)
	assert.False(t, c.attributePending)
	assert.Empty(t, c.attributeRequests)
	assert.Empty(t, c.attributeResults)
}

func TestDescribeRDSInstanceAttributes_MoreThanOneHundredPages(t *testing.T) {
	pages := make([][]rdstypes.DBInstance, 101)
	pages[100] = []rdstypes.DBInstance{rdsInstance("last", "mysql", "8", "db.t3.micro")}
	fake := &fakeRDS{
		pages: pages,
	}
	labels, err := describeRDSInstanceAttributes(context.Background(), fake)
	require.NoError(t, err)
	assert.Contains(t, labels, "last")
	assert.Equal(t, 101, fake.callCount())
}
