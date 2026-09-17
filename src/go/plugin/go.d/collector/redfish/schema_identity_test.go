// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResourceSchemaKind(t *testing.T) {
	require.NoError(t, validateResourceSchemaType("sensor", "#Sensor.v1_9_0.Sensor"))
	require.Error(t, validateResourceSchemaType("sensor", "#Fan.v1_0_0.Fan"))
	require.Error(t, validateResourceSchemaType("sensor", ""))
}
