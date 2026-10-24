// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

func buildProfileMetricChartTemplateYAML(baseTemplate string, rules []*compiledProfileMetricRule, charts map[string]*profileMetricChart) (string, error) {
	spec, err := charttpl.DecodeYAML([]byte(baseTemplate))
	if err != nil {
		return "", fmt.Errorf("failed to decode base chart template: %w", err)
	}
	if len(spec.Groups) == 0 {
		return "", fmt.Errorf("base chart template has no groups")
	}
	group := &spec.Groups[0]
	group.Metrics = append(group.Metrics,
		"snmp_trap_profile_metrics_rule_missed",
		"snmp_trap_profile_metrics_extraction_failed",
		"snmp_trap_profile_metrics_attribution_failed",
		"snmp_trap_profile_metrics_overflow_dropped",
		"snmp_trap_profile_metrics_source_transitions",
	)
	group.Charts = append(group.Charts, profileMetricDiagnosticChart())

	ruleByChart := make(map[string][]*compiledProfileMetricRule)
	for _, rule := range rules {
		group.Metrics = append(group.Metrics, rule.rule.Output.Metric)
		ruleByChart[rule.rule.Output.Chart] = append(ruleByChart[rule.rule.Output.Chart], rule)
	}
	if err := validateSelectedProfileMetricChartDimensions(ruleByChart); err != nil {
		return "", err
	}
	chartIDs := make([]string, 0, len(ruleByChart))
	for id := range ruleByChart {
		chartIDs = append(chartIDs, id)
	}
	slices.Sort(chartIDs)
	for _, id := range chartIDs {
		chart := charts[id]
		if chart == nil {
			return "", fmt.Errorf("profile metric chart %q not found", id)
		}
		group.Charts = append(group.Charts, profileMetricChartToTemplate(chart, ruleByChart[id]))
	}
	raw, err := spec.MarshalTemplate()
	if err != nil {
		return "", fmt.Errorf("invalid chart template: %w", err)
	}
	return raw, nil
}

func validateSelectedProfileMetricChartDimensions(ruleByChart map[string][]*compiledProfileMetricRule) error {
	chartIDs := make([]string, 0, len(ruleByChart))
	for chartID := range ruleByChart {
		chartIDs = append(chartIDs, chartID)
	}
	slices.Sort(chartIDs)
	for _, chartID := range chartIDs {
		rules := append([]*compiledProfileMetricRule(nil), ruleByChart[chartID]...)
		slices.SortFunc(rules, func(a, b *compiledProfileMetricRule) int {
			if c := strings.Compare(a.rule.Output.Dimension, b.rule.Output.Dimension); c != 0 {
				return c
			}
			return strings.Compare(a.rule.Name, b.rule.Name)
		})
		seen := make(map[string]*compiledProfileMetricRule, len(rules))
		for _, rule := range rules {
			if rule == nil || rule.rule == nil {
				continue
			}
			dimension := rule.rule.Output.Dimension
			if existing := seen[dimension]; existing != nil {
				return fmt.Errorf("%s: metric rule %q chart %q reuses output.dimension %q selected by rule %q in %s",
					rule.rule.Source(), rule.rule.Name, chartID, dimension, existing.rule.Name, existing.rule.Source())
			}
			seen[dimension] = rule
		}
	}
	return nil
}

func profileMetricDiagnosticChart() charttpl.Chart {
	return charttpl.Chart{
		ID:    "profile_metric_diagnostics",
		Title: "SNMP trap profile metric diagnostics",
		// Template-local context; the base chart template compiles it under snmp.trap.
		Context:   "profile_metric_diagnostics",
		Units:     "events/s",
		Algorithm: "incremental",
		Type:      "stacked",
		Instances: &charttpl.Instances{ByLabels: []string{"job_name"}},
		Dimensions: []charttpl.Dimension{
			{Selector: "snmp_trap_profile_metrics_rule_missed", Name: "rule_missed"},
			{Selector: "snmp_trap_profile_metrics_extraction_failed", Name: "extraction_failed"},
			{Selector: "snmp_trap_profile_metrics_attribution_failed", Name: "attribution_failed"},
			{Selector: "snmp_trap_profile_metrics_overflow_dropped", Name: "overflow_dropped"},
			{Selector: "snmp_trap_profile_metrics_source_transitions", Name: "source_transitions"},
		},
	}
}

func profileMetricChartToTemplate(chart *profileMetricChart, rules []*compiledProfileMetricRule) charttpl.Chart {
	dims := make([]charttpl.Dimension, 0, len(rules))
	for _, rule := range rules {
		dim := charttpl.Dimension{
			Selector: rule.rule.Output.Metric,
			Name:     rule.rule.Output.Dimension,
		}
		dims = append(dims, dim)
	}
	slices.SortFunc(dims, func(a, b charttpl.Dimension) int {
		return strings.Compare(a.Name, b.Name)
	})
	byLabels := []string{"job_name", "source_id", "source_kind"}
	usesResource := false
	for _, rule := range rules {
		if rule.rule.Identity.Resource != nil {
			usesResource = true
		}
	}
	if usesResource {
		byLabels = append(byLabels, "resource_class", "resource_id")
	}
	context := strings.TrimPrefix(chart.Context, "snmp.trap.")
	return charttpl.Chart{
		ID:         chart.ID,
		Title:      chart.Title,
		Family:     chart.Family,
		Context:    context,
		Units:      chart.Units,
		Algorithm:  chart.Algorithm,
		Type:       chart.Type,
		Instances:  &charttpl.Instances{ByLabels: byLabels},
		Lifecycle:  chart.Lifecycle,
		Dimensions: dims,
	}
}
