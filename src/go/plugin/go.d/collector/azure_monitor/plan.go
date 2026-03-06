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

		for _, m := range p.Metrics {
			if _, ok := seenChartIDs[m.ChartID]; ok {
				return nil, fmt.Errorf("chart id collision: %s", m.ChartID)
			}
			seenChartIDs[m.ChartID] = struct{}{}
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
	}

	seenMetricKeys := map[string]struct{}{}
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

		displayName := stringsTrim(m.DisplayName)
		if displayName == "" {
			displayName = metricName
		}
		units := stringsTrim(m.Units)
		if units == "" {
			units = "value"
		}

		instrumentByAgg := make(map[string]string, len(aggs))
		for _, agg := range aggs {
			instrumentByAgg[agg] = metricInstrumentName(profileKey, metricKey, agg)
		}

		out.Metrics = append(out.Metrics, &metricRuntime{
			Name:             metricName,
			DisplayName:      displayName,
			Units:            units,
			TimeGrain:        grain,
			TimeGrainEvery:   grainEvery,
			Aggregations:     aggs,
			InstrumentByAgg:  instrumentByAgg,
			ChartID:          chartIDFor(profileKey, metricKey),
			ChartContextPart: chartContextPart(profileKey, metricKey),
		})
	}

	sort.Slice(out.Metrics, func(i, j int) bool {
		return out.Metrics[i].ChartID < out.Metrics[j].ChartID
	})

	return out, nil
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
			Charts:  make([]charttpl.Chart, 0, len(p.Metrics)),
		}

		for _, m := range p.Metrics {
			dims := make([]charttpl.Dimension, 0, len(m.Aggregations))
			for _, agg := range m.Aggregations {
				dims = append(dims, charttpl.Dimension{
					Selector: m.InstrumentByAgg[agg],
					Name:     agg,
				})
			}

			grp.Charts = append(grp.Charts, charttpl.Chart{
				ID:            m.ChartID,
				Title:         fmt.Sprintf("%s %s", p.Name, m.DisplayName),
				Context:       m.ChartContextPart,
				Units:         m.Units,
				Algorithm:     "absolute",
				LabelPromoted: []string{"resource_name", "resource_group", "region", "resource_type", "profile"},
				Instances: &charttpl.Instances{
					ByLabels: []string{"resource_uid"},
				},
				Dimensions: dims,
			})
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

func chartIDFor(profileKey, metricKey string) string {
	return "am_" + profileKey + "__" + metricKey
}

func chartContextPart(profileKey, metricKey string) string {
	return profileKey + "." + metricKey
}

func metricInstrumentName(profileKey, metricKey, aggregation string) string {
	return "metric_" + profileKey + "__" + metricKey + "__" + encodeIDPart(aggregation)
}
