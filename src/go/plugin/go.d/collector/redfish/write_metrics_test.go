// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
	"github.com/stretchr/testify/require"
)

func TestHardwareWriterPublishesDeclaredAlarmStates(t *testing.T) {
	for _, state := range []string{"clear", "warning", "critical"} {
		t.Run(state, func(t *testing.T) {
			collector := New()
			managed, ok := metrix.AsCycleManagedStore(collector.store)
			require.True(t, ok)
			cycle := managed.CycleController()
			cycle.BeginCycle()
			collector.hardware.observe([]measurement.Observation{{Metric: "reading_alarm_status", State: state}})
			require.NoError(t, cycle.CommitCycleSuccess())
			point, ok := collector.store.Read().StateSet("reading_alarm_status", nil)
			require.True(t, ok)
			expected := map[string]bool{"clear": false, "warning": false, "critical": false}
			expected[state] = true
			require.Equal(t, expected, point.States)
		})
	}
}
