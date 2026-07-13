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

// instanceKeySep separates dimension values in an instance dedup key. It is the
// ASCII unit separator, which cannot appear in CloudWatch dimension values.
const instanceKeySep = "\x1f"

// discoveredInstance is one instance of a profile: the CloudWatch dimension
// values for the profile's exact instance dimension-set, ordered to match
// Profile.DimensionNames().
type discoveredInstance struct {
	DimensionValues []string
}

// selectedSeriesUseRecentlyActive reports whether ListMetrics may safely use
// RecentlyActive=PT3H for the selected series.
func selectedSeriesUseRecentlyActive(series []compiledSeries, enabled bool) bool {
	if !enabled || len(series) == 0 {
		return false
	}
	for _, item := range series {
		if item.Policy.period == 0 || !item.Policy.useRecentlyActive() {
			return false
		}
	}
	return true
}

// discoveryGroupScanner owns the pagination and matching state for one group.
// discoverAll runs every scanner's first page before allowing any scanner to
// consume the shared continuation-operation budget.
type discoveryGroupScanner struct {
	group              discoveryGroup
	matchers           map[string][]*discoveryMatcher
	instances          map[string][]discoveredInstance
	nextToken          *string
	seenTokens         map[string]struct{}
	pages              int
	scannedMetrics     int
	candidateInstances int
	matcherEvaluations int
	done               bool
}

func newDiscoveryGroupScanner(group discoveryGroup) *discoveryGroupScanner {
	return &discoveryGroupScanner{
		group:      group,
		matchers:   newDiscoveryMatcherIndex(group.Profiles),
		instances:  make(map[string][]discoveredInstance, len(group.Profiles)),
		seenTokens: make(map[string]struct{}),
	}
}

func (s *discoveryGroupScanner) scanPage(ctx context.Context, client cloudwatchClient, budget *discoveryBudget) error {
	if s.done {
		return nil
	}
	if err := budget.reserveListMetricsOperation(); err != nil {
		return err
	}

	in := &cloudwatch.ListMetricsInput{Namespace: aws.String(s.group.Namespace), NextToken: s.nextToken}
	if s.group.RecentlyActive {
		in.RecentlyActive = cwtypes.RecentlyActivePt3h
	}
	out, err := client.ListMetrics(ctx, in)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	s.pages++
	s.scannedMetrics += len(out.Metrics)
	if s.scannedMetrics > maxScannedMetricsPerGroup {
		return safeCollectorErrorf("ListMetrics scanned more than %d metrics for one discovery group", maxScannedMetricsPerGroup)
	}
	if err := budget.reserveScannedMetrics(len(out.Metrics)); err != nil {
		return err
	}
	for _, metric := range out.Metrics {
		if err := ctx.Err(); err != nil {
			return err
		}
		signature, values, ok := canonicalMetricDimensions(metric.Dimensions)
		if !ok {
			continue
		}
		matching := s.matchers[signature]
		if len(matching) > maxDiscoveryMatcherEvaluationsPerGroup-s.matcherEvaluations {
			return safeCollectorErrorf(
				"ListMetrics requires more than %d residual profile matches for one discovery group",
				maxDiscoveryMatcherEvaluationsPerGroup,
			)
		}
		s.matcherEvaluations += len(matching)
		if err := budget.reserveMatcherEvaluations(len(matching)); err != nil {
			return err
		}
		for i, m := range matching {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			if !m.constantsHold(values) {
				continue
			}
			key := m.packedValues(values)
			if _, ok := m.seen[key]; ok {
				continue
			}
			if s.candidateInstances == maxCandidateInstancesPerGroup {
				return safeCollectorErrorf("ListMetrics found more than %d candidate instances for one discovery group", maxCandidateInstancesPerGroup)
			}
			if err := budget.reserveCandidate(retainedCandidateBytes(key, len(m.outputOrder))); err != nil {
				return err
			}
			s.candidateInstances++
			m.seen[key] = struct{}{}
			s.instances[m.profileName] = append(s.instances[m.profileName], discoveredInstance{DimensionValues: strings.Split(key, instanceKeySep)})
		}
	}

	if out.NextToken == nil || *out.NextToken == "" {
		s.done = true
		return nil
	}
	if s.pages == maxListMetricsPages {
		return safeCollectorErrorf("ListMetrics requires more than %d pages for one discovery group", maxListMetricsPages)
	}
	if _, ok := s.seenTokens[*out.NextToken]; ok {
		return safeCollectorError("ListMetrics returned a repeated pagination token")
	}
	s.seenTokens[*out.NextToken] = struct{}{}
	s.nextToken = out.NextToken
	return nil
}

type discoveryDimension struct {
	name  string
	value string
}

type discoveryConstant struct {
	index int
	value string
}

type discoveryMatcher struct {
	profileName string
	outputOrder []int
	constants   []discoveryConstant
	seen        map[string]struct{}
}

// newDiscoveryMatcherIndex groups profiles by their exact dimension-name set.
// ListMetrics dimensions are canonicalized once per metric, so profiles with a
// different shape are rejected by one map lookup instead of a full scan.
func newDiscoveryMatcherIndex(profiles []cwprofiles.ResolvedProfile) map[string][]*discoveryMatcher {
	index := make(map[string][]*discoveryMatcher)
	for _, profile := range profiles {
		dims := profile.Config.Instance.Dimensions
		names := make([]string, len(dims))
		for i, dim := range dims {
			names[i] = dim.Name
		}
		sort.Strings(names)

		m := &discoveryMatcher{
			profileName: profile.Name,
			outputOrder: make([]int, len(dims)),
			seen:        make(map[string]struct{}),
		}
		for i, dim := range dims {
			canonicalIndex := sort.SearchStrings(names, dim.Name)
			m.outputOrder[i] = canonicalIndex
			if dim.Constant != nil {
				m.constants = append(m.constants, discoveryConstant{index: canonicalIndex, value: *dim.Constant})
			}
		}
		signature := dimensionNameSignature(names)
		index[signature] = append(index[signature], m)
	}
	return index
}

// canonicalMetricDimensions returns a collision-safe signature and values in
// canonical name order. Nil components and repeated names fail closed.
func canonicalMetricDimensions(dims []cwtypes.Dimension) (string, []string, bool) {
	canonical := make([]discoveryDimension, len(dims))
	for i, dim := range dims {
		if dim.Name == nil || dim.Value == nil {
			return "", nil, false
		}
		canonical[i] = discoveryDimension{name: *dim.Name, value: *dim.Value}
	}
	sort.Slice(canonical, func(i, j int) bool { return canonical[i].name < canonical[j].name })

	names := make([]string, len(canonical))
	values := make([]string, len(canonical))
	for i, dim := range canonical {
		if i > 0 && canonical[i-1].name == dim.name {
			return "", nil, false
		}
		names[i] = dim.name
		values[i] = dim.value
	}
	return dimensionNameSignature(names), values, true
}

func dimensionNameSignature(names []string) string {
	var b strings.Builder
	for _, name := range names {
		writeLengthPrefixed(&b, name)
	}
	return b.String()
}

func (m *discoveryMatcher) constantsHold(canonicalValues []string) bool {
	for _, constant := range m.constants {
		if canonicalValues[constant.index] != constant.value {
			return false
		}
	}
	return true
}

func (m *discoveryMatcher) packedValues(canonicalValues []string) string {
	var size int
	for _, canonicalIndex := range m.outputOrder {
		size += len(canonicalValues[canonicalIndex])
	}
	size += max(0, len(m.outputOrder)-1) * len(instanceKeySep)
	var b strings.Builder
	b.Grow(size)
	for i, canonicalIndex := range m.outputOrder {
		if i > 0 {
			b.WriteString(instanceKeySep)
		}
		b.WriteString(canonicalValues[canonicalIndex])
	}
	return b.String()
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

// totalInstances is the instance count across all (target, profile, region) keys.
func (s discoverySnapshot) totalInstances() int {
	n := 0
	for _, insts := range s.Instances {
		n += len(insts)
	}
	return n
}

func (s discoverySnapshot) validateRetainedBounds() error {
	instances := 0
	retainedBytes := 0
	for _, discovered := range s.Instances {
		if len(discovered) > maxCandidateInstancesPerRefresh-instances {
			return safeCollectorErrorf(
				"CloudWatch discovery merged snapshot retains more than %d candidate instances",
				maxCandidateInstancesPerRefresh,
			)
		}
		instances += len(discovered)
		for _, instance := range discovered {
			weight := retainedDiscoveredInstanceBytes(instance)
			if weight > maxRetainedCandidateBytesPerRefresh-retainedBytes {
				return safeCollectorErrorf(
					"CloudWatch discovery merged snapshot retains more than %d MiB of candidate instance data",
					maxRetainedCandidateBytesPerRefresh>>20,
				)
			}
			retainedBytes += weight
		}
	}
	return nil
}

// discoveryGroupResult is one shared ListMetrics scan. Keeping failures at the
// group boundary prevents one upstream error from expanding into one warning per
// participating profile.
type discoveryGroupResult struct {
	Group     discoveryGroup
	Instances map[string][]discoveredInstance
	Err       error
}

// discoverAll runs one ListMetrics scan per target/region/namespace group and
// applies all grouped profiles to that streamed response. The group already holds
// the least restrictive RecentlyActive policy required by its selected series.
func discoverAll(
	ctx context.Context,
	newClient func(context.Context, string, string) (cloudwatchClient, error),
	groups []discoveryGroup,
	maxConcurrency int,
	timeout time.Duration,
) ([]discoveryGroupResult, error) {
	controller, err := newDiscoveryController(ctx, groups, timeout)
	if err != nil {
		return nil, err
	}
	defer controller.close()

	results := make([]discoveryGroupResult, len(groups))
	type groupRun struct {
		scanner *discoveryGroupScanner
		client  cloudwatchClient
	}
	runs := make([]groupRun, len(groups))
	for i, group := range groups {
		results[i].Group = group
		runs[i].scanner = newDiscoveryGroupScanner(group)
	}

	// Complete the first-page phase before scheduling any continuation. Every
	// group that reaches ListMetrics gets its first admitted operation before a
	// fast, deep namespace can consume the remaining operation budget.
	p := pool.New().WithMaxGoroutines(max(1, maxConcurrency))
	for i, group := range groups {
		p.Go(func() {
			lane := controller.lane(group)
			if err := context.Cause(lane.ctx); err != nil {
				results[i].Err = err
				return
			}
			client, err := newClient(lane.ctx, group.Target, group.Region)
			if err == nil {
				runs[i].client = client
				err = runs[i].scanner.scanPage(lane.ctx, client, controller.budget)
			}
			if isAWSAuthorizationError(err) {
				controller.denyLane(group, err)
			}
			results[i].Err = err
			if err == nil && runs[i].scanner.done {
				results[i].Instances = runs[i].scanner.instances
			}
		})
	}
	p.Wait()
	if err := controller.failure(); err != nil {
		return results, err
	}

	// Continuations share only the operation budget left after every non-skipped,
	// successfully client-resolved group has attempted its first operation.
	p = pool.New().WithMaxGoroutines(max(1, maxConcurrency))
	for i, group := range groups {
		if results[i].Err != nil || runs[i].scanner.done {
			continue
		}
		p.Go(func() {
			lane := controller.lane(group)
			if err := context.Cause(lane.ctx); err != nil {
				results[i].Err = err
				return
			}
			for !runs[i].scanner.done {
				err := runs[i].scanner.scanPage(lane.ctx, runs[i].client, controller.budget)
				if err != nil {
					if isAWSAuthorizationError(err) {
						controller.denyLane(group, err)
					}
					results[i].Err = err
					return
				}
			}
			results[i].Instances = runs[i].scanner.instances
		})
	}
	p.Wait()
	return results, controller.failure()
}

// buildDiscoverySnapshot assembles a snapshot from namespace-group results and
// returns the number of failed groups. Failed groups retain their previous
// profile instances; successful groups replace theirs. The merged effective
// snapshot must fit the same retained-state bounds as a fresh scan.
func buildDiscoverySnapshot(results []discoveryGroupResult, prev map[discoveryKey][]discoveredInstance, now time.Time, refreshEvery int) (discoverySnapshot, int, error) {
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
	if err := snap.validateRetainedBounds(); err != nil {
		return discoverySnapshot{}, failures, err
	}
	return snap, failures, nil
}

// highInstanceCountWarn is the discovered-instance count at or above which the
// collector logs a cost-visibility warning. Collection is never truncated.
const highInstanceCountWarn = defaultMaxInstances

func (c *Collector) loadCatalog() (cwprofiles.Catalog, error) {
	if c.newCatalog != nil {
		return c.newCatalog()
	}
	return cwprofiles.DefaultCatalog()
}

// markDiscoveryStale forces the next refreshDiscovery to re-run (without treating it
// as a first pass), so a target resolved after the current snapshot was already
// fetched is discovered on the very next cycle instead of waiting out
// discovery.refresh_every. ensureTargets runs before refreshDiscovery in collect(),
// so a same-cycle resolution is picked up the same cycle.
func (c *Collector) markDiscoveryStale() {
	c.discovery.ExpiresAt = time.Time{}
}

// refreshDiscovery refreshes the discovery snapshot when its TTL has expired.
// Per-group failures are logged and tolerated; an aggregate failure keeps the
// previous snapshot, or errors on the very first pass when there is none.
func (c *Collector) refreshDiscovery(ctx context.Context) error {
	now := c.now()
	if !c.discovery.expired(now) {
		return nil
	}

	newClient := func(callCtx context.Context, target, region string) (cloudwatchClient, error) {
		return c.clients.forTargetRegion(callCtx, target, region)
	}
	results, aggregateErr := discoverAll(ctx, newClient, c.discoveryGroups(), apiConcurrency, c.Timeout.Duration())
	// If the parent context was canceled or timed out during the fan-out, abort before
	// committing: buildDiscoverySnapshot would otherwise carry forward instances (or
	// accept a partial first snapshot) and advance the TTL, so the next cycle would skip
	// discovery. Operation-scoped GetMetricData timeouts use derived contexts and
	// do not trip this, so they stay fail-soft.
	if err := ctx.Err(); err != nil {
		return err
	}
	var (
		disposition discoveryDisposition
		diagnostics discoveryDiagnostics
	)
	if aggregateErr != nil {
		diagnostics.aggregateFailure = aggregateErr
		disposition = c.aggregateFailureDisposition(aggregateErr)
	} else {
		// Failed discovery groups carry forward their previous instances, so a
		// transient target/region/namespace failure does not drop series.
		snap, failedGroups, err := buildDiscoverySnapshot(results, c.discovery.Instances, now, c.Discovery.RefreshEvery)
		// Snapshot merging and retained-bound validation happen after fan-out. This
		// early check avoids diagnostics work; applyDiscoveryDisposition remains the
		// correctness boundary for every terminal outcome.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			diagnostics.aggregateFailure = err
			disposition = c.aggregateFailureDisposition(err)
		} else {
			for _, result := range results {
				if result.Err != nil {
					diagnostics.groupFailures = append(diagnostics.groupFailures, operationFailure{
						Target: result.Group.Target, Region: result.Group.Region,
						Scope: fmt.Sprintf("namespace %q", result.Group.Namespace), Err: result.Err,
					})
				}
			}

			// An empty successful group is not a failure; with shared regions across
			// many targets, empty successes are expected.
			if c.discovery.FetchedAt.IsZero() && len(results) > 0 && failedGroups == len(results) {
				disposition = discoveryDisposition{
					kind: discoveryDispositionFail,
					err:  fmt.Errorf("CloudWatch discovery failed for all %d target/region/namespace groups", len(results)),
				}
			} else {
				report := newDiscoveryReport(snap, c.discoverySig)
				diagnostics.report = &report
				disposition = discoveryDisposition{
					kind: discoveryDispositionInstall, snapshot: snap, discoverySig: report.sig,
				}
			}
		}
	}

	c.emitDiscoveryDiagnostics(diagnostics)
	return c.applyDiscoveryDisposition(ctx, disposition)
}

func (c *Collector) discoveryGroups() []discoveryGroup {
	if c.plan == nil {
		return nil
	}

	type profileKey struct {
		target, profile, region string
	}
	recentlyActive := make(map[profileKey]bool)
	for _, scope := range c.plan.Scopes {
		key := profileKey{target: scope.Target.Name, profile: scope.Profile.Name, region: scope.Region}
		recent := selectedSeriesUseRecentlyActive(scope.SelectedSeries, c.recentlyActiveOnly())
		if previous, ok := recentlyActive[key]; !ok || (previous && !recent) {
			recentlyActive[key] = recent
		}
	}
	index := make(map[discoveryGroupKey]int)
	seenProfiles := make(map[profileKey]struct{})
	var groups []discoveryGroup
	for _, scope := range c.plan.Scopes {
		if _, ok := c.resolvedByRef[scope.Target.Name]; !ok {
			continue
		}
		pk := profileKey{target: scope.Target.Name, profile: scope.Profile.Name, region: scope.Region}
		recent := recentlyActive[pk]
		key := discoveryGroupKey{
			target: scope.Target.Name, region: scope.Region,
			namespace: scope.Profile.Config.Namespace,
		}
		if i, ok := index[key]; ok {
			groups[i].RecentlyActive = groups[i].RecentlyActive && recent
			if _, seen := seenProfiles[pk]; !seen {
				groups[i].Profiles = append(groups[i].Profiles, scope.Profile)
				seenProfiles[pk] = struct{}{}
			}
			continue
		}
		index[key] = len(groups)
		groups = append(groups, discoveryGroup{
			Target: scope.Target.Name, Region: scope.Region, Namespace: scope.Profile.Config.Namespace,
			RecentlyActive: recent, Profiles: []cwprofiles.ResolvedProfile{scope.Profile},
		})
		seenProfiles[pk] = struct{}{}
	}
	return groups
}
