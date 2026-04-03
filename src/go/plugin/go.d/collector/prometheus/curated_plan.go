// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/promselector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type collectorRuntime struct {
	compiledProfiles  []compiledProfile
	chartTemplateYAML string
}

type compiledProfile struct {
	key     string
	profile promprofiles.Profile
	match   matcher.Matcher
	blocks  []compiledRelabelBlock
}

type compiledRelabelBlock struct {
	selector  promselector.Selector
	processor *relabel.Processor
}

func buildCollectorRuntimeFromProfiles(profiles []promprofiles.Profile) (*collectorRuntime, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("no curated profiles resolved")
	}

	compiledProfiles, err := compileCuratedProfiles(profiles)
	if err != nil {
		return nil, err
	}

	templateYAML, err := buildCuratedChartTemplateYAML(profiles)
	if err != nil {
		return nil, err
	}

	return &collectorRuntime{
		compiledProfiles:  compiledProfiles,
		chartTemplateYAML: templateYAML,
	}, nil
}

func compileCuratedProfiles(profiles []promprofiles.Profile) ([]compiledProfile, error) {
	seen := make(map[string]struct{}, len(profiles))
	compiled := make([]compiledProfile, 0, len(profiles))

	for _, prof := range profiles {
		key := promprofiles.NormalizeProfileKey(prof.Name)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("profile name collision for profile %q", prof.Name)
		}
		seen[key] = struct{}{}

		cp, err := compileProfile(prof)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, cp)
	}

	return compiled, nil
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
