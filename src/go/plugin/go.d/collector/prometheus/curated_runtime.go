// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"strings"

	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type typedFamilyKey struct {
	name string
	hash uint64
}

type relabeledTypedFamilyState struct {
	ownerName     string
	touched       bool
	keptAny       bool
	hasFinalKey   bool
	split         bool
	finalKey      typedFamilyKey
	outputIndexes []int
}

func (c *Collector) processScrapeBatch(batch *scrapeBatch, checking bool) (promscrapemodel.MetricFamilies, *collectorRuntime, error) {
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

	processed, typedFamilies, err := c.applyCuratedRuntime(batch, runtime, checking)
	if err != nil {
		return nil, runtime, err
	}

	mfs, err := assembleMetricFamilies(processed)
	if err != nil {
		return nil, runtime, err
	}

	invalid, err := c.validateRelabeledTypedFamilies(typedFamilies, mfs, checking)
	if err != nil {
		return nil, runtime, err
	}
	if len(invalid) == 0 {
		return mfs, runtime, nil
	}

	filtered := filterInvalidTypedFamilySamples(processed, typedFamilies, invalid)
	mfs, err = assembleMetricFamilies(filtered)
	if err != nil {
		return nil, runtime, err
	}

	return mfs, runtime, nil
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

func (c *Collector) applyCuratedRuntime(batch *scrapeBatch, runtime *collectorRuntime, checking bool) (*scrapeBatch, map[typedFamilyKey]*relabeledTypedFamilyState, error) {
	out := &scrapeBatch{
		help:    make([]helpEntry, 0, len(batch.help)),
		samples: make(promscrapemodel.Samples, 0, len(batch.samples)),
	}

	familyOwners := make(map[typedFamilyKey]string)
	relabeledFamilies := make(map[typedFamilyKey]*relabeledTypedFamilyState)
	helpTargets := make(map[string][]string)
	helpTargetSeen := make(map[string]map[string]struct{})

	for _, raw := range batch.samples {
		rawKey, rawTyped := typedFamilyOwnershipKey(raw)
		owner, drop, err := c.resolveSampleOwner(raw, runtime.compiledProfiles, checking)
		if err != nil {
			return nil, nil, err
		}
		if drop {
			continue
		}

		if conflict, err := c.checkTypedFamilyOwnership(raw, owner, familyOwners, checking); err != nil {
			return nil, nil, err
		} else if conflict {
			continue
		}

		sample := raw
		if owner != nil {
			var (
				keep    bool
				touched bool
			)
			sample, keep, touched = owner.apply(sample)
			if rawTyped && len(owner.blocks) > 0 {
				recordRelabeledTypedFamily(relabeledFamilies, rawKey, owner.profile.Name, touched, keep, sample, len(out.samples))
			}
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

	return out, relabeledFamilies, nil
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

func (p *compiledProfile) apply(sample promscrapemodel.Sample) (promscrapemodel.Sample, bool, bool) {
	if len(p.blocks) == 0 {
		return sample, true, false
	}

	current := sample
	touched := false
	for _, block := range p.blocks {
		if block.selector != nil && !block.selector.Matches(sampleSelectorLabels(current)) {
			continue
		}
		next, keep := block.processor.Apply(current)
		if !keep {
			return promscrapemodel.Sample{}, false, true
		}
		if next.Name != current.Name || !promlabels.Equal(next.Labels, current.Labels) {
			touched = true
		}
		current = next
	}
	return current, true, touched
}

func recordRelabeledTypedFamily(states map[typedFamilyKey]*relabeledTypedFamilyState, rawKey typedFamilyKey, ownerName string, touched, keep bool, sample promscrapemodel.Sample, outputIndex int) {
	state := states[rawKey]
	if state == nil {
		state = &relabeledTypedFamilyState{ownerName: ownerName}
		states[rawKey] = state
	}
	state.touched = state.touched || touched
	if !keep {
		return
	}

	finalKey, ok := typedFamilyOwnershipKey(sample)
	if !ok {
		return
	}

	state.keptAny = true
	if !state.hasFinalKey {
		state.finalKey = finalKey
		state.hasFinalKey = true
	} else if state.finalKey != finalKey {
		state.split = true
	}
	state.outputIndexes = append(state.outputIndexes, outputIndex)
}

func (c *Collector) validateRelabeledTypedFamilies(states map[typedFamilyKey]*relabeledTypedFamilyState, mfs promscrapemodel.MetricFamilies, checking bool) (map[typedFamilyKey]struct{}, error) {
	if len(states) == 0 {
		return nil, nil
	}

	assembled := assembledTypedFamilyKeys(mfs)
	invalid := make(map[typedFamilyKey]struct{})

	for rawKey, state := range states {
		if state == nil || !state.touched {
			continue
		}

		var msg string
		switch {
		case state.split:
			msg = fmt.Sprintf("curated relabel splits logical typed family %q within profile %q after ownership resolution", rawKey.name, state.ownerName)
		case !state.keptAny:
			continue
		case !state.hasFinalKey:
			continue
		case !assembledHasTypedFamily(assembled, state.finalKey):
			msg = fmt.Sprintf("curated relabel leaves logical typed family %q within profile %q without a valid assembled family", rawKey.name, state.ownerName)
		default:
			continue
		}

		if checking {
			return nil, errors.New(msg)
		}
		c.Warning(msg)
		invalid[rawKey] = struct{}{}
	}

	return invalid, nil
}

func assembledTypedFamilyKeys(mfs promscrapemodel.MetricFamilies) map[typedFamilyKey]struct{} {
	keys := make(map[typedFamilyKey]struct{})
	for _, mf := range mfs {
		if mf == nil {
			continue
		}
		if mf.Type() != commonmodel.MetricTypeSummary && mf.Type() != commonmodel.MetricTypeHistogram {
			continue
		}
		for _, metric := range mf.Metrics() {
			keys[typedFamilyKey{name: mf.Name(), hash: metric.Labels().Hash()}] = struct{}{}
		}
	}
	return keys
}

func assembledHasTypedFamily(keys map[typedFamilyKey]struct{}, key typedFamilyKey) bool {
	_, ok := keys[key]
	return ok
}

func filterInvalidTypedFamilySamples(batch *scrapeBatch, states map[typedFamilyKey]*relabeledTypedFamilyState, invalid map[typedFamilyKey]struct{}) *scrapeBatch {
	if len(invalid) == 0 {
		return batch
	}

	dropIndexes := make(map[int]struct{})
	for rawKey := range invalid {
		state := states[rawKey]
		if state == nil {
			continue
		}
		for _, idx := range state.outputIndexes {
			dropIndexes[idx] = struct{}{}
		}
	}
	if len(dropIndexes) == 0 {
		return batch
	}

	filtered := &scrapeBatch{
		help:    append([]helpEntry(nil), batch.help...),
		samples: make(promscrapemodel.Samples, 0, len(batch.samples)-len(dropIndexes)),
	}
	for i, sample := range batch.samples {
		if _, drop := dropIndexes[i]; drop {
			continue
		}
		filtered.samples.Add(sample)
	}
	return filtered
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
