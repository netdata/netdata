// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
		"missing endpoint":                 {func(c *Config) { c.Endpoint = "" }, "endpoint is not set"},
		"endpoint credentials":             {func(c *Config) { c.Endpoint = "https://key:secret@example.net" }, "without credentials"},
		"endpoint path":                    {func(c *Config) { c.Endpoint = "https://example.net/path" }, "without credentials, path"},
		"missing region":                   {func(c *Config) { c.Region = "" }, "region is not set"},
		"invalid bucket":                   {func(c *Config) { c.Bucket = "invalid_bucket" }, "valid S3 bucket name"},
		"bucket with empty label":          {func(c *Config) { c.Bucket = "invalid..bucket" }, "valid S3 bucket name"},
		"bucket with IP address":           {func(c *Config) { c.Bucket = "192.0.2.1" }, "valid S3 bucket name"},
		"prefix missing trailing slash":    {func(c *Config) { c.Prefix = "probe" }, "prefix must end with"},
		"prefix traversal":                 {func(c *Config) { c.Prefix = "probe/../" }, "must not contain '.'"},
		"timeout too small":                {func(c *Config) { c.Timeout = confopt.Duration(100 * time.Millisecond) }, "timeout must be between"},
		"too many retries":                 {func(c *Config) { c.MaxRetries = 3 }, "max_retries must be between"},
		"source HTTP2 proxy bypass":        {func(c *Config) { c.ForceHTTP2 = true }, "force_http2 is not supported"},
		"malformed source proxy redaction": {func(c *Config) { c.ProxyURL = "http://user:pa%zz@example.net" }, "proxy_url is not a valid absolute URL"},
		"latency objective negative":       {func(c *Config) { c.LatencyThresholdMS = -1 }, "latency_threshold_ms must be between"},
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
			collr.stateDir = t.TempDir()
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
	collr.stateDir = t.TempDir()
	collr.machineGUID = func() string { return "test-machine-guid" }
	collr.Config.UpdateEvery = 120
	assert.Equal(t, 115*time.Second, collr.worstCaseDuration())
	require.NoError(t, collr.Init(context.Background()))

	collr.Config.UpdateEvery = 114
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
	assert.Contains(t, text, "template: s3check_multisite_phase_failed")
	assert.Contains(t, text, "on: s3check.multisite_phase_failure")
	assert.Contains(t, text, "calc: $failed")
	assert.Contains(t, text, "to: sysadmin")
	assert.Contains(t, text, "template: s3check_multisite_payload_mismatch")
	assert.Contains(t, text, "on: s3check.multisite_payload_mismatch")
	assert.Contains(t, text, "calc: $mismatch")
	assert.Contains(t, text, "to: sysadmin")
	assert.Contains(t, text, "template: s3check_multisite_replication_rpo_breach")
	assert.Contains(t, text, "on: s3check.multisite_rpo_status")
	assert.Contains(t, text, "calc: $breached")
	assert.Contains(t, text, "to: silent")
	assert.Contains(t, text, "template: s3check_multisite_delete_propagation_breach")
	assert.Contains(t, text, "on: s3check.multisite_delete_status")
	assert.Contains(t, text, "calc: $breached")
	assert.Contains(t, text, "to: silent")
}

func TestCollector_CheckRejectsVersionedOrSuspendedBucket(t *testing.T) {
	for _, status := range []string{"Enabled", "Suspended"} {
		t.Run(status, func(t *testing.T) {
			collr, client := newTestCollector(t)
			client.versioningStatus = status
			err := collr.Check(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsupported source S3 bucket versioning status")
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

func TestIPv6ZoneRuntimeAndPersistedIdentity(t *testing.T) {
	interfaces, err := net.Interfaces()
	require.NoError(t, err)
	require.NotEmpty(t, interfaces)

	iface := interfaces[0]
	nameEndpoint := fmt.Sprintf("http://[fe80::1%%25%s]:9000", iface.Name)
	indexEndpoint := fmt.Sprintf("http://[fe80:0:0:0:0:0:0:1%%25%d]:9000", iface.Index)

	nameConfig := endpointConfig{Endpoint: nameEndpoint, Bucket: "same-bucket", PathStyle: true}
	indexConfig := endpointConfig{Endpoint: indexEndpoint, Bucket: "same-bucket", PathStyle: true}
	require.Equal(t, endpointBucketKey(indexConfig), endpointBucketKey(nameConfig))
	require.NotEqual(t, canonicalEndpointKey(indexEndpoint), canonicalEndpointKey(nameEndpoint))
}

func TestCollector_InitValidatesMultisiteConfiguration(t *testing.T) {
	tests := map[string]struct {
		modify   func(*Config)
		contains string
	}{
		"missing name": {
			func(c *Config) { c.Name = "" }, "name is not set",
		},
		"missing destination": {
			func(c *Config) { c.Destination = nil }, "destination is not set",
		},
		"missing source site": {
			func(c *Config) { c.SourceSite = "" }, "source_site must",
		},
		"same site labels": {
			func(c *Config) { c.Destination.Site = c.SourceSite }, "must identify different sites",
		},
		"same endpoint and bucket": {
			func(c *Config) {
				c.Destination.Endpoint = c.Endpoint
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"same bucket through FQDN root-dot alias": {
			func(c *Config) {
				c.Destination.Endpoint = "http://127.0.0.1.:9000"
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"same bucket through equivalent IPv6 literals": {
			func(c *Config) {
				c.Endpoint = "http://[::1]:9000"
				c.Destination.Endpoint = "http://[0:0:0:0:0:0:0:1]:9000"
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"same bucket through equivalent scoped IPv6 literals": {
			func(c *Config) {
				c.Endpoint = "http://[fe80::1%25eth0]:9000"
				c.Destination.Endpoint = "http://[fe80:0:0:0:0:0:0:1%25eth0]:9000"
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"same bucket through equivalent numeric IPv6 zones": {
			func(c *Config) {
				c.Endpoint = "http://[fe80::1%253]:9000"
				c.Destination.Endpoint = "http://[fe80:0:0:0:0:0:0:1%2503]:9000"
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"destination HTTP2 proxy bypass": {
			func(c *Config) { c.Destination.ForceHTTP2 = new(true) }, "destination force_http2 is not supported",
		},
		"malformed destination proxy redaction": {
			func(c *Config) { c.Destination.ProxyURL = "http://user:pa%zz@example.net" }, "proxy_url is not a valid absolute URL",
		},
		"destination prefix": {
			func(c *Config) { c.Destination.Prefix = "destination" }, "destination prefix must end with",
		},
		"destination credentials": {
			func(c *Config) { c.Destination.SecretAccessKey = "" }, "destination secret_access_key is not set",
		},
		"same bucket with explicit default port": {
			func(c *Config) {
				c.Endpoint = "http://127.0.0.1"
				c.Destination.Endpoint = "http://127.0.0.1:80"
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"same bucket with leading-zero default port": {
			func(c *Config) {
				c.Endpoint = "https://s3.example.net"
				c.Destination.Endpoint = "https://s3.example.net:0443"
				c.Destination.Bucket = c.Bucket
				c.Destination.PathStyle = new(true)
			}, "must not use the same endpoint and bucket",
		},
		"same bucket through virtual-host addressing": {
			func(c *Config) {
				c.Endpoint = "https://s3.example.net"
				c.PathStyle = false
				c.Destination.Endpoint = "https://" + c.Bucket + ".s3.example.net"
				c.Destination.Bucket = c.Bucket
				c.Destination.PathStyle = new(false)
			}, "must not use the same endpoint and bucket",
		},
		"same bucket through different scheme": {
			func(c *Config) {
				c.Destination.Endpoint = "https://127.0.0.1:9000"
				c.Destination.Bucket = c.Bucket
			}, "must not use the same endpoint and bucket",
		},
		"destination timeout budget": {
			func(c *Config) { c.Destination.Timeout = confopt.Duration(10 * time.Second) }, "does not fit update_every",
		},
		"rpo exceeds polling deadline": {
			func(c *Config) { c.RPOThresholdMS = c.ReplicationTimeoutMS + 1 }, "rpo_threshold_ms must not exceed",
		},
		"delete objective exceeds deadline": {
			func(c *Config) { c.DeleteThresholdMS = c.DeleteTimeoutMS + 1 }, "delete_threshold_ms must not exceed",
		},
		"rpo smaller than polling interval": {
			func(c *Config) { c.RPOThresholdMS = 1; c.ReplicationTimeoutMS = 7200000 }, "rpo_threshold_ms must be at least update_every",
		},
		"delete objective smaller than polling interval": {
			func(c *Config) { c.DeleteThresholdMS = 1; c.DeleteTimeoutMS = 7200000 }, "delete_threshold_ms must be at least update_every",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _ := newMultisiteInitCollector(t)
			test.modify(&collr.Config)
			err := collr.Init(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.contains)
		})
	}
}

func TestCollector_InitAcceptsMultisiteConfiguration(t *testing.T) {
	collr, _ := newMultisiteInitCollector(t)
	require.NoError(t, collr.Init(context.Background()))
	assert.Equal(t, "netdata-s3check-destination/", collr.Destination.Prefix)
	require.NotNil(t, collr.Destination.PathStyle)
	assert.True(t, *collr.Destination.PathStyle)
	require.NotNil(t, collr.Destination.NotFollowRedirect)
	assert.True(t, *collr.Destination.NotFollowRedirect)
	assert.Equal(t, 115*time.Second, collr.worstCaseDuration())
	assert.Equal(t, 85*time.Second, collr.shutdownCleanupDuration())
}

func TestReadAgentMachineGUIDUsesNetdataEnvironment(t *testing.T) {
	t.Setenv("NETDATA_REGISTRY_UNIQUE_ID", "  test-agent-machine-guid\n")
	assert.Equal(t, "test-agent-machine-guid", readAgentMachineGUID())
}

func TestMultisiteOwnerTagHashesMachineAndJobIdentity(t *testing.T) {
	owner := multisiteOwnerTag("test-agent-machine-guid", "job-a")
	assert.NotContains(t, owner, "test-agent-machine-guid")
	assert.NotContains(t, owner, "job-a")
	assert.Len(t, owner, 32)
	assert.NotEqual(t, owner, multisiteOwnerTag("other-agent-machine-guid", "job-a"))
	assert.NotEqual(t, owner, multisiteOwnerTag("test-agent-machine-guid", "job-b"))
}

func TestCollector_MultisiteCandidateDoesNotCleanupIncumbentState(t *testing.T) {
	incumbent, _ := newMultisiteInitCollector(t)
	require.NoError(t, incumbent.Init(context.Background()))
	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	fingerprint := incumbent.ownershipFingerprint()
	ownerTag := incumbent.stateStore.ownerTag
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: fingerprint, Phase: string(multisiteCleanup),
		SourceKey: multisiteStaleProbeKey(15, ownerTag), DestinationKey: multisiteStaleDestinationKey(15, ownerTag),
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt,
	}
	require.NoError(t, incumbent.stateStore.save(state))

	candidate, _ := newMultisiteInitCollector(t)
	candidate.stateDir = incumbent.stateDir
	require.NoError(t, candidate.Init(context.Background()))
	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())

	assert.Nil(t, candidate.pendingOwnershipState)
	require.FileExists(t, incumbent.stateStore.path)
}

func TestCollector_MultisitePendingStateSkipsStartupVersionCheck(t *testing.T) {
	incumbent, _ := newMultisiteInitCollector(t)
	require.NoError(t, incumbent.Init(context.Background()))
	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	fingerprint := incumbent.ownershipFingerprint()
	ownerTag := incumbent.stateStore.ownerTag
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: fingerprint, Phase: string(multisiteCleanup),
		SourceKey: multisiteStaleProbeKey(16, ownerTag), DestinationKey: multisiteStaleDestinationKey(16, ownerTag),
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt,
	}
	require.NoError(t, incumbent.stateStore.save(state))

	candidate, destination := newMultisiteInitCollector(t)
	candidate.stateDir = incumbent.stateDir
	require.NoError(t, candidate.Init(context.Background()))
	source, ok := candidate.client.(*fakeS3Client)
	require.True(t, ok)
	source.failures["versioning"] = errors.New("transient-source-versioning-check")
	destination.failures["versioning"] = errors.New("transient-destination-versioning-check")
	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())

	assert.Nil(t, candidate.pendingOwnershipState)
	require.FileExists(t, incumbent.stateStore.path)
}

func TestConfigSchemaUsesPasswordWidgetsForDestinationCredentials(t *testing.T) {
	raw, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var schema map[string]any
	require.NoError(t, json.Unmarshal(raw, &schema))

	ui, ok := schema["uiSchema"].(map[string]any)
	require.True(t, ok)
	destination, ok := ui["destination"].(map[string]any)
	require.True(t, ok)
	for _, field := range []string{"access_key_id", "secret_access_key", "session_token"} {
		widget, ok := destination[field].(map[string]any)
		require.True(t, ok, field)
		assert.Equal(t, "password", widget["ui:widget"], field)
	}
}

func TestCollector_MultisiteCandidateRejectsStateCreatedAfterInit(t *testing.T) {
	incumbent, _ := newMultisiteInitCollector(t)
	require.NoError(t, incumbent.Init(context.Background()))

	candidate, _ := newMultisiteInitCollector(t)
	candidate.stateDir = incumbent.stateDir
	candidate.Config.Destination = &DestinationConfig{}
	*candidate.Config.Destination = *incumbent.Destination
	candidate.Config.Destination.Prefix = "changed-route/"
	require.NoError(t, candidate.Init(context.Background()))

	createdAt := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	fingerprint := incumbent.ownershipFingerprint()
	ownerTag := incumbent.stateStore.ownerTag
	state := &ownershipState{
		Version: ownershipStateVersion, ConfigFingerprint: fingerprint, Phase: string(multisiteCleanup),
		SourceKey: multisiteStaleProbeKey(18, ownerTag), DestinationKey: multisiteStaleDestinationKey(18, ownerTag),
		PayloadDigest: strings.Repeat("0", 64), CreatedAt: createdAt,
	}
	require.NoError(t, incumbent.stateStore.save(state))

	err := candidate.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "different source, destination, or mode settings")
	require.FileExists(t, incumbent.stateStore.path)
}

func TestCollector_MultisiteCandidateReservationBlocksReconciliationWrite(t *testing.T) {
	candidate, _ := newMultisiteInitCollector(t)
	require.NoError(t, candidate.Init(context.Background()))
	require.NoError(t, candidate.Check(context.Background()))

	incumbent, source, _ := newMultisiteTestCollectorAt(t, candidate.stateStore.path)
	incumbent.stateStore = candidate.stateStore
	source.staleKeys[multisiteStaleProbeKey(23, incumbent.stateStore.ownerTag)] = true

	metrics, err := collecttest.CollectScalarSeries(incumbent, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.NotContains(t, source.operations(), "delete")
	assert.NoFileExists(t, candidate.stateStore.path)
	assert.Len(t, source.staleKeys, 1)

	candidate.Cleanup(context.Background())
}

func TestCollector_MultisiteCleanJobsCheckInParallel(t *testing.T) {
	root := t.TempDir()
	first, _ := newMultisiteInitCollector(t)
	first.Name = "forward"
	first.stateDir = root
	require.NoError(t, first.Init(context.Background()))

	second, _ := newMultisiteInitCollector(t)
	second.Name = "reverse"
	second.stateDir = root
	require.NoError(t, second.Init(context.Background()))

	require.NoError(t, first.Check(context.Background()))
	require.NoError(t, second.Check(context.Background()))
}

func TestCollector_MultisiteCandidateReservationBlocksIncumbentWrite(t *testing.T) {
	candidate, _ := newMultisiteInitCollector(t)
	require.NoError(t, candidate.Init(context.Background()))
	require.NoError(t, candidate.Check(context.Background()))

	incumbent, _, _ := newMultisiteTestCollectorAt(t, candidate.stateStore.path)
	incumbent.stateStore = candidate.stateStore
	metrics, err := collecttest.CollectScalarSeries(incumbent, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("multisite_phase_failure", multisiteSiteLabels())])
	assert.NoFileExists(t, candidate.stateStore.path)

	candidate.Cleanup(context.Background())
	_, err = collecttest.CollectScalarSeries(incumbent, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, candidate.stateStore.path)
}

func TestCollector_CheckRejectsVersionedDestinationBucket(t *testing.T) {
	collr, destination := newMultisiteInitCollector(t)
	require.NoError(t, collr.Init(context.Background()))
	destination.versioningStatus = "Enabled"

	err := collr.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported destination S3 bucket versioning status")
}

func newMultisiteInitCollector(t *testing.T) (*Collector, *fakeS3Client) {
	t.Helper()

	source := &fakeS3Client{failures: make(map[string]error)}
	destination := &fakeS3Client{failures: make(map[string]error)}
	collr := New()
	collr.Config = validMultisiteTestConfig()
	collr.machineGUID = func() string { return "test-machine-guid" }
	collr.stateDir = t.TempDir()
	collr.newClient = func(Config) (*http.Client, s3Client, error) { return &http.Client{}, source, nil }
	collr.newDestinationClient = func(DestinationConfig, int) (*http.Client, s3Client, error) {
		return &http.Client{}, destination, nil
	}
	return collr, destination
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

	var mismatchLifecycle *charttpl.Lifecycle
	var findMismatch func(groups []charttpl.Group)
	findMismatch = func(groups []charttpl.Group) {
		for _, group := range groups {
			for _, chart := range group.Charts {
				if chart.Context == "multisite_payload_mismatch" {
					mismatchLifecycle = chart.Lifecycle
				}
			}
			findMismatch(group.Groups)
		}
	}
	findMismatch(spec.Groups)
	require.NotNil(t, mismatchLifecycle)
	assert.Equal(t, 2147483647, mismatchLifecycle.ExpireAfterCycles)
}

func TestCollector_CollectSuccessfulLifecycle(t *testing.T) {
	collr, client := newTestCollector(t)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	for _, stage := range stageOrder {
		assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stage, reasonOK))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[stateMetricKey("stage_status", "failed", stageLabels(stage, reasonOK))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[stateMetricKey("stage_status", "skipped", stageLabels(stage, reasonOK))], stage)
		expectedAttempts := metrix.SampleValue(1)
		if stage == stageSetup || stage == stageDelete {
			expectedAttempts = 2
		}
		assert.Equal(t, expectedAttempts, metrics[metricKey("stage_attempts_total", stageOnlyLabels(stage))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stage))], stage)
		assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_failures_total", stageOnlyLabels(stage))], stage)
	}

	require.NotNil(t, metrics[metricKey("stage_duration_ms", stageLabels(stagePut, reasonOK))])
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"list", "versioning", "put", "get", "list", "versioning", "delete", "head"}, client.operations())

	// The exact key remains quarantined for one interval so a late PUT commit
	// cannot be bypassed by a route change. After that interval, LIST must prove
	// the owner namespace empty before another probe starts.
	quarantineMetrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), quarantineMetrics[stateMetricKey(
		"stage_status", "skipped", stageLabels(stageSetup, reasonQuarantinePending),
	)])
	assert.Equal(t, metrix.SampleValue(0), quarantineMetrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageSetup, reasonQuarantinePending),
	)])
	assert.Equal(t, []string{"list", "versioning", "put", "get", "list", "versioning", "delete", "head"}, client.operations())
	ageSingleQuarantine(t, collr)
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, []string{
		"list", "versioning", "put", "get", "list", "versioning", "delete", "head",
		"list", "list", "versioning", "put", "get", "list", "versioning", "delete", "head",
	}, client.operations())
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func TestCollector_SingleQuarantineClearsAndProbesOnNextScheduledCycle(t *testing.T) {
	collr, client := newTestCollector(t)
	clock := &cycleClock{
		steppingClock: steppingClock{now: time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC), step: 20 * time.Millisecond},
		interval:      time.Duration(collr.UpdateEvery) * time.Second,
	}
	collr.now = clock.tick
	collr.stateStore.now = clock.tick

	clock.beginCycle()
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	firstOperations := len(client.operations())
	require.FileExists(t, collr.stateStore.path)

	// The fixed scheduler runs the next tick one interval after cycle start, not
	// one interval after the probe was created later in that cycle.
	clock.step = time.Millisecond
	clock.advanceCycle()
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageSetup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stagePut, reasonOK))])
	assert.Equal(t, []string{
		"list", "list", "versioning", "put", "get", "list", "versioning", "delete", "head",
	}, client.operations()[firstOperations:])
	assert.Empty(t, client.objects)
	require.FileExists(t, collr.stateStore.path)
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
	assert.Equal(t, []string{"list", "versioning", "put", "versioning", "delete", "head"}, client.operations())
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
	ageSingleQuarantine(t, collr)
	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(4), metrics[metricKey("stage_operations_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stageCleanup))])
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"list", "versioning", "delete", "head"}, client.operations()[before:])
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
	ageSingleQuarantine(t, collr)
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(2), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stagePut))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stagePut))])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_failures_total", stageOnlyLabels(stagePut))])
}

func TestCollector_CollectUsesConfiguredLatencyObjective(t *testing.T) {
	collr, _ := newTestCollector(t)
	clock := &steppingClock{
		now:  time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
		step: 2 * time.Millisecond,
	}
	collr.now = clock.tick
	collr.stateStore.now = clock.tick
	collr.Config.LatencyThresholdMS = 1

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)

	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_latency_exceeded", stageLabels(stagePut, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[metricKey("stage_latency_exceeded", stageLabels(stageGet, reasonOK))])
}

func TestCollector_SingleReconciliationOwnsFullSupportedKeySet(t *testing.T) {
	collr, client := newTestCollector(t)
	for _, key := range staleProbeKeys(maxOwnedKeys) {
		client.staleKeys[key] = true
	}

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	state, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, state.PendingKeys, maxOwnedKeys-cleanupBatchSize)

	changed := collr.Config
	changed.Prefix = "changed-single-prefix/"
	changedCollector := &Collector{Config: changed}
	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedCollector.ownershipFingerprint(), collr.stateStore.ownerTag,
		modeSingle, changed.Prefix, "",
	)
	_, loadErr = changedStore.load()
	require.Error(t, loadErr)

	for len(client.staleKeys) > 0 {
		_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
		require.NoError(t, err)
	}
	assert.NoFileExists(t, collr.stateStore.path)
}

func TestCollector_SinglePartialReconciliationDurablyBlocksRoute(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["list"] = errors.New("secret-single-list-failure")

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageSetup, reasonRequestFailed),
	)])
	require.FileExists(t, collr.stateStore.path)
	blocker, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	assert.True(t, blocker.ReconciliationPending)
	assert.Empty(t, blocker.PendingKeys)

	changed := collr.Config
	changed.Prefix = "changed-single-prefix/"
	changedCollector := &Collector{Config: changed}
	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedCollector.ownershipFingerprint(), collr.stateStore.ownerTag,
		modeSingle, changed.Prefix, "",
	)
	_, loadErr = changedStore.load()
	require.Error(t, loadErr)

	delete(client.failures, "list")
	_, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.NotNil(t, collr.pendingOwnershipState)
	assert.False(t, collr.pendingOwnershipState.ReconciliationPending)
	require.Len(t, collr.pendingOwnershipState.PendingKeys, 1)
	require.NotNil(t, collr.pendingOwnershipState.QuarantinedAt)
	assert.Empty(t, client.objects)
}

func TestCollector_SingleShutdownRetainsKeylessReconciliationBlocker(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["list"] = errors.New("secret-single-list-failure")

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	operations := len(client.operations())

	results := newStageResults()
	assert.False(t, collr.cleanupOwnedSingleState(context.Background(), results))
	managed, ok := metrix.AsCycleManagedStore(collr.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NotPanics(t, func() { collr.writeMetrics(results) })
	require.NoError(t, cycle.CommitCycleSuccess())

	collr.Cleanup(context.Background())
	require.FileExists(t, collr.stateStore.path)
	blocker, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	assert.True(t, blocker.ReconciliationPending)
	assert.Empty(t, blocker.PendingKeys)
	require.NotNil(t, blocker.RetiredAt)
	assert.Equal(t, operations, len(client.operations()))
}

func TestCollector_SingleReconciliationFailsClosedAboveOwnedKeyLimit(t *testing.T) {
	collr, client := newTestCollector(t)
	for _, key := range staleProbeKeys(maxOwnedKeys + 1) {
		client.staleKeys[key] = true
	}

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageSetup, reasonInternal),
	)])
	assert.Equal(t, []string{"list"}, client.operations())
	assert.Len(t, client.staleKeys, maxOwnedKeys+1)
	require.FileExists(t, collr.stateStore.path)
	blocker, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	assert.True(t, blocker.ReconciliationPending)
	assert.Empty(t, blocker.PendingKeys)

	changed := collr.Config
	changed.Prefix = "changed-single-prefix/"
	changedCollector := &Collector{Config: changed}
	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedCollector.ownershipFingerprint(), collr.stateStore.ownerTag,
		modeSingle, changed.Prefix, "",
	)
	_, loadErr = changedStore.load()
	require.Error(t, loadErr)
}

func TestCollector_SingleQuarantineRefreshFailsClosedAboveOwnedKeyLimit(t *testing.T) {
	collr, client := newTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	before := len(client.operations())
	for _, key := range staleProbeKeys(maxOwnedKeys + 1) {
		client.staleKeys[key] = true
	}
	ageSingleQuarantine(t, collr)

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageSetup, reasonInternal),
	)])
	assert.Equal(t, []string{"list"}, client.operations()[before:])
	assert.Len(t, client.staleKeys, maxOwnedKeys+1)
	require.FileExists(t, collr.stateStore.path)
	state, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.Len(t, state.PendingKeys, 1)
}

func TestCollector_CollectPerformsBoundedRestartCleanup(t *testing.T) {
	collr, client := newTestCollector(t)
	for i := range cleanupBatchSize {
		key := staleProbeKey(i)
		client.staleKeys[key] = true
	}

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(6), metrics[metricKey("stage_operations_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(6), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stageCleanup))])
	assert.Empty(t, client.staleKeys)
	assert.Equal(t, []string{"list", "versioning", "delete", "head", "versioning", "delete", "head"}, client.operations())

	metrics, err = collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageSetup, reasonOK))])
	assert.Equal(t, []string{
		"list", "versioning", "delete", "head", "versioning", "delete", "head",
		"list", "versioning", "put", "get", "list", "versioning", "delete", "head",
	}, client.operations())
}

func TestCollector_SingleCleanupFailureIsDurableAndRecoverable(t *testing.T) {
	collr, client := newTestCollector(t)
	client.failures["delete"] = errors.New("temporary-cleanup-failure")

	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)
	state, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.NotNil(t, state)
	require.NotEmpty(t, state.SourceKey)
	require.Len(t, client.objects, 1)

	collr.Cleanup(context.Background())
	require.FileExists(t, collr.stateStore.path)
	retired, loadErr := collr.stateStore.load()
	require.NoError(t, loadErr)
	require.NotNil(t, retired.RetiredAt)

	restarted, restartedClient := newTestCollectorAt(t, collr.stateStore.path)
	loaded, loadErr := restarted.stateStore.load()
	require.NoError(t, loadErr)
	require.NotNil(t, loaded)
	restartedClient.objects[loaded.SourceKey] = []byte("payload")
	_, err = collecttest.CollectScalarSeries(restarted, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Empty(t, restartedClient.objects)

	recovered, loadErr := restarted.stateStore.load()
	require.NoError(t, loadErr)
	assert.Empty(t, recovered.SourceKey)
	require.Len(t, recovered.PendingKeys, 1)
	require.NotNil(t, recovered.QuarantinedAt)
}

func configureSingleTestIdentity(collr *Collector, name, statePath string) {
	collr.Name = name
	collr.stateStore = newOwnershipStateStore(
		statePath, collr.ownershipFingerprint(), multisiteOwnerTag("test-machine-guid", name),
		modeSingle, collr.Prefix, "",
	)
	collr.stateStore.now = collr.now
	collr.stateLock = filelock.New(filepath.Dir(statePath))
	collr.stateLockName = strings.TrimSuffix(filepath.Base(statePath), filepath.Ext(statePath))
	collr.ownerLockName = collr.stateLockName + ".owner"
}

func newTestCollectorAt(t *testing.T, statePath string) (*Collector, *fakeS3Client) {
	t.Helper()

	client := &fakeS3Client{
		objects:   make(map[string][]byte),
		staleKeys: make(map[string]bool),
		failures:  make(map[string]error),
		attempts:  make(map[string]int),
	}
	collr, _ := newTestCollector(t)
	collr.client = client
	configureSingleTestIdentity(collr, "s3check", statePath)
	return collr, client
}

func ageSingleQuarantine(t *testing.T, collr *Collector) {
	t.Helper()
	state, err := collr.stateStore.load()
	require.NoError(t, err)
	require.NotNil(t, state)
	require.NotNil(t, state.QuarantinedAt)
	old := state.QuarantinedAt.Add(-time.Duration(collr.UpdateEvery)*time.Second - time.Second)
	state.QuarantinedAt = &old
	require.NoError(t, collr.stateStore.save(state))
	collr.pendingOwnershipState = state
}

func TestCollector_ConcurrentSingleJobsShareOnlyOwnershipDirectory(t *testing.T) {
	root := t.TempDir()
	first, firstClient := newTestCollectorAt(t, filepath.Join(root, "first.json"))
	second, secondClient := newTestCollectorAt(t, filepath.Join(root, "second.json"))
	configureSingleTestIdentity(second, "second-s3check", filepath.Join(root, "second.json"))

	_, err := collecttest.CollectScalarSeries(first, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, first.stateStore.path)
	assert.Empty(t, firstClient.objects)

	// The first job retains a durable quarantine journal, but its live owner
	// lock must not suppress an unrelated owner during the same interval.
	first.releaseOwnerLock()
	foreignOwner := filelock.New(filepath.Dir(first.stateStore.path))
	acquired, lockErr := foreignOwner.Lock(first.ownerLockName)
	require.NoError(t, lockErr)
	require.True(t, acquired)
	defer foreignOwner.Unlock(first.ownerLockName)
	first.now = time.Now
	first.stateStore.now = first.now
	require.NoError(t, first.saveOwnership(first.pendingOwnershipState))
	_, err = collecttest.CollectScalarSeries(second, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, second.stateStore.path)
	assert.Empty(t, secondClient.objects)
}

func TestCollector_SingleNormalDeleteStopsWhenVersioningDrifts(t *testing.T) {
	collr, client := newTestCollector(t)
	client.beforeVersioning = func(call int) {
		if call == 2 {
			client.versioningStatus = "Enabled"
		}
	}

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageDelete, reasonBucketVersioned),
	)])
	assert.NotContains(t, client.operations(), "delete")
	require.FileExists(t, collr.stateStore.path)
}

func TestCollector_SingleQuarantineBlocksRouteChangeBeforeRecheck(t *testing.T) {
	collr, _ := newTestCollector(t)
	_, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, collr.stateStore.path)

	changed := collr.Config
	changed.Prefix = "changed-single-prefix/"
	changedCollector := &Collector{Config: changed}
	changedStore := newOwnershipStateStore(
		collr.stateStore.path, changedCollector.ownershipFingerprint(), collr.stateStore.ownerTag,
		modeSingle, changed.Prefix, "",
	)
	_, err = changedStore.load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pending state belongs")
}

func TestCollector_SingleCleanupFailureMarksSetupBlocked(t *testing.T) {
	tests := map[string]struct {
		keys     int
		prepare  func(*Collector, *fakeS3Client, []ownedKey)
		contains string
	}{
		"absence proof fails": {
			keys: 1,
			prepare: func(_ *Collector, client *fakeS3Client, _ []ownedKey) {
				client.failures["head"] = errors.New("secret-head-failure")
			},
			contains: reasonRequestFailed,
		},
		"object remains present": {
			keys: 1,
			prepare: func(_ *Collector, client *fakeS3Client, keys []ownedKey) {
				client.afterDelete = func() { client.objects[keys[0].Key] = []byte("still-present") }
			},
			contains: reasonStillPresent,
		},
		"remaining-key save fails": {
			keys: 2,
			prepare: func(collr *Collector, _ *fakeS3Client, _ []ownedKey) {
				blocker := filepath.Join(t.TempDir(), "not-a-directory")
				require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0o600))
				collr.stateStore.path = filepath.Join(blocker, "pending.json")
			},
			contains: reasonInternal,
		},
		"journal clear fails": {
			keys: 1,
			prepare: func(collr *Collector, _ *fakeS3Client, _ []ownedKey) {
				blocker := filepath.Join(t.TempDir(), "not-a-directory")
				require.NoError(t, os.WriteFile(blocker, []byte("blocker"), 0o600))
				collr.stateStore.path = filepath.Join(blocker, "pending.json")
			},
			contains: reasonInternal,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, client := newTestCollector(t)
			keys := make([]ownedKey, test.keys)
			for i := range keys {
				key := staleProbeKey(90 + i)
				keys[i] = ownedKey{Scope: ownershipSingle, Key: key}
				client.staleKeys[key] = true
			}
			state := &ownershipState{
				Phase: string(multisiteCleanup), PendingKeys: keys, CreatedAt: collr.now(),
			}
			require.NoError(t, collr.saveOwnership(state))
			collr.pendingOwnershipState = state
			test.prepare(collr, client, keys)

			results := newStageResults()
			assert.False(t, collr.cleanupOwnedKeyBatch(context.Background(), results, cleanupBatchSize))
			assert.Equal(t, stateFailed, results[stageSetup].state)
			assert.Equal(t, reasonOrphanCleanupPending, results[stageSetup].reason)
			assert.Equal(t, stateFailed, results[stageCleanup].state)
			assert.Equal(t, test.contains, results[stageCleanup].reason)
		})
	}
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

func TestCollector_CollectAccountsBucketProofAsSeparateOperation(t *testing.T) {
	collr, client := newTestCollector(t)
	client.absentOperations = 2
	client.absentAttempts = 2

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(2), metrics[metricKey("stage_operations_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(2), metrics[metricKey("stage_attempts_total", stageOnlyLabels(stageCleanup))])
	assert.Equal(t, metrix.SampleValue(0), metrics[metricKey("stage_retries_total", stageOnlyLabels(stageCleanup))])
}

func TestCollector_CleanupDeletesPendingObjectAndIsIdempotent(t *testing.T) {
	collr, client := newTestCollector(t)
	key := staleProbeKey(99)
	state := &ownershipState{
		Phase: string(multisiteSourcePut), SourceKey: key, CreatedAt: collr.now(), SourcePutAttempted: true,
	}
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state
	client.objects[key] = []byte("payload")

	collr.Cleanup(context.Background())
	assert.Empty(t, client.objects)
	assert.Equal(t, []string{"head", "versioning", "delete", "head"}, client.operations())
	require.FileExists(t, collr.stateStore.path)
	retired, err := collr.stateStore.load()
	require.NoError(t, err)
	require.NotNil(t, retired.RetiredAt)
	require.NotNil(t, retired.QuarantinedAt)
	require.Len(t, retired.PendingKeys, 1)

	collr.Cleanup(context.Background())
	assert.Len(t, client.operations(), 4)
}

func TestCollector_SingleRestartAbsentObjectIsQuarantined(t *testing.T) {
	collr, client := newTestCollector(t)
	key := staleProbeKey(97)
	createdAt := collr.now()
	state := &ownershipState{
		Phase: string(multisiteSourcePut), SourceKey: key, CreatedAt: createdAt, SourcePutAttempted: true,
	}
	require.NoError(t, collr.saveOwnership(state))
	collr.pendingOwnershipState = state

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageCleanup, reasonOK))])
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending),
	)])
	assert.Equal(t, []string{"head"}, client.operations())
	require.FileExists(t, collr.stateStore.path)

	loaded, err := collr.stateStore.load()
	require.NoError(t, err)
	assert.Empty(t, loaded.SourceKey)
	assert.Equal(t, []ownedKey{{Scope: ownershipSingle, Key: key}}, loaded.PendingKeys)
	require.NotNil(t, loaded.QuarantinedAt)
	assert.Equal(t, createdAt, *loaded.QuarantinedAt)
}

func TestCollector_CandidateCleanupDoesNotAdoptIncumbentSingleState(t *testing.T) {
	root := t.TempDir()
	statePath := ownershipStatePath(root, "s3check")
	incumbent, incumbentClient := newTestCollectorAt(t, statePath)
	key := staleProbeKey(98)
	state := &ownershipState{
		Phase: string(multisiteSourcePut), SourceKey: key, CreatedAt: incumbent.now(), SourcePutAttempted: true,
	}
	require.NoError(t, incumbent.saveOwnership(state))
	incumbentClient.objects[key] = []byte("payload")

	candidate, candidateClient := newTestCollectorAt(t, statePath)
	candidate.newClient = func(Config) (*http.Client, s3Client, error) {
		return &http.Client{}, candidateClient, nil
	}
	candidate.now = incumbent.now
	candidate.stateDir = root
	require.NoError(t, candidate.Init(context.Background()))
	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())

	assert.Empty(t, candidateClient.operations())
	assert.Equal(t, []byte("payload"), incumbentClient.objects[key])
	require.FileExists(t, candidate.stateStore.path)
	loaded, err := candidate.stateStore.load()
	require.NoError(t, err)
	assert.Equal(t, key, loaded.SourceKey)
}

func TestCollector_SameIdentityCheckSucceedsWhileIncumbentOwnerLockIsHeld(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "pending.json")
	incumbent, _ := newTestCollectorAt(t, statePath)
	candidate, _ := newTestCollectorAt(t, statePath)
	require.NoError(t, incumbent.reserveOwnerLock(context.Background()))

	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())

	incumbent.releaseOwnerLock()
	_, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
	require.NoError(t, err)
	require.FileExists(t, candidate.stateStore.path)
}

func TestCollector_OwnerLockNameIsPortable(t *testing.T) {
	collr, _ := newTestCollector(t)
	require.NotEmpty(t, collr.ownerLockName)
	assert.True(t, strings.HasSuffix(collr.ownerLockName, ".owner"))
	assert.NotContains(t, collr.ownerLockName, ":")
}

func TestCollector_PublicationRescansForeignJournalUnderHandoffLock(t *testing.T) {
	root := t.TempDir()
	candidatePath := ownershipStatePath(root, "candidate-s3check-job")
	candidate, candidateClient := newTestCollectorAt(t, candidatePath)
	configureSingleTestIdentity(candidate, "candidate-s3check-job", candidatePath)
	candidate.newClient = func(Config) (*http.Client, s3Client, error) {
		return &http.Client{}, candidateClient, nil
	}
	candidate.stateDir = root
	require.NoError(t, candidate.Init(context.Background()))

	foreignPath := ownershipStatePath(root, "foreign-s3check-job")
	foreign, _ := newTestCollectorAt(t, foreignPath)
	configureSingleTestIdentity(foreign, "foreign-s3check-job", foreignPath)
	created := time.Now().UTC()
	foreign.now = func() time.Time { return created }
	foreign.stateStore.now = foreign.now
	foreignState := &ownershipState{
		Phase: string(multisiteSourcePut), SourceKey: staleProbeKey(20, foreign.stateStore.ownerTag),
		CreatedAt: created, UpdateEvery: defaultUpdateEvery, HeartbeatAt: created, SourcePutAttempted: true,
	}
	originalRandomRead := candidate.randomRead
	injected := false
	candidate.randomRead = func(size int) ([]byte, error) {
		if !injected {
			injected = true
			require.NoError(t, foreign.saveOwnership(foreignState))
			require.FileExists(t, foreignPath)
		}
		return originalRandomRead(size)
	}

	metrics, err := collecttest.CollectScalarSeries(candidate, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey(
		"stage_status", "failed", stageLabels(stageSetup, reasonOrphanCleanupPending),
	)])
	assert.Equal(t, []string{"list", "versioning"}, candidateClient.operations())
	require.NoFileExists(t, candidate.stateStore.path)
	require.FileExists(t, foreignPath)
}

func TestCollector_SingleFunctionalListUsesOwnerNamespace(t *testing.T) {
	collr, client := newTestCollector(t)
	foreignOwner := multisiteOwnerTag("foreign-agent-machine-guid", "foreign-s3check-job")
	foreignPrefix := multisiteProbePrefix(defaultPrefix, foreignOwner)
	for i := range 120 {
		key := fmt.Sprintf("%sprobe-%d-%016x-%s.bin", foreignPrefix, 1_700_000_000+i, i, foreignOwner)
		client.staleKeys[key] = true
	}

	metrics, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, metrix.SampleValue(1), metrics[stateMetricKey("stage_status", "ok", stageLabels(stageList, reasonOK))])
	assert.Empty(t, client.objects)
	assert.Len(t, client.staleKeys, 120)
}

func TestCollector_SameIdentityCheckWaitsForBoundedJobLockContention(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "pending.json")
	incumbent, _ := newTestCollectorAt(t, statePath)
	candidate, _ := newTestCollectorAt(t, statePath)
	require.NoError(t, incumbent.reserveJobState(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := candidate.Check(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.GreaterOrEqual(t, time.Since(start), jobLockPollInterval)
	assert.NoFileExists(t, statePath)

	incumbent.releaseJobState()
	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())
}

//go:fix inline
func ptrTo[T any](value T) *T { return new(value) }

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
	absentOperations  int
	absentAttempts    int
	beforeVersioning  func(call int)
	afterDelete       func()
	afterHead         func()
	versioningCalls   int
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
	f.versioningCalls++
	if f.beforeVersioning != nil {
		f.beforeVersioning(f.versioningCalls)
	}
	f.calls = append(f.calls, fakeCall{operation: "versioning"})
	if err := f.error("versioning"); err != nil {
		return "", 1, err
	}
	return f.versioningStatus, 1, nil
}

func (f *fakeS3Client) PutObject(_ context.Context, _, key string, payload []byte) (int, error) {
	f.calls = append(f.calls, fakeCall{
		operation: "put",
		key:       key,
	})
	if err := f.error("put"); err != nil {
		return f.attemptCount("put"), err
	}
	f.objects[key] = append([]byte(nil), payload...)
	return f.attemptCount("put"), nil
}

func (f *fakeS3Client) GetObject(_ context.Context, _, key string, maxBytes int64) ([]byte, int, error) {
	f.calls = append(f.calls, fakeCall{
		operation: "get",
		key:       key,
	})
	if err := f.error("get"); err != nil {
		return nil, f.attemptCount("get"), err
	}
	payload, ok := f.objects[key]
	if !ok {
		return nil, 1, &types.NoSuchKey{}
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
	f.calls = append(f.calls, fakeCall{
		operation: "list",
		key:       prefix,
	})
	if err := f.error("list"); err != nil {
		return nil, false, f.attemptCount("list"), err
	}

	seen := make(map[string]struct{}, len(f.staleKeys)+len(f.objects))
	keys := make([]string, 0, len(seen))
	for key := range f.staleKeys {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if _, duplicate := seen[key]; !duplicate {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
	}
	for key := range f.objects {
		if f.hideObjectsInList || !strings.HasPrefix(key, prefix) {
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
	f.calls = append(f.calls, fakeCall{
		operation: "delete",
		key:       key,
	})
	if err := f.error("delete"); err != nil {
		return f.attemptCount("delete"), err
	}
	delete(f.objects, key)
	delete(f.staleKeys, key)
	if f.afterDelete != nil {
		f.afterDelete()
	}
	return f.attemptCount("delete"), nil
}

func (f *fakeS3Client) ObjectExists(_ context.Context, _, key string) (bool, s3OperationReport, error) {
	f.calls = append(f.calls, fakeCall{
		operation: "head",
		key:       key,
	})
	if err := f.error("head"); err != nil {
		return false, s3OperationReport{operations: 1, attempts: f.attemptCount("head")}, err
	}
	if f.alwaysExists {
		return true, s3OperationReport{operations: 1, attempts: f.attemptCount("head")}, nil
	}
	_, exists := f.objects[key]
	operations := 1
	attempts := f.attemptCount("head")
	if !exists && f.absentOperations > 0 {
		operations = f.absentOperations
		attempts = f.absentAttempts
	}
	if f.afterHead != nil {
		f.afterHead()
	}
	return exists, s3OperationReport{operations: operations, attempts: attempts}, nil
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

type cycleClock struct {
	steppingClock
	cycleStart time.Time
	interval   time.Duration
}

func (c *cycleClock) beginCycle() {
	c.cycleStart = c.now
}

func (c *cycleClock) advanceCycle() {
	c.now = c.cycleStart.Add(c.interval)
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
	collr.machineGUID = func() string { return "test-machine-guid" }
	collr.client = client
	collr.Name = "s3check"
	statePath := filepath.Join(t.TempDir(), "pending.json")
	configureSingleTestIdentity(collr, collr.Name, statePath)
	clock := &steppingClock{
		now: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
	}
	collr.now = clock.tick
	collr.stateStore.now = clock.tick

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

func staleProbeKey(index int, ownerTag ...string) string {
	const digits = "0123456789abcdef"
	tag := multisiteOwnerTag("test-machine-guid", "s3check")
	if len(ownerTag) == 1 {
		tag = ownerTag[0]
	}
	suffix := make([]byte, 16)
	for i := range suffix {
		suffix[i] = digits[(index+i)%len(digits)]
	}
	return multisiteProbePrefix(defaultPrefix, tag) + "probe-1-" + string(suffix) + "-" + tag + ".bin"
}

func staleProbeKeys(count int) []string {
	tag := multisiteOwnerTag("test-machine-guid", "s3check")
	keys := make([]string, count)
	for i := range count {
		name := fmt.Sprintf("probe-1-%016x-%s.bin", i+1, tag)
		keys[i] = multisiteProbePrefix(defaultPrefix, tag) + name
	}
	return keys
}

func stageOnlyLabels(stage stageID) metrix.Labels {
	return metrix.Labels{
		"stage": string(stage),
	}
}

func stageLabels(stage stageID, reason string) metrix.Labels {
	return metrix.Labels{
		"stage":  string(stage),
		"reason": reason,
	}
}

func stateLabels(name, state string, labels metrix.Labels) metrix.Labels {
	out := make(metrix.Labels, len(labels)+1)
	maps.Copy(out, labels)
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

	var out strings.Builder
	out.WriteString(name + "{")
	for i, key := range keys {
		if i > 0 {
			out.WriteString(",")
		}
		out.WriteString(key + `="` + labels[key] + `"`)
	}
	return out.String() + "}"
}

func stateMetricKey(name, state string, labels metrix.Labels) string {
	return metricKey(name, stateLabels(name, state, labels))
}

func TestConfigSchemaEnforcesModeContracts(t *testing.T) {
	content, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var document any
	require.NoError(t, json.Unmarshal(content, &document))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("s3check.schema.json", document.(map[string]any)["jsonSchema"]))
	schema, err := compiler.Compile("s3check.schema.json")
	require.NoError(t, err)

	base := map[string]any{
		"endpoint": "http://127.0.0.1:9000", "region": "us-east-1", "bucket": "ok-bucket",
		"access_key_id": "access", "secret_access_key": "secret",
	}
	destination := map[string]any{
		"site": "site-b", "endpoint": "http://127.0.0.1:9001", "region": "us-east-1",
		"bucket": "ok-destination-bucket", "access_key_id": "destination-access",
		"secret_access_key": "destination-secret",
	}
	tests := map[string]struct {
		modify func(map[string]any)
		valid  bool
	}{
		"default single without destination":   {func(map[string]any) {}, true},
		"explicit single without destination":  {func(config map[string]any) { config["mode"] = "single" }, true},
		"single with destination":              {func(config map[string]any) { config["destination"] = destination }, false},
		"single with null destination":         {func(config map[string]any) { config["destination"] = nil }, true},
		"default single with null destination": {func(config map[string]any) { config["destination"] = nil }, true},
		"multisite missing destination":        {func(config map[string]any) { config["mode"] = "multisite"; config["source_site"] = "site-a" }, false},
		"multisite null destination": {func(config map[string]any) {
			config["mode"] = "multisite"
			config["source_site"] = "site-a"
			config["destination"] = nil
		}, false},
		"multisite complete": {func(config map[string]any) {
			config["mode"] = "multisite"
			config["source_site"] = "site-a"
			config["destination"] = destination
		}, true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config := maps.Clone(base)
			test.modify(config)
			err := schema.Validate(config)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
