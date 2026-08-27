// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"cmp"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/diagnostic"
)

type topologyDiagnosticRefreshSweep struct {
	recorder              *diagnostic.Recorder
	startedAt             time.Time
	previousGenerationSet bool
	previousGeneration    diagnostic.MemberHandle
	registrations         []diagnostic.RefreshRegistrationV1
	activeRegistration    int
	finished              bool
}

func newTopologyDiagnosticRefreshSweep(
	recorder *diagnostic.Recorder,
	startedAt time.Time,
	entries []ddsnmp.DeviceEntry,
	previous *topologyGeneration,
) *topologyDiagnosticRefreshSweep {
	if recorder == nil {
		return nil
	}
	sweep := &topologyDiagnosticRefreshSweep{
		recorder: recorder, startedAt: startedAt, activeRegistration: -1,
		registrations: make([]diagnostic.RefreshRegistrationV1, 0, len(entries)),
	}
	if previous != nil {
		sweep.previousGenerationSet = true
		sweep.previousGeneration = previous.diagnosticMember
	}
	for _, entry := range entries {
		dev := entry.Info
		sweep.registrations = append(sweep.registrations, diagnostic.RefreshRegistrationV1{
			Registration: uint64(entry.RegistrationID),
			Device: diagnostic.RefreshDeviceIdentityV1{
				Hostname: dev.Hostname, Port: dev.Port, SNMPVersion: dev.SNMPVersion,
				SysObjectID: dev.SysObjectID, SysName: dev.SysName, Vendor: dev.Vendor, Model: dev.Model,
				VnodeGUID: dev.VnodeGUID, VnodeHostname: dev.VnodeHostname,
			},
			Selection:        diagnostic.RefreshSelectionSkippedNotDue,
			TargetResolution: diagnostic.TargetResolutionNotAttempted,
			Outcome:          diagnostic.RefreshOutcomeSkippedNotDue,
		})
	}
	return sweep
}

func (s *topologyDiagnosticRefreshSweep) markDue(index int) {
	if s == nil || index < 0 || index >= len(s.registrations) {
		return
	}
	s.registrations[index].Selection = diagnostic.RefreshSelectionDue
	s.registrations[index].Outcome = ""
}

func (s *topologyDiagnosticRefreshSweep) setTargetResolution(index int, state string) {
	if s == nil || index < 0 || index >= len(s.registrations) {
		return
	}
	s.registrations[index].TargetResolution = state
}

func (s *topologyDiagnosticRefreshSweep) start(index int) {
	if s != nil {
		s.activeRegistration = index
	}
}

func (s *topologyDiagnosticRefreshSweep) complete(index int, outcome string) {
	if s == nil || index < 0 || index >= len(s.registrations) {
		return
	}
	s.registrations[index].Outcome = outcome
	s.activeRegistration = -1
}

func (s *topologyDiagnosticRefreshSweep) terminalizePending(inFlight, notStarted string) {
	if s == nil {
		return
	}
	for i := range s.registrations {
		row := &s.registrations[i]
		if row.Selection != diagnostic.RefreshSelectionDue || row.Outcome != "" {
			continue
		}
		if i == s.activeRegistration {
			row.Outcome = inFlight
		} else {
			row.Outcome = notStarted
		}
	}
	s.activeRegistration = -1
}

func (s *topologyDiagnosticRefreshSweep) finishUnpublished(finishedAt time.Time, publication string) {
	if s == nil || s.finished {
		return
	}
	transaction, err := s.recorder.Begin(diagnostic.RefreshCapabilityV1())
	if err != nil {
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Abort(errors.New("refresh sweep capture did not terminalize"))
		}
	}()
	if err := transaction.DefineSection(diagnostic.RefreshSectionGeneration, diagnostic.StateNotApplicable, 0); err != nil {
		return
	}
	if err := transaction.DefineSection(diagnostic.RefreshSectionSweep, diagnostic.StateSuccess, 1); err != nil {
		return
	}
	base := s.baseSweep(transaction.CaptureID(), finishedAt, publication)
	if _, err := s.addSweepMember(transaction, base, diagnostic.MemberHandle{}); err != nil {
		return
	}
	if err := transaction.Commit(diagnostic.StateFailed); err != nil {
		return
	}
	committed = true
	s.finished = true
}

func (s *topologyDiagnosticRefreshSweep) finishPublished(
	collector *Collector,
	finishedAt time.Time,
	generation *topologyGeneration,
	states map[ddsnmp.DeviceRegistrationID]deviceRefreshState,
	refreshed map[ddsnmp.DeviceRegistrationID]*topologyDeviceSnapshot,
) diagnostic.MemberHandle {
	if s == nil || s.finished || collector == nil || generation == nil {
		return diagnostic.MemberHandle{}
	}
	transaction, err := s.recorder.Begin(diagnostic.RefreshCapabilityV1())
	if err != nil {
		return diagnostic.MemberHandle{}
	}
	committed := false
	defer func() {
		if !committed {
			_ = transaction.Abort(errors.New("refresh sweep capture did not terminalize"))
		}
	}()
	if err := transaction.DefineSection(diagnostic.RefreshSectionGeneration, diagnostic.StateSuccess, 1); err != nil {
		return diagnostic.MemberHandle{}
	}
	base, dependencies, rows := s.generationBase(collector, generation, states, refreshed)
	retained := diagnosticRetainedBytes(reflect.ValueOf(base)) + uint64(len(dependencies))*512
	var generationHandle diagnostic.MemberHandle
	if len(dependencies) == 0 {
		generationHandle, err = transaction.AddOwned(
			diagnostic.RefreshSectionGeneration,
			diagnostic.MemberType{Kind: diagnostic.KindGeneration, Schema: diagnostic.SchemaV1},
			base,
			retained,
		)
	} else {
		generationHandle, err = transaction.AddOptionalDerivedOwned(
			diagnostic.RefreshSectionGeneration,
			diagnostic.MemberType{Kind: diagnostic.KindGeneration, Schema: diagnostic.SchemaV1},
			dependencies,
			func(resolved []diagnostic.MemberResolution) (any, error) {
				value := base
				value.Devices = append([]diagnostic.GenerationDeviceV1(nil), base.Devices...)
				for i, resolution := range resolved {
					row := rows[i]
					if resolution.State != diagnostic.HandleSealed {
						continue
					}
					ref := resolution.Ref
					value.Devices[row].ObservationState = diagnostic.ObservationStateAvailable
					value.Devices[row].Observation = &ref
					value.Observations = append(value.Observations, ref)
				}
				return value, nil
			},
			retained,
		)
	}
	if err != nil {
		return diagnostic.MemberHandle{}
	}
	if err := transaction.DefineSection(diagnostic.RefreshSectionSweep, diagnostic.StateSuccess, 1); err != nil {
		return diagnostic.MemberHandle{}
	}
	sweep := s.baseSweep(transaction.CaptureID(), finishedAt, diagnostic.RefreshPublicationPublished)
	if _, err := s.addSweepMember(transaction, sweep, generationHandle); err != nil {
		return diagnostic.MemberHandle{}
	}
	if err := transaction.Commit(diagnostic.StateSuccess); err != nil {
		return diagnostic.MemberHandle{}
	}
	committed = true
	s.finished = true
	return generationHandle
}

func (s *topologyDiagnosticRefreshSweep) baseSweep(captureID uint64, finishedAt time.Time, publication string) diagnostic.RefreshSweepV1 {
	previous := diagnostic.GenerationReferenceV1{State: diagnostic.GenerationReferenceNone}
	if s.previousGenerationSet {
		previous.State = diagnostic.GenerationReferenceUnavailable
	}
	return diagnostic.RefreshSweepV1{
		CaptureID: captureID, StartedAt: canonicalDiagnosticTime(s.startedAt), FinishedAt: canonicalDiagnosticTime(finishedAt),
		PreviousGeneration: previous,
		Registrations:      append([]diagnostic.RefreshRegistrationV1(nil), s.registrations...),
		Publication: diagnostic.RefreshPublicationV1{
			State: publication, Generation: diagnostic.GenerationReferenceV1{State: diagnostic.GenerationReferenceNone},
		},
	}
}

func (s *topologyDiagnosticRefreshSweep) addSweepMember(
	transaction *diagnostic.CaptureTransaction,
	base diagnostic.RefreshSweepV1,
	resultGeneration diagnostic.MemberHandle,
) (diagnostic.MemberHandle, error) {
	dependencies := make([]diagnostic.MemberHandle, 0, 2)
	resultIndex := -1
	previousIndex := -1
	if resultGeneration.ID() != 0 {
		resultIndex = len(dependencies)
		dependencies = append(dependencies, resultGeneration)
	}
	if s.previousGeneration.ID() != 0 {
		previousIndex = len(dependencies)
		dependencies = append(dependencies, s.previousGeneration)
	}
	retained := diagnosticRetainedBytes(reflect.ValueOf(base)) + uint64(len(dependencies))*512
	if len(dependencies) == 0 {
		return transaction.AddOwned(
			diagnostic.RefreshSectionSweep,
			diagnostic.MemberType{Kind: diagnostic.KindRefreshSweep, Schema: diagnostic.SchemaV1},
			base,
			retained,
		)
	}
	return transaction.AddOptionalDerivedOwned(
		diagnostic.RefreshSectionSweep,
		diagnostic.MemberType{Kind: diagnostic.KindRefreshSweep, Schema: diagnostic.SchemaV1},
		dependencies,
		func(resolved []diagnostic.MemberResolution) (any, error) {
			value := base
			if resultIndex >= 0 {
				if resolved[resultIndex].State != diagnostic.HandleSealed {
					return nil, errors.New("published generation capture is unavailable")
				}
				ref := resolved[resultIndex].Ref
				value.Publication.Generation = diagnostic.GenerationReferenceV1{
					State: diagnostic.GenerationReferenceAvailable, Ref: &ref,
				}
			}
			if previousIndex >= 0 && resolved[previousIndex].State == diagnostic.HandleSealed {
				ref := resolved[previousIndex].Ref
				value.PreviousGeneration = diagnostic.GenerationReferenceV1{
					State: diagnostic.GenerationReferenceAvailable, Ref: &ref,
				}
			}
			return value, nil
		},
		retained,
	)
}

func (s *topologyDiagnosticRefreshSweep) generationBase(
	collector *Collector,
	generation *topologyGeneration,
	states map[ddsnmp.DeviceRegistrationID]deviceRefreshState,
	refreshed map[ddsnmp.DeviceRegistrationID]*topologyDeviceSnapshot,
) (diagnostic.GenerationV1, []diagnostic.MemberHandle, []int) {
	base := diagnostic.GenerationV1{
		Sequence: generation.sequence, PublishedAt: canonicalDiagnosticTime(generation.publishedAt),
		ProducerScopeID: collector.topologyRegistry.producerScope(),
		Devices:         make([]diagnostic.GenerationDeviceV1, 0, len(s.registrations)),
	}
	rowByRegistration := make(map[ddsnmp.DeviceRegistrationID]int, len(s.registrations))
	for _, registration := range s.registrations {
		registrationID := ddsnmp.DeviceRegistrationID(registration.Registration)
		state := states[registrationID]
		row := diagnostic.GenerationDeviceV1{
			Registration:     registration.Registration,
			State:            diagnostic.GenerationStateAbsent,
			ObservationState: diagnostic.ObservationStateNotApplicable,
		}
		if state.generation != nil {
			switch {
			case refreshed[registrationID] != nil:
				row.State = diagnostic.GenerationStateRefreshed
			case state.generation.freshAt(generation.publishedAt):
				row.State = diagnostic.GenerationStateRetained
			default:
				row.State = diagnostic.GenerationStateExpired
			}
			row.Renderable = state.generation.hasObservation && state.generation.freshAt(generation.publishedAt)
			if row.Renderable {
				row.ObservationState = diagnostic.ObservationStateUnavailable
			}
		}
		rowByRegistration[registrationID] = len(base.Devices)
		base.Devices = append(base.Devices, row)
	}
	devices := append([]*topologyDeviceGeneration(nil), generation.renderableDevices...)
	sort.SliceStable(devices, func(i, j int) bool {
		if value := compareTopologyObservationSnapshots(devices[i].observation, devices[j].observation); value != 0 {
			return value < 0
		}
		return cmp.Compare(devices[i].registrationID, devices[j].registrationID) < 0
	})
	dependencies := make([]diagnostic.MemberHandle, 0, len(devices))
	rows := make([]int, 0, len(devices))
	for _, device := range devices {
		if device == nil || device.diagnosticObservation.ID() == 0 {
			continue
		}
		dependencies = append(dependencies, device.diagnosticObservation)
		rows = append(rows, rowByRegistration[device.registrationID])
	}
	return base, dependencies, rows
}

func topologyDiagnosticGraphKernel() diagnostic.GraphKernelV1 {
	return diagnostic.GraphKernelV1{
		Name:         "snmp_topology_graph",
		Revision:     1,
		ModelSchema:  "2.0",
		OutputSchema: "netdata.topology.v1",
	}
}
