// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type observationState struct {
	instruments  map[string]*instrumentRuntime
	accumulators map[string]float64
	lastObserved map[string]lastObservation
}

func newObservationState(instruments map[string]*instrumentRuntime) *observationState {
	return &observationState{
		instruments:  instruments,
		accumulators: make(map[string]float64),
		lastObserved: make(map[string]lastObservation),
	}
}

func (s *observationState) reset() {
	s.accumulators = make(map[string]float64)
	s.lastObserved = make(map[string]lastObservation)
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

func (s *observationState) observeSamples(samples []metricSample) map[string]bool {
	observedThisCycle := make(map[string]bool, len(s.lastObserved))
	for _, sample := range samples {
		key, ok := s.observeSample(sample)
		if !ok {
			continue
		}
		observedThisCycle[key] = true
	}
	return observedThisCycle
}

func (s *observationState) observeSample(sample metricSample) (string, bool) {
	inst, ok := s.instruments[sample.Instrument]
	if !ok {
		return "", false
	}

	values := labelValues(sample.Labels)
	key := sampleObservationKey(sample.Instrument, values)
	value := sample.Value

	if sample.Kind == azureprofiles.SeriesKindCounter {
		s.accumulators[key] += value
		value = s.accumulators[key]
	}

	inst.observe(values, value)
	s.lastObserved[key] = lastObservation{
		instrument:  sample.Instrument,
		labelValues: append([]string(nil), values...),
		value:       value,
	}

	return key, true
}

func (s *observationState) reobserveCachedObservations(dueInstruments, observedThisCycle map[string]bool) {
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
		inst.observe(obs.labelValues, obs.value)
	}
}

// pruneStaleResources removes cache entries for resources that are no longer
// active for a specific profile/label identity.
func (s *observationState) pruneStaleResources(current map[string][]resourceInfo) {
	activeLabels := make(map[string]struct{})
	for profileName, resources := range current {
		for _, resource := range resources {
			activeLabels[labelIdentity(labelValues(resourceLabels(resource, profileName)))] = struct{}{}
		}
	}

	for key, obs := range s.lastObserved {
		if len(obs.labelValues) == 0 {
			continue
		}
		if _, ok := activeLabels[labelIdentity(obs.labelValues)]; ok {
			continue
		}
		delete(s.lastObserved, key)
	}

	for key := range s.accumulators {
		if _, ok := activeLabels[labelIdentityFromObservationKey(key)]; ok {
			continue
		}
		delete(s.accumulators, key)
	}
}

func sampleObservationKey(instrument string, values []string) string {
	return instrument + "\x00" + strings.Join(values, "\x00")
}

func labelIdentity(values []string) string {
	return strings.Join(values, "\x00")
}

func labelIdentityFromObservationKey(key string) string {
	parts := strings.SplitN(key, "\x00", 2)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
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
