// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestBuildMultiValue_BitmaskZeroKeyMatchesOnlyZero(t *testing.T) {
	mapping := ddprofiledefinition.NewBitmaskMapping(map[string]string{
		"0": "noFaults",
		"1": "warning",
		"2": "failure",
	})

	require.Equal(t, map[string]int64{
		"noFaults": 1,
		"warning":  0,
		"failure":  0,
	}, buildMultiValue(0, mapping))

	require.Equal(t, map[string]int64{
		"noFaults": 0,
		"warning":  1,
		"failure":  0,
	}, buildMultiValue(1, mapping))
}

func TestBuildMultiValue_BitmaskDuplicateDimsAreCombined(t *testing.T) {
	mapping := ddprofiledefinition.NewBitmaskMapping(map[string]string{
		"1": "fault",
		"2": "fault",
		"4": "present",
	})

	require.Equal(t, map[string]int64{
		"fault":   1,
		"present": 1,
	}, buildMultiValue(5, mapping))

	require.Equal(t, map[string]int64{
		"fault":   0,
		"present": 0,
	}, buildMultiValue(0, mapping))
}
