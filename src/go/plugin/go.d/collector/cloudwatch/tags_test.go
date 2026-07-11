// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRGTA is a Resource Groups Tagging API stub returning canned resources, or an
// error when err is set. calls counts GetResources invocations across pages/cycles.
type fakeRGTA struct {
	resources   []rgtatypes.ResourceTagMapping   // single page (used when pages is nil)
	pages       [][]rgtatypes.ResourceTagMapping // multi-page: call N returns page N, with a token until the last
	err         error
	pageErr     error // returned after the first page when a pagination token is present
	calls       int
	gotFilters  []string // ResourceTypeFilters seen on the most recent request
	gotTags     []rgtatypes.TagFilter
	gotPageSize int32
}

type recordingRGTA struct {
	mu        sync.Mutex
	resources []rgtatypes.ResourceTagMapping
	inputs    []*resourcegroupstaggingapi.GetResourcesInput
}

type staticRGTA struct {
	resources []rgtatypes.ResourceTagMapping
}

func (f staticRGTA) GetResources(context.Context, *resourcegroupstaggingapi.GetResourcesInput, ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	return &resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: f.resources}, nil
}

func (f *recordingRGTA) GetResources(_ context.Context, in *resourcegroupstaggingapi.GetResourcesInput, _ ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	f.mu.Lock()
	f.inputs = append(f.inputs, in)
	f.mu.Unlock()
	return &resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: f.resources}, nil
}

func (f *recordingRGTA) requests() []*resourcegroupstaggingapi.GetResourcesInput {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.inputs)
}

func (f *fakeRGTA) GetResources(_ context.Context, in *resourcegroupstaggingapi.GetResourcesInput, _ ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	f.calls++
	f.gotFilters = in.ResourceTypeFilters
	f.gotTags = in.TagFilters
	f.gotPageSize = aws.ToInt32(in.ResourcesPerPage)
	if f.err != nil {
		return nil, f.err
	}
	if f.pageErr != nil {
		if in.PaginationToken != nil {
			return nil, f.pageErr
		}
		return &resourcegroupstaggingapi.GetResourcesOutput{
			ResourceTagMappingList: f.resources,
			PaginationToken:        aws.String("next"),
		}, nil
	}
	if f.pages != nil {
		idx := f.calls - 1
		out := &resourcegroupstaggingapi.GetResourcesOutput{}
		if idx < len(f.pages) {
			out.ResourceTagMappingList = f.pages[idx]
		}
		if idx < len(f.pages)-1 {
			out.PaginationToken = aws.String("next")
		}
		return out, nil
	}
	return &resourcegroupstaggingapi.GetResourcesOutput{ResourceTagMappingList: f.resources}, nil
}

func rgtaResource(arn string, kv ...string) rgtatypes.ResourceTagMapping {
	m := rgtatypes.ResourceTagMapping{ResourceARN: aws.String(arn)}
	for i := 0; i+1 < len(kv); i += 2 {
		m.Tags = append(m.Tags, rgtatypes.Tag{Key: aws.String(kv[i]), Value: aws.String(kv[i+1])})
	}
	return m
}

func resourceTagPolicyConfig(count int) Config {
	defaults := false
	cfg := validBaseConfig()
	cfg.Rules = nil
	for i := range count {
		filters := []ResourceTagFilterConfig{{Key: "policy", Values: []string{strconv.Itoa(i)}}}
		cfg.Rules = append(cfg.Rules, RuleConfig{
			Name: fmt.Sprintf("policy-%d", i), Targets: []string{"base"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
			Filters:  &RuleFiltersConfig{ResourceTags: &filters},
		})
	}
	return cfg
}

// seriesLabels returns the label set of the single emitted series whose name has the
// given prefix (fails if not exactly one), read from the collector's metric store.
func seriesLabels(t *testing.T, c *Collector, namePrefix string) map[string]string {
	t.Helper()
	out := make(map[string]string)
	found := 0
	c.MetricStore().Read().ForEachSeries(func(name string, labels metrix.LabelView, _ metrix.SampleValue) {
		if len(name) < len(namePrefix) || name[:len(namePrefix)] != namePrefix {
			return
		}
		found++
		labels.Range(func(k, v string) bool {
			out[k] = v
			return true
		})
	})
	require.Equalf(t, 1, found, "exactly one series with name prefix %q", namePrefix)
	return out
}

// ec2TagCollector wires the end-to-end EC2 collector with a fake RGTA client and the
// given resource-tag label config. The instance is i-1 in account 000000000000, region us-east-1.
func ec2TagCollector(t *testing.T, tags []ResourceTagLabelConfig, rgta rgtaClient) *Collector {
	t.Helper()
	c, _ := endToEndCollector(10)
	c.Config.Labels.ResourceTags = tags
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
	return c
}

func TestCollect_TagEnrichment(t *testing.T) {
	tests := map[string]struct {
		tags  []ResourceTagLabelConfig
		rgta  *fakeRGTA
		check func(t *testing.T, labels map[string]string)
	}{
		"attaches selected tags as non-identity labels (with rename)": {
			tags: []ResourceTagLabelConfig{{Key: "owner"}, {Key: "team", Label: "squad"}},
			rgta: &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
				rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice", "team", "sre"),
			}},
			check: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "alice", labels["owner"])
				assert.Equal(t, "sre", labels["squad"])
				// identity labels intact and untouched
				assert.Equal(t, "i-1", labels["instance_id"])
				assert.Equal(t, "000000000000", labels["account_id"])
				assert.Equal(t, "us-east-1", labels["region"])
			},
		},
		"an untagged instance still collects when tags are labels only": {
			tags: []ResourceTagLabelConfig{{Key: "owner"}},
			rgta: &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
				rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-999", "owner", "bob"),
			}},
			check: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "i-1", labels["instance_id"], "untagged instance still collects")
				_, ok := labels["owner"]
				assert.False(t, ok, "no tag label for an untagged instance")
			},
		},
		"a labels-only RGTA failure leaves the series": {
			tags: []ResourceTagLabelConfig{{Key: "owner"}},
			rgta: &fakeRGTA{err: errors.New("access denied")},
			check: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "i-1", labels["instance_id"], "series survives an RGTA failure")
				_, ok := labels["owner"]
				assert.False(t, ok)
			},
		},
		"a tag colliding with a reserved label is skipped, not overwriting it": {
			tags: []ResourceTagLabelConfig{{Key: "region"}, {Key: "owner"}},
			rgta: &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
				rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "region", "bogus", "owner", "alice"),
			}},
			check: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "us-east-1", labels["region"], "reserved region label is never overwritten by a tag")
				assert.Equal(t, "alice", labels["owner"], "the valid sibling tag still attaches")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := ec2TagCollector(t, tc.tags, tc.rgta)
			_, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)
			tc.check(t, seriesLabels(t, c, "ec2.cpu_utilization_average"))
		})
	}
}

func TestCollect_ResourceTagFilteringFromPublicConfig(t *testing.T) {
	defaults := false
	replacement := []ResourceTagFilterConfig{{Key: "owner", Values: []string{"platform"}}}
	disabled := []ResourceTagFilterConfig{}
	cfg := validBaseConfig()
	cfg.Targets = []TargetConfig{
		{Name: "inherited", Credentials: "sdk_default"},
		{Name: "replacement", Credentials: "sdk_default"},
		{Name: "fallback", Credentials: "sdk_default"},
	}
	cfg.Defaults.Filters.ResourceTags = []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
	cfg.Rules = []RuleConfig{
		{
			Name: "inherited", Targets: []string{"inherited"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
		},
		{
			Name: "replacement", Targets: []string{"replacement"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
			Filters:  &RuleFiltersConfig{ResourceTags: &replacement},
		},
		{
			Name: "fallback", Targets: []string{"fallback"}, Regions: []string{"us-east-1"},
			Profiles: &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2"}},
			Filters:  &RuleFiltersConfig{ResourceTags: &disabled},
		},
	}

	cw := &nsCloudWatch{byNS: map[string][]cwtypes.Metric{"AWS/EC2": {
		mkMetric("CPUUtilization", "InstanceId", "i-prod"),
		mkMetric("CPUUtilization", "InstanceId", "i-owner"),
		mkMetric("CPUUtilization", "InstanceId", "i-other"),
	}}}
	rgta := &recordingRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-prod", "environment", "production"),
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-owner", "owner", "platform"),
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-other", "environment", "development"),
	}}
	c := New()
	c.Config = cfg
	c.newCatalog = cwprofiles.DefaultCatalog
	c.newAWSConfig = func(_ context.Context, _ awsauth.Identity, region string) (aws.Config, error) {
		return aws.Config{Region: region}, nil
	}
	c.newSTSClient = func(aws.Config) stsClient { return &fakeSTS{account: "000000000000"} }
	c.newCloudWatchClient = func(aws.Config) cloudwatchClient { return cw }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Collect(context.Background()))
	require.Len(t, c.plan.Scopes, 3)
	assert.Equal(t, "environment", c.plan.Scopes[0].TagFilter[0].key, "omitted rule filter inherits the job default")
	assert.Equal(t, "owner", c.plan.Scopes[1].TagFilter[0].key, "present rule filter replaces the job default")
	assert.Empty(t, c.plan.Scopes[2].TagFilter, "an explicit empty rule filter disables the job default")
	assert.Equal(t, map[string]string{
		"i-prod": "inherited", "i-owner": "replacement", "i-other": "fallback",
	}, queryOwnersByInstance(c.queryPlan))

	requests := rgta.requests()
	require.Len(t, requests, 2, "the unfiltered fallback rule does not make an RGTA request")
	gotFilters := make(map[string][]string)
	for _, request := range requests {
		require.Len(t, request.TagFilters, 1)
		gotFilters[aws.ToString(request.TagFilters[0].Key)] = request.TagFilters[0].Values
		assert.Equal(t, []string{"ec2:instance"}, request.ResourceTypeFilters)
	}
	assert.Equal(t, map[string][]string{
		"environment": {"production"}, "owner": {"platform"},
	}, gotFilters)
}

func TestCollect_TagRetentionReEmitCarriesTags(t *testing.T) {
	// A not-due series is re-emitted from cache every cycle; its tag labels must ride
	// along (they are cached with the series, not re-resolved).
	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice"),
	}}
	c := ec2TagCollector(t, []ResourceTagLabelConfig{{Key: "owner"}}, rgta)
	base := time.Unix(1_000_000_000, 0)

	// Cycle 1: queried + tagged.
	c.now = func() time.Time { return base }
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Equal(t, "alice", seriesLabels(t, c, "ec2.cpu_utilization_average")["owner"])

	// Cycle 2 at +60s: the 300s period is not due -> re-emit. Tags must persist.
	c.now = func() time.Time { return base.Add(60 * time.Second) }
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, "alice", seriesLabels(t, c, "ec2.cpu_utilization_average")["owner"],
		"re-emitted (not-due) series keeps its tag labels")
}

func TestCollect_TagLabelAddChangeRemoveUpdatesExistingSeries(t *testing.T) {
	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "environment", "production"),
	}}
	c := ec2TagCollector(t, []ResourceTagLabelConfig{{Key: "owner"}}, rgta)
	base := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return base }

	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	_, hasOwner := seriesLabels(t, c, "ec2.cpu_utilization_average")["owner"]
	assert.False(t, hasOwner)

	rgta.resources = []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "environment", "production", "owner", "alice"),
	}
	c.markTagsStale()
	c.now = func() time.Time { return base.Add(60 * time.Second) }
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, "alice", seriesLabels(t, c, "ec2.cpu_utilization_average")["owner"])

	rgta.resources = []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "environment", "production", "owner", "bob"),
	}
	c.markTagsStale()
	c.now = func() time.Time { return base.Add(120 * time.Second) }
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, "bob", seriesLabels(t, c, "ec2.cpu_utilization_average")["owner"])

	rgta.resources = []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "environment", "production"),
	}
	c.markTagsStale()
	c.now = func() time.Time { return base.Add(180 * time.Second) }
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	_, hasOwner = seriesLabels(t, c, "ec2.cpu_utilization_average")["owner"]
	assert.False(t, hasOwner, "removing an AWS tag removes the non-identity label without recreating the series")
}

func TestRefreshTags_FirstFailureNoneThenSuccessThenCarryForward(t *testing.T) {
	rgta := &fakeRGTA{
		err: errors.New("throttled"),
		resources: []rgtatypes.ResourceTagMapping{
			rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice"),
		},
	}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	c.Config.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "owner"}}
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.computeTagLabelPlans()
	require.NotEmpty(t, c.tagLabelPlans, "owner resolves to a plan for ec2")

	key := tagCacheKey{target: "base", account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: "i-1"}
	base := time.Unix(1_000_000_000, 0)
	ttl := time.Duration(c.Discovery.RefreshEvery) * time.Second

	// First refresh fails with no prior cache -> no tags (not last-known, none exists).
	c.now = func() time.Time { return base }
	c.refreshTags(context.Background())
	assert.NotContains(t, c.tags.labels, key, "first-run RGTA failure yields no tags")

	// TTL expires and RGTA succeeds -> tags cached.
	rgta.err = nil
	c.now = func() time.Time { return base.Add(ttl) }
	c.refreshTags(context.Background())
	require.Contains(t, c.tags.labels, key)
	assert.Equal(t, []metrix.Label{{Key: "owner", Value: "alice"}}, c.tags.labels[key])

	// TTL expires and RGTA fails again -> last-known tags carried forward.
	rgta.err = errors.New("throttled again")
	c.now = func() time.Time { return base.Add(2 * ttl) }
	c.refreshTags(context.Background())
	assert.Equal(t, []metrix.Label{{Key: "owner", Value: "alice"}}, c.tags.labels[key],
		"a later RGTA failure keeps the last-known tags")
}

func TestRefreshTags_FilterMembershipFailureLifecycle(t *testing.T) {
	rgta := &fakeRGTA{
		err: errors.New("throttled"),
		resources: []rgtatypes.ResourceTagMapping{
			rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "environment", "production"),
		},
	}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	c.plan.Scopes[0].ID = 7
	c.plan.Scopes[0].TagFilter = []resourceTagFilter{{key: "environment", values: []string{"production"}}}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	base := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return base }

	c.refreshTags(context.Background())
	assert.True(t, c.tags.scopeUnknown(7), "a first failure is unknown")
	assert.False(t, c.tags.scopeSelected(7, "i-1"))
	assert.True(t, c.tags.expired(base), "a failed group retries next collect")

	rgta.err = nil
	c.refreshTags(context.Background())
	assert.False(t, c.tags.scopeUnknown(7))
	assert.True(t, c.tags.scopeSelected(7, "i-1"))
	assert.False(t, c.tags.expired(base), "a successful refresh advances the TTL")
	require.Len(t, rgta.gotTags, 1)
	assert.Equal(t, "environment", aws.ToString(rgta.gotTags[0].Key))
	assert.Equal(t, []string{"production"}, rgta.gotTags[0].Values)

	rgta.err = errors.New("throttled again")
	c.markTagsStale()
	c.refreshTags(context.Background())
	assert.True(t, c.tags.scopeUnknown(7))
	assert.True(t, c.tags.scopeSelected(7, "i-1"), "a later failure retains last-known membership")
}

func TestRefreshTags_FailedGroupRetriesWithoutRefetchingHealthyGroup(t *testing.T) {
	healthy := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-east", "environment", "production"),
	}}
	failing := &fakeRGTA{err: errors.New("throttled")}
	c := New()
	configureExactRule(c, []string{"us-east-1", "us-west-2"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1", "us-west-2"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	for i := range c.plan.Scopes {
		c.plan.Scopes[i].ID = i
		c.plan.Scopes[i].TagFilter = []resourceTagFilter{{key: "environment", values: []string{"production"}}}
	}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-east"}}},
		{Target: "base", Profile: "ec2", Region: "us-west-2"}: {{DimensionValues: []string{"i-west"}}},
	}}
	c.rgtaClients = newClientCache(func(_ context.Context, _, region string) (rgtaClient, error) {
		if region == "us-east-1" {
			return healthy, nil
		}
		return failing, nil
	})
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

	c.refreshTags(context.Background())
	require.Equal(t, 1, healthy.calls)
	require.Equal(t, 1, failing.calls)
	assert.False(t, c.tags.scopeUnknown(0))
	assert.True(t, c.tags.scopeSelected(0, "i-east"))
	assert.True(t, c.tags.scopeUnknown(1))

	healthy.err = errors.New("healthy group must not be called before its TTL")
	c.refreshTags(context.Background())

	assert.Equal(t, 1, healthy.calls, "a healthy group remains cached until its own TTL")
	assert.Equal(t, 2, failing.calls, "the failed group retries on the next collect")
	assert.False(t, c.tags.scopeUnknown(0), "an unnecessary retry must not turn fresh membership unknown")
	assert.True(t, c.tags.scopeSelected(0, "i-east"))
}

func TestRefreshTags_FirstSuccessfulEmptySnapshotInvalidatesImplicitUnknown(t *testing.T) {
	rgta := &fakeRGTA{}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	c.plan.Scopes[0].ID = 7
	c.plan.Scopes[0].TagFilter = []resourceTagFilter{{key: "environment", values: []string{"production"}}}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
	c.planDirty = false

	c.refreshTags(context.Background())

	assert.False(t, c.tags.fetchedAt.IsZero())
	assert.False(t, c.tags.scopeUnknown(7))
	assert.True(t, c.planDirty, "known-empty membership must rebuild a plan previously compiled from implicit unknown state")
}

func TestRefreshTags_LaterPageFailureIsAtomic(t *testing.T) {
	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "environment", "production"),
	}}
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	c.plan.Scopes[0].ID = 7
	c.plan.Scopes[0].TagFilter = []resourceTagFilter{{key: "environment", values: []string{"production"}}}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {
			{DimensionValues: []string{"i-1"}}, {DimensionValues: []string{"i-2"}},
		},
	}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

	c.refreshTags(context.Background())
	require.True(t, c.tags.scopeSelected(7, "i-1"))
	require.False(t, c.tags.scopeUnknown(7))

	rgta.resources = []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-2", "environment", "production"),
	}
	rgta.pageErr = errors.New("second page failed")
	c.markTagsStale()
	c.refreshTags(context.Background())

	assert.True(t, c.tags.scopeUnknown(7))
	assert.True(t, c.tags.scopeSelected(7, "i-1"), "the prior complete snapshot is retained")
	assert.False(t, c.tags.scopeSelected(7, "i-2"), "partial data from the failed refresh is discarded")
}

func TestWalkResourceTags_PaginatesAndUsesNativeFilters(t *testing.T) {
	fake := &fakeRGTA{pages: [][]rgtatypes.ResourceTagMapping{
		{rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "a")},
		{rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-2", "owner", "b")},
	}}

	var got []rgtatypes.ResourceTagMapping
	err := walkResourceTags(context.Background(), fake, []string{"ec2:instance"}, []resourceTagFilter{
		{key: "env", values: []string{"prod", "staging"}},
	}, 0, func(resource rgtatypes.ResourceTagMapping) { got = append(got, resource) })
	require.NoError(t, err)
	assert.Len(t, got, 2, "both pages are fetched")
	assert.Equal(t, 2, fake.calls, "the pagination token is followed")
	assert.Equal(t, []string{"ec2:instance"}, fake.gotFilters, "ResourceTypeFilters is passed through to RGTA")
	require.Equal(t, []rgtatypes.TagFilter{{Key: aws.String("env"), Values: []string{"prod", "staging"}}}, fake.gotTags)
	assert.Equal(t, int32(100), fake.gotPageSize)
}

func TestTagFetchGroups_SeparateFetchIdentityFromPolicyScopes(t *testing.T) {
	catalog, err := cwprofiles.DefaultCatalog()
	require.NoError(t, err)

	t.Run("same predicate shares one fetch across profiles", func(t *testing.T) {
		defaults := false
		filters := []ResourceTagFilterConfig{{Key: "environment", Values: []string{"production"}}}
		cfg := validBaseConfig()
		cfg.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &defaults, Include: []string{"ec2", "ebs"}}
		cfg.Rules[0].Filters = &RuleFiltersConfig{ResourceTags: &filters}
		cfg.applyDefaults()
		plan, _, err := compileConfig(cfg, catalog)
		require.NoError(t, err)
		c := New()
		c.plan = plan
		c.resolvedByRef = map[string]resolvedTarget{"base": {target: plan.Targets[0], accountID: "000000000000"}}
		c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
			{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
			{Target: "base", Profile: "ebs", Region: "us-east-1"}: {{DimensionValues: []string{"vol-1"}}},
		}}
		c.tagLabelPlans = map[string][]resolvedTag{}

		groups := c.tagFetchGroups()
		require.Len(t, groups, 1)
		assert.ElementsMatch(t, []string{"ec2:instance", "ec2:volume"}, groups[0].resourceTypes)
		assert.Len(t, groups[0].joins, 2)
	})

	t.Run("different predicates remain separate", func(t *testing.T) {
		cfg := resourceTagPolicyConfig(4)
		cfg.applyDefaults()
		plan, _, err := compileConfig(cfg, catalog)
		require.NoError(t, err)
		c := New()
		c.plan = plan
		c.resolvedByRef = map[string]resolvedTarget{"base": {target: plan.Targets[0], accountID: "000000000000"}}
		c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
			{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
		}}
		c.tagLabelPlans = map[string][]resolvedTag{}

		groups := c.tagFetchGroups()
		require.Len(t, groups, 4)
		for _, group := range groups {
			assert.Len(t, group.scopeIDsByProfile["ec2"], 1)
		}
	})
}

func tagUnitCollector(t *testing.T, rgta rgtaClient) *Collector {
	t.Helper()
	c := New()
	configureExactRule(c, []string{"us-east-1"}, []string{"ec2"})
	c.Config.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "owner"}}
	setSingleTargetPlan(c, "000000000000", []string{"us-east-1"}, []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "base", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	return c
}

func TestRefreshTags_MarkStaleForcesRefetchWithinTTL(t *testing.T) {
	// A newly-resolved target marks tags stale so its tags are fetched the same cycle
	// (before its charts are created), even though the global TTL is still valid.
	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice"),
	}}
	c := tagUnitCollector(t, rgta)

	base := time.Unix(1_000_000_000, 0)
	c.now = func() time.Time { return base }
	c.refreshTags(context.Background())
	require.Equal(t, 1, rgta.calls)

	c.now = func() time.Time { return base.Add(30 * time.Second) }
	c.refreshTags(context.Background())
	require.Equal(t, 1, rgta.calls, "within the TTL there is no re-fetch")

	c.markTagsStale()
	c.planDirty = false
	c.refreshTags(context.Background())
	assert.Equal(t, 2, rgta.calls, "markTagsStale forces a re-fetch within the TTL")
	assert.False(t, c.planDirty, "an unchanged effective snapshot does not rebuild the query plan")
}

func TestRefreshTags_CanceledContextDoesNotAdvanceTTL(t *testing.T) {
	// A collect context canceled during the RGTA fan-out must not commit a snapshot or
	// advance the TTL, so the next cycle retries instead of suppressing tags.
	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice"),
	}}
	c := tagUnitCollector(t, rgta)
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c.refreshTags(ctx)

	assert.True(t, c.tags.expired(c.now()), "a canceled refresh must not advance the TTL")
	assert.Empty(t, c.tags.labels, "a canceled refresh must not commit a snapshot")
}

type failingRGTA struct{ err error }

func (f failingRGTA) GetResources(context.Context, *resourcegroupstaggingapi.GetResourcesInput, ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	return nil, f.err
}

func TestRefreshTags_ReportsIndependentTargetFailures(t *testing.T) {
	const sensitive = "SENSITIVE_TAG_API_MESSAGE"
	var logs bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&logs)
	c.Config = twoTargetConfig()
	c.Config.Labels.ResourceTags = []ResourceTagLabelConfig{{Key: "owner"}}
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.ensurePlan())
	c.resolvedByRef = make(map[string]resolvedTarget)
	for _, target := range c.plan.Targets {
		resolved := resolvedTarget{target: target, accountID: "000000000000"}
		c.resolvedByRef[target.Name] = resolved
	}
	c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
		{Target: "first", Profile: "ec2", Region: "us-east-1"}:  {{DimensionValues: []string{"i-1"}}},
		{Target: "second", Profile: "ec2", Region: "us-east-1"}: {{DimensionValues: []string{"i-1"}}},
	}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return failingRGTA{err: errors.New(sensitive)} }

	c.refreshTags(context.Background())

	assert.Contains(t, logs.String(), "first")
	assert.Contains(t, logs.String(), "second")
	assert.Contains(t, logs.String(), "us-east-1")
	assert.NotContains(t, logs.String(), sensitive)
}

func TestIndexFetchedResource_SkipsForeignAccountRegion(t *testing.T) {
	// RGTA is queried per (account, region); a resource whose ARN names a different
	// account or region must not be cached against the queried target.
	plans := map[string][]resolvedTag{"ec2": {{awsKey: "owner", label: "owner"}}}
	members := tagMembership{}
	labels := map[tagCacheKey][]metrix.Label{}
	confirmed := map[tagCacheKey]struct{}{}
	join, err := resolveTagJoinProfile(cwprofiles.ResolvedProfile{Name: "ec2", Config: ec2QueryProfile()})
	require.NoError(t, err)
	group := tagFetchGroup{
		key: tagFetchKey{target: "base", region: "us-east-1"}, account: "000000000000",
		joins:                  map[string]*tagJoin{"ec2": join},
		profilesByResourceType: map[string][]string{"ec2:instance": {"ec2"}},
		scopeIDsByProfile:      map[string][]int{"ec2": {7}},
		tagKeys:                map[string]struct{}{"owner": {}},
		candidatesByProfile: map[string]map[string]struct{}{"ec2": {
			"i-ok": {}, "i-acct": {}, "i-region": {},
		}},
	}

	resources := []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-ok", "owner", "a"),
		rgtaResource("arn:aws:ec2:us-east-1:999999999999:instance/i-acct", "owner", "b"),   // wrong account
		rgtaResource("arn:aws:ec2:eu-west-1:000000000000:instance/i-region", "owner", "c"), // wrong region
	}
	for _, resource := range resources {
		indexFetchedResource(members, labels, confirmed, group, resource, plans)
	}

	wantLabelKey := tagCacheKey{target: "base", account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: "i-ok"}
	assert.Equal(t, tagMembership{7: {"i-ok": {}}}, members)
	assert.Equal(t, map[tagCacheKey][]metrix.Label{wantLabelKey: {{Key: "owner", Value: "a"}}}, labels)
	assert.Equal(t, map[tagCacheKey]struct{}{wantLabelKey: {}}, confirmed)
}

func TestMergeTagGroupSnapshots_RetainsIndependentGroupState(t *testing.T) {
	matching := tagCacheKey{target: "first", account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: "i-1"}
	healthy := tagCacheKey{target: "second", account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: "i-2"}
	now := time.Unix(1_000_000_000, 0)
	failedGroup := tagFetchGroup{
		key: tagFetchKey{target: "first", region: "us-east-1"}, account: "000000000000",
		scopeIDsByProfile:   map[string][]int{"ec2": {7}},
		candidatesByProfile: map[string]map[string]struct{}{"ec2": {"i-1": {}}},
	}
	healthyGroup := tagFetchGroup{
		key: tagFetchKey{target: "second", region: "us-east-1"}, account: "000000000000",
		scopeIDsByProfile: map[string][]int{"ec2": {8}},
	}
	states := map[tagFetchKey]tagGroupSnapshot{
		failedGroup.key: {
			group: failedGroup, members: tagMembership{7: {"i-1": {}}},
			labels:          map[tagCacheKey][]metrix.Label{matching: {{Key: "owner", Value: "one"}}},
			confirmedLabels: map[tagCacheKey]struct{}{matching: {}},
			lastSuccess:     now.Add(-time.Minute), unknown: true,
		},
		healthyGroup.key: {
			group: healthyGroup, members: tagMembership{8: {"i-2": {}}},
			labels:          map[tagCacheKey][]metrix.Label{healthy: {{Key: "owner", Value: "two"}}},
			confirmedLabels: map[tagCacheKey]struct{}{healthy: {}},
			lastSuccess:     now, expiresAt: now.Add(time.Minute),
		},
	}

	got := mergeTagGroupSnapshots(states, now, now.Add(time.Minute))

	assert.Equal(t, map[tagCacheKey][]metrix.Label{
		matching: {{Key: "owner", Value: "one"}}, healthy: {{Key: "owner", Value: "two"}},
	}, got.labels)
	assert.Equal(t, tagMembership{7: {"i-1": {}}, 8: {"i-2": {}}}, got.members)
	assert.Equal(t, map[int]struct{}{7: {}}, got.unknown)
	assert.True(t, got.expiresAt.IsZero(), "the failed group remains due without expiring the healthy group's own state")
}

func BenchmarkMergeTagGroupSnapshots(b *testing.B) {
	const cachedTags = 8192
	group := tagFetchGroup{
		key: tagFetchKey{target: "target", region: "us-east-1"}, account: "000000000000",
		scopeIDsByProfile:   map[string][]int{"ec2": {7}},
		candidatesByProfile: map[string]map[string]struct{}{"ec2": make(map[string]struct{}, cachedTags)},
	}
	state := tagGroupSnapshot{
		group: group, members: tagMembership{7: make(map[string]struct{}, cachedTags)},
		labels:          make(map[tagCacheKey][]metrix.Label, cachedTags),
		confirmedLabels: make(map[tagCacheKey]struct{}, cachedTags), unknown: true,
	}
	for i := range cachedTags {
		joinKey := strconv.Itoa(i)
		key := tagCacheKey{
			target: "target", account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: joinKey,
		}
		state.labels[key] = nil
		state.confirmedLabels[key] = struct{}{}
		state.members.add(7, joinKey)
		group.candidatesByProfile["ec2"][joinKey] = struct{}{}
	}
	states := map[tagFetchKey]tagGroupSnapshot{group.key: state}

	b.ReportAllocs()
	for range b.N {
		got := mergeTagGroupSnapshots(states, time.Time{}, time.Time{})
		if len(got.labels) != cachedTags || len(got.members[7]) != cachedTags {
			b.Fatalf("merged %d labels and %d members, want %d", len(got.labels), len(got.members[7]), cachedTags)
		}
	}
}

func BenchmarkIndexFetchedResources(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			join, err := resolveTagJoinProfile(cwprofiles.ResolvedProfile{Name: "ec2", Config: ec2QueryProfile()})
			if err != nil {
				b.Fatal(err)
			}
			resources := make([]rgtatypes.ResourceTagMapping, count)
			candidates := make(map[string]struct{}, count)
			for i := range resources {
				joinKey := "i-" + strconv.Itoa(i)
				candidates[joinKey] = struct{}{}
				resources[i] = rgtaResource(
					"arn:aws:ec2:us-east-1:000000000000:instance/"+joinKey,
					"environment", "production", "owner", "platform",
				)
			}
			group := tagFetchGroup{
				key: tagFetchKey{target: "base", region: "us-east-1"}, account: "000000000000",
				joins:                  map[string]*tagJoin{"ec2": join},
				filters:                []resourceTagFilter{{key: "environment", values: []string{"production"}}},
				profilesByResourceType: map[string][]string{"ec2:instance": {"ec2"}},
				scopeIDsByProfile:      map[string][]int{"ec2": {7}},
				candidatesByProfile:    map[string]map[string]struct{}{"ec2": candidates},
				tagKeys:                map[string]struct{}{"environment": {}, "owner": {}},
			}
			plans := map[string][]resolvedTag{"ec2": {{awsKey: "owner", label: "owner"}}}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				members := make(tagMembership)
				labels := make(map[tagCacheKey][]metrix.Label)
				confirmed := make(map[tagCacheKey]struct{})
				for _, resource := range resources {
					indexFetchedResource(members, labels, confirmed, group, resource, plans)
				}
				if len(members[7]) != count || len(labels) != count {
					b.Fatalf("indexed %d members and %d labels, want %d", len(members[7]), len(labels), count)
				}
			}
		})
	}
}

func BenchmarkRefreshTagsPolicyCount(b *testing.B) {
	const resources = 1000
	catalog, err := cwprofiles.DefaultCatalog()
	if err != nil {
		b.Fatal(err)
	}
	for _, policies := range []int{1, 4} {
		b.Run(fmt.Sprintf("policies=%d", policies), func(b *testing.B) {
			cfg := resourceTagPolicyConfig(policies)
			cfg.applyDefaults()
			plan, _, err := compileConfig(cfg, catalog)
			if err != nil {
				b.Fatal(err)
			}
			mappings := make([]rgtatypes.ResourceTagMapping, resources)
			instances := make([]discoveredInstance, resources)
			for i := range resources {
				id := fmt.Sprintf("i-%d", i)
				instances[i] = discoveredInstance{DimensionValues: []string{id}}
				mappings[i] = rgtaResource(
					"arn:aws:ec2:us-east-1:000000000000:instance/"+id,
					"policy", strconv.Itoa(i%policies),
				)
			}
			c := New()
			c.Config = cfg
			c.plan = plan
			c.resolvedByRef = map[string]resolvedTarget{
				"base": {target: plan.Targets[0], accountID: "000000000000"},
			}
			c.discovery = discoverySnapshot{Instances: map[discoveryKey][]discoveredInstance{
				{Target: "base", Profile: "ec2", Region: "us-east-1"}: instances,
			}}
			client := staticRGTA{resources: mappings}
			c.rgtaClients = newClientCache(func(context.Context, string, string) (rgtaClient, error) {
				return client, nil
			})
			c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				c.markTagsStale()
				c.refreshTags(context.Background())
			}
			b.StopTimer()
			if got := len(c.tags.members); got != policies {
				b.Fatalf("got %d policy membership sets, want %d", got, policies)
			}
		})
	}
}

func BenchmarkTagSnapshotSameEffective(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			left := tagSnapshot{labels: make(map[tagCacheKey][]metrix.Label, count), members: tagMembership{7: make(map[string]struct{}, count)}, unknown: map[int]struct{}{}}
			right := tagSnapshot{labels: make(map[tagCacheKey][]metrix.Label, count), members: tagMembership{7: make(map[string]struct{}, count)}, unknown: map[int]struct{}{}}
			for i := range count {
				joinKey := strconv.Itoa(i)
				key := tagCacheKey{target: "base", account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: joinKey}
				labels := []metrix.Label{{Key: "owner", Value: "platform"}}
				left.labels[key] = labels
				right.labels[key] = labels
				left.members.add(7, joinKey)
				right.members.add(7, joinKey)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if !left.sameEffective(right) {
					b.Fatal("equal snapshots differ")
				}
			}
		})
	}
}
