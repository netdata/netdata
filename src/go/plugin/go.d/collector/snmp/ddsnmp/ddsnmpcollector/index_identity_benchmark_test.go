// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type decodedTagWorkload struct {
	name, index, want string
	config            ddprofiledefinition.MetricTagConfig
}

func decodedTagWorkloads() []decodedTagWorkload {
	fixed48 := uint(48)
	octets := func(n int) string { return strings.TrimSuffix(strings.Repeat("97.", n), ".") }
	config := func(component uint, components ...ddprofiledefinition.MetricIndexComponent) ddprofiledefinition.MetricTagConfig {
		return ddprofiledefinition.MetricTagConfig{
			Tag:         "identity",
			IndexDecode: &ddprofiledefinition.MetricIndexDecode{Component: component, Components: components},
			Symbol:      ddprofiledefinition.SymbolConfigCompat{Format: "octet_string"},
		}
	}
	integer := ddprofiledefinition.MetricIndexComponent{Type: "integer"}
	variable := ddprofiledefinition.MetricIndexComponent{Type: "octet_string"}
	return []decodedTagWorkload{
		{
			name:   "multiple-variable-strings",
			index:  "3.70.79.79.3.98.97.114.2",
			want:   "bar",
			config: config(2, variable, variable, integer),
		},
		{
			name:   "variable-112-octets",
			index:  "1.112." + octets(112),
			want:   strings.Repeat("a", 112),
			config: config(2, integer, variable),
		},
		{
			name:  "fixed-48-octets",
			index: octets(48) + ".7",
			want:  strings.Repeat("a", 48),
			config: config(
				1,
				ddprofiledefinition.MetricIndexComponent{Type: "octet_string", Length: &fixed48},
				integer,
			),
		},
	}
}

// Full-layout validation and final text formatting are O(index arcs), with a constant allocation count. These
// opt-in layouts do not add decoding work to legacy single-index tags. ns/op is a workstation trend, not a CI gate.
func BenchmarkIndexIdentityDecodedTag(b *testing.B) {
	processor := newTableRowProcessor(logger.New())
	for _, tc := range decodedTagWorkloads() {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, value, err := processor.processIndexTag(tc.config, tc.index)
				if err != nil || value != tc.want {
					b.Fatalf("decoded identity %q: %v", value, err)
				}
			}
		})
	}
}

func TestIndexIdentityAllocationEnvelopes(t *testing.T) {
	processor := newTableRowProcessor(logger.New())
	legacy := ddprofiledefinition.MetricTagConfig{Tag: "identity", Index: 1}
	var value string
	var err error
	legacyAllocs := testing.AllocsPerRun(100, func() { _, value, err = processor.processIndexTag(legacy, "1.2.3.4") })
	require.NoError(t, err)
	require.Equal(t, "1", value)
	require.Zero(t, legacyAllocs, "single-index common path stays allocation-free")
	for _, tc := range decodedTagWorkloads() {
		t.Run(tc.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(
				100,
				func() { _, value, err = processor.processIndexTag(tc.config, tc.index) },
			)
			require.NoError(t, err)
			require.Equal(t, tc.want, value)
			require.LessOrEqual(
				t,
				allocs,
				float64(5),
				"split/layout selection and final octet conversion have a constant allocation envelope",
			)
			t.Logf("decoded tag: %.0f allocs", allocs)
		})
	}
}

func indexIdentityCacheConfigs() map[string]ddprofiledefinition.MetricsConfig {
	const root = "1.3.6.1.4.1.99999.991"
	legacy := ddprofiledefinition.MetricsConfig{
		MIB:     "EXAMPLE-MIB",
		Table:   ddprofiledefinition.SymbolConfig{OID: root, Name: "rows"},
		Symbols: []ddprofiledefinition.SymbolConfig{{OID: root + ".1.1", Name: "value"}},
		MetricTags: []ddprofiledefinition.MetricTagConfig{
			{Symbol: ddprofiledefinition.SymbolConfigCompat{Name: "row_index"}, Index: 1},
			{Tag: "name", Symbol: ddprofiledefinition.SymbolConfigCompat{OID: root + ".1.2", Name: "rowName"}},
			{
				Tag:     "state",
				Symbol:  ddprofiledefinition.SymbolConfigCompat{OID: root + ".1.3", Name: "rowState"},
				Mapping: ddprofiledefinition.NewExactMapping(map[string]string{"1": "up", "2": "down"}),
			},
		},
	}
	decoded := legacy.Clone()
	decoded.MetricTags[0] = decodedTagWorkloads()[0].config
	// Preserve fallback naming and ordinary companion tags: all participate in opt-in cached identity.
	decoded.MetricTags[0].Tag = ""
	decoded.MetricTags[0].Symbol.Name = "counter_name"
	lookup := legacy.Clone()
	lookup.MetricTags[0].LookupSymbol = ddprofiledefinition.SymbolConfigCompat{OID: root + ".2.1", Name: "lookupKey", Format: "octet_string"}
	return map[string]ddprofiledefinition.MetricsConfig{
		"lookup-with-companions":  lookup,
		"legacy-three-tags":       legacy,
		"decoded-with-companions": decoded,
	}
}

// Config IDs are computed per table/configuration, not per metric row. Opt-in identity encodes the complete tag
// configuration in O(encoded tag configuration size); legacy tables keep their established compact path.
func BenchmarkIndexIdentityCacheConfigID(b *testing.B) {
	cache := &tableCache{}
	for name, config := range indexIdentityCacheConfigs() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if cache.generateConfigID(config) == "" {
					b.Fatal("empty config identity")
				}
			}
		})
	}
}
