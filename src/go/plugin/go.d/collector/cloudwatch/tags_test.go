// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"

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
	resources  []rgtatypes.ResourceTagMapping   // single page (used when pages is nil)
	pages      [][]rgtatypes.ResourceTagMapping // multi-page: call N returns page N, with a token until the last
	err        error
	calls      int
	gotFilters []string // ResourceTypeFilters seen on the most recent request
}

func (f *fakeRGTA) GetResources(_ context.Context, in *resourcegroupstaggingapi.GetResourcesInput, _ ...func(*resourcegroupstaggingapi.Options)) (*resourcegroupstaggingapi.GetResourcesOutput, error) {
	f.calls++
	f.gotFilters = in.ResourceTypeFilters
	if f.err != nil {
		return nil, f.err
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
// given tag allowlist. The instance is i-1 in account 000000000000, region us-east-1.
func ec2TagCollector(t *testing.T, tags []TagConfig, rgta rgtaClient) *Collector {
	t.Helper()
	c, _ := endToEndCollector(10)
	c.Config.Tags = tags
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.now = func() time.Time { return time.Unix(1_000_000_000, 0) }
	return c
}

func TestCollect_TagEnrichment(t *testing.T) {
	tests := map[string]struct {
		tags  []TagConfig
		rgta  *fakeRGTA
		check func(t *testing.T, labels map[string]string)
	}{
		"attaches allowlisted tags as non-identity labels (with rename)": {
			tags: []TagConfig{{Name: "owner"}, {Name: "team", Rename: "squad"}},
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
		"INV.2: an untagged instance still collects, without tag labels": {
			tags: []TagConfig{{Name: "owner"}},
			rgta: &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
				rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-999", "owner", "bob"),
			}},
			check: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "i-1", labels["instance_id"], "untagged instance still collects")
				_, ok := labels["owner"]
				assert.False(t, ok, "no tag label for an untagged instance")
			},
		},
		"INV.2: an RGTA failure leaves the series (no tags, no panic)": {
			tags: []TagConfig{{Name: "owner"}},
			rgta: &fakeRGTA{err: errors.New("access denied")},
			check: func(t *testing.T, labels map[string]string) {
				assert.Equal(t, "i-1", labels["instance_id"], "series survives an RGTA failure")
				_, ok := labels["owner"]
				assert.False(t, ok)
			},
		},
		"a tag colliding with a reserved label is skipped, not overwriting it": {
			tags: []TagConfig{{Name: "region"}, {Name: "owner"}},
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

func TestCollect_TagRetentionReEmitCarriesTags(t *testing.T) {
	// A not-due series is re-emitted from cache every cycle; its tag labels must ride
	// along (they are cached with the series, not re-resolved).
	rgta := &fakeRGTA{resources: []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice"),
	}}
	c := ec2TagCollector(t, []TagConfig{{Name: "owner"}}, rgta)
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

func TestRefreshTags_FirstFailureNoneThenSuccessThenCarryForward(t *testing.T) {
	rgta := &fakeRGTA{
		err: errors.New("throttled"),
		resources: []rgtatypes.ResourceTagMapping{
			rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "alice"),
		},
	}
	c := New()
	c.Config.Regions = []string{"us-east-1"}
	c.Config.Tags = []TagConfig{{Name: "owner"}}
	c.applyDefaults()
	c.accounts = []cwAccount{{accountID: "000000000000"}}
	c.profiles = []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	c.computeTagPlans()
	require.NotEmpty(t, c.tagPlans, "owner resolves to a plan for ec2")

	key := tagCacheKey{account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: "i-1"}
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

func TestGetAllResources_PaginatesAndFiltersByType(t *testing.T) {
	fake := &fakeRGTA{pages: [][]rgtatypes.ResourceTagMapping{
		{rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-1", "owner", "a")},
		{rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-2", "owner", "b")},
	}}

	got, err := getAllResources(context.Background(), fake, []string{"ec2:instance"}, 0)
	require.NoError(t, err)
	assert.Len(t, got, 2, "both pages are fetched")
	assert.Equal(t, 2, fake.calls, "the pagination token is followed")
	assert.Equal(t, []string{"ec2:instance"}, fake.gotFilters, "ResourceTypeFilters is passed through to RGTA")
}

func tagUnitCollector(t *testing.T, rgta rgtaClient) *Collector {
	t.Helper()
	c := New()
	c.Config.Regions = []string{"us-east-1"}
	c.Config.Tags = []TagConfig{{Name: "owner"}}
	c.applyDefaults()
	c.accounts = []cwAccount{{accountID: "000000000000"}}
	c.profiles = []cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}}
	c.newAWSConfig = func(context.Context, awsauth.Identity, string) (aws.Config, error) { return aws.Config{}, nil }
	c.newRGTAClient = func(aws.Config) rgtaClient { return rgta }
	return c
}

func TestRefreshTags_MarkStaleForcesRefetchWithinTTL(t *testing.T) {
	// A newly-resolved account marks tags stale so its tags are fetched the same cycle
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
	c.refreshTags(context.Background())
	assert.Equal(t, 2, rgta.calls, "markTagsStale forces a re-fetch within the TTL")
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

func TestIndexResourceTags_SkipsForeignAccountRegion(t *testing.T) {
	// RGTA is queried per (account, region); a resource whose ARN names a different
	// account or region must not be cached against the queried target.
	joins := selectedTagJoins([]cwprofiles.ResolvedProfile{{Name: "ec2", Config: ec2QueryProfile()}})
	rtIndex := resourceTypeIndex(joins)
	plans := map[string][]resolvedTag{"ec2": {{awsKey: "owner", label: "owner"}}}
	dst := map[tagCacheKey][]metrix.Label{}

	resources := []rgtatypes.ResourceTagMapping{
		rgtaResource("arn:aws:ec2:us-east-1:000000000000:instance/i-ok", "owner", "a"),
		rgtaResource("arn:aws:ec2:us-east-1:999999999999:instance/i-acct", "owner", "b"),   // wrong account
		rgtaResource("arn:aws:ec2:eu-west-1:000000000000:instance/i-region", "owner", "c"), // wrong region
	}
	indexResourceTags(dst, "000000000000", "us-east-1", resources, rtIndex, joins, plans)

	require.Len(t, dst, 1, "only the same-account, same-region resource is cached")
	assert.Contains(t, dst, tagCacheKey{account: "000000000000", region: "us-east-1", profile: "ec2", joinKey: "i-ok"})
}
