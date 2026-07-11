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

type chartLabelAccumulator struct {
	mode                   program.PromotionMode
	promoteKeys            map[string]struct{}
	excluded               map[string]struct{}
	dimensionKeyExclusions map[string]struct{}
	instance               map[string]string
	selected               map[string]string
	initialized            bool

	instancePlan      compiledInstanceLabelPlan
	instanceKeys      map[string]struct{}
	resolvedScratch   []instanceLabelValue
	includeAllScratch []string
}

type chartLabelMembership struct {
	seriesID          metrix.SeriesID
	dimensionKeyLabel string
}

type chartLabelObservation struct {
	chartLabelMembership
	rawLabels []metrix.Label
}

type chartLabelScratch struct {
	accumulator  *chartLabelAccumulator
	observations []chartLabelObservation
	raw          bool
}

func newChartLabelAccumulator(chart program.Chart) *chartLabelAccumulator {
	plan := compileInstanceLabelPlan(chart.Identity)
	acc := &chartLabelAccumulator{
		mode:        chart.Labels.Mode,
		promoteKeys: make(map[string]struct{}, len(chart.Labels.PromoteKeys)),
		excluded: make(map[string]struct{},
			len(chart.Labels.Exclusions.SelectorConstrainedKeys)+len(chart.Labels.Exclusions.DimensionKeyLabels)),
		dimensionKeyExclusions: make(map[string]struct{}),
		instance:               make(map[string]string),
		selected:               make(map[string]string),
		instancePlan:           plan,
		instanceKeys:           make(map[string]struct{}, len(plan.explicitKeys)),
		resolvedScratch:        make([]instanceLabelValue, 0, len(plan.explicitKeys)),
		includeAllScratch:      make([]string, 0, len(plan.explicitKeys)),
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
		mode:                   program.PromotionModeAutoIntersection,
		promoteKeys:            make(map[string]struct{}),
		excluded:               make(map[string]struct{}),
		dimensionKeyExclusions: make(map[string]struct{}),
		instance:               make(map[string]string),
		selected:               make(map[string]string),
		instancePlan:           compileInstanceLabelPlan(program.ChartIdentity{}),
		instanceKeys:           make(map[string]struct{}),
		resolvedScratch:        make([]instanceLabelValue, 0),
		includeAllScratch:      make([]string, 0),
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

func (s *chartLabelScratch) resetObservations() {
	if s == nil {
		return
	}
	s.observations = s.observations[:0]
}

func (s *chartLabelScratch) observe(
	identity metrix.SeriesIdentity,
	labels metrix.LabelView,
	rawLabels []metrix.Label,
	raw bool,
	dimensionKeyLabel string,
) error {
	if len(s.observations) == 0 {
		s.raw = raw
		if !raw {
			s.accumulator.reset()
		}
	}
	s.observations = append(s.observations, chartLabelObservation{
		chartLabelMembership: chartLabelMembership{
			seriesID:          identity.ID,
			dimensionKeyLabel: dimensionKeyLabel,
		},
		rawLabels: rawLabels,
	})
	if !raw {
		return s.accumulator.observe(labels, dimensionKeyLabel)
	}
	return nil
}

func (s *chartLabelScratch) membershipMatches(current []chartLabelMembership) bool {
	if s == nil || len(s.observations) != len(current) {
		return false
	}
	for i := range s.observations {
		if s.observations[i].chartLabelMembership != current[i] {
			return false
		}
	}
	return true
}

func (s *chartLabelScratch) reconcile() error {
	if s == nil || s.accumulator == nil {
		return nil
	}
	if !s.raw {
		return nil
	}
	s.accumulator.reset()
	view := &labelSliceView{}
	for i := range s.observations {
		observation := &s.observations[i]
		view.items = observation.rawLabels
		if err := s.accumulator.observe(view, observation.dimensionKeyLabel); err != nil {
			return err
		}
	}
	return nil
}

func (s *chartLabelScratch) cloneMembership() []chartLabelMembership {
	if s == nil || len(s.observations) == 0 {
		return nil
	}
	out := make([]chartLabelMembership, len(s.observations))
	for i := range s.observations {
		out[i] = s.observations[i].chartLabelMembership
	}
	return out
}

func (a *chartLabelAccumulator) isExcluded(key string) bool {
	if _, ok := a.excluded[key]; ok {
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

	switch a.mode {
	case program.PromotionModeExplicitIntersection:
		if !a.initialized {
			for key := range a.promoteKeys {
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
