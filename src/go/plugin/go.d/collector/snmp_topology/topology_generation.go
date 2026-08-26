// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

// topologyDeviceGeneration is the immutable output of one successful device
// collection. Builders are never published to runtime readers.
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
	sequence    uint64
	publishedAt time.Time
	devices     []*topologyDeviceGeneration
}

func freezeTopologyBuilder(key string, builder *topologyBuilder) (*topologyDeviceGeneration, topologyBuilderFinalizeStats) {
	if builder == nil {
		return nil, topologyBuilderFinalizeStats{}
	}
	stats := builder.finalize()

	collectedAt := builder.lastUpdate
	if collectedAt.IsZero() {
		collectedAt = builder.updateTime
	}
	expiresAt := time.Time{}
	if !collectedAt.IsZero() && builder.staleAfter > 0 {
		expiresAt = collectedAt.Add(builder.staleAfter)
	}

	return &topologyDeviceGeneration{
		key:            key,
		collectedAt:    collectedAt,
		expiresAt:      expiresAt,
		observation:    builder.preparedSnapshot,
		hasObservation: builder.hasPreparedSnapshot,
		trap:           newTopologyTrapDeviceGeneration(builder),
	}, stats
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
	for _, key := range keys {
		devices = append(devices, states[key].generation)
	}
	return &topologyGeneration{
		sequence:    sequence,
		publishedAt: publishedAt,
		devices:     devices,
	}
}

func (g *topologyDeviceGeneration) freshAt(now time.Time) bool {
	if g == nil || g.collectedAt.IsZero() {
		return false
	}
	return g.expiresAt.IsZero() || !now.After(g.expiresAt)
}

func (g *topologyGeneration) observationSnapshotsAt(now time.Time) []topologymodel.ObservationSnapshot {
	if g == nil || len(g.devices) == 0 {
		return nil
	}

	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(g.devices))
	for _, device := range g.devices {
		if device == nil || !device.hasObservation || !device.freshAt(now) {
			continue
		}
		snapshots = append(snapshots, device.observation)
	}
	return snapshots
}

func (g *topologyGeneration) hasRenderableObservationsAt(now time.Time) bool {
	if g == nil {
		return false
	}
	for _, device := range g.devices {
		if device != nil && device.hasObservation && device.freshAt(now) {
			return true
		}
	}
	return false
}

func (g *topologyGeneration) deviceCount() int {
	if g == nil {
		return 0
	}
	return len(g.devices)
}
