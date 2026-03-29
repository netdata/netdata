// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type curatedPlanner struct {
	profileSelectionMode string
	profiles             []string
	loadProfileCatalog   func() (promprofiles.Catalog, error)
}

func (p *curatedPlanner) buildRuntimeForCheck(samples prompkg.Samples) (*collectorRuntime, error) {
	profiles, err := p.selectProfilesForCheck(samples)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	return buildCollectorRuntimeFromProfiles(profiles)
}

func (p *curatedPlanner) selectProfilesForCheck(samples prompkg.Samples) ([]promprofiles.Profile, error) {
	catalog, err := p.catalog()
	if err != nil {
		return nil, err
	}

	switch p.profileSelectionMode {
	case profileSelectionModeAuto:
		return p.autoMatchedProfiles(catalog, samples)
	case profileSelectionModeExact:
		profiles, err := catalog.Resolve(p.profiles)
		if err != nil {
			return nil, err
		}
		for _, prof := range profiles {
			matched, err := profileMatchesSamples(prof, samples)
			if err != nil {
				return nil, err
			}
			if !matched {
				return nil, fmt.Errorf("selected profile %q matches nothing during probe", prof.Name)
			}
		}
		return profiles, nil
	case profileSelectionModeCombined:
		explicit, err := catalog.Resolve(p.profiles)
		if err != nil {
			return nil, err
		}

		selected := append([]promprofiles.Profile(nil), explicit...)
		seen := make(map[string]struct{}, len(selected))
		for _, prof := range selected {
			seen[promprofiles.NormalizeProfileKey(prof.Name)] = struct{}{}
		}

		auto, err := p.autoMatchedProfiles(catalog, samples)
		if err != nil {
			return nil, err
		}
		for _, prof := range auto {
			key := promprofiles.NormalizeProfileKey(prof.Name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			selected = append(selected, prof)
		}
		return selected, nil
	default:
		return nil, fmt.Errorf("unsupported profile selection mode %q", p.profileSelectionMode)
	}
}

func (p *curatedPlanner) catalog() (promprofiles.Catalog, error) {
	if p.loadProfileCatalog == nil {
		return promprofiles.Catalog{}, nil
	}

	catalog, err := p.loadProfileCatalog()
	if err != nil {
		return promprofiles.Catalog{}, fmt.Errorf("load profiles catalog: %w", err)
	}
	return catalog, nil
}

func (p *curatedPlanner) autoMatchedProfiles(catalog promprofiles.Catalog, samples prompkg.Samples) ([]promprofiles.Profile, error) {
	ordered := catalog.OrderedProfiles()
	if len(ordered) == 0 {
		return nil, nil
	}

	selected := make([]promprofiles.Profile, 0, len(ordered))
	for _, prof := range ordered {
		matched, err := profileMatchesSamples(prof, samples)
		if err != nil {
			return nil, err
		}
		if matched {
			selected = append(selected, prof)
		}
	}
	return selected, nil
}

func profileMatchesSamples(prof promprofiles.Profile, samples prompkg.Samples) (bool, error) {
	match, err := compileProfileMatch(prof)
	if err != nil {
		return false, err
	}
	for _, sample := range samples {
		if match.MatchString(sample.Name) {
			return true, nil
		}
	}
	return false, nil
}
