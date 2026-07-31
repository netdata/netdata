// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"testing"

	"github.com/stmcginnis/gofish/schemas"
	"github.com/stretchr/testify/require"
)

func TestDecodeTypedResourceDoesNotRetainRawPayload(t *testing.T) {
	raw := []byte(`{
		"@odata.id":"/redfish/v1/Systems/1",
		"@odata.type":"#ComputerSystem.v1_21_0.ComputerSystem",
		"Id":"1",
		"Name":"System"
	}`)

	decoded, err := decodeTypedResource("system", raw)
	require.NoError(t, err)

	system, ok := decoded.(*schemas.ComputerSystem)
	require.True(t, ok)
	require.Nil(t, system.RawData)
	require.Equal(t, "System", system.Name)
}

func TestDecodeTypedSummaryMetricsUsesStandardSchemaTypes(t *testing.T) {
	tests := map[string]struct {
		odataType string
		wantType  any
	}{
		"processor_summary_metrics": {
			odataType: "#ProcessorMetrics.v1_6_0.ProcessorMetrics",
			wantType:  &schemas.ProcessorMetrics{},
		},
		"memory_summary_metrics": {
			odataType: "#MemoryMetrics.v1_7_0.MemoryMetrics",
			wantType:  &schemas.MemoryMetrics{},
		},
	}
	for kind, test := range tests {
		t.Run(kind, func(t *testing.T) {
			raw := []byte(`{
				"@odata.id":"/redfish/v1/Systems/1/Summary/Metrics",
				"@odata.type":"` + test.odataType + `",
				"Id":"Metrics",
				"Name":"Summary Metrics"
			}`)
			decoded, err := decodeTypedResource(kind, raw)
			require.NoError(t, err)
			require.IsType(t, test.wantType, decoded)
		})
	}
}
