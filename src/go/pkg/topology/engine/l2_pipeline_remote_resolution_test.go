// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsMACCompatibleWithDevice_NormalizesRemoteMAC(t *testing.T) {
	state := newL2BuildState(1)
	state.devices["known-device"] = Device{
		ID:        "known-device",
		Hostname:  "switch-a",
		ChassisID: "00:11:22:33:44:55",
	}

	require.True(t, state.isMACCompatibleWithDevice("known-device", "0011.2233.4455"))
	require.True(t, state.isMACCompatibleWithDevice("known-device", "0x001122334455"))
	require.False(t, state.isMACCompatibleWithDevice("known-device", "00-11-22-33-44-66"))
}
