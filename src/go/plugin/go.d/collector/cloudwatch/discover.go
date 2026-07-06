// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"errors"
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

// discoverInstances lists every metric in the profile's namespace once (MetricName
// omitted) and returns the distinct instances whose dimension-NAME set EXACTLY
// matches the profile's instance dimensions. This collapses CloudWatch's
// multi-granularity fan-out (a metric published under several dimension sets) to
// the chosen granularity, and dedups instances shared across metrics.
func discoverInstances(ctx context.Context, client cloudwatchClient, prof cwprofiles.Profile, useRecentlyActive bool) ([]discoveredInstance, error) {
	dimNames := prof.DimensionNames()

	seen := make(map[string]struct{})
	var instances []discoveredInstance
	var nextToken *string

	for {
		in := &cloudwatch.ListMetricsInput{Namespace: aws.String(prof.Namespace), NextToken: nextToken}
		if useRecentlyActive {
			in.RecentlyActive = cwtypes.RecentlyActivePt3h
		}

		out, err := client.ListMetrics(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, m := range out.Metrics {
			values, ok := matchInstanceDimensions(m.Dimensions, dimNames)
			if !ok {
				continue
			}
			if !constantDimensionsHold(prof.Instance.Dimensions, values) {
				continue // fail-closed: a pinned constant dimension had a different value
			}
			key := strings.Join(values, instanceKeySep)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			instances = append(instances, discoveredInstance{DimensionValues: values})
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

// discoveryKey identifies a discovered instance set by profile and region.
type discoveryKey struct {
	Profile string
	Region  string
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

// totalInstances is the instance count across all (profile, region) keys.
func (s discoverySnapshot) totalInstances() int {
	n := 0
	for _, insts := range s.Instances {
		n += len(insts)
	}
	return n
}

// discoveryResult is the outcome of discovering one (profile, region) target.
type discoveryResult struct {
	Key       discoveryKey
	Instances []discoveredInstance
	Err       error
}

// discoverAll runs ListMetrics-based discovery for every (profile, region)
// target concurrently (bounded by maxConcurrency) and returns one result per
// target. Failures are per-target (carried in result.Err); the caller decides
// fail-soft handling. One CloudWatch client is built per region and shared
// across that region's profiles.
func discoverAll(
	ctx context.Context,
	newClient func(region string) (cloudwatchClient, error),
	profiles []cwprofiles.ResolvedProfile,
	regions []string,
	recentlyActiveOnly bool,
	maxConcurrency int,
	timeout time.Duration,
) []discoveryResult {
	clients := make(map[string]cloudwatchClient, len(regions))
	clientErrs := make(map[string]error, len(regions))
	for _, region := range regions {
		c, err := newClient(region)
		if err != nil {
			clientErrs[region] = err
			continue
		}
		clients[region] = c
	}

	type target struct {
		profile cwprofiles.ResolvedProfile
		region  string
	}
	var targets []target
	for _, p := range profiles {
		for _, r := range regions {
			targets = append(targets, target{profile: p, region: r})
		}
	}

	results := make([]discoveryResult, len(targets))
	p := pool.New().WithMaxGoroutines(max(1, maxConcurrency))

	for i, t := range targets {
		key := discoveryKey{Profile: t.profile.Name, Region: t.region}
		if err := clientErrs[t.region]; err != nil {
			results[i] = discoveryResult{Key: key, Err: fmt.Errorf("build client for region %q: %w", t.region, err)}
			continue
		}

		prof := t.profile.Config
		client := clients[t.region]
		p.Go(func() {
			cctx, cancel := withTimeout(ctx, timeout)
			defer cancel()

			useRecentlyActive := profileUsesRecentlyActive(prof, recentlyActiveOnly)
			insts, err := discoverInstances(cctx, client, prof, useRecentlyActive)
			// Per-target errors are carried in results[i] for fail-soft handling.
			results[i] = discoveryResult{Key: key, Instances: insts, Err: err}
		})
	}

	p.Wait()
	return results
}

// buildDiscoverySnapshot assembles a snapshot from per-target results. Targets
// that errored are returned as a separate error list (the caller logs them and
// applies fail-soft); only non-empty successful targets populate the snapshot.
func buildDiscoverySnapshot(results []discoveryResult, prev map[discoveryKey][]discoveredInstance, now time.Time, refreshEvery int) (discoverySnapshot, []error) {
	snap := discoverySnapshot{
		Instances: make(map[discoveryKey][]discoveredInstance),
		FetchedAt: now,
		ExpiresAt: now.Add(time.Duration(refreshEvery) * time.Second),
	}

	var errs []error
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, fmt.Errorf("discovery %s/%s: %w", r.Key.Profile, r.Key.Region, r.Err))
			if insts, ok := prev[r.Key]; ok && len(insts) > 0 {
				snap.Instances[r.Key] = insts // fail-soft: keep this target's last-known instances
			}
			continue
		}
		if len(r.Instances) > 0 {
			snap.Instances[r.Key] = r.Instances
		}
	}
	return snap, errs
}

// highInstanceCountWarn is the discovered-instance count at or above which the
// collector logs a cost-visibility warning. Collection is never truncated.
const highInstanceCountWarn = 1000

// ensureProfiles resolves the candidate profiles once, per profiles.mode.
func (c *Collector) ensureProfiles() error {
	if c.profiles != nil {
		return nil
	}

	catalog, err := c.loadCatalog()
	if err != nil {
		return fmt.Errorf("load CloudWatch profiles: %w", err)
	}
	profiles, err := c.selectProfiles(catalog)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return errors.New("no CloudWatch profiles selected")
	}

	c.profiles = profiles

	tpl, err := buildChartTemplate(c.profiles)
	if err != nil {
		return fmt.Errorf("build CloudWatch chart template: %w", err)
	}
	c.chartTemplateYAML = tpl

	names := make([]string, len(c.profiles))
	for i, p := range c.profiles {
		names[i] = p.Name
	}
	c.Infof("CloudWatch: monitoring %d service profile(s) [%s] across region(s) %v (profiles.mode=%s)",
		len(c.profiles), strings.Join(names, ", "), c.regions(), c.Profiles.Mode)
	c.Debugf("CloudWatch tuning: update_every=%ds, discovery.refresh_every=%ds, query_offset=%ds, recently_active_only=%v",
		c.UpdateEvery, c.Discovery.RefreshEvery, c.QueryOffset, c.recentlyActiveOnly())

	return nil
}

func (c *Collector) loadCatalog() (cwprofiles.Catalog, error) {
	if c.newCatalog != nil {
		return c.newCatalog()
	}
	return cwprofiles.DefaultCatalog()
}

func (c *Collector) selectProfiles(catalog cwprofiles.Catalog) ([]cwprofiles.ResolvedProfile, error) {
	switch strings.ToLower(strings.TrimSpace(c.Profiles.Mode)) {
	case profilesModeAuto:
		return enabledProfiles(catalog.AllProfiles()), nil
	case profilesModeCombined:
		// auto plus the default-disabled (deep-grain) profiles.
		return catalog.AllProfiles(), nil
	case profilesModeExact:
		var names []string
		if c.Profiles.ModeExact != nil {
			for _, e := range c.Profiles.ModeExact.Entries {
				names = append(names, e.Name)
			}
		}
		// Exact mode selects the named profiles by basename regardless of their
		// default-enabled/disabled flag, so a deep-grain profile can be picked by name.
		profiles := catalog.ProfilesByBaseNames(names)
		if len(profiles) == 0 {
			return nil, fmt.Errorf("no CloudWatch profiles match the configured names: %v", names)
		}
		return profiles, nil
	default:
		return nil, fmt.Errorf("unsupported profiles.mode %q", c.Profiles.Mode)
	}
}

// enabledProfiles drops profiles marked disabled — those are selected only by
// profiles.mode combined, by naming them in profiles.mode exact, or by a user-dir
// override that omits the flag.
func enabledProfiles(in []cwprofiles.ResolvedProfile) []cwprofiles.ResolvedProfile {
	out := make([]cwprofiles.ResolvedProfile, 0, len(in))
	for _, p := range in {
		if !p.Config.Disabled {
			out = append(out, p)
		}
	}
	return out
}

// refreshDiscovery refreshes the discovery snapshot when its TTL has expired.
// Per-target failures are logged and tolerated; a total failure keeps the
// previous snapshot, or errors on the very first pass when there is none.
func (c *Collector) refreshDiscovery(ctx context.Context) error {
	now := c.now()
	if !c.discovery.expired(now) {
		return nil
	}

	newClient := func(region string) (cloudwatchClient, error) {
		return c.clients.forRegion(ctx, region)
	}
	results := discoverAll(ctx, newClient, c.profiles, c.regions(), c.recentlyActiveOnly(), apiConcurrency, c.Timeout.Duration())
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
	snap, errs := buildDiscoverySnapshot(results, c.discovery.Instances, now, c.Discovery.RefreshEvery)

	// Per-target failures are tolerated (last-known instances are carried forward),
	// so throttle each target's warning: a persistently unreachable region must not
	// warn every cycle.
	for _, r := range results {
		if r.Err != nil {
			c.Limit(logKeyDiscoveryTargetFailed+":"+r.Key.Profile+"/"+r.Key.Region, 1, recurringLogEvery).
				Warningf("CloudWatch discovery %s/%s failed (using last-known instances): %v", r.Key.Profile, r.Key.Region, r.Err)
		}
	}

	// Only a first-ever pass with nothing discovered and errors is fatal; otherwise
	// the carried-forward snapshot keeps the collector running.
	if c.discovery.FetchedAt.IsZero() && snap.totalInstances() == 0 && len(errs) > 0 {
		return fmt.Errorf("CloudWatch discovery failed for all %d (namespace, region) targets", len(results))
	}

	c.discovery = snap
	c.logDiscovery(snap)

	if n := snap.totalInstances(); n >= highInstanceCountWarn {
		c.Limit(logKeyHighInstanceCount, 1, recurringLogEvery).
			Warningf("CloudWatch discovered %d instances; this scales GetMetricData cost — narrow 'regions'/'profiles' if this is unexpected", n)
	}
	return nil
}

// logDiscovery reports the discovered-resources summary: at Info when it changes
// (first discovery, or a per-service count change) so operators can see what the
// collector found, and the full per-(profile,region) breakdown at Debug every refresh.
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

	c.Debugf("CloudWatch discovery: %d instance(s) across %d (profile,region) target(s): %s",
		snap.totalInstances(), len(snap.Instances), summary)

	if sig := fmt.Sprintf("%d|%s", snap.totalInstances(), summary); sig != c.discoverySig {
		c.discoverySig = sig
		c.Infof("CloudWatch discovered %d instance(s): %s", snap.totalInstances(), summary)
	}
}
