// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestConfigurationSerialization(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestCollector_InitValidatesConfiguration(t *testing.T) {
	tests := map[string]struct {
		modify   func(*Config)
		contains string
	}{
		"missing endpoint":              {func(c *Config) { c.Endpoint = "" }, "endpoint is not set"},
		"endpoint credentials":          {func(c *Config) { c.Endpoint = "https://key:secret@example.net" }, "without credentials"},
		"endpoint path":                 {func(c *Config) { c.Endpoint = "https://example.net/path" }, "without credentials, path"},
		"missing region":                {func(c *Config) { c.Region = "" }, "region is not set"},
		"invalid bucket":                {func(c *Config) { c.Bucket = "invalid_bucket" }, "valid S3 bucket name"},
		"bucket with empty label":       {func(c *Config) { c.Bucket = "invalid..bucket" }, "valid S3 bucket name"},
		"bucket with IP address":        {func(c *Config) { c.Bucket = "192.0.2.1" }, "valid S3 bucket name"},
		"prefix missing trailing slash": {func(c *Config) { c.Prefix = "probe" }, "prefix must end with"},
		"prefix traversal":              {func(c *Config) { c.Prefix = "probe/../" }, "must not contain '.'"},
		"timeout too small":             {func(c *Config) { c.Timeout = confopt.Duration(100 * time.Millisecond) }, "timeout must be between"},
		"too many retries":              {func(c *Config) { c.MaxRetries = 3 }, "max_retries must be between"},
		"latency objective negative":    {func(c *Config) { c.LatencyThresholdMS = -1 }, "latency_threshold_ms must be between"},
		"retry backoff omitted by short interval": {
			func(c *Config) { c.UpdateEvery = 33 },
			"does not fit update_every",
		},
		"worst case exceeds update every": {
			func(c *Config) { c.UpdateEvery = 10; c.MaxRetries = 2 },
			"does not fit update_every",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = validTestConfig()
			test.modify(&collr.Config)
			err := collr.Init(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.contains)
		})
	}
}

func TestCollector_InitAcceptsCompleteRetryDeadlineBudget(t *testing.T) {
	collr := New()
	collr.Config = validTestConfig()
	collr.Config.UpdateEvery = 80
	assert.Equal(t, 80*time.Second, collr.worstCaseDuration())
	require.NoError(t, collr.Init(context.Background()))

	collr.Config.UpdateEvery = 79
	err := collr.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not fit update_every")
}

func TestHealthAlertsMatchCollectorContract(t *testing.T) {
	content, err := os.ReadFile("../../../../../health/health.d/s3check.conf")
	require.NoError(t, err)
	text := string(content)
	assert.Contains(t, text, "template: s3check_stage_failed")
	assert.Contains(t, text, "on: s3check.stage_status")
	assert.Contains(t, text, "calc: $failed")
	assert.Contains(t, text, "crit: ($this == nan or $this == inf) ? (nan) : ($this == 1)")
	assert.Contains(t, text, "to: sysadmin")
	assert.Contains(t, text, "template: s3check_stage_latency")
	assert.Contains(t, text, "on: s3check.stage_latency_status")
	assert.Contains(t, text, "calc: $exceeded")
	assert.Contains(t, text, "to: silent")
}

func TestCollector_CheckRejectsVersionedOrSuspendedBucket(t *testing.T) {
	for _, status := range []string{"Enabled", "Suspended"} {
		t.Run(status, func(t *testing.T) {
			collr, client := newTestCollector(t)
			client.versioningStatus = status
			err := collr.Check(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported S3 bucket versioning status")
		})
	}
}

func TestCollector_CheckRedactsVersioningFailure(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["versioning"] = errors.New("secret-endpoint-detail")

	err := collr.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "S3 bucket versioning check failed")
	assert.NotContains(t, err.Error(), "secret-endpoint-detail")
}

func TestCollector_CheckAcceptsUnversionedBucket(t *testing.T) {
	collr, _ := newTestCollector(t)
	assert.NoError(t, collr.Check(context.Background()))
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	collr := New()

	collecttest.AssertChartTemplateSchema(t, collr.ChartTemplateYAML())
	spec, err := charttpl.DecodeYAML([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestCollector_CollectSuccessfulLifecycle(t *testing.T) {
	collr, client := newTestCollector(t)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	for _, stage := range stageOrder {
		assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stage, reasonOK))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[stateMetricKey("stage_status", "failed", stageLabels(stage, reasonOK))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[stateMetricKey("stage_status", "skipped", stageLabels(stage, reasonOK))], stage)
		assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stage))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stage))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_failures_total", stageOnlyLabels(stage))], stage)
	}

	require.NotNil(t, metrics[metricKey("stage_duration_ms", stageLabels(stagePut, reasonOK))])
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"list", "put", "get", "list", "delete", "head"}, client.operations())

	// Every cycle reconciles the prefix before a new write, catching a PUT that
	// commits after an earlier ambiguous timeout and immediate absence check.
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"list", "put", "get", "list", "delete", "head",
		"list", "put", "get", "list", "delete", "head",
	}, client.operations())
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_CollectReportsPayloadMismatchAndCleansObject(t *testing.T) {
	collr, client := newTestCollector(t)
	client.mutateGet = true

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageGet, reasonPayloadMismatch))])
	assert.Equal(t, metrix.SampleValue(0), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageList, reasonNotRun))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_failures_total", stageOnlyLabels(stageGet))])
	assert.Empty(t, client.objects)
	assert.Contains(t, client.operations(), "delete")
	assert.Contains(t, client.operations(), "head")
}

func TestCollector_CollectCleansAfterPutFailure(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["put"] = errors.New("secret-endpoint-detail")

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stagePut, reasonRequestFailed))])
	assert.Equal(t, metrix.SampleValue(0), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageGet, reasonNotRun))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	for key := range metrics {
		assert.NotContains(t, key, "secret-endpoint-detail")
	}
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"list", "put", "delete", "head"}, client.operations())
}

func TestCollector_CollectKeepsDeleteAndCleanupFailureDistinct(t *testing.T) {
	collr, client := newTestCollector(t)
	client.alwaysExists = true

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageDelete, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageCleanup, reasonStillPresent))])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_failures_total", stageOnlyLabels(stageCleanup))])
}

func TestCollector_CollectClassifiesPartialTimeout(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["get"] = context.DeadlineExceeded

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageGet, reasonTimeout))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Empty(t, client.objects)
}

func TestCollector_CollectReportsMissingListVisibility(t *testing.T) {
	collr, client := newTestCollector(t)
	client.hideObjectsInList = true

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageList, reasonNotVisible))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Empty(t, client.objects)
}

func TestCollector_CollectReportsHeadFailureAfterDelete(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["head"] = errors.New("secret-endpoint-detail")

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageDelete, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageCleanup, reasonRequestFailed))])
	assert.Empty(t, client.objects)
}

func TestCollector_CollectReconcilesLateCommitBeforeNewWrite(t *testing.T) {
	collr, client := newTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	before := len(client.operations())

	lateKey := staleProbeKey(77)
	client.objects[lateKey] = []byte("late-commit")
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(2), metrics[metricKey("stage_operations_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stageCleanup))])
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"list", "delete"}, client.operations()[before:])
}

func TestCollector_CollectAccountsRetries(t *testing.T) {
	collr, client := newTestCollector(t)
	client.attempts["put"] = 3

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	assert.Equal(t, metrix.SampleValue(3), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stagePut))])
	assert.Equal(t, metrix.SampleValue(2), metrics[metricKey("stage_retries_total", stageOnlyLabels(stagePut))])
}

func TestCollector_CollectKeepsCumulativeCountersAcrossReasonChanges(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["put"] = errors.New("temporary failure")

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	delete(client.failures, "put")

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(2), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stagePut))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stagePut))])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_failures_total", stageOnlyLabels(stagePut))])
}

func TestCollector_CollectUsesConfiguredLatencyObjective(t *testing.T) {
	collr, _ := newTestCollector(t)
	clock := &steppingClock{now: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC), step: 2 * time.Millisecond}
	collr.now = clock.tick
	collr.Config.LatencyThresholdMS = 1

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_latency_exceeded", stageLabels(stagePut, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_latency_exceeded", stageLabels(stageGet, reasonOK))])
}

func TestCollector_CollectPerformsBoundedRestartCleanup(t *testing.T) {
	collr, client := newTestCollector(t)
	for i := 0; i < cleanupBatchSize; i++ {
		key := staleProbeKey(i)
		client.staleKeys[key] = true
	}

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(3), metrics[metricKey("stage_operations_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(3), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stageCleanup))])
	assert.Empty(t, client.staleKeys)
	assert.Equal(t, []string{"list", "delete", "delete", "delete"}, client.operations())

	metrics, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageSetup, reasonOK))])
	assert.Equal(t, []string{
		"list", "delete", "delete", "delete",
		"list", "put", "get", "list", "delete", "head",
	}, client.operations())
}

func TestCollector_CollectReportsRestartCleanupFailure(t *testing.T) {
	collr, client := newTestCollector(t)
	client.staleKeys[staleProbeKey(0)] = true
	client.failures["delete"] = context.DeadlineExceeded

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageCleanup, reasonTimeout))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending))])
	assert.Len(t, client.staleKeys, 1)
}

func TestCollector_CollectReturnsRuntimeCancellation(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["list"] = context.Canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	managed, ok := metrix.AsCycleManagedStore(collr.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	err := collr.Collect(ctx)
	require.ErrorIs(t, err, context.Canceled)
	cycle.AbortCycle()
}

func TestCollector_CleanupDeletesPendingObjectAndIsIdempotent(t *testing.T) {
	collr, client := newTestCollector(t)
	key := staleProbeKey(99)
	collr.currentKey = key
	collr.objectMayExist = true
	collr.cleanupCompleted = false
	client.objects[key] = []byte("payload")

	collr.Cleanup(context.Background())
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"head", "delete", "head"}, client.operations())
	assert.Empty(t, collr.currentKey)
	assert.False(t, collr.objectMayExist)

	collr.Cleanup(context.Background())
	assert.Len(t, client.operations(), 3)
}

func validTestConfig() Config {
	return Config{
		UpdateEvery:        defaultUpdateEvery,
		Endpoint:           "http://127.0.0.1:9000",
		Region:             "us-east-1",
		Bucket:             "netdata-s3check",
		Prefix:             defaultPrefix,
		AccessKeyID:        "test-access-key-id",
		SecretAccessKey:    "test-secret-access-key",
		PathStyle:          true,
		MaxRetries:         defaultMaxRetries,
		LatencyThresholdMS: 0,
		ClientConfig: web.ClientConfig{
			Timeout:           confopt.Duration(defaultTimeout),
			NotFollowRedirect: true,
		},
	}
}

type fakeS3Client struct {
	versioningStatus  string
	objects           map[string][]byte
	staleKeys         map[string]bool
	failures          map[string]error
	attempts          map[string]int
	mutateGet         bool
	alwaysExists      bool
	hideObjectsInList bool
	calls             []fakeCall
}

type fakeCall struct {
	operation string
	key       string
}

func (f *fakeS3Client) operations() []string {
	out := make([]string, len(f.calls))
	for i, call := range f.calls {
		out[i] = call.operation
	}
	return out
}

func (f *fakeS3Client) GetBucketVersioning(context.Context, string) (string, int, error) {
	if err := f.error("versioning"); err != nil {
		return "", 1, err
	}
	return f.versioningStatus, 1, nil
}

func (f *fakeS3Client) PutObject(_ context.Context, _, key string, payload []byte) (int, error) {
	f.calls = append(f.calls, fakeCall{operation: "put", key: key})
	if err := f.error("put"); err != nil {
		return f.attemptCount("put"), err
	}
	f.objects[key] = append([]byte(nil), payload...)
	return f.attemptCount("put"), nil
}

func (f *fakeS3Client) GetObject(_ context.Context, _, key string, maxBytes int64) ([]byte, int, error) {
	f.calls = append(f.calls, fakeCall{operation: "get", key: key})
	if err := f.error("get"); err != nil {
		return nil, f.attemptCount("get"), err
	}
	payload, ok := f.objects[key]
	if !ok {
		return nil, 1, errors.New("not found")
	}
	if int64(len(payload)) > maxBytes {
		payload = payload[:maxBytes]
	}
	if f.mutateGet {
		payload = append(payload, 'x')
	}
	return payload, f.attemptCount("get"), nil
}

func (f *fakeS3Client) ListObjects(_ context.Context, _, prefix string, maxKeys int32) ([]string, bool, int, error) {
	f.calls = append(f.calls, fakeCall{operation: "list", key: prefix})
	if err := f.error("list"); err != nil {
		return nil, false, f.attemptCount("list"), err
	}

	seen := make(map[string]struct{}, len(f.staleKeys)+len(f.objects))
	keys := make([]string, 0, len(seen))
	for key := range f.staleKeys {
		if _, duplicate := seen[key]; !duplicate {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	for key := range f.objects {
		if f.hideObjectsInList {
			continue
		}
		if _, duplicate := seen[key]; !duplicate {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}

	truncated := int32(len(keys)) > maxKeys
	if int32(len(keys)) > maxKeys {
		keys = keys[:maxKeys]
	}
	return keys, truncated, f.attemptCount("list"), nil
}

func (f *fakeS3Client) DeleteObject(_ context.Context, _, key string) (int, error) {
	f.calls = append(f.calls, fakeCall{operation: "delete", key: key})
	if err := f.error("delete"); err != nil {
		return f.attemptCount("delete"), err
	}
	delete(f.objects, key)
	delete(f.staleKeys, key)
	return f.attemptCount("delete"), nil
}

func (f *fakeS3Client) ObjectExists(_ context.Context, _, key string) (bool, int, error) {
	f.calls = append(f.calls, fakeCall{operation: "head", key: key})
	if err := f.error("head"); err != nil {
		return false, f.attemptCount("head"), err
	}
	if f.alwaysExists {
		return true, f.attemptCount("head"), nil
	}
	_, exists := f.objects[key]
	return exists, f.attemptCount("head"), nil
}

func (f *fakeS3Client) error(operation string) error {
	if f.failures == nil {
		return nil
	}
	return f.failures[operation]
}

func (f *fakeS3Client) attemptCount(operation string) int {
	if attempts := f.attempts[operation]; attempts > 0 {
		return attempts
	}
	return 1
}

type steppingClock struct {
	now  time.Time
	step time.Duration
}

func (c *steppingClock) tick() time.Time {
	value := c.now
	c.now = c.now.Add(c.step)
	return value
}

func newTestCollector(t *testing.T) (*Collector, *fakeS3Client) {
	t.Helper()

	client := &fakeS3Client{
		objects:   make(map[string][]byte),
		staleKeys: make(map[string]bool),
		failures:  make(map[string]error),
		attempts:  make(map[string]int),
	}

	collr := New()
	collr.Config.Endpoint = "http://127.0.0.1:9000"
	collr.Config.Region = "us-east-1"
	collr.Config.Bucket = "netdata-s3check"
	collr.Config.AccessKeyID = "test-access-key-id"
	collr.Config.SecretAccessKey = "test-secret-access-key"
	collr.client = client
	clock := &steppingClock{now: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)}
	collr.now = clock.tick

	sequence := 0
	collr.randomRead = func(size int) ([]byte, error) {
		sequence++
		payload := make([]byte, size)
		for i := range payload {
			payload[i] = byte(sequence + i)
		}
		return payload, nil
	}
	return collr, client
}

func staleProbeKey(index int) string {
	const digits = "0123456789abcdef"
	suffix := make([]byte, 16)
	for i := range suffix {
		suffix[i] = digits[(index+i)%len(digits)]
	}
	return defaultPrefix + "probe-1-" + string(suffix) + ".bin"
}

func stageOnlyLabels(stage stageID) metrix.Labels {
	return metrix.Labels{"stage": string(stage)}
}

func stageLabels(stage stageID, reason string) metrix.Labels {
	return metrix.Labels{"stage": string(stage), "reason": reason}
}

func stateLabels(name, state string, labels metrix.Labels) metrix.Labels {
	out := make(metrix.Labels, len(labels)+1)
	for key, value := range labels {
		out[key] = value
	}
	out[name] = state
	return out
}

func metricKey(name string, labels metrix.Labels) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := name + "{"
	for i, key := range keys {
		if i > 0 {
			out += ","
		}
		out += key + `="` + labels[key] + `"`
	}
	return out + "}"
}

func stateMetricKey(name, state string, labels metrix.Labels) string {
	return metricKey(name, stateLabels(name, state, labels))
}
