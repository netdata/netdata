// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

type chartLabelAccumulator struct {
	mode        program.PromotionMode
	promoteKeys map[string]struct{}
	excluded    map[string]struct{}
	instance    map[string]string
	selected    map[string]string
	initialized bool
}

func newChartLabelAccumulator(chart program.Chart) *chartLabelAccumulator {
	acc := &chartLabelAccumulator{
		mode:        chart.Labels.Mode,
		promoteKeys: make(map[string]struct{}, len(chart.Labels.PromoteKeys)),
		excluded: make(map[string]struct{},
			len(chart.Labels.Exclusions.SelectorConstrainedKeys)+len(chart.Labels.Exclusions.DimensionKeyLabels)),
		instance: make(map[string]string),
		selected: make(map[string]string),
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
		mode:        program.PromotionModeAutoIntersection,
		promoteKeys: make(map[string]struct{}),
		excluded:    make(map[string]struct{}),
		instance:    make(map[string]string),
		selected:    make(map[string]string),
	}
}

func (a *chartLabelAccumulator) observe(identity program.ChartIdentity, labels metrix.LabelView, dimensionKeyLabel string) error {
	if a == nil || labels.Len() == 0 {
		return nil
	}

	if key := strings.TrimSpace(dimensionKeyLabel); key != "" {
		a.excluded[key] = struct{}{}
	}

	instanceLabels, ok, err := resolveInstanceLabelValues(identity, labelViewAccessor{view: labels})
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	instanceSet := make(map[string]struct{}, len(instanceLabels))
	for _, label := range instanceLabels {
		instanceSet[label.Key] = struct{}{}
		if _, exists := a.instance[label.Key]; !exists {
			a.instance[label.Key] = label.Value
		}
	}

	eligible := make(map[string]string)
	switch a.mode {
	case program.PromotionModeExplicitIntersection:
		for key := range a.promoteKeys {
			if _, excluded := a.excluded[key]; excluded {
				continue
			}
			if _, identityKey := instanceSet[key]; identityKey {
				continue
			}
			value, ok := labels.Get(key)
			if !ok {
				continue
			}
			eligible[key] = value
		}
	default:
		labels.Range(func(key, value string) bool {
			if _, excluded := a.excluded[key]; excluded {
				return true
			}
			if _, identityKey := instanceSet[key]; identityKey {
				return true
			}
			eligible[key] = value
			return true
		})
	}

	if !a.initialized {
		for key, value := range eligible {
			a.selected[key] = value
		}
		a.initialized = true
		return nil
	}

	for key, value := range a.selected {
		next, ok := eligible[key]
		if !ok || next != value {
			delete(a.selected, key)
		}
	}
	return nil
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
