// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestRenderRejectsInvalidActorHandles(t *testing.T) {
	handles := graph.NewActorHandleAllocator()
	first := handles.Next()
	second := handles.Next()
	otherGeneration := graph.NewActorHandleAllocator()
	dangling := otherGeneration.Next()

	tests := map[string]topologymodel.Data{
		"zero actor": {
			Actors: []topologymodel.Actor{{ActorID: "device-a"}},
		},
		"duplicate actor": {
			Actors: []topologymodel.Actor{
				{ActorHandle: first, ActorID: "device-a"},
				{ActorHandle: first, ActorID: "device-b"},
			},
		},
		"dangling link": {
			Actors: []topologymodel.Actor{
				{ActorHandle: first, ActorID: "device-a"},
				{ActorHandle: second, ActorID: "device-b"},
			},
			Links: []topologymodel.Link{{SrcActorHandle: first, DstActorHandle: dangling}},
		},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Render(data, nil)
			require.Error(t, err)
		})
	}
}

func TestRenderIsDeterministicForProducerLabels(t *testing.T) {
	handle := topologyV1TestActorHandle("device-a")
	data := topologymodel.Data{
		AgentID:     "agent-a",
		CollectedAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		Actors: []topologymodel.Actor{{
			ActorHandle: handle,
			ActorID:     "device-a",
			ActorType:   "device",
			Labels: map[string]string{
				"alpha": "1", "bravo": "2", "charlie": "3", "delta": "4", "echo": "5",
				"foxtrot": "6", "golf": "7", "hotel": "8", "india": "9", "juliet": "10",
			},
		}},
	}

	first, err := Render(data, nil)
	require.NoError(t, err)
	want, err := json.Marshal(first)
	require.NoError(t, err)
	for range 100 {
		next, err := Render(data, nil)
		require.NoError(t, err)
		got, err := json.Marshal(next)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestRenderWorkLimiterPreservesOutputAndRejectsBeforeRendering(t *testing.T) {
	data := topologymodel.Data{
		AgentID:     "agent-a",
		CollectedAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		Actors: []topologymodel.Actor{{
			ActorHandle: topologyV1TestActorHandle("limited-device"),
			ActorID:     "limited-device",
			ActorType:   "device",
			Labels:      map[string]string{"site": "lab"},
		}},
	}
	unbounded, err := Render(data, nil)
	require.NoError(t, err)
	var charged uint64
	bounded, err := Render(data, func(units uint64) error {
		charged += units
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, charged)
	require.Equal(t, unbounded, bounded)

	limitErr := errors.New("render work exhausted")
	_, err = Render(data, func(uint64) error { return limitErr })
	require.ErrorIs(t, err, limitErr)
}

func TestRenderWorkLimiterChargesLabelSortsBeforeRendering(t *testing.T) {
	labels := make(map[string]string, 64)
	for i := range 64 {
		labels[string(rune('a'+i%26))+string(rune('A'+i/26))] = "value"
	}
	data := topologymodel.Data{
		AgentID:     "agent-a",
		CollectedAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		Actors: []topologymodel.Actor{{
			ActorHandle: topologyV1TestActorHandle("label-heavy-device"),
			ActorID:     "label-heavy-device",
			ActorType:   "device",
			Labels:      labels,
		}},
	}
	// The current linear actor+label charge is 65. The data-dependent label
	// sorts must reject a budget that covers only that linear work.
	remaining := uint64(65)
	limitErr := errors.New("label sort work exhausted")
	_, err := Render(data, func(units uint64) error {
		if units > remaining {
			return limitErr
		}
		remaining -= units
		return nil
	})
	require.ErrorIs(t, err, limitErr)
}
