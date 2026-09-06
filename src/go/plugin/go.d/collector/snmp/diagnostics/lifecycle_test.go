// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
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
