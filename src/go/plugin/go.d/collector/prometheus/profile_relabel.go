// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"sort"
	"strings"
	"time"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

type profileOwnerConflict struct {
	family   string
	profiles []string
}

type profileScopeEscape struct {
	profile string
	source  string
	target  string
}

func (c *Collector) profileRelabelAndAssemble(
	batch prompkg.SampleBatch,
	normalizers []profileNormalizer,
	checking bool,
) (prompkg.SampleBatch, prompkg.MetricFamilies, error) {
	owners, conflicts := resolveProfileOwners(batch, normalizers)
	if checking && len(conflicts) > 0 {
		return prompkg.SampleBatch{}, nil, profileConflictError(conflicts)
	}

	conflictSources := make(map[string]struct{}, len(conflicts))
	invalidSources := make(map[string]struct{}, len(conflicts))
	if len(conflicts) > 0 {
		for _, conflict := range conflicts {
			conflictSources[conflict.family] = struct{}{}
			invalidSources[conflict.family] = struct{}{}
		}
		c.Limit("profile-relabel-ownership-conflict", 1, 10*time.Minute).
			Warningf("profile relabeling found %d ownership conflict(s); dropped the affected source families (enable debug for names)", len(conflicts))
		for _, conflict := range conflicts {
			c.Debugf("profile relabeling dropped source family %q: matched profiles %s", conflict.family, strings.Join(conflict.profiles, ", "))
		}
	}

	results := make([]relabelResult, 0, len(batch.Samples))
	sourceFamilies := make([]string, 0, len(batch.Samples))
	escapeSet := make(map[profileScopeEscape]struct{})
	for _, raw := range batch.Samples {
		source := helpFamilyName(raw)
		result := relabelResult{raw: raw, sample: raw}
		if _, conflict := conflictSources[source]; conflict {
			result.discard = true
			results = append(results, result)
			sourceFamilies = append(sourceFamilies, source)
			continue
		}

		if owner := owners[source]; owner != nil {
			result.sample, result.drop = owner.pipeline.Apply(raw)
			if !result.drop.Dropped() {
				target := helpFamilyName(result.sample)
				if !owner.root.MatchString(target) {
					escape := profileScopeEscape{profile: owner.name, source: source, target: target}
					escapeSet[escape] = struct{}{}
					invalidSources[source] = struct{}{}
				}
			}
		}
		results = append(results, result)
		sourceFamilies = append(sourceFamilies, source)
	}

	escapes := sortedProfileScopeEscapes(escapeSet)
	if checking && len(escapes) > 0 {
		return prompkg.SampleBatch{}, nil, profileScopeError(escapes)
	}
	if len(escapes) > 0 {
		c.Limit("profile-relabel-scope-escape", 1, 10*time.Minute).
			Warningf("profile relabeling produced %d namespace escape(s); dropped the affected source families (enable debug for names)", len(escapes))
		for _, escape := range escapes {
			c.Debugf("profile %q relabeling dropped source family %q: output family %q is outside its match", escape.profile, escape.source, escape.target)
		}
	}

	if len(invalidSources) > 0 {
		for i, source := range sourceFamilies {
			if _, invalid := invalidSources[source]; invalid {
				results[i].discard = true
			}
		}
	}

	processed, tracking := c.materializeRelabel(batch.Help, results, profileRelabelStage)
	return c.assembleRelabeled(processed, tracking, profileRelabelStage, checking)
}

func resolveProfileOwners(batch prompkg.SampleBatch, normalizers []profileNormalizer) (map[string]*profileNormalizer, []profileOwnerConflict) {
	families := make(map[string][]string)
	for _, sample := range batch.Samples {
		base := helpFamilyName(sample)
		families[base] = append(families[base], sample.Name)
	}

	owners := make(map[string]*profileNormalizer, len(families))
	var conflicts []profileOwnerConflict
	for base, physicalNames := range families {
		var candidates []*profileNormalizer
		for i := range normalizers {
			normalizer := &normalizers[i]
			if !normalizer.root.MatchString(base) {
				continue
			}
			for _, physicalName := range physicalNames {
				if normalizer.pipeline.Matches(physicalName) {
					candidates = append(candidates, normalizer)
					break
				}
			}
		}

		switch len(candidates) {
		case 0:
		case 1:
			owners[base] = candidates[0]
		default:
			names := make([]string, len(candidates))
			for i, candidate := range candidates {
				names[i] = candidate.name
			}
			sort.Strings(names)
			conflicts = append(conflicts, profileOwnerConflict{family: base, profiles: names})
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].family < conflicts[j].family })
	return owners, conflicts
}

func profileConflictError(conflicts []profileOwnerConflict) error {
	conflict := conflicts[0]
	return fmt.Errorf("profile relabeling ownership conflict for source family %q: profiles %s", conflict.family, strings.Join(conflict.profiles, ", "))
}

func sortedProfileScopeEscapes(set map[profileScopeEscape]struct{}) []profileScopeEscape {
	escapes := make([]profileScopeEscape, 0, len(set))
	for escape := range set {
		escapes = append(escapes, escape)
	}
	sort.Slice(escapes, func(i, j int) bool {
		if escapes[i].source != escapes[j].source {
			return escapes[i].source < escapes[j].source
		}
		if escapes[i].profile != escapes[j].profile {
			return escapes[i].profile < escapes[j].profile
		}
		return escapes[i].target < escapes[j].target
	})
	return escapes
}

func profileScopeError(escapes []profileScopeEscape) error {
	escape := escapes[0]
	return fmt.Errorf("profile %q relabeling moves source family %q to output family %q outside profile match", escape.profile, escape.source, escape.target)
}
