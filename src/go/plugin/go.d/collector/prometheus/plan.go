// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

func buildCollectorRuntimeFromProfiles(profiles []promprofiles.Profile) (*collectorRuntime, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no curated profiles resolved")
	}

	runtime := &collectorRuntime{
		Profiles: append([]promprofiles.Profile(nil), profiles...),
	}

	seenProfileNames := make(map[string]struct{}, len(profiles))
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
		if _, ok := seenProfileNames[promprofiles.NormalizeProfileKey(prof.Name)]; ok {
			return nil, fmt.Errorf("profile name collision for profile %q", prof.Name)
		}
		seenProfileNames[promprofiles.NormalizeProfileKey(prof.Name)] = struct{}{}

		compiled, err := compileProfile(prof)
		if err != nil {
			return nil, err
		}
		runtime.compiledProfiles = append(runtime.compiledProfiles, compiled)

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

func compileProfile(prof promprofiles.Profile) (compiledProfile, error) {
	match, err := compileProfileMatch(prof)
	if err != nil {
		return compiledProfile{}, err
	}

	cp := compiledProfile{
		key:     promprofiles.NormalizeProfileKey(prof.Name),
		profile: prof,
		match:   match,
		blocks:  make([]compiledRelabelBlock, 0, len(prof.MetricsRelabeling)),
	}
	for i, block := range prof.MetricsRelabeling {
		sel, err := promselector.Parse(block.Selector)
		if err != nil {
			return compiledProfile{}, fmt.Errorf("compile profile %q metrics_relabeling[%d].selector: %w", prof.Name, i, err)
		}
		proc, err := relabel.New(block.Rules)
		if err != nil {
			return compiledProfile{}, fmt.Errorf("compile profile %q metrics_relabeling[%d].rules: %w", prof.Name, i, err)
		}
		cp.blocks = append(cp.blocks, compiledRelabelBlock{
			selector:  sel,
			processor: proc,
		})
	}

	return cp, nil
}

func compileProfileMatch(prof promprofiles.Profile) (matcher.Matcher, error) {
	match, err := matcher.NewSimplePatternsMatcher(prof.Match)
	if err != nil {
		return nil, fmt.Errorf("compile profile %q match: %w", prof.Name, err)
	}
	return match, nil
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
