// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

func TestLifecyclePhasesAreCompleteAndRoundTrip(t *testing.T) {
	require.Len(t, lifecyclePhases, int(ddsnmp.DeviceLifecyclePhaseCollect)+1)
	for i, want := range []string{"unknown", "init", "check", "collect"} {
		phase := ddsnmp.DeviceLifecyclePhase(i)
		name, err := LifecyclePhaseName(phase)
		require.NoError(t, err)
		require.Equal(t, want, name)
		restored, err := ParseLifecyclePhase(name)
		require.NoError(t, err)
		require.Equal(t, phase, restored)
	}
	_, err := LifecyclePhaseName(ddsnmp.DeviceLifecyclePhase(255))
	require.Error(t, err)
	_, err = ParseLifecyclePhase("invalid")
	require.Error(t, err)
}

func TestLifecycleOutcomesAreCompleteAndRoundTrip(t *testing.T) {
	require.Len(t, lifecycleOutcomes, int(ddsnmp.DeviceLifecycleOutcomeFailed)+1)
	for i, want := range []string{"unknown", "success", "failed"} {
		outcome := ddsnmp.DeviceLifecycleOutcome(i)
		name, err := LifecycleOutcomeName(outcome)
		require.NoError(t, err)
		require.Equal(t, want, name)
		restored, err := ParseLifecycleOutcome(name)
		require.NoError(t, err)
		require.Equal(t, outcome, restored)
	}
	_, err := LifecycleOutcomeName(ddsnmp.DeviceLifecycleOutcome(255))
	require.Error(t, err)
	_, err = ParseLifecycleOutcome("invalid")
	require.Error(t, err)
}

type fixedLifecycleSource struct{ cut ddsnmp.DeviceLifecycleCut }

func (s fixedLifecycleSource) LifecycleCut() ddsnmp.DeviceLifecycleCut { return s.cut }

func TestProfileContextLimitPreservesLifecycleAndSourceCut(t *testing.T) {
	profile, err := ddsnmp.RestoreProfileContext(
		ddsnmp.ProfileContextData{State: "available", ManualPolicy: "fallback", BGPMode: "absent", SysDescr: strings.Repeat("s", 8192)},
		MaxRecords,
		MaxLogicalBytes,
	)
	require.NoError(t, err)
	source := fixedLifecycleSource{
		cut: ddsnmp.DeviceLifecycleCut{
			Sequence: 7,
			Entries: []ddsnmp.DeviceLifecycleEntry{
				{
					RegistrationID: 1,
					Info:           ddsnmp.DeviceLifecycleInfo{Hostname: "192.0.2.1", Profiles: profile},
					LastCompleted: ddsnmp.DeviceLifecycleStatus{
						Phase:   ddsnmp.DeviceLifecyclePhaseCheck,
						Outcome: ddsnmp.DeviceLifecycleOutcomeFailed,
					},
				},
			},
		},
	}
	result := CaptureLifecycle(source, MaxRecords, 4096)
	require.Equal(t, "available", result.State)
	require.Len(t, result.Cut.Entries, 1)
	require.Equal(t, "failed", result.Cut.Entries[0].LastCompleted.Outcome)
	require.Equal(t, "limit_exceeded", result.Cut.Entries[0].Profiles.State)
	require.Equal(t, "available", source.cut.Entries[0].Info.Profiles.Snapshot().State, "admission must not mutate a source-owned cut")
}
