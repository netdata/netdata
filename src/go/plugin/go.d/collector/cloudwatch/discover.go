// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// recentlyActiveMaxPeriod is the upper bound (seconds) for using ListMetrics
// RecentlyActive=PT3H. PT3H is the only value CloudWatch accepts and it hides
// metrics whose last datapoint is older than ~3h, so it MUST NOT be used for
// profiles with longer periods (e.g. S3 daily), or they would vanish ~21h/day.
const recentlyActiveMaxPeriod = 3 * 60 * 60

// instanceKeySep separates dimension values in an instance dedup key. It is the
// ASCII unit separator, which cannot appear in CloudWatch dimension values.
const instanceKeySep = "\x1f"

// discoveredInstance is one instance of a profile: the CloudWatch dimension
// values for the profile's exact instance dimension-set, ordered to match
// Profile.DimensionNames().
type discoveredInstance struct {
	DimensionValues []string
}

// profileUsesRecentlyActive reports whether a profile's ListMetrics call should
// set RecentlyActive=PT3H: only when enabled by config AND every metric's
// effective period is within the PT3H window.
func profileUsesRecentlyActive(prof cwprofiles.Profile, enabled bool) bool {
	return enabled && prof.MaxEffectivePeriod() <= recentlyActiveMaxPeriod
}

// discoverProfileGroup scans one namespace once and applies each participating
// profile's exact dimension matcher while streaming pages.
func discoverProfileGroup(ctx context.Context, client cloudwatchClient, group discoveryGroup) (map[string][]discoveredInstance, error) {
	profiles := group.Profiles
	if len(profiles) == 0 {
		return nil, nil
	}
	type matcher struct {
		profile cwprofiles.ResolvedProfile
		dims    []string
		seen    map[string]struct{}
	}
	matchers := make([]matcher, len(profiles))
	for i, profile := range profiles {
		matchers[i] = matcher{profile: profile, dims: profile.Config.DimensionNames(), seen: make(map[string]struct{})}
	}

	instances := make(map[string][]discoveredInstance, len(profiles))
	var nextToken *string
	for {
		in := &cloudwatch.ListMetricsInput{Namespace: aws.String(group.Namespace), NextToken: nextToken}
		if group.RecentlyActive {
			in.RecentlyActive = cwtypes.RecentlyActivePt3h
		}
		out, err := client.ListMetrics(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, metric := range out.Metrics {
			for i := range matchers {
				m := &matchers[i]
				values, ok := matchInstanceDimensions(metric.Dimensions, m.dims)
				if !ok || !constantDimensionsHold(m.profile.Config.Instance.Dimensions, values) {
					continue
				}
				key := strings.Join(values, instanceKeySep)
				if _, ok := m.seen[key]; ok {
					continue
				}
				m.seen[key] = struct{}{}
				instances[m.profile.Name] = append(instances[m.profile.Name], discoveredInstance{DimensionValues: values})
			}
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		nextToken = out.NextToken
	}
	return instances, nil
}

// matchInstanceDimensions returns the metric's dimension values ordered by
// dimNames, but only if the metric's dimension-NAME set EXACTLY equals dimNames
// (same cardinality, same names). A metric with extra or missing dimensions does
// not match, so e.g. an ALB metric under {LoadBalancer,TargetGroup} is rejected
// by a {LoadBalancer}-only profile.
func matchInstanceDimensions(dims []cwtypes.Dimension, dimNames []string) ([]string, bool) {
	if len(dims) != len(dimNames) {
		return nil, false
	}

	byName := make(map[string]string, len(dims))
	for _, d := range dims {
		if d.Name == nil || d.Value == nil {
			return nil, false
		}
		byName[*d.Name] = *d.Value
	}
	if len(byName) != len(dimNames) {
		return nil, false // duplicate dimension names in the metric
	}

	values := make([]string, len(dimNames))
	for i, name := range dimNames {
		v, ok := byName[name]
		if !ok {
			return nil, false
		}
		values[i] = v
	}
	return values, true
}

// constantDimensionsHold reports whether every match-and-query-only (constant)
// dimension in the profile has its pinned value in this metric's dimension values
// (ordered to match the profile's declared dimensions). A mismatch fails the match
// closed: a constant dimension is not emitted as a label, so admitting a differing
// value would collapse distinct instances onto one unlabeled series.
func constantDimensionsHold(dims []cwprofiles.InstanceDimension, values []string) bool {
	for i, d := range dims {
		if d.Constant != nil && values[i] != *d.Constant {
			return false
		}
	}
	return true
}

// discoveryKey identifies a discovered instance set by target, profile, and region.
type discoveryKey struct {
	Target  string
	Profile string
	Region  string
}

type discoveryGroup struct {
	Target         string
	Region         string
	Namespace      string
	RecentlyActive bool
	Profiles       []cwprofiles.ResolvedProfile
}

// discoverySnapshot is the cached result of one discovery pass across all
// candidate profiles and regions.
type discoverySnapshot struct {
	Instances map[discoveryKey][]discoveredInstance
	FetchedAt time.Time
	ExpiresAt time.Time
}

// expired reports whether the snapshot must be refreshed at now (never fetched,
// or past its TTL).
func (s discoverySnapshot) expired(now time.Time) bool {
	return s.FetchedAt.IsZero() || !now.Before(s.ExpiresAt)
}

// totalInstances is the instance count across all (account, profile, region) keys.
func (s discoverySnapshot) totalInstances() int {
	n := 0
	for _, insts := range s.Instances {
		n += len(insts)
	}
	return n
}

// discoveryGroupResult is one shared ListMetrics scan. Keeping failures at the
// group boundary prevents one upstream error from expanding into one warning per
// participating profile.
type discoveryGroupResult struct {
	Group     discoveryGroup
	Instances map[string][]discoveredInstance
	Err       error
}

// discoverAll runs one ListMetrics scan per compatible target/region/namespace/
// RecentlyActive group and applies all grouped profiles to that streamed response.
func discoverAll(
	ctx context.Context,
	newClient func(target, region string) (cloudwatchClient, error),
	groups []discoveryGroup,
	maxConcurrency int,
	timeout time.Duration,
) []discoveryGroupResult {
	results := make([]discoveryGroupResult, len(groups))
	p := pool.New().WithMaxGoroutines(max(1, maxConcurrency))
	for i, group := range groups {
		p.Go(func() {
			result := discoveryGroupResult{Group: group}
			client, err := newClient(group.Target, group.Region)
			if err == nil {
				cctx, cancel := withTimeout(ctx, timeout)
				defer cancel()
				result.Instances, err = discoverProfileGroup(cctx, client, group)
			}
			result.Err = err
			results[i] = result
		})
	}
	p.Wait()
	return results
}

// buildDiscoverySnapshot assembles a snapshot from per-target results. Targets
// that errored are returned as a separate error list (the caller logs them and
// applies fail-soft); only non-empty successful targets populate the snapshot.
func buildDiscoverySnapshot(results []discoveryGroupResult, prev map[discoveryKey][]discoveredInstance, now time.Time, refreshEvery int) (discoverySnapshot, int) {
	snap := discoverySnapshot{
		Instances: make(map[discoveryKey][]discoveredInstance),
		FetchedAt: now,
		ExpiresAt: now.Add(time.Duration(refreshEvery) * time.Second),
	}

	failures := 0
	for _, result := range results {
		if result.Err != nil {
			failures++
			for _, profile := range result.Group.Profiles {
				key := discoveryKey{Target: result.Group.Target, Profile: profile.Name, Region: result.Group.Region}
				if insts, ok := prev[key]; ok && len(insts) > 0 {
					snap.Instances[key] = insts
				}
			}
			continue
		}
		for _, profile := range result.Group.Profiles {
			key := discoveryKey{Target: result.Group.Target, Profile: profile.Name, Region: result.Group.Region}
			if instances := result.Instances[profile.Name]; len(instances) > 0 {
				snap.Instances[key] = instances
			}
		}
	}
	return snap, failures
}

// highInstanceCountWarn is the discovered-instance count at or above which the
// collector logs a cost-visibility warning. Collection is never truncated.
const highInstanceCountWarn = 1000

func (c *Collector) loadCatalog() (cwprofiles.Catalog, error) {
	if c.newCatalog != nil {
		return c.newCatalog()
	}
	return cwprofiles.DefaultCatalog()
}

// markDiscoveryStale forces the next refreshDiscovery to re-run (without treating it
// as a first pass), so an account resolved after the current snapshot was already
// fetched is discovered on the very next cycle instead of waiting out
// discovery.refresh_every. ensureTargets runs before refreshDiscovery in collect(),
// so a same-cycle resolution is picked up the same cycle.
func (c *Collector) markDiscoveryStale() {
	c.discovery.ExpiresAt = time.Time{}
}

// refreshDiscovery refreshes the discovery snapshot when its TTL has expired.
// Per-target failures are logged and tolerated; a total failure keeps the
// previous snapshot, or errors on the very first pass when there is none.
func (c *Collector) refreshDiscovery(ctx context.Context) error {
	now := c.now()
	if !c.discovery.expired(now) {
		return nil
	}

	newClient := func(target, region string) (cloudwatchClient, error) {
		return c.clients.forTargetRegion(ctx, target, region)
	}
	results := discoverAll(ctx, newClient, c.discoveryGroups(), apiConcurrency, c.Timeout.Duration())
	// If the parent context was canceled or timed out during the fan-out, abort before
	// committing: buildDiscoverySnapshot would otherwise carry forward instances (or
	// accept a partial first snapshot) and advance the TTL, so the next cycle would skip
	// discovery. A per-call GetMetricData/ListMetrics timeout uses a derived context and
	// does not trip this, so it stays fail-soft.
	if err := ctx.Err(); err != nil {
		return err
	}
	// Failed targets carry forward their previous instances, so a transient
	// per-region/namespace failure does not drop series (spec: keep last snapshot).
	snap, failedGroups := buildDiscoverySnapshot(results, c.discovery.Instances, now, c.Discovery.RefreshEvery)

	var failures []operationFailure
	for _, result := range results {
		if result.Err != nil {
			failures = append(failures, operationFailure{
				Target: result.Group.Target, Region: result.Group.Region,
				Scope: fmt.Sprintf("namespace %q", result.Group.Namespace), Err: result.Err,
			})
		}
	}
	c.warnOperationFailures(logKeyDiscoveryTargetFailed, "discovery", " (using last-known instances)", failures)

	// Only a first-ever pass where EVERY target errored is fatal. An empty but
	// successful target (a resource-free account/region/profile) is not a failure —
	// with shared regions across many accounts, empty successes are expected — and
	// any carried-forward snapshot keeps the collector running.
	if c.discovery.FetchedAt.IsZero() && len(results) > 0 && failedGroups == len(results) {
		return fmt.Errorf("CloudWatch discovery failed for all %d target/region/namespace groups", len(results))
	}

	c.discovery = snap
	c.invalidateQueryPlan()
	c.logDiscovery(snap)

	if n := snap.totalInstances(); n >= highInstanceCountWarn {
		c.Limit(logKeyHighInstanceCount, 1, recurringLogEvery).
			Warningf("CloudWatch discovered %d instances; this scales GetMetricData cost — narrow collection rules if this is unexpected", n)
	}
	return nil
}

// logDiscovery reports the discovered-resources summary: at Info when it changes
// (first discovery, or a per-service count change) so operators can see what the
// collector found, and the full per-(account, profile, region) breakdown at Debug every refresh.
func (c *Collector) logDiscovery(snap discoverySnapshot) {
	byProfile := make(map[string]int)
	for k, insts := range snap.Instances {
		byProfile[k.Profile] += len(insts)
	}
	names := make([]string, 0, len(byProfile))
	for name := range byProfile {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for i, name := range names {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s=%d", name, byProfile[name])
	}
	summary := b.String()

	c.Debugf("CloudWatch discovery: %d instance(s) across %d (target, profile, region) scope(s): %s",
		snap.totalInstances(), len(snap.Instances), summary)

	if sig := fmt.Sprintf("%d|%s", snap.totalInstances(), summary); sig != c.discoverySig {
		c.discoverySig = sig
		c.Infof("CloudWatch discovered %d instance(s): %s", snap.totalInstances(), summary)
	}
}

func (c *Collector) discoveryGroups() []discoveryGroup {
	if c.plan == nil {
		return nil
	}

	index := make(map[string]int)
	var groups []discoveryGroup
	for _, scope := range c.plan.Scopes {
		if _, ok := c.resolvedByRef[scope.Target.Name]; !ok {
			continue
		}
		recent := profileUsesRecentlyActive(scope.Profile.Config, c.recentlyActiveOnly())
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%t", scope.Target.Name, scope.Region, scope.Profile.Config.Namespace, recent)
		if i, ok := index[key]; ok {
			// Compiled scopes are unique by target/profile/region, so a profile can
			// occur only once in a compatible discovery group.
			groups[i].Profiles = append(groups[i].Profiles, scope.Profile)
			continue
		}
		index[key] = len(groups)
		groups = append(groups, discoveryGroup{
			Target: scope.Target.Name, Region: scope.Region, Namespace: scope.Profile.Config.Namespace,
			RecentlyActive: recent, Profiles: []cwprofiles.ResolvedProfile{scope.Profile},
		})
	}
	return groups
}
