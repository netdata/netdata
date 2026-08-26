// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"time"

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
}

// topologyDeviceGeneration is an immutable collected snapshot activated at a
// global publication boundary. Builders and unactivated snapshots are never
// published to runtime readers.
type topologyDeviceGeneration struct {
	key            string
	collectedAt    time.Time
	expiresAt      time.Time
	observation    topologymodel.ObservationSnapshot
	hasObservation bool
	trap           topologyTrapDeviceGeneration
}

// topologyGeneration is the immutable device vector published after one
// complete refresh sweep.
type topologyGeneration struct {
	sequence          uint64
	publishedAt       time.Time
	devices           []*topologyDeviceGeneration
	renderableDevices []*topologyDeviceGeneration
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
	key string,
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
		key:            key,
		collectedAt:    snapshot.collectedAt,
		expiresAt:      expiresAt,
		observation:    snapshot.observation,
		hasObservation: snapshot.hasObservation,
		trap:           snapshot.trap,
	}
}

func newTopologyGeneration(sequence uint64, publishedAt time.Time, states map[string]deviceRefreshState) *topologyGeneration {
	keys := make([]string, 0, len(states))
	for key, state := range states {
		if state.generation != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	devices := make([]*topologyDeviceGeneration, 0, len(keys))
	renderableDevices := make([]*topologyDeviceGeneration, 0, len(keys))
	for _, key := range keys {
		device := states[key].generation
		devices = append(devices, device)
		if device.hasObservation && device.freshAt(publishedAt) {
			renderableDevices = append(renderableDevices, device)
		}
	}
	return &topologyGeneration{
		sequence:          sequence,
		publishedAt:       publishedAt,
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
