// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"fmt"
	"slices"
	"strings"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// Automatic metadata may only affect presentation. Evaluate the same defaulted
// template and instance-label semantics as the engine without rewriting either
// the authored template or explicit resource-tag behavior.
func attributeTemplateExclusions(profile cwprofiles.ResolvedProfile, labels []string) ([]string, error) {
	groups, err := charttpl.NormalizeGroups([]charttpl.Group{profileChartGroup(profile)})
	if err != nil {
		return nil, err
	}
	excluded := make(map[string]struct{})
	if err := excludeTemplateAttributeInputs(groups[0], labels, excluded); err != nil {
		return nil, err
	}
	var out []string
	for _, label := range labels {
		if _, ok := excluded[label]; ok {
			out = append(out, label)
		}
	}
	return out, nil
}

func excludeTemplateAttributeInputs(group charttpl.Group, labels []string, excluded map[string]struct{}) error {
	for _, chart := range group.Charts {
		policy, err := chartengine.ResolveInstanceLabelPolicy(chart.Instances)
		if err != nil {
			return fmt.Errorf("chart %q: %w", chart.ID, err)
		}
		for _, label := range labels {
			if slices.Contains(policy.RequiredKeys, label) || slices.Contains(policy.OptionalKeys, label) ||
				(policy.IncludeAll && !slices.Contains(policy.ExcludedKeys, label)) {
				excluded[label] = struct{}{}
			}
		}
		for _, dimension := range chart.Dimensions {
			selector, err := metrixselector.ParseCompiled(dimension.Selector)
			if err != nil {
				return fmt.Errorf("chart %q dimension selector: %w", chart.ID, err)
			}
			keys := selector.Meta().ConstrainedLabelKeys
			for _, label := range labels {
				if label == strings.TrimSpace(dimension.NameFromLabel) || slices.Contains(keys, label) {
					excluded[label] = struct{}{}
				}
			}
		}
	}
	for _, child := range group.Groups {
		if err := excludeTemplateAttributeInputs(child, labels, excluded); err != nil {
			return err
		}
	}
	return nil
}
