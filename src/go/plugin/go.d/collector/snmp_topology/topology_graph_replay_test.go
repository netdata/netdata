// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/require"
)

func TestTopologyDiagnosticsReplayMatchesLiveTypedPayload(t *testing.T) {
	tests := map[string]struct {
		build     func() *topologyScenario
		configure func(*topologyoptions.QueryOptions)
	}{
		"managed-fabric-l2-l3": {build: newMixedL2L3ControlScenario},
		"lldp-cdp-managed": {
			build: newMixedL2L3ControlScenario,
			configure: func(options *topologyoptions.QueryOptions) {
				options.MapType = topologyoptions.MapTypeLLDPCDPManaged
			},
		},
		"high-confidence-fdb-minimum": {
			build: newMixedL2L3ControlScenario,
			configure: func(options *topologyoptions.QueryOptions) {
				options.MapType = topologyoptions.MapTypeHighConfidenceInferred
				options.InferenceStrategy = topologyoptions.InferenceStrategyFDBMinimumKnowledge
			},
		},
		"probable-fdb-minimum": {
			build: newMixedL2L3ControlScenario,
			configure: func(options *topologyoptions.QueryOptions) {
				options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
				options.InferenceStrategy = topologyoptions.InferenceStrategyFDBMinimumKnowledge
			},
		},
		"stp-parent-tree": {build: newSTPInferredScenario},
		"fdb-pairwise":    {build: newFDBPairwiseScenario},
		"stp-fdb-correlated": {
			build: newMixedL2L3ControlScenario,
			configure: func(options *topologyoptions.QueryOptions) {
				options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
				options.InferenceStrategy = topologyoptions.InferenceStrategySTPFDBCorrelated
			},
		},
		"cdp-fdb-hybrid": {
			build: newMixedL2L3ControlScenario,
			configure: func(options *topologyoptions.QueryOptions) {
				options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
				options.InferenceStrategy = topologyoptions.InferenceStrategyCDPFDBHybrid
			},
		},
		"focus-depth": {build: newFocusDepthL2Scenario},
		"include-non-ip-inferred": {
			build: newMixedL2L3ControlScenario,
			configure: func(options *topologyoptions.QueryOptions) {
				options.EliminateNonIPInferred = false
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scenario := tc.build()
			if tc.configure != nil {
				tc.configure(&scenario.opts)
			}
			registry, diagnostics := newTopologyScenarioReplayFixture(t, scenario)

			live, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(scenario.opts)
			require.NoError(t, err)
			require.True(t, ok)

			replayed, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t,
				topologyv1test.NormalizeData(t, live),
				topologyv1test.NormalizeData(t, replayed),
			)
			require.NoError(t, topologyv1test.ValidateData(replayed))
			require.Equal(t, topologyScenarioCollectedAt, replayed.CollectedAt)
		})
	}
}

func TestTopologyDiagnosticsReplayExcludesOnlyPTRDerivedPresentation(t *testing.T) {
	scenario := newLLDPDirectScenario().PTR("192.0.2.71", "ptr-switch-a.example")
	registry, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	configureTopologyScenarioReverseDNS(t, registry, scenario)

	live, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	replayed, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)

	liveNormalized := topologyv1test.NormalizeData(t, live)
	replayedNormalized := topologyv1test.NormalizeData(t, replayed)
	require.NotEqual(t, liveNormalized, replayedNormalized)
	liveActorID := topologyNormalizedActorIDByManagementIP(t, liveNormalized, "192.0.2.71")
	replayedActorID := topologyNormalizedActorIDByManagementIP(t, replayedNormalized, "192.0.2.71")
	require.Equal(t, liveActorID, replayedActorID)
	require.Equal(t,
		stripTopologyPTRPresentation(liveNormalized, liveActorID),
		stripTopologyPTRPresentation(replayedNormalized, replayedActorID),
	)
}

func TestTopologyDiagnosticsReplayUsesCompiledOUIKernel(t *testing.T) {
	scenario := newTopologyScenario("compiled-oui-replay")
	switchA := scenario.Switch("oui-switch-a", "192.0.2.81", "08:ea:44:11:22:33")
	switchB := scenario.Switch("oui-switch-b", "192.0.2.82", "02:00:00:00:09:02")
	scenario.LLDP(switchA.Port("a-b", 1), switchB.Port("b-a", 1))
	registry, diagnostics := newTopologyScenarioReplayFixture(t, scenario)

	live, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	replayed, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)

	liveNormalized := topologyv1test.NormalizeData(t, live)
	replayedNormalized := topologyv1test.NormalizeData(t, replayed)
	require.Equal(t, liveNormalized, replayedNormalized)
	require.Equal(t,
		"Extreme Networks Headquarters",
		topologyNormalizedActorByManagementIP(t, replayedNormalized, "192.0.2.81")["vendor"],
	)
}

func TestTopologyDiagnosticsReplayRejectsIncompleteRenderableEvidence(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	require.NotEmpty(t, diagnostics.topology.devices)
	diagnostics.topology.devices[0].acquisition = nil

	_, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.Error(t, err)
	require.False(t, ok)
}

func TestTopologyGenerationOwnsProducerScope(t *testing.T) {
	scenario := newMixedL2L3ControlScenario()
	registry, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	registry.mu.Lock()
	registry.producerScopeID = "later-registry-scope"
	registry.mu.Unlock()

	live, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	replayed, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t,
		topologyv1test.NormalizeData(t, live),
		topologyv1test.NormalizeData(t, replayed),
	)
}

func TestTopologyDiagnosticsReplayRejectsMissingObservationTime(t *testing.T) {
	scenario := newLLDPDirectScenario()
	_, diagnostics := newTopologyScenarioReplayFixture(t, scenario)
	require.NotNil(t, diagnostics.topology.devices[0].acquisition.evidence)
	diagnostics.topology.devices[0].acquisition.evidence.collectedAt = time.Time{}

	_, ok, err := replayTopologyDiagnostics(diagnostics, scenario.opts)
	require.Error(t, err)
	require.False(t, ok)
}

func newTopologyScenarioReplayFixture(
	t testing.TB,
	scenario *topologyScenario,
) (*topologyRegistry, topologyDiagnostics) {
	t.Helper()

	const sequence = uint64(1)
	registry := newTopologyRegistry()
	registry.producerScopeID = topologyScenarioProducerScopeID
	states := make(map[ddsnmp.DeviceRegistrationID]deviceRefreshState, len(scenario.devs))
	entries := make([]ddsnmp.DeviceEntry, 0, len(scenario.devs))
	selected := make(map[ddsnmp.DeviceRegistrationID]bool, len(scenario.devs))
	seen := make(map[ddsnmp.DeviceRegistrationID]bool, len(scenario.devs))

	for index, device := range scenario.devs {
		registrationID := ddsnmp.DeviceRegistrationID(index + 1)
		info := ddsnmp.DeviceConnectionInfo{
			Hostname:    device.target,
			SysObjectID: device.sysObjectID,
			SysName:     device.name,
			SysDescr:    topologyScenarioSysDescr,
		}
		metrics := &ddsnmp.ProfileMetrics{
			DeviceMetadata:  scenario.deviceMetadata(device),
			TopologyMetrics: scenario.topologyMetricsForDevice(device),
			BGPRows:         scenario.bgpRowsForDevice(device),
		}
		recorder := newTopologyAcquisitionRecorder(
			topologyAcquisitionAttemptID{registrationID: registrationID, ordinal: 1},
			topologySemanticDeviceInputFromConnection(info),
			topologyTargetResolutionEvidence{outcome: topologyTargetResolutionEmpty},
			defaultTopologyAcquisitionLimits,
		)
		observer := recorder.beginContext(0, "", "")
		observer.ObserveProfile(acquisitionReportForMetrics(
			0,
			ddsnmpcollector.AcquisitionProfileOutcomeSuccess,
			metrics,
		), metrics)
		recorder.completeContext(0, successfulAcquisitionPhase())
		recorder.setCollectedShape(topologyScenarioCollectedAt, time.Hour, 3600)
		capture := recorder.finish()
		require.Equal(t, diagnosticCaptureAvailable, capture.state)

		snapshot, _ := freezeTopologyBuilder(scenario.cacheForDevice(t, device))
		snapshot.acquisition = capture
		generation := activateTopologyDeviceSnapshot(
			registrationID,
			sequence,
			topologyScenarioCollectedAt,
			snapshot,
		)
		state := deviceRefreshState{
			generation:     generation,
			latestAttempt:  capture,
			attemptOrdinal: 1,
			lastAttempt:    topologyScenarioCollectedAt,
			lastSuccess:    topologyScenarioCollectedAt,
			outcome:        deviceRefreshOutcomeSuccess,
		}
		states[registrationID] = state
		entries = append(entries, ddsnmp.DeviceEntry{RegistrationID: registrationID, Info: info})
		selected[registrationID] = true
		seen[registrationID] = true
	}

	generation := newTopologyGeneration(
		sequence,
		topologyScenarioCollectedAt,
		topologyScenarioProducerScopeID,
		states,
	)
	cut, err := projectTopologyDiagnosticCut(topologyDiagnosticCutInput{
		sequence:    sequence,
		startedAt:   topologyScenarioCollectedAt.Add(-time.Minute),
		publishedAt: topologyScenarioCollectedAt,
		entries:     entries,
		selected:    selected,
		seen:        seen,
		states:      states,
		limits:      defaultTopologyDiagnosticGlobalLimits,
	})
	require.NoError(t, err)
	generation.diagnostic = cut
	registry.publishGeneration(generation)

	return registry, topologyDiagnostics{
		producerScopeID: generation.producerScopeID,
		topology:        generation.diagnostic,
	}
}

func configureTopologyScenarioReverseDNS(
	t testing.TB,
	registry *topologyRegistry,
	scenario *topologyScenario,
) {
	t.Helper()
	if len(scenario.ptr) == 0 {
		return
	}
	dns := newTestTopologyReverseDNSWarmer(testTopologyReverseDNSConfig{
		now: newReverseDNSTestClock().Now,
		lookup: func(_ context.Context, ip string) ([]string, error) {
			if name := scenario.ptr[ip]; name != "" {
				return []string{name}, nil
			}
			return nil, nil
		},
	})
	ips := make([]string, 0, len(scenario.ptr))
	for ip := range scenario.ptr {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	dns.warm(context.Background(), ips)
	registry.reverseDNS = dns.resolver
}

func topologyNormalizedActorIDByManagementIP(
	t testing.TB,
	data topologyv1test.NormalizedData,
	managementIP string,
) string {
	t.Helper()
	actor := topologyNormalizedActorByManagementIP(t, data, managementIP)
	actorID, ok := actor["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, actorID)
	return actorID
}

func topologyNormalizedActorByManagementIP(
	t testing.TB,
	data topologyv1test.NormalizedData,
	managementIP string,
) map[string]any {
	t.Helper()
	for _, row := range data.Actors.Rows {
		if row["management_ip"] == managementIP {
			return row
		}
	}
	require.FailNow(t, "topology actor not found", "management_ip=%s", managementIP)
	return nil
}

func stripTopologyPTRPresentation(data topologyv1test.NormalizedData, actorID string) topologyv1test.NormalizedData {
	for _, row := range data.Actors.Rows {
		if row["id"] != actorID {
			continue
		}
		delete(row, "display_name")
		delete(row, "dns_names")
	}
	if data.Tables == nil {
		return data
	}
	for tableID, ref := range data.Tables.Actor {
		rows := ref.Table.Rows[:0]
		for _, row := range ref.Table.Rows {
			if row["actor"] != actorID {
				rows = append(rows, row)
				continue
			}
			key, _ := row["key"].(string)
			if key == "display_name" || key == "display_source" || key == "dns_name" {
				continue
			}
			if labels, ok := row["labels"].(map[string]any); ok {
				delete(labels, "display_name")
				delete(labels, "display_source")
			}
			rows = append(rows, row)
		}
		ref.Table.Rows = rows
		data.Tables.Actor[tableID] = ref
	}
	return data
}
