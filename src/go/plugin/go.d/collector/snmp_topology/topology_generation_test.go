// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func TestPublishTestTopologyBuilderUsesProductionFinalization(t *testing.T) {
	registry := newTopologyRegistry()
	builder := newTopologyBuilder()
	builder.updateTime = time.Now()
	builder.staleAfter = time.Hour
	builder.agentID = "agent-test"
	builder.localDevice = topologymodel.Device{
		ChassisID:     "00:11:22:33:44:55",
		ChassisIDType: "macAddress",
		SysName:       "switch-a",
	}
	builder.targetManagementIPs = []netip.Addr{netip.MustParseAddr("192.0.2.10")}

	publishTestTopologyBuilder(registry, builder)
	generation := registry.acquireGeneration()
	require.NotNil(t, generation)
	require.Len(t, generation.devices, 1)
	require.Equal(t, "192.0.2.10", generation.devices[0].observation.LocalDevice.ManagementIP)
	require.Equal(t, "management_ip", generation.devices[0].trap.matchMethodByIP["192.0.2.10"])
}

func TestTopologyRegistryPublishesWholeGenerationVectors(t *testing.T) {
	registry := newTopologyRegistry()
	now := time.Now()
	generationA := testTopologyGenerationVector(1, now, "generation-a")
	generationB := testTopologyGenerationVector(2, now, "generation-b")
	registry.publishGeneration(generationA)

	const iterations = 2000
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		<-start
		for i := range iterations {
			if i%2 == 0 {
				registry.publishGeneration(generationB)
			} else {
				registry.publishGeneration(generationA)
			}
		}
	}()

	errors := make(chan string, 4)
	for range 4 {
		go func() {
			defer wg.Done()
			<-start
			for range iterations {
				generation := registry.acquireGeneration()
				snapshots := topologyObservationSnapshots(generation)
				if len(snapshots) != 2 {
					errors <- fmt.Sprintf("sequence %d exposed %d devices", generation.sequence, len(snapshots))
					return
				}
				if snapshots[0].AgentID != snapshots[1].AgentID {
					errors <- fmt.Sprintf("sequence %d mixed %q and %q", generation.sequence, snapshots[0].AgentID, snapshots[1].AgentID)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errors)
	require.Empty(t, errors)
}

func testTopologyGenerationVector(sequence uint64, now time.Time, identity string) *topologyGeneration {
	devices := make([]*topologyDeviceGeneration, 2)
	for i := range devices {
		devices[i] = &topologyDeviceGeneration{
			registrationID: ddsnmp.DeviceRegistrationID(i + 1),
			collectedAt:    now,
			expiresAt:      now.Add(time.Hour),
			hasObservation: true,
			observation: topologymodel.ObservationSnapshot{
				AgentID:       identity,
				LocalDeviceID: fmt.Sprintf("device-%d", i),
				CollectedAt:   now,
			},
		}
	}
	return &topologyGeneration{
		sequence:          sequence,
		publishedAt:       now,
		devices:           devices,
		renderableDevices: append([]*topologyDeviceGeneration(nil), devices...),
	}
}
