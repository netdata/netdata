// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func buildCollectorRuntimeFromProfiles(profiles []promprofiles.Profile) (*collectorRuntime, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no curated profiles resolved")
	}

	runtime := &collectorRuntime{
		Profiles: append([]promprofiles.Profile(nil), profiles...),
	}

	seenProfileIDs := make(map[string]struct{}, len(profiles))
	seenChartIDs := make(map[string]struct{})

	spec := charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: "prometheus",
		Engine: &charttpl.Engine{
			Autogen: &charttpl.EngineAutogen{Enabled: true},
		},
		Groups: make([]charttpl.Group, 0, len(profiles)),
	}

	for _, prof := range profiles {
		if _, ok := seenProfileIDs[prof.ID]; ok {
			return nil, fmt.Errorf("profile id collision for profile %q", prof.ID)
		}
		seenProfileIDs[prof.ID] = struct{}{}

		if err := walkCharts(prof.Template, func(chart charttpl.Chart) error {
			if _, ok := seenChartIDs[chart.ID]; ok {
				return fmt.Errorf("chart id collision: %s", chart.ID)
			}
			seenChartIDs[chart.ID] = struct{}{}
			return nil
		}); err != nil {
			return nil, err
		}

		spec.Groups = append(spec.Groups, prof.Template)
	}

	if err := spec.Validate(); err != nil {
		return nil, err
	}

	raw, err := yaml.Marshal(spec)
	if err != nil {
		return nil, err
	}

	runtime.ChartTemplateYAML = string(raw)
	return runtime, nil
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
