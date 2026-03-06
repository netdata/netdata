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

// pruneStaleResources removes cache entries for resources no longer in the discovery set.
func (s *observationState) pruneStaleResources(current []resourceInfo) {
	activeUIDs := make(map[string]struct{}, len(current))
	for _, r := range current {
		activeUIDs[r.UID] = struct{}{}
	}

	for key, obs := range s.lastObserved {
		if len(obs.labelValues) == 0 {
			continue
		}
		if _, ok := activeUIDs[obs.labelValues[0]]; ok {
			continue
		}
		delete(s.lastObserved, key)
	}

	for key := range s.accumulators {
		uid := accumulatorResourceUID(key)
		if uid == "" {
			continue
		}
		if _, ok := activeUIDs[uid]; ok {
			continue
		}
		delete(s.accumulators, key)
	}
}

func sampleObservationKey(instrument string, values []string) string {
	return instrument + "\x00" + strings.Join(values, "\x00")
}

func accumulatorResourceUID(key string) string {
	parts := strings.SplitN(key, "\x00", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func labelValues(labels metrix.Labels) []string {
	return []string{
		labels["resource_uid"],
		labels["resource_name"],
		labels["resource_group"],
		labels["region"],
		labels["resource_type"],
		labels["profile"],
	}
}
