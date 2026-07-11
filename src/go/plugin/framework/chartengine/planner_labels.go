// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

const collectJobLabel = "_collect_job"

type compiledInstanceLabelPlan struct {
	explicitKeys []string
	explicitSet  map[string]struct{}
	excludeSet   map[string]struct{}
	includeAll   bool
}

type chartLabelPolicy struct {
	mode         program.PromotionMode
	promoteKeys  map[string]struct{}
	excluded     map[string]struct{}
	instancePlan compiledInstanceLabelPlan
}

type chartLabelAccumulator struct {
	policy                 *chartLabelPolicy
	dimensionKeyExclusions map[string]struct{}
	instance               map[string]string
	selected               map[string]string
	initialized            bool

	instanceKeys      map[string]struct{}
	resolvedScratch   []instanceLabelValue
	includeAllScratch []string
}

type chartLabelMembership struct {
	seriesID          metrix.SeriesID
	dimensionKeyLabel string
}

type chartLabelTracker struct {
	previous *materializedChartPresentation
	change   *chartLabelChange
	observed int
}

type chartLabelChange struct {
	proposedMembership  []chartLabelMembership
	accumulator         *chartLabelAccumulator
	reconcileDuringScan bool
}

func compileChartLabelPolicy(chart program.Chart) *chartLabelPolicy {
	plan := compileInstanceLabelPlan(chart.Identity)
	policy := &chartLabelPolicy{
		mode:        chart.Labels.Mode,
		promoteKeys: make(map[string]struct{}, len(chart.Labels.PromoteKeys)),
		excluded: make(map[string]struct{},
			len(chart.Labels.Exclusions.SelectorConstrainedKeys)+len(chart.Labels.Exclusions.DimensionKeyLabels)),
		instancePlan: plan,
	}
	for _, key := range chart.Labels.PromoteKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		policy.promoteKeys[key] = struct{}{}
	}
	for _, key := range chart.Labels.Exclusions.SelectorConstrainedKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		policy.excluded[key] = struct{}{}
	}
	for _, key := range chart.Labels.Exclusions.DimensionKeyLabels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		policy.excluded[key] = struct{}{}
	}
	return policy
}

func compileAutogenChartLabelPolicy() *chartLabelPolicy {
	return &chartLabelPolicy{
		mode:         program.PromotionModeAutoIntersection,
		promoteKeys:  make(map[string]struct{}),
		excluded:     make(map[string]struct{}),
		instancePlan: compileInstanceLabelPlan(program.ChartIdentity{}),
	}
}

func newChartLabelAccumulator(policy *chartLabelPolicy) *chartLabelAccumulator {
	plan := policy.instancePlan
	return &chartLabelAccumulator{
		policy:                 policy,
		dimensionKeyExclusions: make(map[string]struct{}),
		instance:               make(map[string]string, len(plan.explicitKeys)),
		selected:               make(map[string]string, len(policy.promoteKeys)),
		instanceKeys:           make(map[string]struct{}, len(plan.explicitKeys)),
		resolvedScratch:        make([]instanceLabelValue, 0, len(plan.explicitKeys)),
		includeAllScratch:      make([]string, 0, len(plan.explicitKeys)),
	}
}

func (a *chartLabelAccumulator) reset() {
	if a == nil {
		return
	}
	clear(a.dimensionKeyExclusions)
	clear(a.instance)
	clear(a.selected)
	clear(a.instanceKeys)
	a.resolvedScratch = a.resolvedScratch[:0]
	a.includeAllScratch = a.includeAllScratch[:0]
	a.initialized = false
}

func newChartLabelTracker(previous *materializedChartPresentation) chartLabelTracker {
	return chartLabelTracker{previous: previous}
}

func (c *chartLabelChange) ensureAccumulator(policy *chartLabelPolicy) *chartLabelAccumulator {
	if c.accumulator == nil {
		c.accumulator = newChartLabelAccumulator(policy)
	}
	return c.accumulator
}

func (t *chartLabelTracker) observeMembership(
	identity metrix.SeriesIdentity,
	dimensionKeyLabel string,
) bool {
	previousMembership := t.previousMembership()
	membership := chartLabelMembership{
		seriesID:          identity.ID,
		dimensionKeyLabel: dimensionKeyLabel,
	}
	index := t.observed
	t.observed++
	if t.change == nil && index < len(previousMembership) && previousMembership[index] == membership {
		return false
	}

	if t.change == nil {
		capacity := max(len(previousMembership), t.observed)
		t.change = &chartLabelChange{
			proposedMembership:  make([]chartLabelMembership, 0, capacity),
			reconcileDuringScan: t.previous == nil,
		}
		t.change.proposedMembership = append(t.change.proposedMembership, previousMembership[:index]...)
	}
	t.change.proposedMembership = append(t.change.proposedMembership, membership)
	return t.change.reconcileDuringScan
}

func (t *chartLabelTracker) observeDuringScan(policy *chartLabelPolicy, labels metrix.LabelView, dimensionKeyLabel string) error {
	return t.change.ensureAccumulator(policy).observe(labels, dimensionKeyLabel)
}

func (t *chartLabelTracker) finishMembership() bool {
	previousMembership := t.previousMembership()
	if t.change == nil && t.observed != len(previousMembership) {
		t.change = &chartLabelChange{
			proposedMembership: append(
				make([]chartLabelMembership, 0, t.observed),
				previousMembership[:t.observed]...,
			),
			reconcileDuringScan: t.previous == nil,
		}
	}
	return t.change != nil
}

func (t *chartLabelTracker) previousMembership() []chartLabelMembership {
	if t.previous == nil {
		return nil
	}
	return t.previous.labelMembership
}

func (t *chartLabelTracker) beginReplay(policy *chartLabelPolicy) {
	t.change.ensureAccumulator(policy).reset()
}

func (t *chartLabelTracker) observeReplay(labels metrix.LabelView, dimensionKeyLabel string) error {
	return t.change.accumulator.observe(labels, dimensionKeyLabel)
}

func (t *chartLabelTracker) needsReplay() bool {
	return t.change != nil && !t.change.reconcileDuringScan
}

func (t *chartLabelTracker) changed() bool {
	return t.change != nil
}

func (t *chartLabelTracker) membership() []chartLabelMembership {
	if t.change != nil {
		return t.change.proposedMembership
	}
	return t.previousMembership()
}

func (t *chartLabelTracker) accumulator() *chartLabelAccumulator {
	if t.change == nil {
		return nil
	}
	return t.change.accumulator
}

func (a *chartLabelAccumulator) isExcluded(key string) bool {
	if _, ok := a.policy.excluded[key]; ok {
		return true
	}
	_, ok := a.dimensionKeyExclusions[key]
	return ok
}

func compileInstanceLabelPlan(identity program.ChartIdentity) compiledInstanceLabelPlan {
	plan := compiledInstanceLabelPlan{
		explicitKeys: make([]string, 0, len(identity.InstanceByLabels)),
		explicitSet:  make(map[string]struct{}, len(identity.InstanceByLabels)),
		excludeSet:   make(map[string]struct{}),
	}

	for _, token := range identity.InstanceByLabels {
		switch {
		case token.Exclude:
			if token.Key != "" {
				plan.excludeSet[token.Key] = struct{}{}
			}
		case token.IncludeAll:
			plan.includeAll = true
		}
	}

	seenExplicit := make(map[string]struct{}, len(identity.InstanceByLabels))
	for _, token := range identity.InstanceByLabels {
		if token.Exclude || token.IncludeAll || token.Key == "" {
			continue
		}

		key := token.Key
		if _, excluded := plan.excludeSet[key]; excluded {
			continue
		}
		if _, exists := seenExplicit[key]; exists {
			continue
		}
		seenExplicit[key] = struct{}{}
		plan.explicitKeys = append(plan.explicitKeys, key)
		plan.explicitSet[key] = struct{}{}
	}
	return plan
}

func (a *chartLabelAccumulator) observe(labels metrix.LabelView, dimensionKeyLabel string) error {
	if a == nil || labels.Len() == 0 {
		return nil
	}

	if key := strings.TrimSpace(dimensionKeyLabel); key != "" {
		a.dimensionKeyExclusions[key] = struct{}{}
	}

	ok := a.resolveInstanceLabelsForObserve(labels)
	if !ok {
		return nil
	}

	switch a.policy.mode {
	case program.PromotionModeExplicitIntersection:
		if !a.initialized {
			for key := range a.policy.promoteKeys {
				if a.isExcluded(key) {
					continue
				}
				if a.isInstanceKey(key) {
					continue
				}
				value, ok := labels.Get(key)
				if !ok {
					continue
				}
				a.selected[key] = value
			}
			a.initialized = true
			return nil
		}
		for key, value := range a.selected {
			if a.isExcluded(key) {
				delete(a.selected, key)
				continue
			}
			next, ok := labels.Get(key)
			if !ok {
				delete(a.selected, key)
				continue
			}
			if a.isInstanceKey(key) {
				delete(a.selected, key)
				continue
			}
			if next != value {
				delete(a.selected, key)
			}
		}
		return nil
	default:
		if !a.initialized {
			labels.Range(func(key, value string) bool {
				if a.isExcluded(key) {
					return true
				}
				if a.isInstanceKey(key) {
					return true
				}
				a.selected[key] = value
				return true
			})
			a.initialized = true
			return nil
		}
		for key, value := range a.selected {
			if a.isExcluded(key) {
				delete(a.selected, key)
				continue
			}
			next, ok := labels.Get(key)
			if !ok {
				delete(a.selected, key)
				continue
			}
			if a.isInstanceKey(key) {
				delete(a.selected, key)
				continue
			}
			if next != value {
				delete(a.selected, key)
			}
		}
		return nil
	}
}

func (a *chartLabelAccumulator) resetInstanceKeys() {
	clear(a.instanceKeys)
}

func (a *chartLabelAccumulator) addInstanceKey(key, value string) {
	if _, exists := a.instanceKeys[key]; !exists {
		a.instanceKeys[key] = struct{}{}
	}
	if _, exists := a.instance[key]; !exists {
		a.instance[key] = value
	}
}

func (a *chartLabelAccumulator) resolveInstanceLabelsForObserve(labels metrix.LabelView) bool {
	a.resetInstanceKeys()

	resolved := a.resolvedScratch[:0]
	for _, key := range a.policy.instancePlan.explicitKeys {
		value, ok := labels.Get(key)
		if !ok {
			a.resolvedScratch = resolved[:0]
			return false
		}
		resolved = append(resolved, instanceLabelValue{Key: key, Value: value})
	}

	if a.policy.instancePlan.includeAll {
		extra := a.includeAllScratch[:0]
		labels.Range(func(key, _ string) bool {
			if _, excluded := a.policy.instancePlan.excludeSet[key]; excluded {
				return true
			}
			if _, already := a.policy.instancePlan.explicitSet[key]; already {
				return true
			}
			extra = append(extra, key)
			return true
		})
		sort.Strings(extra)
		for _, key := range extra {
			value, ok := labels.Get(key)
			if !ok {
				a.resolvedScratch = resolved[:0]
				a.includeAllScratch = extra[:0]
				return false
			}
			resolved = append(resolved, instanceLabelValue{Key: key, Value: value})
		}
		a.includeAllScratch = extra
	}

	for _, item := range resolved {
		a.addInstanceKey(item.Key, item.Value)
	}
	a.resolvedScratch = resolved
	return true
}

func (a *chartLabelAccumulator) isInstanceKey(key string) bool {
	_, ok := a.instanceKeys[key]
	return ok
}

func (a *chartLabelAccumulator) materialize() map[string]string {
	if a == nil {
		return nil
	}
	out := make(map[string]string, len(a.instance)+len(a.selected))
	for key, value := range a.instance {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	for key, value := range a.selected {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out[key] = value
	}
	delete(out, collectJobLabel)
	return out
}

func (a *chartLabelAccumulator) materializedEquals(values map[string]string) bool {
	if a == nil {
		return len(values) == 0
	}

	count := 0
	for key := range a.instance {
		if strings.TrimSpace(key) == "" || key == collectJobLabel {
			continue
		}
		if _, overridden := a.selected[key]; overridden {
			continue
		}
		count++
	}
	for key := range a.selected {
		if strings.TrimSpace(key) == "" || key == collectJobLabel {
			continue
		}
		count++
	}
	if count != len(values) {
		return false
	}

	for key, want := range values {
		got, ok := a.selected[key]
		if !ok {
			got, ok = a.instance[key]
		}
		if !ok || got != want {
			return false
		}
	}
	return true
}
