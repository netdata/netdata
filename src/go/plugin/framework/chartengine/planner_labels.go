// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

type compiledInstanceLabelPlan struct {
	explicitKeys []string
	explicitSet  map[string]struct{}
	excludeSet   map[string]struct{}
	includeAll   bool
}

type chartLabelAccumulator struct {
	mode        program.PromotionMode
	promoteKeys map[string]struct{}
	excluded    map[string]struct{}
	instance    map[string]string
	selected    map[string]string
	initialized bool

	instancePlan      compiledInstanceLabelPlan
	instanceKeys      map[string]struct{}
	resolvedScratch   []instanceLabelValue
	includeAllScratch []string
}

func newChartLabelAccumulator(chart program.Chart) *chartLabelAccumulator {
	plan := compileInstanceLabelPlan(chart.Identity)
	acc := &chartLabelAccumulator{
		mode:        chart.Labels.Mode,
		promoteKeys: make(map[string]struct{}, len(chart.Labels.PromoteKeys)),
		excluded: make(map[string]struct{},
			len(chart.Labels.Exclusions.SelectorConstrainedKeys)+len(chart.Labels.Exclusions.DimensionKeyLabels)),
		instance:          make(map[string]string),
		selected:          make(map[string]string),
		instancePlan:      plan,
		instanceKeys:      make(map[string]struct{}, len(plan.explicitKeys)),
		resolvedScratch:   make([]instanceLabelValue, 0, len(plan.explicitKeys)),
		includeAllScratch: make([]string, 0, len(plan.explicitKeys)),
	}
	for _, key := range chart.Labels.PromoteKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		acc.promoteKeys[key] = struct{}{}
	}
	for _, key := range chart.Labels.Exclusions.SelectorConstrainedKeys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		acc.excluded[key] = struct{}{}
	}
	for _, key := range chart.Labels.Exclusions.DimensionKeyLabels {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		acc.excluded[key] = struct{}{}
	}
	return acc
}

func newAutogenChartLabelAccumulator() *chartLabelAccumulator {
	return &chartLabelAccumulator{
		mode:              program.PromotionModeAutoIntersection,
		promoteKeys:       make(map[string]struct{}),
		excluded:          make(map[string]struct{}),
		instance:          make(map[string]string),
		selected:          make(map[string]string),
		instancePlan:      compileInstanceLabelPlan(program.ChartIdentity{}),
		instanceKeys:      make(map[string]struct{}),
		resolvedScratch:   make([]instanceLabelValue, 0),
		includeAllScratch: make([]string, 0),
	}
}

func compileInstanceLabelPlan(identity program.ChartIdentity) compiledInstanceLabelPlan {
	plan := compiledInstanceLabelPlan{
		explicitKeys: make([]string, 0, len(identity.InstanceByLabels)),
		explicitSet:  make(map[string]struct{}, len(identity.InstanceByLabels)),
		excludeSet:   make(map[string]struct{}),
	}

	seenExplicit := make(map[string]struct{}, len(identity.InstanceByLabels))
	for _, token := range identity.InstanceByLabels {
		switch {
		case token.Exclude:
			if token.Key != "" {
				plan.excludeSet[token.Key] = struct{}{}
			}
		case token.IncludeAll:
			plan.includeAll = true
		case token.Key != "":
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
	}
	return plan
}

func (a *chartLabelAccumulator) observe(labels metrix.LabelView, dimensionKeyLabel string) error {
	if a == nil || labels.Len() == 0 {
		return nil
	}

	if key := strings.TrimSpace(dimensionKeyLabel); key != "" {
		a.excluded[key] = struct{}{}
	}

	ok := a.resolveInstanceLabelsForObserve(labels)
	if !ok {
		return nil
	}

	switch a.mode {
	case program.PromotionModeExplicitIntersection:
		if !a.initialized {
			for key := range a.promoteKeys {
				if _, excluded := a.excluded[key]; excluded {
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
			if _, excluded := a.excluded[key]; excluded {
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
				if _, excluded := a.excluded[key]; excluded {
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
			if _, excluded := a.excluded[key]; excluded {
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
	for _, key := range a.instancePlan.explicitKeys {
		value, ok := labels.Get(key)
		if !ok {
			a.resolvedScratch = resolved[:0]
			return false
		}
		resolved = append(resolved, instanceLabelValue{Key: key, Value: value})
	}

	if a.instancePlan.includeAll {
		extra := a.includeAllScratch[:0]
		labels.Range(func(key, _ string) bool {
			if _, excluded := a.instancePlan.excludeSet[key]; excluded {
				return true
			}
			if _, already := a.instancePlan.explicitSet[key]; already {
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

func (a *chartLabelAccumulator) materialize() (map[string]string, error) {
	if a == nil {
		return nil, nil
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
	delete(out, "_collect_job")
	return out, nil
}
