// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"
	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// tagCacheKey identifies a resource's resolved tag labels by account, region, the
// profile they enrich, and the profile's ARN-projectable join key.
type tagCacheKey struct {
	account string
	region  string
	profile string
	joinKey string
}

// tagSnapshot is the cached result of one tag refresh: resolved labels per resource,
// with a TTL. On a per-(account, region) RGTA failure the previous snapshot's entries
// for that (account, region) are carried forward (last-known); a first-run failure
// simply yields no tags. Never gates series existence (INV.2).
type tagSnapshot struct {
	labels    map[tagCacheKey][]metrix.Label
	fetchedAt time.Time
	expiresAt time.Time
}

func (s tagSnapshot) expired(now time.Time) bool {
	return s.fetchedAt.IsZero() || !now.Before(s.expiresAt)
}

// markTagsStale forces the next refreshTags to re-fetch even within the TTL. Called
// when a new account resolves: its tags must be fetched this cycle, before its charts
// are created, because chart labels are set only at chart creation.
func (c *Collector) markTagsStale() {
	c.tags.expiresAt = time.Time{}
}

// profileDimLabels returns a profile's identifying (non-constant) dimension labels —
// the labels a tag must not collide with (else metrix would panic on a duplicate key).
func profileDimLabels(prof cwprofiles.Profile) []string {
	var out []string
	for _, d := range prof.Instance.Dimensions {
		if !d.IsConstant() {
			out = append(out, d.Label)
		}
	}
	return out
}

// computeTagPlans resolves the per-profile tag-label plan once (the allowlist and
// profiles are fixed per job) and logs each skip once. Profiles with no registered
// ARN join or an empty resolved plan are omitted, so they carry no tags.
func (c *Collector) computeTagPlans() {
	if len(c.Tags) == 0 {
		return
	}
	plans := make(map[string][]resolvedTag)
	for _, p := range c.profiles {
		if _, ok := tagJoins[p.Name]; !ok {
			continue
		}
		plan, warnings := resolveTagPlan(c.Tags, profileDimLabels(p.Config))
		for _, w := range warnings {
			c.Limit(logKeyTagPlanWarn+":"+p.Name+":"+w, 1, recurringLogEvery).
				Warningf("CloudWatch tags (profile %q): %s", p.Name, w)
		}
		if len(plan) > 0 {
			plans[p.Name] = plan
		}
	}
	c.tagPlans = plans
}

// refreshTags rebuilds the tag cache when its TTL has expired. It is best-effort and
// NEVER gates collection: a per-(account, region) failure carries last-known tags
// forward (or yields none on the first pass). Opt-in: with no configured tags (so no
// resolved plans) it is a no-op and no RGTA client is ever built.
func (c *Collector) refreshTags(ctx context.Context) {
	if len(c.Tags) == 0 {
		return // opt-in: no tags configured, so never build an RGTA client
	}
	if c.tagPlans == nil {
		c.computeTagPlans() // resolve per-profile plans once (allowlist + profiles are fixed)
	}
	if len(c.tagPlans) == 0 {
		return // every configured tag was skipped, or no selected profile has an ARN join
	}
	now := c.now()
	if !c.tags.expired(now) {
		return
	}

	joins := selectedTagJoins(c.profiles)
	filters := resourceTypeFilters(joins)
	rtIndex := resourceTypeIndex(joins)
	ttl := time.Duration(c.Discovery.RefreshEvery) * time.Second

	next := tagSnapshot{
		labels:    make(map[tagCacheKey][]metrix.Label),
		fetchedAt: now,
		expiresAt: now.Add(ttl),
	}

	// Fetch each (account, region) concurrently (bounded), so a slow or hung RGTA
	// endpoint cannot serialize into an accounts x regions x timeout collection stall;
	// the per-call timeout bounds each fetch. Indexing into the shared map is done
	// serially afterwards to avoid a data race. Mirrors discovery's fan-out.
	type tagFetch struct {
		account, region string
		resources       []rgtatypes.ResourceTagMapping
		err             error
	}
	var targets []tagFetch
	for _, account := range c.accountIDs() {
		for _, region := range c.regions() {
			targets = append(targets, tagFetch{account: account, region: region})
		}
	}

	p := pool.New().WithMaxGoroutines(max(1, apiConcurrency))
	for i := range targets {
		t := &targets[i]
		p.Go(func() {
			client, err := c.rgtaClients.forAccountRegion(ctx, t.account, t.region)
			if err != nil {
				t.err = fmt.Errorf("build client: %w", err)
				return
			}
			t.resources, t.err = getAllResources(ctx, client, filters, c.Timeout.Duration())
		})
	}
	p.Wait()

	// If the parent collect context was canceled or timed out during the fan-out, do
	// NOT commit next or advance the TTL: that would suppress tag retries until the next
	// TTL expiry (and risk creating charts untagged). Keeping the previous snapshot (its
	// TTL is already expired, which is why we ran) makes the next cycle retry. Per-call
	// timeouts use derived contexts and do not trip this, so they stay fail-soft.
	if ctx.Err() != nil {
		return
	}

	for _, t := range targets {
		if t.err != nil {
			c.carryForwardTags(next.labels, t.account, t.region)
			c.Limit(logKeyTagRefreshFailed+":"+t.account+"/"+t.region, 1, recurringLogEvery).
				Warningf("CloudWatch tag lookup for account %q region %q failed: %v (using last-known tags)", t.account, t.region, t.err)
			continue
		}
		indexResourceTags(next.labels, t.account, t.region, t.resources, rtIndex, joins, c.tagPlans)
	}

	c.tags = next
}

// carryForwardTags copies the previous snapshot's entries for one (account, region)
// into dst, so a transient RGTA failure keeps that scope's last-known tags.
func (c *Collector) carryForwardTags(dst map[tagCacheKey][]metrix.Label, account, region string) {
	for k, v := range c.tags.labels {
		if k.account == account && k.region == region {
			dst[k] = v
		}
	}
}

// getAllResources pages through RGTA GetResources, narrowed to the given resource
// types, and returns every tagged resource mapping.
func getAllResources(ctx context.Context, client rgtaClient, filters []string, timeout time.Duration) ([]rgtatypes.ResourceTagMapping, error) {
	cctx, cancel := withTimeout(ctx, timeout)
	defer cancel()

	var out []rgtatypes.ResourceTagMapping
	var token *string
	for {
		in := &resourcegroupstaggingapi.GetResourcesInput{PaginationToken: token}
		if len(filters) > 0 {
			in.ResourceTypeFilters = filters
		}
		resp, err := client.GetResources(cctx, in)
		if err != nil {
			return nil, err
		}
		out = append(out, resp.ResourceTagMappingList...)
		if resp.PaginationToken == nil || *resp.PaginationToken == "" {
			break
		}
		token = resp.PaginationToken
	}
	return out, nil
}

// indexResourceTags resolves each tagged resource to its (profile, joinKey) and
// stores the resolved tag labels. A resource whose ARN does not parse, whose type
// no selected profile claims, or whose join key cannot be built is skipped.
func indexResourceTags(
	dst map[tagCacheKey][]metrix.Label,
	account, region string,
	resources []rgtatypes.ResourceTagMapping,
	rtIndex map[string][]string,
	joins map[string]tagJoin,
	plans map[string][]resolvedTag,
) {
	for _, res := range resources {
		if res.ResourceARN == nil {
			continue
		}
		a, err := arn.Parse(*res.ResourceARN)
		if err != nil {
			continue
		}
		// RGTA is queried per (account, region), so a resource whose ARN names a
		// different account or region should not appear; skip it if it does, so tags
		// are never cached against the wrong target. Empty ARN fields (e.g. S3, which
		// carries no account/region) are allowed.
		if (a.AccountID != "" && a.AccountID != account) || (a.Region != "" && a.Region != region) {
			continue
		}
		profNames := rtIndex[deriveResourceType(a)]
		if len(profNames) == 0 {
			continue
		}
		tags := tagMap(res.Tags)
		for _, profName := range profNames {
			plan := plans[profName]
			if len(plan) == 0 {
				continue
			}
			jk, ok := joins[profName].arnJoinKey(a)
			if !ok {
				continue
			}
			labels := applyTagPlan(plan, tags)
			if len(labels) == 0 {
				continue
			}
			dst[tagCacheKey{account: account, region: region, profile: profName, joinKey: jk}] = labels
		}
	}
}

func tagMap(tags []rgtatypes.Tag) map[string]string {
	out := make(map[string]string, len(tags))
	for _, t := range tags {
		if t.Key != nil && t.Value != nil {
			out[*t.Key] = *t.Value
		}
	}
	return out
}

// applyTagPlan produces the emitted tag labels for a resource: for each surviving
// (awsKey -> label) in plan, the label if the resource carries that tag. The result
// is a fresh slice stored read-only in the cache; the write path concatenates it
// into a new slice, so it is never mutated.
func applyTagPlan(plan []resolvedTag, tags map[string]string) []metrix.Label {
	var out []metrix.Label
	for _, rt := range plan {
		if v, ok := tags[rt.awsKey]; ok {
			out = append(out, metrix.Label{Key: rt.label, Value: v})
		}
	}
	return out
}

// tagLabelsFor returns the cached, non-identity tag labels for one discovered
// instance, or nil when tags are off, the profile has no join, or none are cached.
func (c *Collector) tagLabelsFor(account, region string, prof cwprofiles.ResolvedProfile, values []string) []metrix.Label {
	if len(c.tags.labels) == 0 {
		return nil
	}
	tj, ok := tagJoins[prof.Name]
	if !ok {
		return nil
	}
	jk, ok := tj.instanceJoinKey(prof.Config.DimensionNames(), values)
	if !ok {
		return nil
	}
	return c.tags.labels[tagCacheKey{account: account, region: region, profile: prof.Name, joinKey: jk}]
}
