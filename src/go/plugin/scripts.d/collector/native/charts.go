// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

const (
	// Built-in check charts own this chart ID prefix.
	checkChartIDPrefix = "native_check_"
	contextNamespace   = "native_script"
)

// chartTemplates compiles immutable package templates. Checks are selected by
// each job's current snapshot. Without a template, metrics get automatic charts.
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
		if spec.Policy.Selector != nil {
			return nil, errors.New("global chart selector cannot filter built-in checks; use chart selectors instead")
		}
		for _, entry := range spec.Entries {
			if err := validateChartIDs(entry.Groups); err != nil {
				return nil, err
			}
		}
	}
	return chartengine.NewTemplateSet(spec)
}

// One stable entry per check isolates replacements from unchanged siblings.
func (c *Collector) updateCheckTemplates(families []checkFamily) error {
	var definitions []checkDefinition
	for _, family := range families {
		definitions = append(definitions, family.checkDefinition)
	}
	if reflect.DeepEqual(c.checks, definitions) {
		return nil
	}
	base := c.definition.templates
	if base == nil {
		return errors.New("collection requires chart templates")
	}
	entries := make([]chartengine.TemplateEntry, 0, len(definitions)+len(base.Entries()))
	for _, check := range definitions {
		entries = append(entries, chartengine.TemplateEntry{
			ID:               checkChartIDPrefix + check.ID,
			ContextNamespace: contextNamespace,
			Groups:           checkChartGroups(check),
		})
	}
	entries = append(entries, base.Entries()...)
	set, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		FallbackContextNamespace: base.FallbackContextNamespace(),
		Policy:                   base.GlobalPolicy(),
		Entries:                  entries,
	})
	if err != nil {
		return errors.New("invalid check chart templates")
	}
	c.templates, c.checks = set, definitions
	return nil
}

// checkChartGroups builds one state chart, instanced by its identity labels.
func checkChartGroups(check checkDefinition) []charttpl.Group {
	var instances *charttpl.Instances
	if len(check.ByLabels) > 0 {
		instances = &charttpl.Instances{
			ByLabels: check.ByLabels,
		}
	}
	metric := checkMetricName(check.ID)
	return []charttpl.Group{{
		Family:  "Checks",
		Metrics: []string{metric},
		Charts: []charttpl.Chart{{
			ID:            checkChartIDPrefix + check.ID,
			Title:         check.Title,
			Context:       "check_state",
			Units:         "state",
			Instances:     instances,
			LabelPromoted: append([]string{}, check.ByLabels...),
			Lifecycle: &charttpl.Lifecycle{
				ExpireAfterCycles: 1,
			},
			Dimensions: []charttpl.Dimension{{
				Selector:      metric,
				NameFromLabel: metric,
			}},
		}},
	}}
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
