// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// resourceAttributeProvider enriches the instances of one CloudWatch namespace with
// non-identity labels read from the owning service's control-plane API, for facts
// CloudWatch does not publish per instance (for example the RDS engine).
type resourceAttributeProvider struct {
	name      string   // diagnostic name of the control-plane operation
	namespace string   // CloudWatch namespace whose profiles the provider enriches
	joinDim   string   // identifying CloudWatch dimension that carries the resource id
	labels    []string // every label key the provider may emit; reserved against tag labels
	// fetch returns the labels of every resource visible to one target in one
	// region, keyed by the joinDim value.
	fetch func(ctx context.Context, c *Collector, target, region string) (map[string][]metrix.Label, error)
}

var resourceAttributeProviders = []*resourceAttributeProvider{rdsInstanceAttributes}

// profileAttributes binds a profile to its provider and the position of the join
// dimension in the profile's dimension order.
type profileAttributes struct {
	provider *resourceAttributeProvider
	dimIndex int
}

// resolveAttributeProvider returns the provider for a profile whose namespace has
// one and whose join dimension is identifying. A constant join dimension would
// describe one aggregate, not one resource, so it gets no attributes. A profile
// whose identity labels already use a provider label gets none either, because a
// duplicate label key would panic metrix; the returned reason explains the skip.
func resolveAttributeProvider(prof cwprofiles.Profile) (attrs profileAttributes, ok bool, skipped string) {
	for _, provider := range resourceAttributeProviders {
		if prof.Namespace != provider.namespace {
			continue
		}
		dimIndex := -1
		for i, dim := range prof.Instance.Dimensions {
			if dim.Name == provider.joinDim && !dim.IsConstant() {
				dimIndex = i
				break
			}
		}
		if dimIndex < 0 {
			continue
		}
		identity := append([]string{"account_id", "region"}, profileDimLabels(prof)...)
		for _, label := range provider.labels {
			if slices.Contains(identity, label) {
				return profileAttributes{}, false, fmt.Sprintf("skipped %s labels: label %q collides with an identity label", provider.name, label)
			}
		}
		return profileAttributes{provider: provider, dimIndex: dimIndex}, true, ""
	}
	return profileAttributes{}, false, ""
}

// profileAttributeLabels returns the label keys a profile's provider may emit.
func profileAttributeLabels(prof cwprofiles.Profile) []string {
	if attrs, ok, _ := resolveAttributeProvider(prof); ok {
		return attrs.provider.labels
	}
	return nil
}

type attributeGroupKey struct {
	target, region, provider string
}

func lessAttributeGroupKey(a, b attributeGroupKey) bool {
	if a.target != b.target {
		return a.target < b.target
	}
	if a.region != b.region {
		return a.region < b.region
	}
	return a.provider < b.provider
}

// attributeGroup is the cached result of one provider fetch for one target and
// region. A failed refresh keeps the last-known labels: attributes are optional
// presentation and never control identity or selection.
type attributeGroup struct {
	labels    map[string][]metrix.Label
	expiresAt time.Time
}

type attributeFetch struct {
	key      attributeGroupKey
	provider *resourceAttributeProvider
	dimIndex int
	// instances are the discovered instances of every scope sharing the group;
	// read-only views into the discovery snapshot.
	instances [][]collectionInstance
}

// candidates returns the discovered join-dimension values. The cache keeps only
// these, so unrelated resources in the region neither use memory nor rebuild the
// query plan when they change. It is built only for groups being fetched.
func (f attributeFetch) candidates() map[string]struct{} {
	out := make(map[string]struct{})
	for _, instances := range f.instances {
		for _, inst := range instances {
			if f.dimIndex < len(inst.DimensionValues) {
				out[inst.DimensionValues[f.dimIndex]] = struct{}{}
			}
		}
	}
	return out
}

type attributeFetchResult struct {
	labels map[string][]metrix.Label
	err    error
}

// computeProfileAttributes resolves the per-profile providers once; the selected
// profiles are fixed per compiled plan.
func (c *Collector) computeProfileAttributes() {
	out := make(map[string]profileAttributes)
	for _, p := range c.plan.Profiles {
		attrs, ok, skipped := resolveAttributeProvider(p.Config)
		if skipped != "" {
			c.Warningf("CloudWatch resource attributes (profile %q): %s", p.Name, skipped)
		}
		if ok {
			out[p.Name] = attrs
		}
	}
	c.profileAttributes = out
}

// activeAttributeFetches lists one fetch per (target, region, provider) that has
// discovered instances to enrich, so a job that selects RDS but has no RDS
// instances makes no control-plane calls.
func (c *Collector) activeAttributeFetches() []attributeFetch {
	index := make(map[attributeGroupKey]int)
	var out []attributeFetch
	for _, scope := range c.plan.Scopes {
		attrs, ok := c.profileAttributes[scope.Profile.Name]
		if !ok {
			continue
		}
		if _, ok := c.resolvedTargetByRef(scope.Target.Name); !ok {
			continue
		}
		instances := scope.instances(c.discovery)
		if len(instances) == 0 {
			continue
		}
		key := attributeGroupKey{target: scope.Target.Name, region: scope.Region, provider: attrs.provider.name}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(out, attributeFetch{key: key, provider: attrs.provider, dimIndex: attrs.dimIndex})
		}
		out[i].instances = append(out[i].instances, instances)
	}
	sort.Slice(out, func(i, j int) bool { return lessAttributeGroupKey(out[i].key, out[j].key) })
	return out
}

// refreshAttributes re-fetches expired attribute groups on the discovery cadence.
// Failed groups keep their last-known labels and retry at the next TTL rather than
// every collect, which bounds control-plane calls and log volume when the optional
// permission is missing.
func (c *Collector) refreshAttributes(ctx context.Context) {
	if c.profileAttributes == nil {
		c.computeProfileAttributes()
	}
	if len(c.profileAttributes) == 0 {
		return
	}

	now := c.now()
	active := c.activeAttributeFetches()
	next := make(map[attributeGroupKey]attributeGroup, len(active))
	var due []attributeFetch
	for _, fetch := range active {
		state, ok := c.attributes[fetch.key]
		if ok && now.Before(state.expiresAt) {
			next[fetch.key] = state
			continue
		}
		if ok {
			next[fetch.key] = state
		}
		due = append(due, fetch)
	}
	if len(due) == 0 && len(next) == len(c.attributes) {
		return
	}

	results := c.fetchAttributeGroups(ctx, due)
	// Commit nothing on parent cancellation so the next cycle retries.
	if ctx.Err() != nil {
		return
	}

	ttl := time.Duration(c.Discovery.RefreshEvery) * time.Second
	changed := false
	var failures []operationFailure
	for i, fetch := range due {
		state := next[fetch.key]
		state.expiresAt = now.Add(ttl)
		if err := results[i].err; err != nil {
			failures = append(failures, operationFailure{
				Target: fetch.key.target, Region: fetch.key.region, Scope: fetch.provider.name, Err: err,
			})
		} else {
			labels := results[i].labels
			candidates := fetch.candidates()
			maps.DeleteFunc(labels, func(id string, _ []metrix.Label) bool {
				_, ok := candidates[id]
				return !ok
			})
			if !sameAttributeLabels(state.labels, labels) {
				changed = true
			}
			state.labels = labels
		}
		next[fetch.key] = state
	}
	c.warnOperationFailures(logKeyAttributeRefreshFailed, "resource attribute lookup", " (retaining last-known attribute labels; collection is unaffected)", failures)

	// Groups without discovered instances are dropped; their instances already
	// left the plan through discovery invalidation.
	c.attributes = next
	if changed {
		c.invalidateQueryPlan()
	}
}

func (c *Collector) fetchAttributeGroups(ctx context.Context, fetches []attributeFetch) []attributeFetchResult {
	results := make([]attributeFetchResult, len(fetches))
	p := pool.New().WithMaxGoroutines(max(1, apiConcurrency))
	for i := range fetches {
		p.Go(func() {
			labels, err := fetches[i].provider.fetch(ctx, c, fetches[i].key.target, fetches[i].key.region)
			results[i] = attributeFetchResult{labels: labels, err: err}
		})
	}
	p.Wait()
	return results
}

func sameAttributeLabels(a, b map[string][]metrix.Label) bool {
	return maps.EqualFunc(a, b, func(x, y []metrix.Label) bool { return slices.Equal(x, y) })
}

// markAttributesStale forces the next refreshAttributes to re-fetch every group, so
// instances from a newly installed discovery snapshot get their first charts with
// attribute labels even when the group's TTL has not expired. A newly resolved
// target needs no hook: its groups have no state and are fetched immediately.
func (c *Collector) markAttributesStale() {
	for key, group := range c.attributes {
		group.expiresAt = time.Time{}
		c.attributes[key] = group
	}
}

// attributeLabelsFor returns the cached attribute labels for one discovered
// instance, or nil when the profile has no provider or nothing is cached.
func (c *Collector) attributeLabelsFor(target, region string, prof cwprofiles.ResolvedProfile, values []string) []metrix.Label {
	attrs, ok := c.profileAttributes[prof.Name]
	if !ok || attrs.dimIndex >= len(values) {
		return nil
	}
	group, ok := c.attributes[attributeGroupKey{target: target, region: region, provider: attrs.provider.name}]
	if !ok {
		return nil
	}
	return group.labels[values[attrs.dimIndex]]
}

// enrichmentLabels joins attribute and tag labels into one read-only slice. Tag
// plans already skip attribute label keys, so the two sets never share a key.
func enrichmentLabels(attributes, tags []metrix.Label) []metrix.Label {
	switch {
	case len(attributes) == 0:
		return tags
	case len(tags) == 0:
		return attributes
	}
	out := make([]metrix.Label, 0, len(attributes)+len(tags))
	out = append(out, attributes...)
	return append(out, tags...)
}

func appendAttributeLabel(labels []metrix.Label, key, value string) []metrix.Label {
	if value == "" {
		return labels
	}
	return append(labels, metrix.Label{Key: key, Value: value})
}
