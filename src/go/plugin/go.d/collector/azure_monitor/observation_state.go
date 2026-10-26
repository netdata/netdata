// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type resourceIdentity struct {
	ScopeKey string
	Profile  string
	UID      string
}

type observationKey struct {
	Instrument string
	Resource   resourceIdentity
}

type observationState struct {
	instruments  map[string]*instrumentRuntime
	profiles     map[string]*profileRuntime
	lastObserved map[observationKey]lastObservation
}

func newObservationState(runtime *collectorRuntime) *observationState {
	profiles := make(map[string]*profileRuntime, len(runtime.Profiles))
	for _, profile := range runtime.Profiles {
		profiles[profile.Name] = profile
	}
	return &observationState{
		instruments:  runtime.Instruments,
		profiles:     profiles,
		lastObserved: make(map[observationKey]lastObservation),
	}
}

func (s *observationState) reset() {
	s.lastObserved = make(map[observationKey]lastObservation)
}

func dueInstrumentsForBatches(batches []queryBatch) map[string]bool {
	dueInstruments := make(map[string]bool)
	for _, batch := range batches {
		for _, metric := range batch.Metrics {
			for _, series := range metric.Series {
				dueInstruments[series.Instrument] = true
			}
		}
	}
	return dueInstruments
}

func (s *observationState) observeSamples(samples []metricSample) map[observationKey]bool {
	observedThisCycle := make(map[observationKey]bool, len(s.lastObserved))
	for _, sample := range samples {
		key, ok := s.observeSample(sample)
		if !ok {
			continue
		}
		observedThisCycle[key] = true
	}
	return observedThisCycle
}

func (s *observationState) observeSample(sample metricSample) (observationKey, bool) {
	inst, ok := s.instruments[sample.Instrument]
	if !ok {
		return observationKey{}, false
	}

	values := labelValues(inst.LabelKeys, sample.Labels)
	key := observationKey{
		Instrument: sample.Instrument,
		Resource: resourceIdentity{
			ScopeKey: sample.Scope.ScopeKey,
			Profile:  sample.Labels["profile"],
			UID:      sample.Labels["resource_uid"],
		},
	}
	value := sample.Value

	if inst.Kind == azureprofiles.SeriesKindCounter {
		value += s.lastObserved[key].value
	}

	inst.observe(sample.Scope, values, value)
	s.lastObserved[key] = lastObservation{
		scope:       sample.Scope,
		labelValues: values,
		value:       value,
	}

	return key, true
}

func (s *observationState) reobserveCachedObservations(dueInstruments map[string]bool, observedThisCycle map[observationKey]bool) {
	for key, obs := range s.lastObserved {
		if observedThisCycle[key] {
			continue
		}
		if dueInstruments[key.Instrument] {
			continue
		}
		inst, ok := s.instruments[key.Instrument]
		if !ok {
			continue
		}
		inst.observe(obs.scope, obs.labelValues, obs.value)
	}
}

// reconcileResources retires departed memberships and refreshes presentation metadata
// without resetting cumulative values or losing observations between metric polls.
func (s *observationState) reconcileResources(current map[string][]resourceInfo) {
	type resourceMetadata struct {
		scope       metrix.HostScope
		labelValues []string
	}
	active := make(map[resourceIdentity]resourceMetadata)
	for profileName, resources := range current {
		profile := s.profiles[profileName]
		for _, resource := range resources {
			active[resourceIdentity{
				ScopeKey: resource.HostScope.ScopeKey,
				Profile:  profileName,
				UID:      resource.UID,
			}] = resourceMetadata{
				scope:       resource.HostScope,
				labelValues: labelValues(profile.LabelKeys, profileResourceLabels(resource, profile)),
			}
		}
	}

	for key, obs := range s.lastObserved {
		metadata, ok := active[key.Resource]
		if !ok {
			delete(s.lastObserved, key)
			continue
		}
		obs.scope = metadata.scope
		obs.labelValues = metadata.labelValues
		s.lastObserved[key] = obs
	}
}

// labelKeysFor orders the reserved resource labels first, then the profile's labels by name.
func labelKeysFor(labelNames []string) []string {
	extra := slices.Clone(labelNames)
	slices.Sort(extra)
	return append(slices.Clone(azureprofiles.ReservedLabelNames), extra...)
}

func labelValues(keys []string, labels metrix.Labels) []string {
	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = labels[key]
	}
	return values
}
