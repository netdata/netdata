// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
)

type resourceIdentity struct {
	ScopeKey string
	Profile  string
	UID      string
}

func resourceIdentityFor(resource resourceInfo, profile string) resourceIdentity {
	return resourceIdentity{ScopeKey: resource.HostScope.ScopeKey, Profile: profile, UID: resource.UID}
}

type observationState struct {
	meter     metrix.SnapshotMeter
	profiles  map[string]*profileRuntime
	kinds     map[string]string
	resources map[resourceIdentity]*resourceObservations
}

// One current membership owns its metadata-bound handles and numeric history.
// Direct handles avoid retaining historical label tuples in a long-lived Vec.
type resourceObservations struct {
	scope   metrix.HostScope
	labels  metrix.Labels
	meter   metrix.SnapshotMeter
	metrics map[string]*metricObservation
	states  map[string]metrix.StateSetInstrument
}

type metricObservation struct {
	kind    string
	value   float64
	gauge   metrix.SnapshotGauge
	counter metrix.SnapshotCounter
}

func newObservationState(runtime *collectorRuntime, meter metrix.SnapshotMeter) *observationState {
	s := &observationState{
		meter:    meter,
		profiles: make(map[string]*profileRuntime, len(runtime.Profiles)),
		kinds:    make(map[string]string),
	}
	for _, profile := range runtime.Profiles {
		s.profiles[profile.Name] = profile
		for _, metric := range profile.Metrics {
			for _, series := range metric.Series {
				s.kinds[series.Instrument] = series.Kind
			}
		}
	}
	return s
}

func (s *observationState) reset() { s.resources = nil }

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

func (s *observationState) observeSamples(samples []metricSample) {
	for _, sample := range samples {
		resource, ok := s.resources[sample.Resource]
		if !ok {
			continue
		}
		kind, ok := s.kinds[sample.Instrument]
		if !ok {
			continue
		}
		obs := resource.metrics[sample.Instrument]
		if obs == nil {
			obs = &metricObservation{kind: kind}
			obs.bind(resource.meter, sample.Instrument)
			if resource.metrics == nil {
				resource.metrics = make(map[string]*metricObservation)
			}
			resource.metrics[sample.Instrument] = obs
		}
		value := sample.Value
		if kind == azureprofiles.SeriesKindCounter {
			value += obs.value
		}
		obs.value = value
		obs.observe()
	}
}

func (s *observationState) reobserveCachedObservations(due map[string]bool) {
	for _, resource := range s.resources {
		for name, obs := range resource.metrics {
			if due[name] {
				continue
			}
			obs.observe()
		}
	}
}

// Reconcile at the discovery boundary: discard departed memberships and replace
// metadata-bound handles without resetting cumulative values or held samples.
func (s *observationState) reconcileResources(current map[string][]resourceInfo) {
	next := make(map[resourceIdentity]*resourceObservations, len(s.resources))
	for profileName, resources := range current {
		profile := s.profiles[profileName]
		for _, info := range resources {
			identity := resourceIdentityFor(info, profileName)
			labels := profileResourceLabels(info, profile)
			resource := s.resources[identity]
			if resource == nil {
				resource = &resourceObservations{}
			}
			if resource.meter == nil || !maps.Equal(resource.labels, labels) ||
				resource.scope.GUID != info.HostScope.GUID || resource.scope.Hostname != info.HostScope.Hostname ||
				!maps.Equal(resource.scope.Labels, info.HostScope.Labels) {
				resource.bind(s.meter, info.HostScope, labels)
			}
			next[identity] = resource
		}
	}
	s.resources = next
}

func (r *resourceObservations) bind(meter metrix.SnapshotMeter, scope metrix.HostScope, labels metrix.Labels) {
	r.scope, r.labels = scope, labels
	pairs := make([]metrix.Label, 0, len(labels))
	for key, value := range labels {
		pairs = append(pairs, metrix.Label{Key: key, Value: value})
	}
	r.meter = meter.WithHostScope(scope).WithLabels(pairs...)
	for name, obs := range r.metrics {
		obs.bind(r.meter, name)
	}
	r.states = nil
}

func (o *metricObservation) bind(meter metrix.SnapshotMeter, name string) {
	if o.kind == azureprofiles.SeriesKindCounter {
		o.counter = meter.Counter(name)
	} else {
		o.gauge = meter.Gauge(name)
	}
}

func (o *metricObservation) observe() {
	if o.kind == azureprofiles.SeriesKindCounter {
		o.counter.ObserveTotal(o.value)
	} else {
		o.gauge.Observe(o.value)
	}
}

func (r *resourceObservations) observeState(state *resourceStateRuntime, active string) {
	instrument := r.states[state.Name]
	if instrument == nil {
		instrument = r.meter.StateSet(state.Name, metrix.WithStateSetStates(state.States...), metrix.WithStateSetMode(metrix.ModeEnum))
		if r.states == nil {
			r.states = make(map[string]metrix.StateSetInstrument)
		}
		r.states[state.Name] = instrument
	}
	instrument.Enable(active)
}

func profileResourceLabels(resource resourceInfo, profile *profileRuntime) metrix.Labels {
	labels := metrix.Labels{
		"resource_uid":    resource.UID,
		"subscription_id": resource.SubscriptionID,
		"resource_name":   resource.Name,
		"resource_group":  resource.ResourceGroup,
		"region":          resource.Region,
		"resource_type":   resource.Type,
		"profile":         profile.Name,
	}
	for _, label := range profile.ResourceLabels {
		labels[label.Name] = resourceLabelValue(resource.Properties[label.Property], label.Value)
	}
	return labels
}

func resourceLabelValue(value, format string) string {
	if format != azureprofiles.ResourceLabelValueARMName {
		return value
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	return parts[len(parts)-1]
}
