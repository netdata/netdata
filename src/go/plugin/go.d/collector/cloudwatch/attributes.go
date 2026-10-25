// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

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
	labels    []string // every label key the provider may emit
	// fetch returns the labels of every resource visible to one target in one
	// region, keyed by the joinDim value.
	fetch func(ctx context.Context, clients *attributeClients, target, region string) (map[string][]metrix.Label, error)
}

var resourceAttributeProviders = []*resourceAttributeProvider{rdsInstanceAttributes}

// profileAttributes binds a profile to its provider and the position of the join
// dimension in the profile's dimension order.
type profileAttributes struct {
	provider       *resourceAttributeProvider
	dimIndex       int
	excludedLabels []string
}

// resolveAttributeProvider returns the provider for a profile whose namespace has
// one and whose join dimension is identifying. Constant dimensions do not belong
// to the discovered instance identity and are not eligible for enrichment. A profile
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
				return profileAttributes{}, false, fmt.Sprintf(
					"skipped %s labels: label %q collides with an identity label",
					provider.name,
					label,
				)
			}
		}
		return profileAttributes{
			provider: provider,
			dimIndex: dimIndex,
		}, true, ""
	}
	return profileAttributes{}, false, ""
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
	labels map[string][]metrix.Label
}

type attributeInstances struct {
	dimIndex  int
	instances []collectionInstance
}

type attributeFetch struct {
	key      attributeGroupKey
	provider *resourceAttributeProvider
	// Each profile owns its dimension order; the fetch only shares AWS I/O.
	profiles map[string]attributeInstances
}

func (f attributeFetch) candidates() map[string]struct{} {
	out := make(map[string]struct{})
	for _, profile := range f.profiles {
		for _, inst := range profile.instances {
			if profile.dimIndex < len(inst.DimensionValues) {
				out[inst.DimensionValues[profile.dimIndex]] = struct{}{}
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
			for _, tag := range c.tagLabelPlans[p.Name] {
				if slices.Contains(attrs.provider.labels, tag.label) {
					attrs.excludedLabels = append(attrs.excludedLabels, tag.label)
				}
			}
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
		key := attributeGroupKey{
			target:   scope.Target.Name,
			region:   scope.Region,
			provider: attrs.provider.name,
		}
		i, ok := index[key]
		if !ok {
			i = len(out)
			index[key] = i
			out = append(
				out,
				attributeFetch{
					key:      key,
					provider: attrs.provider,
					profiles: make(map[string]attributeInstances),
				},
			)
		}
		out[i].profiles[scope.Profile.Name] = attributeInstances{
			dimIndex:  attrs.dimIndex,
			instances: instances,
		}
	}
	sort.Slice(out, func(i, j int) bool { return lessAttributeGroupKey(out[i].key, out[j].key) })
	return out
}

// refreshAttributes only exchanges snapshots with Run. Collect owns accepted
// labels and plan invalidation; the worker never reads mutable discovery state.
func (c *Collector) refreshAttributes(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if c.profileAttributes == nil {
		c.computeProfileAttributes()
	}
	if len(c.profileAttributes) == 0 {
		return
	}

	now := c.now()
	if c.attributePending {
		select {
		case batch := <-c.attributeResults:
			c.attributePending = false
			newInstances := c.applyAttributeResult(batch)
			c.attributeRefreshAt = now.Add(time.Duration(c.Discovery.RefreshEvery) * time.Second)
			if newInstances {
				c.attributeRefreshAt = time.Time{}
			}
		default:
			return
		}
	}
	if now.Before(c.attributeRefreshAt) {
		return
	}

	fetches := c.activeAttributeFetches()
	if len(fetches) == 0 {
		c.attributes = nil
		c.attributeRefreshAt = now.Add(time.Duration(c.Discovery.RefreshEvery) * time.Second)
		return
	}
	// One outstanding batch, including its completed result, is sufficient:
	// intermediate discovery snapshots are superseded by the next collection.
	select {
	case c.attributeRequests <- attributeBatch{
		fetches: fetches,
	}:
		c.attributePending = true
	default:
	}
}

func (c *Collector) applyAttributeResult(batch attributeBatch) bool {
	fetched := make(map[attributeGroupKey]int, len(batch.fetches))
	var failures []operationFailure
	for i, fetch := range batch.fetches {
		result := batch.results[i]
		fetched[fetch.key] = i
		if result.err != nil {
			failures = append(failures, operationFailure{
				Target: fetch.key.target,
				Region: fetch.key.region,
				Scope:  fetch.provider.name,
				Err:    result.err,
			})
		}
	}

	active := c.activeAttributeFetches()
	next := make(map[attributeGroupKey]attributeGroup, len(active))
	changed := len(active) != len(c.attributes)
	newInstances := false
	for _, fetch := range active {
		previous := c.attributes[fetch.key].labels
		labels := previous
		i, wasRequested := fetched[fetch.key]
		if wasRequested && batch.results[i].err == nil {
			labels = batch.results[i].labels
		} else {
			labels = maps.Clone(labels)
		}
		// Prune against current discovery, including when the lookup failed or
		// an older request completed after resources were removed.
		candidates := fetch.candidates()
		var requested map[string]struct{}
		if wasRequested {
			requested = batch.fetches[i].candidates()
		}
		for id := range candidates {
			if _, ok := requested[id]; !ok {
				newInstances = true
				break
			}
		}
		maps.DeleteFunc(labels, func(id string, _ []metrix.Label) bool {
			_, ok := candidates[id]
			return !ok
		})
		changed = changed || !sameAttributeLabels(previous, labels)
		next[fetch.key] = attributeGroup{
			labels: labels,
		}
	}
	c.attributes = next
	c.warnOperationFailures(logKeyAttributeRefreshFailed, "resource attribute lookup",
		" (retaining last-known attribute labels; collection is unaffected)", failures)
	if changed {
		c.invalidateQueryPlan()
	}
	// A discovery refresh with unchanged resources must not cause immediate
	// retries when the previous metadata request outlasted the discovery TTL.
	return newInstances
}

func sameAttributeLabels(a, b map[string][]metrix.Label) bool {
	return maps.EqualFunc(a, b, func(x, y []metrix.Label) bool { return slices.Equal(x, y) })
}

// markAttributesStale requests a new snapshot after discovery changes. An
// outstanding lookup may still supply labels for surviving resources.
func (c *Collector) markAttributesStale() {
	c.attributeRefreshAt = time.Time{}
}

// attributeLabelsFor returns the cached attribute labels for one discovered
// instance, or nil when the profile has no provider or nothing is cached.
func (c *Collector) attributeLabelsFor(
	target, region string,
	prof cwprofiles.ResolvedProfile,
	values []string,
) []metrix.Label {
	attrs, ok := c.profileAttributes[prof.Name]
	if !ok || attrs.dimIndex >= len(values) {
		return nil
	}
	group, ok := c.attributes[attributeGroupKey{
		target:   target,
		region:   region,
		provider: attrs.provider.name,
	}]
	if !ok {
		return nil
	}
	labels := group.labels[values[attrs.dimIndex]]
	if len(attrs.excludedLabels) == 0 {
		return labels
	}
	out := make([]metrix.Label, 0, len(labels))
	for _, label := range labels {
		if !slices.Contains(attrs.excludedLabels, label.Key) {
			out = append(out, label)
		}
	}
	return out
}

// enrichmentLabels joins attribute and tag labels into one read-only slice. Explicit tag
// mappings have already excluded conflicting automatic attribute keys.
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
	return append(labels, metrix.Label{
		Key:   key,
		Value: value,
	})
}
