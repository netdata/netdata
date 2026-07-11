// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"slices"
	"time"

	rgtatypes "github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi/types"
	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// tagCacheKey identifies resolved tag labels by target execution identity, account
// attribution, region, profile, and the profile's ARN-projectable join key.
type tagCacheKey struct {
	target  string
	account string
	region  string
	profile string
	joinKey string
}

type tagMembership map[int]map[string]struct{}

func (m tagMembership) add(scopeID int, joinKey string) {
	if m[scopeID] == nil {
		m[scopeID] = make(map[string]struct{})
	}
	m[scopeID][joinKey] = struct{}{}
}

func (m tagMembership) merge(other tagMembership) {
	for scopeID, joinKeys := range other {
		if m[scopeID] == nil && len(joinKeys) > 0 {
			m[scopeID] = make(map[string]struct{}, len(joinKeys))
		}
		for joinKey := range joinKeys {
			m[scopeID][joinKey] = struct{}{}
		}
	}
}

func (m tagMembership) equal(other tagMembership) bool {
	if len(m) != len(other) {
		return false
	}
	for scopeID, joinKeys := range m {
		otherJoinKeys, ok := other[scopeID]
		if !ok || len(joinKeys) != len(otherJoinKeys) {
			return false
		}
		for joinKey := range joinKeys {
			if _, ok := otherJoinKeys[joinKey]; !ok {
				return false
			}
		}
	}
	return true
}

// tagSnapshot is one immutable view of tag-filter membership and emitted resource
// labels. A failed scope is unknown: last-known members remain selected, while all
// other candidates are reserved from lower-priority rules until the next retry.
type tagSnapshot struct {
	labels    map[tagCacheKey][]metrix.Label
	members   tagMembership
	unknown   map[int]struct{}
	fetchedAt time.Time
	expiresAt time.Time
}

func (s tagSnapshot) expired(now time.Time) bool {
	return s.fetchedAt.IsZero() || !now.Before(s.expiresAt)
}

func (s tagSnapshot) scopeUnknown(scopeID int) bool {
	if s.fetchedAt.IsZero() {
		return true
	}
	_, ok := s.unknown[scopeID]
	return ok
}

func (s tagSnapshot) scopeSelected(scopeID int, joinKey string) bool {
	_, ok := s.members[scopeID][joinKey]
	return ok
}

func (s tagSnapshot) sameEffective(other tagSnapshot) bool {
	if s.fetchedAt.IsZero() != other.fetchedAt.IsZero() {
		return false
	}
	if !s.members.equal(other.members) || len(s.unknown) != len(other.unknown) || len(s.labels) != len(other.labels) {
		return false
	}
	for key := range s.unknown {
		if _, ok := other.unknown[key]; !ok {
			return false
		}
	}
	for key, labels := range s.labels {
		otherLabels, ok := other.labels[key]
		if !ok || !slices.Equal(labels, otherLabels) {
			return false
		}
	}
	return true
}

// markTagsStale forces the next refreshTags to re-fetch even within the TTL.
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

// computeTagLabelPlans resolves the per-profile tag-label plan once (the allowlist and
// profiles are fixed per job) and logs each skip once. Profiles with no registered
// ARN join or an empty resolved plan are omitted, so they carry no tags.
func (c *Collector) computeTagLabelPlans() {
	plans := make(map[string][]resolvedTag)
	if len(c.Labels.ResourceTags) == 0 {
		c.tagLabelPlans = plans
		return
	}
	for _, p := range c.plan.Profiles {
		if _, ok := tagJoins[p.Name]; !ok {
			continue
		}
		plan, warnings := resolveTagPlan(c.Labels.ResourceTags, profileDimLabels(p.Config))
		for _, w := range warnings {
			c.Warningf("CloudWatch tags (profile %q): %s", p.Name, w)
		}
		if len(plan) > 0 {
			plans[p.Name] = plan
		}
	}
	c.tagLabelPlans = plans
}

func (c *Collector) hasTagWork() bool {
	if len(c.Labels.ResourceTags) > 0 {
		return true
	}
	for _, scope := range c.plan.Scopes {
		if scope.hasTagFilter() {
			return true
		}
	}
	return false
}

// refreshTags replaces the tag snapshot when its TTL expires. Filter failures are
// fail-closed: first-run membership is unknown, and later failures retain last-known
// members. Failed groups retry on the next collect instead of advancing the TTL.
func (c *Collector) refreshTags(ctx context.Context) {
	if !c.hasTagWork() {
		return
	}
	if c.tagLabelPlans == nil {
		c.computeTagLabelPlans()
	}
	now := c.now()
	if !c.tags.expired(now) {
		return
	}

	ttl := time.Duration(c.Discovery.RefreshEvery) * time.Second

	next := tagSnapshot{
		labels:    make(map[tagCacheKey][]metrix.Label),
		members:   make(tagMembership),
		unknown:   make(map[int]struct{}),
		fetchedAt: now,
		expiresAt: now.Add(ttl),
	}

	groups := c.tagFetchGroups()
	results := make([]tagFetchResult, len(groups))
	p := pool.New().WithMaxGoroutines(max(1, apiConcurrency))
	for i := range groups {
		i := i
		p.Go(func() {
			results[i] = c.fetchTagGroup(ctx, groups[i])
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

	var failures []operationFailure
	confirmedLabels := make(map[tagCacheKey]struct{})
	for _, result := range results {
		if result.err != nil {
			carryForwardTagGroup(&next, c.tags, result.group, confirmedLabels)
			failures = append(failures, operationFailure{
				Target: result.group.key.target,
				Region: result.group.key.region,
				Scope:  fmt.Sprintf("profiles=%d", len(result.group.profiles)),
				Err:    result.err,
			})
			continue
		}
		next.members.merge(result.members)
		for key := range result.confirmedLabels {
			confirmedLabels[key] = struct{}{}
			delete(next.labels, key)
		}
		for key, labels := range result.labels {
			next.labels[key] = labels
		}
	}
	if len(failures) > 0 {
		next.expiresAt = time.Time{}
	}
	c.warnOperationFailures(logKeyTagRefreshFailed, "resource tag lookup", " (retaining fail-closed membership and last-known labels)", failures)

	changed := !next.sameEffective(c.tags)
	c.tags = next
	if changed {
		c.invalidateQueryPlan()
	}
}

func carryForwardTagGroup(dst *tagSnapshot, previous tagSnapshot, group tagFetchGroup, confirmedLabels map[tagCacheKey]struct{}) {
	for _, scopeIDs := range group.scopeIDsByProfile {
		for _, scopeID := range scopeIDs {
			dst.unknown[scopeID] = struct{}{}
		}
	}
	for _, scopeIDs := range group.scopeIDsByProfile {
		for _, scopeID := range scopeIDs {
			joinKeys := previous.members[scopeID]
			if len(joinKeys) == 0 {
				continue
			}
			copied := make(map[string]struct{}, len(joinKeys))
			for joinKey := range joinKeys {
				copied[joinKey] = struct{}{}
			}
			dst.members[scopeID] = copied
		}
	}
	if !group.hasLabels {
		return
	}
	for profile, candidates := range group.candidatesByProfile {
		for joinKey := range candidates {
			key := tagCacheKey{
				target: group.key.target, account: group.account, region: group.key.region,
				profile: profile, joinKey: joinKey,
			}
			if _, confirmed := confirmedLabels[key]; confirmed {
				continue
			}
			if labels, ok := previous.labels[key]; ok {
				dst.labels[key] = labels
			}
		}
	}
}

func selectedTagMap(tags []rgtatypes.Tag, selected map[string]struct{}) map[string]string {
	out := make(map[string]string, len(selected))
	for _, t := range tags {
		if t.Key == nil || t.Value == nil {
			continue
		}
		if _, ok := selected[*t.Key]; ok {
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
func (c *Collector) tagLabelsFor(target, account, region string, prof cwprofiles.ResolvedProfile, values []string) []metrix.Label {
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
	return c.tags.labels[tagCacheKey{target: target, account: account, region: region, profile: prof.Name, joinKey: jk}]
}
