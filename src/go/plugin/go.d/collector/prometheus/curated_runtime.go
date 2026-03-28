// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"strings"

	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type typedFamilyKey struct {
	name string
	hash uint64
}

func (c *Collector) processScrapeBatch(batch *scrapeBatch, checking bool) (prompkg.MetricFamilies, *collectorRuntime, error) {
	runtime := c.runtime
	if checking {
		var err error
		runtime, err = c.buildRuntimeForCheck(batch.samples)
		if err != nil {
			return nil, nil, err
		}
	}

	if runtime == nil || len(runtime.compiledProfiles) == 0 {
		mfs, err := assembleMetricFamilies(batch)
		return mfs, runtime, err
	}

	processed, err := c.applyCuratedRuntime(batch, runtime, checking)
	if err != nil {
		return nil, runtime, err
	}

	mfs, err := assembleMetricFamilies(processed)
	return mfs, runtime, err
}

func (c *Collector) buildRuntimeForCheck(samples promscrapemodel.Samples) (*collectorRuntime, error) {
	profiles, err := c.selectProfilesForCheck(samples)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	return buildCollectorRuntimeFromProfiles(profiles)
}

func (c *Collector) selectProfilesForCheck(samples promscrapemodel.Samples) ([]promprofiles.Profile, error) {
	switch c.ProfileSelectionMode {
	case profileSelectionModeAuto:
		return c.autoMatchedProfiles(samples)
	case profileSelectionModeExact:
		profiles, err := c.profileCatalog.Resolve(c.Profiles)
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
		explicit, err := c.profileCatalog.Resolve(c.Profiles)
		if err != nil {
			return nil, err
		}

		selected := append([]promprofiles.Profile(nil), explicit...)
		seen := make(map[string]struct{}, len(selected))
		for _, prof := range selected {
			seen[promprofiles.NormalizeProfileKey(prof.Name)] = struct{}{}
		}

		auto, err := c.autoMatchedProfiles(samples)
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
		return nil, fmt.Errorf("unsupported profile selection mode %q", c.ProfileSelectionMode)
	}
}

func (c *Collector) autoMatchedProfiles(samples promscrapemodel.Samples) ([]promprofiles.Profile, error) {
	ordered := c.profileCatalog.OrderedProfiles()
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

func profileMatchesSamples(prof promprofiles.Profile, samples promscrapemodel.Samples) (bool, error) {
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

func (c *Collector) applyCuratedRuntime(batch *scrapeBatch, runtime *collectorRuntime, checking bool) (*scrapeBatch, error) {
	out := &scrapeBatch{
		help:    make([]helpEntry, 0, len(batch.help)),
		samples: make(promscrapemodel.Samples, 0, len(batch.samples)),
	}

	familyOwners := make(map[typedFamilyKey]string)
	helpTargets := make(map[string][]string)
	helpTargetSeen := make(map[string]map[string]struct{})

	for _, raw := range batch.samples {
		owner, drop, err := c.resolveSampleOwner(raw, runtime.compiledProfiles, checking)
		if err != nil {
			return nil, err
		}
		if drop {
			continue
		}

		if conflict, err := c.checkTypedFamilyOwnership(raw, owner, familyOwners, checking); err != nil {
			return nil, err
		} else if conflict {
			continue
		}

		sample := raw
		if owner != nil {
			var keep bool
			sample, keep = owner.apply(sample)
			if !keep {
				continue
			}
		}

		out.samples.Add(sample)
		addHelpTarget(helpTargets, helpTargetSeen, helpFamilyName(raw), helpFamilyName(sample))
	}

	for _, entry := range batch.help {
		targets := helpTargets[entry.name]
		for _, target := range targets {
			out.help = append(out.help, helpEntry{name: target, help: entry.help})
		}
	}

	return out, nil
}

func (c *Collector) resolveSampleOwner(sample promscrapemodel.Sample, profiles []compiledProfile, checking bool) (*compiledProfile, bool, error) {
	var owner *compiledProfile
	for i := range profiles {
		if !profiles[i].match.MatchString(sample.Name) {
			continue
		}
		if owner != nil {
			msg := fmt.Sprintf("sample %q matches multiple selected profiles (%q, %q)", sample.Name, owner.profile.Name, profiles[i].profile.Name)
			if checking {
				return nil, false, errors.New(msg)
			}
			c.Warning(msg)
			return nil, true, nil
		}
		owner = &profiles[i]
	}
	return owner, false, nil
}

func (c *Collector) checkTypedFamilyOwnership(sample promscrapemodel.Sample, owner *compiledProfile, seen map[typedFamilyKey]string, checking bool) (bool, error) {
	key, ok := typedFamilyOwnershipKey(sample)
	if !ok {
		return false, nil
	}

	ownerKey := ""
	ownerName := "<generic>"
	if owner != nil {
		ownerKey = owner.key
		ownerName = owner.profile.Name
	}

	if prev, ok := seen[key]; ok && prev != ownerKey {
		msg := fmt.Sprintf("logical typed family %q is split across profile ownership (%q vs %q)", key.name, displayProfileKey(prev), ownerName)
		if checking {
			return false, errors.New(msg)
		}
		c.Warning(msg)
		return true, nil
	}

	seen[key] = ownerKey
	return false, nil
}

func displayProfileKey(prevKey string) string {
	if prevKey == "" {
		return "<generic>"
	}
	return prevKey
}

func (p *compiledProfile) apply(sample promscrapemodel.Sample) (promscrapemodel.Sample, bool) {
	if len(p.blocks) == 0 {
		return sample, true
	}

	current := sample
	for _, block := range p.blocks {
		if block.selector != nil && !block.selector.Matches(sampleSelectorLabels(current)) {
			continue
		}
		next, keep := block.processor.Apply(current)
		if !keep {
			return promscrapemodel.Sample{}, false
		}
		current = next
	}
	return current, true
}

func sampleSelectorLabels(sample promscrapemodel.Sample) promlabels.Labels {
	lbs := make(promlabels.Labels, 0, len(sample.Labels)+1)
	lbs = append(lbs, promlabels.Label{Name: promlabels.MetricName, Value: sample.Name})
	lbs = append(lbs, sample.Labels...)
	return lbs
}

func typedFamilyOwnershipKey(sample promscrapemodel.Sample) (typedFamilyKey, bool) {
	switch sample.Kind {
	case promscrapemodel.SampleKindHistogramBucket:
		base := labelsWithout(sample.Labels, "le")
		return typedFamilyKey{name: trimFamilySuffix(sample.Name, sample.Kind), hash: base.Hash()}, true
	case promscrapemodel.SampleKindSummaryQuantile:
		base := labelsWithout(sample.Labels, "quantile")
		return typedFamilyKey{name: trimFamilySuffix(sample.Name, sample.Kind), hash: base.Hash()}, true
	case promscrapemodel.SampleKindHistogramSum,
		promscrapemodel.SampleKindHistogramCount,
		promscrapemodel.SampleKindSummarySum,
		promscrapemodel.SampleKindSummaryCount:
		return typedFamilyKey{name: trimFamilySuffix(sample.Name, sample.Kind), hash: sample.Labels.Hash()}, true
	default:
		if sample.FamilyType != commonmodel.MetricTypeSummary && sample.FamilyType != commonmodel.MetricTypeHistogram {
			return typedFamilyKey{}, false
		}
		return typedFamilyKey{name: sample.Name, hash: sample.Labels.Hash()}, true
	}
}

func helpFamilyName(sample promscrapemodel.Sample) string {
	return trimFamilySuffix(sample.Name, sample.Kind)
}

func trimFamilySuffix(name string, kind promscrapemodel.SampleKind) string {
	switch kind {
	case promscrapemodel.SampleKindHistogramBucket:
		return trimSuffix(name, "_bucket")
	case promscrapemodel.SampleKindHistogramSum, promscrapemodel.SampleKindSummarySum:
		return trimSuffix(name, "_sum")
	case promscrapemodel.SampleKindHistogramCount, promscrapemodel.SampleKindSummaryCount:
		return trimSuffix(name, "_count")
	default:
		return name
	}
}

func trimSuffix(name, suffix string) string {
	return strings.TrimSuffix(name, suffix)
}

func labelsWithout(lbs promlabels.Labels, name string) promlabels.Labels {
	out := make(promlabels.Labels, 0, len(lbs))
	for _, lb := range lbs {
		if lb.Name == name {
			continue
		}
		out = append(out, lb)
	}
	return out
}

func addHelpTarget(targets map[string][]string, seen map[string]map[string]struct{}, source, target string) {
	if source == "" || target == "" {
		return
	}
	set, ok := seen[source]
	if !ok {
		set = make(map[string]struct{})
		seen[source] = set
	}
	if _, ok := set[target]; ok {
		return
	}
	set[target] = struct{}{}
	targets[source] = append(targets[source], target)
}
