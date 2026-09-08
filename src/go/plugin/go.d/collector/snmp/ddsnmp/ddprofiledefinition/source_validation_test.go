// SPDX-License-Identifier: GPL-3.0-or-later

package ddprofiledefinition

import (
	"fmt"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestValidateEnrichProfile_SourceRequirements(t *testing.T) {
	for name, tc := range map[string]struct{ input, wantError string }{
		"metric columns need table OID": {
			input:     `metrics: [{symbols: [{OID: 1.2.3.1, name: column}], metric_tags: [{index: 1}]}]`,
			wantError: "metrics[0].table.OID",
		},
		"topology columns need table OID": {
			input:     `topology: [{kind: if_name, symbols: [{OID: 1.2.3.1, name: column}], metric_tags: [{index: 1}]}]`,
			wantError: "topology[0].table.OID",
		},
		"table name remains optional": {
			input:     `metrics: [{table: {OID: 1.2.3}, symbols: [{OID: 1.2.3.1, name: column}], metric_tags: [{index: 1}]}]`,
			wantError: "",
		},
		"topology table name remains optional": {
			input:     `topology: [{kind: if_name, table: {OID: 1.2.3}, symbols: [{OID: 1.2.3.1, name: column}]}]`,
			wantError: "",
		},
		"global index rejected": {
			input:     `metric_tags: [{tag: row, index: 1}]`,
			wantError: "metric_tags[0]: scalar metric_tags do not support `index`",
		},
		"global table rejected": {
			input:     `metric_tags: [{tag: row, table: example, symbol: {OID: 1.2.3, name: row}}]`,
			wantError: "metric_tags[0]: scalar metric_tags do not support `table`",
		},
		"global transform rejected": {
			input:     `metric_tags: [{tag: row, index_transform: [{start: 1}]}]`,
			wantError: "metric_tags[0]: scalar metric_tags do not support `index_transform`",
		},
		"global missing source rejected": {
			input:     `metric_tags: [{tag: row}]`,
			wantError: "metric_tags[0]: scalar metric_tags require `symbol.OID`",
		},
		"global legacy alias accepted": {
			input:     `metric_tags: [{tag: row, OID: 1.2.3.0, symbol: row}]`,
			wantError: "",
		},
		"named index accepted": {
			input:     `metrics: [{table: {OID: 1.2.3}, symbols: [{OID: 1.2.3.1, name: column}], metric_tags: [{index: 1, symbol: {name: row}}]}]`,
			wantError: "",
		},
		"empty metadata rejected": {
			input:     `metadata: {device: {fields: {model: {}}}}`,
			wantError: "metadata.device.fields.model: must have either value or symbol(s)",
		},
		"name only metadata rejected": {
			input:     `metadata: {device: {fields: {model: {symbol: {name: model}}}}}`,
			wantError: "metadata.device.fields.model.symbol: symbol oid missing",
		},
		"malformed metadata with static value rejected": {
			input:     `metadata: {device: {fields: {model: {value: fallback, symbol: {name: model}}}}}`,
			wantError: "metadata.device.fields.model.symbol: symbol oid missing",
		},
		"format only metadata rejected": {
			input:     `metadata: {device: {fields: {model: {value: fallback, symbol: {format: hex}}}}}`,
			wantError: "metadata.device.fields.model.symbol: symbol name missing",
		},
		"ordinary metadata fallback accepted": {
			input:     `metadata: {device: {fields: {model: {value: fallback, symbol: {OID: 1.2.3.0, name: model}}}}}`,
			wantError: "",
		},
		"malformed sysobjectid metadata rejected": {
			input:     `sysobjectid_metadata: [{sysobjectid: 1.2.3, metadata: {model: {value: fallback, symbol: {name: model}}}}]`,
			wantError: "sysobjectid_metadata[0].model.symbol: symbol oid missing",
		},
		"topology symbol path": {
			input:     `topology: [{kind: if_name, table: {OID: 1.2.3}, symbols: [{OID: 1.2.3.1, name: first}, {OID: 1.2.3.2}]}]`,
			wantError: "topology[0].symbols[1]: symbol name missing",
		},
		"topology tag path": {
			input:     `topology: [{kind: if_name, table: {OID: 1.2.3}, symbols: [{OID: 1.2.3.1, name: first}], metric_tags: [{index: 1}, {symbol: {name: broken}}]}]`,
			wantError: "topology[0].metric_tags[1]",
		},
		"global tag path": {
			input:     `metric_tags: [{symbol: {OID: 1.2.3}}]`,
			wantError: "metric_tags[0]",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var profile ProfileDefinition
			require.NoError(t, yaml.Unmarshal([]byte(tc.input), &profile))
			err := ValidateEnrichProfile(&profile)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEnrichProfile_LicenseSourceBounds(t *testing.T) {
	for name, tc := range map[string]struct{ source, wantError string }{
		"from inside": {
			source:    `from: 1.2.3.1`,
			wantError: "",
		},
		"symbol inside": {
			source:    `symbol: {OID: 1.2.3.1, name: state}`,
			wantError: "",
		},
		"legacy inside": {
			source:    `OID: 1.2.3.1, name: state`,
			wantError: "",
		},
		"from outside": {
			source:    `from: 9.9.1`,
			wantError: "outside table",
		},
		"symbol outside": {
			source:    `symbol: {OID: 9.9.1, name: state}`,
			wantError: "outside table",
		},
		"legacy outside": {
			source:    `OID: 9.9.1, name: state`,
			wantError: "outside table",
		},
		"legacy unnamed outside": {
			source:    `OID: 9.9.1`,
			wantError: "outside table",
		},
		"symbol wins outside": {
			source:    `from: 1.2.3.1, symbol: {OID: 9.9.1, name: state}`,
			wantError: "outside table",
		},
		"symbol wins inside": {
			source:    `from: 9.9.1, symbol: {OID: 1.2.3.1, name: state}`,
			wantError: "",
		},
		"from wins legacy": {
			source:    `from: 1.2.3.1, OID: 9.9.1, name: state`,
			wantError: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var profile ProfileDefinition
			require.NoError(
				t,
				yaml.Unmarshal(
					[]byte(`licensing: [{table: {OID: 1.2.3, name: example}, state: {`+tc.source+`}}]`),
					&profile,
				),
			)
			err := ValidateEnrichProfile(&profile)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEnrichProfile_VirtualSourceDimensionUnion(t *testing.T) {
	for name, tc := range map[string]struct {
		first, second, dim string
		wantError          bool
	}{
		"known dimensions": {
			first:     `mapping: {1: first}`,
			second:    `mapping: {1: second}`,
			dim:       "first",
			wantError: false,
		},
		"known and raw": {
			first:     `mapping: {1: first}`,
			second:    `metric_type: gauge`,
			dim:       "first",
			wantError: false,
		},
		"mapping and transform dimensions": {
			first:     `mapping: {1: first}`,
			second:    `transform: '{{ setMultivalue .Metric (i64map "dynamic" 1) }}'`,
			dim:       "dynamic",
			wantError: false,
		},
		"missing dimension": {
			first:     `mapping: {1: first}`,
			second:    `mapping: {1: second}`,
			dim:       "absent",
			wantError: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			for order, fields := range map[string][]string{"forward": {tc.first, tc.second}, "reverse": {tc.second, tc.first}} {
				t.Run(order, func(t *testing.T) {
					var profile ProfileDefinition
					input := fmt.Sprintf(`metrics:
 - table: {OID: 1.2.3, name: example}
   symbols:
    - {OID: 1.2.3.1, name: _shared, %s}
    - {OID: 1.2.3.2, name: _shared, %s}
   metric_tags: [{index: 1}]
virtual_metrics:
 - name: result
   per_row: true
   sources: [{metric: _shared, table: example, dim: %s}]
`, fields[0], fields[1], tc.dim)
					require.NoError(t, yaml.Unmarshal([]byte(input), &profile))
					err := ValidateEnrichProfile(&profile)
					if tc.wantError {
						require.ErrorContains(t, err, "is not available")
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
}

func TestCollectVirtualMetricSourceSpecs_DimensionUnion(t *testing.T) {
	for name, tc := range map[string]struct {
		symbols []SymbolConfig
		want    virtualMetricDimSupport
	}{
		"known union": {
			symbols: []SymbolConfig{{Mapping: NewExactMapping(map[string]string{"1": "first"})}, {Mapping: NewExactMapping(map[string]string{"1": "second"})}},
			want: virtualMetricDimSupport{
				mode: virtualMetricDimKnown,
				dims: map[string]bool{"first": true, "second": true},
			},
		},
		"dynamic dominates": {
			symbols: []SymbolConfig{{Transform: `{{ transformEntitySensorValue .Metric }}`}, {Mapping: NewExactMapping(map[string]string{"1": "first"})}},
			want: virtualMetricDimSupport{
				mode: virtualMetricDimDynamic,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				symbols := slices.Clone(tc.symbols)
				for i := range symbols {
					symbols[i].Name = "shared"
				}
				if reverse {
					slices.Reverse(symbols)
				}
				got := collectVirtualMetricSourceSpecs([]MetricsConfig{{Table: SymbolConfig{
					Name: "example",
				}, Symbols: symbols}})
				assert.Equal(t, tc.want, got["shared"]["example"].dimSupport)
			}
		})
	}
}

func TestValidateEnrichProfile_BitmaskKeys(t *testing.T) {
	for name, tc := range map[string]struct {
		key       string
		wantError bool
	}{
		"zero":          {key: "0"},
		"low bit":       {key: "1"},
		"highest bit":   {key: "9223372036854775808"},
		"overflow":      {key: "18446744073709551616", wantError: true},
		"multiple bits": {key: "9223372036854775809", wantError: true},
		"negative":      {key: "-1", wantError: true},
	} {
		t.Run(name, func(t *testing.T) {
			var profile ProfileDefinition
			require.NoError(
				t,
				yaml.Unmarshal(
					[]byte(
						fmt.Sprintf(
							`metrics: [{symbol: {OID: 1.2.3.0, name: flags, mapping: {mode: bitmask, items: {%q: state}}}}]`,
							tc.key,
						),
					),
					&profile,
				),
			)
			err := ValidateEnrichProfile(&profile)
			if tc.wantError {
				require.ErrorContains(t, err, "single power-of-two bit")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateEnrichProfile_BGPSourceBounds(t *testing.T) {
	for contextName, context := range map[string]struct {
		tableField string
		wantError  string
	}{
		"implicit row table":       {wantError: "outside table"},
		"explicit table reference": {tableField: "table: peers,", wantError: "outside referenced table"},
	} {
		t.Run(contextName, func(t *testing.T) {
			for name, tc := range map[string]struct {
				fields    string
				wantError bool
			}{
				"symbol inside":                 {fields: `symbol: {OID: 1.2.3.1, name: state}`},
				"symbol outside":                {fields: `symbol: {OID: 9.9.1, name: state}`, wantError: true},
				"from inside":                   {fields: `from: 1.2.3.1`},
				"from outside":                  {fields: `from: 9.9.1`, wantError: true},
				"legacy inside":                 {fields: `OID: 1.2.3.1, name: state`},
				"legacy outside":                {fields: `OID: 9.9.1, name: state`, wantError: true},
				"symbol outside wins":           {fields: `from: 1.2.3.1, symbol: {OID: 9.9.1, name: state}`, wantError: true},
				"symbol inside wins":            {fields: `from: 9.9.1, symbol: {OID: 1.2.3.1, name: state}`},
				"from inside wins over legacy":  {fields: `from: 1.2.3.1, OID: 9.9.1, name: state`},
				"from outside wins over legacy": {fields: `from: 9.9.1, OID: 1.2.3.1, name: state`, wantError: true},
			} {
				t.Run(name, func(t *testing.T) {
					var profile ProfileDefinition
					require.NoError(t, yaml.Unmarshal([]byte(`bgp:
 - kind: peer
   table: {OID: 1.2.3, name: peers}
   identity: {neighbor: {value: "192.0.2.1"}, remote_as: {value: "65001"}}
   state: {`+context.tableField+` mapping: {1: idle, 2: connect, 3: active, 4: opensent, 5: openconfirm, 6: established}, `+tc.fields+`}
`), &profile))
					err := ValidateEnrichProfile(&profile)
					if tc.wantError {
						require.ErrorContains(t, err, context.wantError)
					} else {
						require.NoError(t, err)
					}
				})
			}
		})
	}
}
