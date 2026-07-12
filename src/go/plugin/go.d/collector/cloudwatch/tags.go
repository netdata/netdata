// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
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

func (m tagMembership) add(membershipID int, joinKey string) {
	if m[membershipID] == nil {
		m[membershipID] = make(map[string]struct{})
	}
	m[membershipID][joinKey] = struct{}{}
}

// tagGroupSnapshot is the independently cached result of one RGTA fetch group.
// A failed refresh keeps the last complete result but marks its membership
// identities unknown and expires only this group for retry.
type tagGroupSnapshot struct {
	membershipIDs   []int
	members         tagMembership
	labels          map[tagCacheKey][]metrix.Label
	confirmedLabels map[tagCacheKey]struct{}
	lastSuccess     time.Time
	expiresAt       time.Time
	unknown         bool
}

// share copies only the scope index; the per-scope member sets are immutable once
// a fetch result is installed and can be shared by the effective snapshot.
func (m tagMembership) share(other tagMembership) {
	maps.Copy(m, other)
}

func (m tagMembership) equal(other tagMembership) bool {
	if len(m) != len(other) {
		return false
	}
	for membershipID, joinKeys := range m {
		otherJoinKeys, ok := other[membershipID]
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
// labels. A failed membership is unknown: last-known members remain selected,
// while other candidates reserve the affected selected series until the next retry.
type tagSnapshot struct {
	labels    map[tagCacheKey][]metrix.Label
	members   tagMembership
	unknown   map[int]struct{}
	groups    map[tagFetchKey]tagGroupSnapshot
	fetchedAt time.Time
	expiresAt time.Time
}

func (s tagSnapshot) expired(now time.Time) bool {
	return s.fetchedAt.IsZero() || !now.Before(s.expiresAt)
}

func (s tagSnapshot) membershipUnknown(membershipID int) bool {
	if s.fetchedAt.IsZero() {
		return true
	}
	_, ok := s.unknown[membershipID]
	return ok
}

func (s tagSnapshot) membershipSelected(membershipID int, joinKey string) bool {
	_, ok := s.members[membershipID][joinKey]
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
	for key, state := range c.tags.groups {
		state.expiresAt = time.Time{}
		c.tags.groups[key] = state
	}
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

// computeTagLabelPlans resolves the per-profile tag-label plan once (the selection and
// profiles are fixed per job) and logs each skip once. Profiles with no registered
// ARN join or an empty resolved plan are omitted, so they carry no tags.
func (c *Collector) computeTagLabelPlans() {
	plans := make(map[string][]resolvedTag)
	if len(c.Labels.ResourceTags) == 0 {
		c.tagLabelPlans = plans
		return
	}
	for _, p := range c.plan.Profiles {
		if c.plan.TagJoins[p.Name] == nil {
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
	states, due := selectDueTagGroups(c.currentTagFetchPlan(), c.tags.groups, now)
	results := c.fetchTagGroups(ctx, due)

	// If the parent collect context was canceled or timed out during the fan-out, do
	// NOT commit next or advance the TTL: that would suppress tag retries until the next
	// TTL expiry (and risk creating charts untagged). Keeping the previous snapshot (its
	// TTL is already expired, which is why we ran) makes the next cycle retry. Per-call
	// timeouts use derived contexts and do not trip this, so they stay fail-soft.
	if ctx.Err() != nil {
		return
	}

	failures := applyTagFetchResults(states, c.tags.groups, results, now, ttl)
	next := mergeTagGroupSnapshots(states, now, now.Add(ttl))
	c.warnOperationFailures(logKeyTagRefreshFailed, "resource tag lookup", " (retaining fail-closed membership and last-known labels)", failures)

	changed := !next.sameEffective(c.tags)
	c.tags = next
	if changed {
		c.invalidateQueryPlan()
	}
}

func selectDueTagGroups(groups []tagFetchGroup, previous map[tagFetchKey]tagGroupSnapshot, now time.Time) (map[tagFetchKey]tagGroupSnapshot, []tagFetchGroup) {
	states := make(map[tagFetchKey]tagGroupSnapshot, len(groups))
	var due []tagFetchGroup
	for _, group := range groups {
		state, ok := previous[group.key]
		if ok && now.Before(state.expiresAt) {
			states[group.key] = state
			continue
		}
		due = append(due, group)
	}
	return states, due
}

func (c *Collector) fetchTagGroups(ctx context.Context, groups []tagFetchGroup) []tagFetchResult {
	results := make([]tagFetchResult, len(groups))
	p := pool.New().WithMaxGoroutines(max(1, apiConcurrency))
	for i := range groups {
		p.Go(func() {
			results[i] = c.fetchTagGroup(ctx, groups[i])
		})
	}
	p.Wait()
	return results
}

func applyTagFetchResults(states, previous map[tagFetchKey]tagGroupSnapshot, results []tagFetchResult, now time.Time, ttl time.Duration) []operationFailure {
	var failures []operationFailure
	for _, result := range results {
		if result.err == nil {
			states[result.group.key] = tagGroupSnapshot{
				membershipIDs: tagGroupMembershipIDs(result.group), members: result.members, labels: result.labels,
				confirmedLabels: result.confirmedLabels,
				lastSuccess:     now,
				expiresAt:       now.Add(ttl),
			}
			continue
		}

		state := previous[result.group.key]
		state.membershipIDs = tagGroupMembershipIDs(result.group)
		state.unknown = true
		state.expiresAt = time.Time{}
		if state.members == nil {
			state.members = make(tagMembership)
		}
		if state.labels == nil {
			state.labels = make(map[tagCacheKey][]metrix.Label)
		}
		if state.confirmedLabels == nil {
			state.confirmedLabels = make(map[tagCacheKey]struct{})
		}
		states[result.group.key] = state
		failures = append(failures, operationFailure{
			Target: result.group.key.target,
			Region: result.group.key.region,
			Scope:  fmt.Sprintf("profiles=%d", len(result.group.joins)),
			Err:    result.err,
		})
	}
	return failures
}

func tagGroupMembershipIDs(group tagFetchGroup) []int {
	var membershipIDs []int
	for _, ids := range group.membershipIDsByProfile {
		membershipIDs = append(membershipIDs, ids...)
	}
	sort.Ints(membershipIDs)
	return membershipIDs
}

func mergeTagGroupSnapshots(states map[tagFetchKey]tagGroupSnapshot, fetchedAt, emptyExpiresAt time.Time) tagSnapshot {
	labelCapacity := 0
	membershipCapacity := 0
	unknownCapacity := 0
	for _, state := range states {
		labelCapacity = max(labelCapacity, len(state.confirmedLabels))
		membershipCapacity += len(state.members)
		if state.unknown {
			unknownCapacity += len(state.membershipIDs)
		}
	}
	next := tagSnapshot{
		labels: make(map[tagCacheKey][]metrix.Label, labelCapacity), members: make(tagMembership, membershipCapacity),
		unknown: make(map[int]struct{}, unknownCapacity), groups: states,
		fetchedAt: fetchedAt, expiresAt: emptyExpiresAt,
	}
	keys := make([]tagFetchKey, 0, len(states))
	for key := range states {
		keys = append(keys, key)
	}
	if len(keys) > 1 {
		sort.Slice(keys, func(i, j int) bool {
			a, b := states[keys[i]], states[keys[j]]
			if !a.lastSuccess.Equal(b.lastSuccess) {
				return a.lastSuccess.Before(b.lastSuccess)
			}
			return lessTagFetchKey(keys[i], keys[j])
		})
	}
	for _, key := range keys {
		state := states[key]
		next.members.share(state.members)
		if state.unknown {
			for _, membershipID := range state.membershipIDs {
				next.unknown[membershipID] = struct{}{}
			}
		}
		if state.expiresAt.IsZero() || state.expiresAt.Before(next.expiresAt) {
			next.expiresAt = state.expiresAt
		}
		for labelKey := range state.confirmedLabels {
			delete(next.labels, labelKey)
			if labels, ok := state.labels[labelKey]; ok {
				next.labels[labelKey] = labels
			}
		}
	}
	return next
}

func selectedTagMap(tags []rgtatypes.Tag, selected map[string]struct{}) map[string]string {
	out := make(map[string]string, min(len(tags), len(selected)))
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
func (c *Collector) tagLabelsFor(target, account, region string, prof cwprofiles.ResolvedProfile, join *tagJoin, values []string) []metrix.Label {
	if len(c.tags.labels) == 0 {
		return nil
	}
	if join == nil {
		return nil
	}
	jk, ok := join.instanceJoinKey(prof.Config.DimensionNames(), values)
	if !ok {
		return nil
	}
	return c.tags.labels[tagCacheKey{target: target, account: account, region: region, profile: prof.Name, joinKey: jk}]
}
