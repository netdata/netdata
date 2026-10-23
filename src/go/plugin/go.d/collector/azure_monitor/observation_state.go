// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type observationLabelIdentity string

type scopedLabelIdentity struct {
	ScopeKey string
	Labels   observationLabelIdentity
}

type observationKey struct {
	Instrument string
	ScopeKey   string
	Labels     observationLabelIdentity
}

type observationState struct {
	instruments  map[string]*instrumentRuntime
	profiles     map[string]*profileRuntime
	accumulators map[observationKey]float64
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
		accumulators: make(map[observationKey]float64),
		lastObserved: make(map[observationKey]lastObservation),
	}
}

func (s *observationState) resourceLabels(resource resourceInfo, profileName string) metrix.Labels {
	if profile, ok := s.profiles[profileName]; ok {
		return profileResourceLabels(resource, profile)
	}
	return resourceLabels(resource, profileName)
}

func (s *observationState) reset() {
	s.accumulators = make(map[observationKey]float64)
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

	values := labelValues(sample.Labels)
	key := sampleObservationKey(sample.Instrument, sample.Scope, values)
	value := sample.Value

	if sample.Kind == azureprofiles.SeriesKindCounter {
		s.accumulators[key] += value
		value = s.accumulators[key]
	}

	inst.observe(sample.Scope, values, value)
	s.lastObserved[key] = lastObservation{
		instrument:  sample.Instrument,
		scope:       sample.Scope,
		labelValues: append([]string(nil), values...),
		value:       value,
	}

	return key, true
}

func (s *observationState) reobserveCachedObservations(dueInstruments map[string]bool, observedThisCycle map[observationKey]bool) {
	for key, obs := range s.lastObserved {
		if observedThisCycle[key] {
			continue
		}
		if dueInstruments[obs.instrument] {
			continue
		}
		inst, ok := s.instruments[obs.instrument]
		if !ok {
			continue
		}
		inst.observe(obs.scope, obs.labelValues, obs.value)
	}
}

// pruneStaleResources removes cache entries for resources that are no longer
// active for a specific profile/label identity.
func (s *observationState) pruneStaleResources(current map[string][]resourceInfo) {
	activeLabels := make(map[scopedLabelIdentity]struct{})
	for profileName, resources := range current {
		for _, resource := range resources {
			activeLabels[scopedLabelIdentity{
				ScopeKey: resource.HostScope.ScopeKey,
				Labels:   labelIdentity(labelValues(s.resourceLabels(resource, profileName))),
			}] = struct{}{}
		}
	}

	for key, obs := range s.lastObserved {
		if len(obs.labelValues) == 0 {
			continue
		}
		if _, ok := activeLabels[scopedLabelIdentity{ScopeKey: obs.scope.ScopeKey, Labels: labelIdentity(obs.labelValues)}]; ok {
			continue
		}
		delete(s.lastObserved, key)
	}

	for key := range s.accumulators {
		if _, ok := activeLabels[scopedLabelIdentity{ScopeKey: key.ScopeKey, Labels: key.Labels}]; ok {
			continue
		}
		delete(s.accumulators, key)
	}
}

func sampleObservationKey(instrument string, scope metrix.HostScope, values []string) observationKey {
	return observationKey{
		Instrument: instrument,
		ScopeKey:   scope.ScopeKey,
		Labels:     labelIdentity(values),
	}
}

func labelIdentity(values []string) observationLabelIdentity {
	return observationLabelIdentity(strings.Join(values, "\x00"))
}

// labelKeysFor orders the reserved resource labels first, then the profile's labels by name.
func labelKeysFor(labelNames []string) []string {
	extra := slices.Clone(labelNames)
	slices.Sort(extra)
	return append(slices.Clone(azureprofiles.ReservedLabelNames), extra...)
}

func labelValues(labels metrix.Labels) []string {
	extra := make([]string, 0, len(labels))
	for key := range labels {
		if !slices.Contains(azureprofiles.ReservedLabelNames, key) {
			extra = append(extra, key)
		}
	}

	keys := labelKeysFor(extra)
	values := make([]string, len(keys))
	for i, key := range keys {
		values[i] = labels[key]
	}
	return values
}
