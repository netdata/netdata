// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
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

type curatedExecutor struct {
	runtime  *collectorRuntime
	checking bool
	*logger.Logger
}

func (c *Collector) processScrapeBatch(batch *scrapeBatch, checking bool) (promscrapemodel.MetricFamilies, *collectorRuntime, error) {
	runtime := c.runtime
	if checking {
		planner := &curatedPlanner{
			profileSelectionMode: c.cfgState.profileSelectionMode,
			profiles:             c.cfgState.profiles,
			loadProfileCatalog:   c.loadProfilesCatalog,
		}
		var err error
		runtime, err = planner.buildRuntimeForCheck(batch.samples)
		if err != nil {
			return nil, nil, err
		}
	}

	if runtime == nil || len(runtime.compiledProfiles) == 0 {
		mfs, err := assembleMetricFamilies(batch)
		return mfs, runtime, err
	}

	executor := curatedExecutor{
		runtime:  runtime,
		checking: checking,
		Logger:   c.Logger,
	}
	mfs, err := executor.process(batch)
	if err != nil {
		return nil, runtime, err
	}
	return mfs, runtime, nil
}

func (e curatedExecutor) process(batch *scrapeBatch) (promscrapemodel.MetricFamilies, error) {
	processed, typedFamilies, err := e.apply(batch)
	if err != nil {
		return nil, err
	}

	mfs, err := assembleMetricFamilies(processed)
	if err != nil {
		return nil, err
	}

	invalid, err := e.validateRelabeledTypedFamilies(typedFamilies, mfs)
	if err != nil {
		return nil, err
	}
	if len(invalid) == 0 {
		return mfs, nil
	}

	filtered := filterInvalidTypedFamilySamples(processed, typedFamilies, invalid)
	return assembleMetricFamilies(filtered)
}

func (e curatedExecutor) apply(batch *scrapeBatch) (*scrapeBatch, map[typedFamilyKey]*relabeledTypedFamilyState, error) {
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
		owner, drop, err := e.resolveSampleOwner(raw)
		if err != nil {
			return nil, nil, err
		}
		if drop {
			continue
		}

		if conflict, err := e.checkTypedFamilyOwnership(raw, owner, familyOwners); err != nil {
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

func (e curatedExecutor) resolveSampleOwner(sample promscrapemodel.Sample) (*compiledProfile, bool, error) {
	var owner *compiledProfile
	for i := range e.runtime.compiledProfiles {
		if !e.runtime.compiledProfiles[i].match.MatchString(sample.Name) {
			continue
		}
		if owner != nil {
			msg := fmt.Sprintf("sample %q matches multiple selected profiles (%q, %q)", sample.Name, owner.profile.Name, e.runtime.compiledProfiles[i].profile.Name)
			if e.checking {
				return nil, false, errors.New(msg)
			}
			e.Warning(msg)
			return nil, true, nil
		}
		owner = &e.runtime.compiledProfiles[i]
	}
	return owner, false, nil
}

func (e curatedExecutor) checkTypedFamilyOwnership(sample promscrapemodel.Sample, owner *compiledProfile, seen map[typedFamilyKey]string) (bool, error) {
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
		if e.checking {
			return false, errors.New(msg)
		}
		e.Warning(msg)
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

func (e curatedExecutor) validateRelabeledTypedFamilies(states map[typedFamilyKey]*relabeledTypedFamilyState, mfs promscrapemodel.MetricFamilies) (map[typedFamilyKey]struct{}, error) {
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

		if e.checking {
			return nil, errors.New(msg)
		}
		e.Warning(msg)
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
