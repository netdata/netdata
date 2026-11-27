// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

func buildCollectorRuntimeFromConfig(profileNames []string, profileEntries map[string]ProfileEntryConfig, catalog azureprofiles.Catalog, workloadResourceTagKey string, maxMetrics int) (*collectorRuntime, error) {
	profiles, err := catalog.Resolve(profileNames)
	if err != nil {
		return nil, err
	}

	runtime := &collectorRuntime{
		Profiles:               make([]*profileRuntime, 0, len(profiles)),
		WorkloadResourceTagKey: stringsLowerTrim(workloadResourceTagKey),
	}

	seenProfileNames := make(map[string]struct{}, len(profiles))
	seenChartIDs := make(map[string]struct{})

	for _, src := range profiles {
		p, err := buildProfileRuntime(src, profileEntries[src.Name], maxMetrics)
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

func buildProfileRuntime(resolved azureprofiles.ResolvedProfile, entry ProfileEntryConfig, maxMetrics int) (*profileRuntime, error) {
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
		namespaceKey:    strings.ToLower(metricNamespace),
		Filters:         cloneResourceFilters(entry.Filters),
	}

	groups := make(map[metricQueryGroup][]*metricRuntime)
	for _, m := range resolved.Config.Metrics {
		grain := strings.ToUpper(stringsTrim(m.TimeGrain))
		if grain == "" {
			grain = "PT1M"
		}
		if _, ok := azureprofiles.SupportedTimeGrains[grain]; !ok {
			return nil, fmt.Errorf("profile %q metric %q has unsupported time grain %q", profileName, m.ID, grain)
		}
		for _, s := range m.Series {
			aggregation := azureprofiles.NormalizeAggregation(s.Aggregation)
			group := metricQueryGroup{TimeGrain: grain, Aggregation: aggregation}
			groups[group] = append(groups[group], &metricRuntime{
				ID: stringsTrim(m.ID), AzureName: stringsTrim(m.AzureName),
				Aggregation: aggregation, Kind: azureprofiles.NormalizeSeriesKind(s.Kind),
				Instrument: azureprofiles.ExportedSeriesName(profileName, m.ID, aggregation),
			})
		}
	}
	for group, metrics := range groups {
		sort.Slice(metrics, func(i, j int) bool { return metrics[i].ID < metrics[j].ID })
		for part := range slices.Chunk(metrics, maxMetrics) {
			out.chunks = append(out.chunks, &metricChunk{profile: out, metricQueryGroup: group, metrics: part, names: batchMetricNames(part)})
		}
	}

	for _, label := range resolved.Config.ResourceLabels {
		name := stringsTrim(label.Name)
		out.ResourceLabels = append(out.ResourceLabels, azureprofiles.ResourceLabel{
			Name:     name,
			Property: stringsTrim(label.Property),
			Value:    azureprofiles.NormalizeResourceLabelValue(label.Value),
		})
	}

	for _, state := range resolved.Config.ResourceStates {
		sr := &resourceStateRuntime{
			Name:     azureprofiles.ExportedStateName(profileName, state.ID),
			Property: stringsTrim(state.Property),
			Values:   make(map[string]string, len(state.States)),
		}
		for _, option := range state.States {
			name := stringsTrim(option.Name)
			sr.States = append(sr.States, name)
			sr.Values[stringsLowerTrim(option.Value)] = name
		}
		out.ResourceStates = append(out.ResourceStates, sr)
	}

	out.Template = resolved.Config.Template.Clone()
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

	return spec.MarshalTemplate()
}

func profileMetricsList(p *profileRuntime) []string {
	list := make([]string, 0)
	for _, chunk := range p.chunks {
		for _, series := range chunk.metrics {
			list = append(list, series.Instrument)
		}
	}
	for _, state := range p.ResourceStates {
		list = append(list, state.Name)
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
