// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type observationLabelIdentity [7]string

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
	accumulators map[observationKey]float64
	lastObserved map[observationKey]lastObservation
}

func newObservationState(instruments map[string]*instrumentRuntime) *observationState {
	return &observationState{
		instruments:  instruments,
		accumulators: make(map[observationKey]float64),
		lastObserved: make(map[observationKey]lastObservation),
	}
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
				Labels:   labelIdentity(labelValues(resourceLabels(resource, profileName))),
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
	var out observationLabelIdentity
	copy(out[:], values)
	return out
}

func labelValues(labels metrix.Labels) []string {
	return []string{
		labels["resource_uid"],
		labels["subscription_id"],
		labels["resource_name"],
		labels["resource_group"],
		labels["region"],
		labels["resource_type"],
		labels["profile"],
	}
}
