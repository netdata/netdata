// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"gopkg.in/yaml.v3"
)

func buildCollectorRuntimeFromConfig(profileNames []string, profileEntries map[string]ProfileEntryConfig, catalog azureprofiles.Catalog) (*collectorRuntime, error) {
	profiles, err := catalog.Resolve(profileNames)
	if err != nil {
		return nil, err
	}

	runtime := &collectorRuntime{
		Profiles: make([]*profileRuntime, 0, len(profiles)),
	}

	seenProfileNames := make(map[string]struct{}, len(profiles))
	seenChartIDs := make(map[string]struct{})

	for _, src := range profiles {
		p, err := buildProfileRuntime(src, profileEntries[src.Name])
		if err != nil {
			return nil, err
		}
		if _, ok := seenProfileNames[p.Name]; ok {
			return nil, fmt.Errorf("profile name collision for profile %q", src.Name)
		}
		seenProfileNames[p.Name] = struct{}{}

		if err := walkCharts(p.Template, func(chart charttpl.Chart) error {
			if _, ok := seenChartIDs[chart.ID]; ok {
				return fmt.Errorf("chart id collision: %s", chart.ID)
			}
			seenChartIDs[chart.ID] = struct{}{}
			return nil
		}); err != nil {
			return nil, err
		}

		runtime.Profiles = append(runtime.Profiles, p)
	}

	tpl, err := buildChartTemplate(runtime)
	if err != nil {
		return nil, err
	}
	runtime.ChartTemplateYAML = tpl

	return runtime, nil
}

func buildProfileRuntime(resolved azureprofiles.ResolvedProfile, entry ProfileEntryConfig) (*profileRuntime, error) {
	profileName := stringsTrim(resolved.Name)
	if profileName == "" {
		return nil, fmt.Errorf("profile has empty name")
	}

	displayName := stringsTrim(resolved.Config.DisplayName)
	if displayName == "" {
		return nil, fmt.Errorf("profile %q has empty display_name", profileName)
	}

	resourceType := stringsTrim(resolved.Config.ResourceType)
	metricNamespace := stringsTrim(resolved.Config.MetricNamespace)
	if metricNamespace == "" {
		metricNamespace = resourceType
	}

	out := &profileRuntime{
		Name:            profileName,
		DisplayName:     displayName,
		ResourceType:    resourceType,
		MetricNamespace: metricNamespace,
		Filters:         cloneResourceFilters(entry.Filters),
		Metrics:         make([]*metricRuntime, 0, len(resolved.Config.Metrics)),
	}

	for _, m := range resolved.Config.Metrics {
		grain := strings.ToUpper(stringsTrim(m.TimeGrain))
		if grain == "" {
			grain = "PT1M"
		}
		grainEvery, ok := azureprofiles.SupportedTimeGrains[grain]
		if !ok {
			return nil, fmt.Errorf("profile %q metric %q has unsupported time grain %q", profileName, m.ID, grain)
		}

		mr := &metricRuntime{
			ID:             stringsTrim(m.ID),
			AzureName:      stringsTrim(m.AzureName),
			TimeGrain:      grain,
			TimeGrainEvery: grainEvery,
			Series:         make([]*seriesRuntime, 0, len(m.Series)),
		}

		for _, s := range m.Series {
			aggregation := azureprofiles.NormalizeAggregation(s.Aggregation)
			mr.Series = append(mr.Series, &seriesRuntime{
				Aggregation: aggregation,
				Kind:        azureprofiles.NormalizeSeriesKind(s.Kind),
				Instrument:  azureprofiles.ExportedSeriesName(profileName, m.ID, aggregation),
			})
		}

		sort.Slice(mr.Series, func(i, j int) bool {
			return mr.Series[i].Aggregation < mr.Series[j].Aggregation
		})
		out.Metrics = append(out.Metrics, mr)
	}

	sort.Slice(out.Metrics, func(i, j int) bool {
		return out.Metrics[i].ID < out.Metrics[j].ID
	})

	out.Template = resolved.Config.Template
	out.Template.Metrics = profileMetricsList(out)
	return out, nil
}

func buildChartTemplate(runtime *collectorRuntime) (string, error) {
	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: "azure_monitor",
		Groups:           make([]charttpl.Group, 0, len(runtime.Profiles)),
	}

	for _, p := range runtime.Profiles {
		spec.Groups = append(spec.Groups, p.Template)
	}

	if err := spec.Validate(); err != nil {
		return "", err
	}

	raw, err := yaml.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func profileMetricsList(p *profileRuntime) []string {
	list := make([]string, 0)
	for _, metric := range p.Metrics {
		for _, series := range metric.Series {
			list = append(list, series.Instrument)
		}
	}
	sort.Strings(list)
	return list
}

func walkCharts(group charttpl.Group, fn func(charttpl.Chart) error) error {
	for _, chart := range group.Charts {
		if err := fn(chart); err != nil {
			return err
		}
	}
	for _, child := range group.Groups {
		if err := walkCharts(child, fn); err != nil {
			return err
		}
	}
	return nil
}
