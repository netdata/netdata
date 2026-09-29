// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	// Built-in check charts own this chart ID prefix.
	checkChartIDPrefix = "native_check_"
	contextNamespace   = "native_script"
)

// chartTemplates compiles an optional authored template together with the
// built-in check charts. Without a template, metrics get automatic charts.
func (d packageDefinition) chartTemplates(data []byte) (*chartengine.TemplateSet, error) {
	if d.functionOnly() {
		if data != nil {
			return nil, errors.New("function-only packages cannot declare charts")
		}
		return nil, nil
	}
	spec := chartengine.TemplateSetSpec{
		FallbackContextNamespace: contextNamespace,
		Policy: chartengine.EnginePolicy{
			Autogen: &chartengine.AutogenPolicy{
				Enabled:                  true,
				ExpireAfterSuccessCycles: 1,
			},
		},
	}
	if data != nil {
		set, err := chartengine.NewTemplateSetYAML(data)
		if err != nil {
			return nil, errors.New("invalid chart template")
		}
		spec.Entries, spec.Policy, spec.FallbackContextNamespace = set.Entries(), set.GlobalPolicy(), set.FallbackContextNamespace()
		if len(d.Checks) > 0 && spec.Policy.Selector != nil {
			return nil, errors.New("global chart selector cannot filter built-in checks; use chart selectors instead")
		}
		for _, entry := range spec.Entries {
			if err := validateChartIDs(entry.Groups); err != nil {
				return nil, err
			}
		}
	}
	if groups := d.checkChartGroups(); len(groups) > 0 {
		checks := chartengine.TemplateEntry{
			ID:               "native_checks",
			ContextNamespace: contextNamespace,
			Groups:           groups,
		}
		spec.Entries = append([]chartengine.TemplateEntry{checks}, spec.Entries...)
	}
	return chartengine.NewTemplateSet(spec)
}

// checkChartGroups builds one state chart per check, instanced by its identity labels.
func (d packageDefinition) checkChartGroups() []charttpl.Group {
	var groups []charttpl.Group
	for _, check := range d.Checks {
		var instances *charttpl.Instances
		if len(check.ByLabels) > 0 {
			instances = &charttpl.Instances{
				ByLabels: check.ByLabels,
			}
		}
		metric := checkMetricName(check.ID)
		groups = append(groups, charttpl.Group{
			Family:  "Checks",
			Metrics: []string{metric},
			Charts: []charttpl.Chart{{
				ID:        checkChartIDPrefix + check.ID,
				Title:     check.Title,
				Context:   "check_state",
				Units:     "state",
				Instances: instances,
				Lifecycle: &charttpl.Lifecycle{
					ExpireAfterCycles: 1,
				},
				Dimensions: []charttpl.Dimension{{
					Selector:      metric,
					NameFromLabel: metric,
				}},
			}},
		})
	}
	return groups
}

func validateChartIDs(groups []charttpl.Group) error {
	for _, group := range groups {
		for _, chart := range group.Charts {
			if strings.HasPrefix(chart.ID, checkChartIDPrefix) {
				return fmt.Errorf("chart id prefix %s is reserved", checkChartIDPrefix)
			}
		}
		if err := validateChartIDs(group.Groups); err != nil {
			return err
		}
	}
	return nil
}
