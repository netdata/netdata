// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

// topologyDeviceSnapshot is the immutable output of one successful device
// collection. It is not visible to readers until activated at global publish.
type topologyDeviceSnapshot struct {
	collectedAt    time.Time
	freshFor       time.Duration
	observation    topologymodel.ObservationSnapshot
	hasObservation bool
	trap           topologyTrapDeviceGeneration
	acquisition    *topologyAcquisitionCapture
}

type topologyEvidenceRef struct {
	registrationID ddsnmp.DeviceRegistrationID
	generation     uint64
}

// topologyDeviceGeneration is an immutable collected snapshot activated at a
// global publication boundary. Builders and unactivated snapshots are never
// published to runtime readers.
type topologyDeviceGeneration struct {
	registrationID ddsnmp.DeviceRegistrationID
	evidenceRef    topologyEvidenceRef
	collectedAt    time.Time
	expiresAt      time.Time
	observation    topologymodel.ObservationSnapshot
	hasObservation bool
	trap           topologyTrapDeviceGeneration
	acquisition    *topologyAcquisitionCapture
}

// topologyGeneration is the immutable device vector published after one
// complete refresh sweep.
type topologyGeneration struct {
	sequence          uint64
	publishedAt       time.Time
	producerScopeID   string
	devices           []*topologyDeviceGeneration
	renderableDevices []*topologyDeviceGeneration
	diagnostic        *topologySweepDiagnosticCut
}

func freezeTopologyBuilder(builder *topologyBuilder) (*topologyDeviceSnapshot, topologyBuilderFinalizeStats) {
	if builder == nil {
		return nil, topologyBuilderFinalizeStats{}
	}
	stats := builder.finalize()

	collectedAt := builder.lastUpdate
	if collectedAt.IsZero() {
		collectedAt = builder.updateTime
	}
	return &topologyDeviceSnapshot{
		collectedAt:    collectedAt,
		freshFor:       builder.staleAfter,
		observation:    builder.preparedSnapshot,
		hasObservation: builder.hasPreparedSnapshot,
		trap:           newTopologyTrapDeviceGeneration(builder),
	}, stats
}

func activateTopologyDeviceSnapshot(
	registrationID ddsnmp.DeviceRegistrationID,
	generation uint64,
	publishedAt time.Time,
	snapshot *topologyDeviceSnapshot,
) *topologyDeviceGeneration {
	if snapshot == nil {
		return nil
	}
	expiresAt := time.Time{}
	if snapshot.freshFor > 0 {
		expiresAt = publishedAt.Add(snapshot.freshFor)
	}
	return &topologyDeviceGeneration{
		registrationID: registrationID,
		evidenceRef: topologyEvidenceRef{
			registrationID: registrationID,
			generation:     generation,
		},
		collectedAt:    snapshot.collectedAt,
		expiresAt:      expiresAt,
		observation:    snapshot.observation,
		hasObservation: snapshot.hasObservation,
		trap:           snapshot.trap,
		acquisition:    snapshot.acquisition,
	}
}

func newTopologyGeneration(
	sequence uint64,
	publishedAt time.Time,
	producerScopeID string,
	states map[ddsnmp.DeviceRegistrationID]deviceRefreshState,
) *topologyGeneration {
	registrationIDs := make([]ddsnmp.DeviceRegistrationID, 0, len(states))
	for registrationID, state := range states {
		if state.generation != nil {
			registrationIDs = append(registrationIDs, registrationID)
		}
	}
	slices.Sort(registrationIDs)

	devices := make([]*topologyDeviceGeneration, 0, len(registrationIDs))
	renderableDevices := make([]*topologyDeviceGeneration, 0, len(registrationIDs))
	for _, registrationID := range registrationIDs {
		device := states[registrationID].generation
		devices = append(devices, device)
		if device.hasObservation && device.freshAt(publishedAt) {
			renderableDevices = append(renderableDevices, device)
		}
	}
	return &topologyGeneration{
		sequence:          sequence,
		publishedAt:       publishedAt,
		producerScopeID:   strings.TrimSpace(producerScopeID),
		devices:           devices,
		renderableDevices: renderableDevices,
	}
}

func (g *topologyDeviceGeneration) freshAt(now time.Time) bool {
	if g == nil || g.collectedAt.IsZero() {
		return false
	}
	return g.expiresAt.IsZero() || !now.After(g.expiresAt)
}

func (g *topologyGeneration) observationSnapshots() []topologymodel.ObservationSnapshot {
	if g == nil || len(g.renderableDevices) == 0 {
		return nil
	}

	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(g.renderableDevices))
	for _, device := range g.renderableDevices {
		snapshots = append(snapshots, device.observation)
	}
	return snapshots
}

func (g *topologyGeneration) hasRenderableObservations() bool {
	return g != nil && len(g.renderableDevices) > 0
}

func (g *topologyGeneration) deviceCount() int {
	if g == nil {
		return 0
	}
	return len(g.devices)
}
