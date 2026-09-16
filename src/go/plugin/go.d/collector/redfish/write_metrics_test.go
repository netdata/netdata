// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

func TestHardwareWriterPublishesDeclaredAlarmStates(t *testing.T) {
	for _, state := range alarmStates {
		t.Run(state, func(t *testing.T) {
			collector := New()
			managed, ok := metrix.AsCycleManagedStore(collector.store)
			require.True(t, ok)
			cycle := managed.CycleController()
			cycle.BeginCycle()
			collector.hardware.observe([]hardwareObservation{{Metric: "reading_alarm", State: state}})
			require.NoError(t, cycle.CommitCycleSuccess())
			point, ok := collector.store.Read().StateSet("reading_alarm", nil)
			require.True(t, ok)
			expected := map[string]bool{"clear": false, "warning": false, "critical": false}
			expected[state] = true
			require.Equal(t, expected, point.States)
		})
	}
}
