// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"gopkg.in/yaml.v3"
)

func (c *Collector) buildRuntimePlan() (*runtimePlan, error) {
	return c.buildRuntimePlanFromConfig(c.Config, c.profiles)
}

func (c *Collector) buildRuntimePlanFromConfig(cfg Config, catalog profileCatalog) (*runtimePlan, error) {
	profiles, err := catalog.resolve(cfg.Profiles)
	if err != nil {
		return nil, err
	}

	plan := &runtimePlan{
		Profiles: make([]*profileRuntime, 0, len(profiles)),
	}

	seenProfileKeys := make(map[string]struct{}, len(profiles))
	seenChartIDs := make(map[string]struct{})

	for _, src := range profiles {
		p, err := buildProfileRuntime(src)
		if err != nil {
			return nil, err
		}
		if _, ok := seenProfileKeys[p.Key]; ok {
			return nil, fmt.Errorf("profile key collision for profile %q", src.Name)
		}
		seenProfileKeys[p.Key] = struct{}{}

		for _, ch := range p.Charts {
			if _, ok := seenChartIDs[ch.ID]; ok {
				return nil, fmt.Errorf("chart id collision: %s", ch.ID)
			}
			seenChartIDs[ch.ID] = struct{}{}
		}

		plan.Profiles = append(plan.Profiles, p)
	}

	tpl, err := buildChartTemplate(plan)
	if err != nil {
		return nil, err
	}
	plan.ChartTemplateYAML = tpl

	return plan, nil
}

func buildProfileRuntime(p ProfileConfig) (*profileRuntime, error) {
	name := stringsTrim(p.Name)
	if name == "" {
		return nil, fmt.Errorf("profile has empty name")
	}

	resourceType := stringsTrim(p.ResourceType)
	metricNamespace := stringsTrim(p.MetricNamespace)
	if metricNamespace == "" {
		metricNamespace = resourceType
	}

	profileKey := profileKeyFromName(name)
	out := &profileRuntime{
		Key:             profileKey,
		Name:            name,
		ResourceType:    resourceType,
		MetricNamespace: metricNamespace,
		Metrics:         make([]*metricRuntime, 0, len(p.Metrics)),
		Charts:          make([]charttpl.Chart, 0, len(p.Charts)),
	}

	seenMetricKeys := map[string]struct{}{}
	metricByName := make(map[string]*metricRuntime, len(p.Metrics))
	for _, m := range p.Metrics {
		metricName := stringsTrim(m.Name)
		if metricName == "" {
			return nil, fmt.Errorf("profile %q contains a metric with empty name", name)
		}
		metricKey := metricKeyFromName(metricName)
		if _, ok := seenMetricKeys[metricKey]; ok {
			return nil, fmt.Errorf("profile %q contains duplicate metric %q", name, metricName)
		}
		seenMetricKeys[metricKey] = struct{}{}

		aggs := normalizeAggregations(m.Aggregations)
		grain := strings.ToUpper(stringsTrim(m.TimeGrain))
		if grain == "" {
			grain = "PT1M"
		}
		grainEvery, ok := supportedTimeGrains[grain]
		if !ok {
			return nil, fmt.Errorf("profile %q metric %q has unsupported time grain %q", name, metricName, grain)
		}

		units := stringsTrim(m.Units)
		if units == "" {
			units = "value"
		}

		instrumentByAgg := make(map[string]string, len(aggs))
		for _, agg := range aggs {
			instrumentByAgg[agg] = metricInstrumentName(profileKey, metricKey, agg)
		}

		mr := &metricRuntime{
			Name:            metricName,
			Units:           units,
			TimeGrain:       grain,
			TimeGrainEvery:  grainEvery,
			Aggregations:    aggs,
			InstrumentByAgg: instrumentByAgg,
		}
		out.Metrics = append(out.Metrics, mr)
		metricByName[stringsLowerTrim(metricName)] = mr
	}

	for i, ch := range p.Charts {
		compiled, err := buildProfileChart(name, ch, metricByName)
		if err != nil {
			return nil, fmt.Errorf("profile %q chart[%d]: %w", name, i, err)
		}
		out.Charts = append(out.Charts, compiled)
	}

	sort.Slice(out.Metrics, func(i, j int) bool {
		return out.Metrics[i].Name < out.Metrics[j].Name
	})
	sort.Slice(out.Charts, func(i, j int) bool {
		return out.Charts[i].ID < out.Charts[j].ID
	})

	return out, nil
}

func buildProfileChart(profileName string, ch ProfileChart, metricByName map[string]*metricRuntime) (charttpl.Chart, error) {
	dims := make([]charttpl.Dimension, 0, len(ch.Dimensions))
	for i, dim := range ch.Dimensions {
		m, ok := metricByName[stringsLowerTrim(dim.Metric)]
		if !ok {
			return charttpl.Chart{}, fmt.Errorf("dimension[%d] references unknown metric %q", i, dim.Metric)
		}

		agg := normalizeAggregation(dim.Aggregation)
		if agg == "" {
			return charttpl.Chart{}, fmt.Errorf("dimension[%d] has invalid aggregation %q", i, dim.Aggregation)
		}

		selector, ok := m.InstrumentByAgg[agg]
		if !ok {
			return charttpl.Chart{}, fmt.Errorf(
				"dimension[%d] references aggregation %q not defined for metric %q in profile %q",
				i, agg, dim.Metric, profileName,
			)
		}

		dims = append(dims, charttpl.Dimension{
			Selector:      selector,
			Name:          stringsTrim(dim.Name),
			NameFromLabel: stringsTrim(dim.NameFromLabel),
			Options:       dim.Options,
		})
	}

	algo := stringsLowerTrim(ch.Algorithm)
	if algo == "" {
		algo = "absolute"
	}

	return charttpl.Chart{
		ID:            stringsTrim(ch.ID),
		Title:         stringsTrim(ch.Title),
		Family:        stringsTrim(ch.Family),
		Context:       stringsTrim(ch.Context),
		Units:         stringsTrim(ch.Units),
		Algorithm:     algo,
		Type:          stringsLowerTrim(ch.Type),
		Priority:      ch.Priority,
		LabelPromoted: append([]string(nil), ch.LabelPromoted...),
		Instances:     ch.Instances,
		Lifecycle:     ch.Lifecycle,
		Dimensions:    dims,
	}, nil
}

func buildChartTemplate(plan *runtimePlan) (string, error) {
	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: "azure_monitor",
		Groups:           make([]charttpl.Group, 0, len(plan.Profiles)),
	}

	for _, p := range plan.Profiles {
		grp := charttpl.Group{
			Family:  p.Name,
			Metrics: profileMetricsList(p),
			Charts:  append([]charttpl.Chart(nil), p.Charts...),
		}

		spec.Groups = append(spec.Groups, grp)
	}

	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func profileMetricsList(p *profileRuntime) []string {
	list := make([]string, 0, len(p.Metrics)*2)
	for _, m := range p.Metrics {
		for _, agg := range m.Aggregations {
			list = append(list, m.InstrumentByAgg[agg])
		}
	}
	sort.Strings(list)
	return list
}

func profileKeyFromName(name string) string {
	return encodeIDPart(name)
}

func metricKeyFromName(name string) string {
	return encodeIDPart(name)
}

func metricInstrumentName(profileKey, metricKey, aggregation string) string {
	return "metric_" + profileKey + "__" + metricKey + "__" + encodeIDPart(aggregation)
}
