// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"testing"

	"github.com/gosnmp/gosnmp"
	"gopkg.in/yaml.v2"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestMetricBuilder_WithStaticTagsFillsMissingAndEmptyValues(t *testing.T) {
	metric := newMetricBuilder("testMetric", 1).
		withTags(map[string]string{
			"region":   "",
			"neighbor": "192.0.2.10",
		}).
		withStaticTags(map[string]string{
			"region": "eu-west",
			"site":   "athens",
		}).
		build()

	require.Equal(t, map[string]string{
		"region":   "eu-west",
		"neighbor": "192.0.2.10",
		"site":     "athens",
	}, metric.Tags)
	require.Equal(t, map[string]string{
		"region": "eu-west",
		"site":   "athens",
	}, metric.StaticTags)
}

func TestMetricBuilder_WithStaticTagsKeepsExistingNonEmptyValues(t *testing.T) {
	metric := newMetricBuilder("testMetric", 1).
		withTags(map[string]string{
			"region": "edge",
		}).
		withStaticTags(map[string]string{
			"region": "core",
		}).
		build()

	require.Equal(t, map[string]string{"region": "edge"}, metric.Tags)
	require.Equal(t, map[string]string{"region": "core"}, metric.StaticTags)
}

func TestBuildMultiValue_BitmaskZeroKeyMatchesOnlyZero(t *testing.T) {
	mapping := ddprofiledefinition.MappingConfig{
		Mode: ddprofiledefinition.MappingModeBitmask,
		Items: map[string]string{
			"0": "noFaults",
			"1": "warning",
			"2": "failure",
		},
	}

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
	mapping := ddprofiledefinition.MappingConfig{
		Mode: ddprofiledefinition.MappingModeBitmask,
		Items: map[string]string{
			"1": "fault",
			"2": "fault",
			"4": "present",
		},
	}

	require.Equal(t, map[string]int64{
		"fault":   1,
		"present": 1,
	}, buildMultiValue(5, mapping))

	require.Equal(t, map[string]int64{
		"fault":   0,
		"present": 0,
	}, buildMultiValue(0, mapping))
}

// Legacy row overrides must reach the same metric builder as symbol-level overrides.
func TestBuildTableMetric_LegacyMetricType(t *testing.T) {
	tests := map[string]struct {
		rowType    string
		symbolType string
		want       ddprofiledefinition.ProfileMetricType
	}{
		"PDU default": {want: ddprofiledefinition.ProfileMetricTypeRate},
		"row gauge":   {rowType: "gauge", want: ddprofiledefinition.ProfileMetricTypeGauge},
		"row monotonic count": {
			rowType: "monotonic_count",
			want:    ddprofiledefinition.ProfileMetricTypeMonotonicCount,
		},
		"symbol override wins": {
			rowType:    "gauge",
			symbolType: "rate",
			want:       ddprofiledefinition.ProfileMetricTypeRate,
		},
		"legacy counter remains a rate type": {rowType: "counter", want: "counter"},
		"legacy percent remains a rate type": {rowType: "percent", want: "percent"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			input := fmt.Sprintf(`metrics:
  - table: {OID: "1.3.6.1.2.1.2.2", name: ifTable}
    metric_type: %q
    symbols:
      - OID: "1.3.6.1.2.1.2.2.1.10"
        name: ifInOctets
        metric_type: %q
    metric_tags:
      - {tag: interface, index: 1}
`, tc.rowType, tc.symbolType)
			var def ddprofiledefinition.ProfileDefinition
			require.NoError(t, yaml.Unmarshal([]byte(input), &def))
			require.NoError(t, ddprofiledefinition.ValidateEnrichProfile(&def))
			cfg := def.Metrics[0]
			metric, err := buildTableMetric(
				cfg.Symbols[0],
				gosnmp.SnmpPDU{
					Type:  gosnmp.Counter32,
					Value: uint32(100),
				},
				100,
				map[string]string{"interface": "1"},
				nil,
				cfg.Table.Name,
			)
			require.NoError(t, err)
			require.Equal(t, tc.want, metric.MetricType)
			require.Empty(t, cfg.MetricType)
			require.Empty(t, cfg.Symbol.MetricType)
		})
	}
}
