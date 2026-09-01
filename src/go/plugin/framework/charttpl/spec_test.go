// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testLabelView map[string]string

func (m testLabelView) Len() int { return len(m) }

func (m testLabelView) Get(key string) (string, bool) {
	value, ok := m[key]
	return value, ok
}

func (m testLabelView) Range(fn func(key, value string) bool) {
	for key, value := range m {
		if !fn(key, value) {
			return
		}
	}
}

func (m testLabelView) CloneMap() map[string]string {
	return map[string]string(m)
}

var _ metrix.LabelView = testLabelView{}

func TestDecodeYAMLScenarios(t *testing.T) {
	tests := map[string]struct {
		input   string
		wantErr bool
		assert  func(t *testing.T, spec *Spec)
	}{
		"valid grouped spec and default chart type": {
			input: `
version: v1
context_namespace: mysql
engine:
  selector:
    allow:
      - mysql_queries_total{db="main"}
  autogen:
    enabled: true
    rules:
      - scope: "mysql_*"
        selector:
          deny:
            - " Business Unit "
            - "μέτρο*"
    max_type_id_len: 512
    expire_after_success_cycles: 9
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        algorithm: incremental
        aggregation: min
        dimensions:
          - selector: mysql_queries_total
            name: total
            options:
              multiplier: -8
              divisor: 1000
              hidden: true
              float: true
`,
			assert: func(t *testing.T, spec *Spec) {
				t.Helper()
				require.Len(t, spec.Groups, 1)
				require.Len(t, spec.Groups[0].Charts, 1)
				assert.Equal(t, "line", spec.Groups[0].Charts[0].Type)
				assert.Equal(t, AggregationMin, spec.Groups[0].Charts[0].Aggregation)
				require.Len(t, spec.Groups[0].Charts[0].Dimensions, 1)
				require.NotNil(t, spec.Groups[0].Charts[0].Dimensions[0].Options)
				assert.Equal(t, -8, spec.Groups[0].Charts[0].Dimensions[0].Options.Multiplier)
				assert.Equal(t, 1000, spec.Groups[0].Charts[0].Dimensions[0].Options.Divisor)
				assert.True(t, spec.Groups[0].Charts[0].Dimensions[0].Options.Hidden)
				assert.True(t, spec.Groups[0].Charts[0].Dimensions[0].Options.Float)
				require.NotNil(t, spec.Engine)
				require.NotNil(t, spec.Engine.Selector)
				assert.Equal(t, []string{`mysql_queries_total{db="main"}`}, spec.Engine.Selector.Allow)
				require.NotNil(t, spec.Engine.Autogen)
				assert.True(t, spec.Engine.Autogen.Enabled)
				require.Len(t, spec.Engine.Autogen.Rules, 1)
				assert.Equal(t, []string{" Business Unit ", "μέτρο*"}, spec.Engine.Autogen.Rules[0].Selector.Deny)
				assert.Equal(t, 512, spec.Engine.Autogen.MaxTypeIDLen)
				assert.Equal(t, uint64(9), spec.Engine.Autogen.ExpireAfterSuccessCycles)
			},
		},
		"group chart defaults apply recursively with nearest-scope replace semantics": {
			input: `
version: v1
groups:
  - family: Root
    metrics:
      - mysql_queries_total
    chart_defaults:
      priority: 100
      label_promotion: [resource_name, region]
      instances:
        by_labels: [resource_uid]
    groups:
      - family: Child
        chart_defaults:
          priority: 200
          instances:
            by_labels: [resource_uid, region]
            optional_by_labels: [pid]
        charts:
          - title: Queries
            context: queries
            units: queries/s
            dimensions:
              - selector: mysql_queries_total
                name: total
        groups:
          - family: Leaf
            charts:
              - title: Overrides
                priority: 300
                context: overrides
                units: queries/s
                label_promotion: []
                instances:
                  by_labels: [region]
                dimensions:
                  - selector: mysql_queries_total
                    name: total
      - family: Sibling
        charts:
          - title: Inherits Root
            priority: 0
            context: inherits_root
            units: queries/s
            dimensions:
              - selector: mysql_queries_total
                name: total
          - title: Resets Chart
            priority: 70000
            context: resets_chart
            units: queries/s
            dimensions:
              - selector: mysql_queries_total
                name: total
      - family: Zero Default
        chart_defaults:
          priority: 0
        charts:
          - title: Inherits Root Through Zero
            context: inherits_root_through_zero
            units: queries/s
            dimensions:
              - selector: mysql_queries_total
                name: total
      - family: Explicit Reset
        chart_defaults:
          priority: 70000
        charts:
          - title: Resets Subtree
            context: resets_subtree
            units: queries/s
            dimensions:
              - selector: mysql_queries_total
                name: total
`,
			assert: func(t *testing.T, spec *Spec) {
				t.Helper()
				root := spec.Groups[0]
				child := root.Groups[0]
				require.Len(t, child.Charts, 1)
				assert.Equal(t, "line", child.Charts[0].Type)
				assert.Equal(t, []string{"resource_name", "region"}, child.Charts[0].LabelPromoted)
				require.NotNil(t, child.Charts[0].Instances)
				assert.Equal(t, []string{"resource_uid", "region"}, child.Charts[0].Instances.ByLabels)
				assert.Equal(t, []string{"pid"}, child.Charts[0].Instances.OptionalByLabels)
				assert.Equal(t, 200, child.Charts[0].Priority)

				leaf := child.Groups[0]
				require.Len(t, leaf.Charts, 1)
				assert.Equal(t, "line", leaf.Charts[0].Type)
				assert.NotNil(t, leaf.Charts[0].LabelPromoted)
				assert.Empty(t, leaf.Charts[0].LabelPromoted)
				require.NotNil(t, leaf.Charts[0].Instances)
				assert.Equal(t, []string{"region"}, leaf.Charts[0].Instances.ByLabels)
				assert.Empty(t, leaf.Charts[0].Instances.OptionalByLabels)
				assert.Equal(t, 300, leaf.Charts[0].Priority)

				sibling := root.Groups[1]
				require.Len(t, sibling.Charts, 2)
				assert.Equal(t, 100, sibling.Charts[0].Priority)
				assert.Equal(t, 70000, sibling.Charts[1].Priority)

				zeroDefault := root.Groups[2]
				require.Len(t, zeroDefault.Charts, 1)
				assert.Equal(t, 100, zeroDefault.Charts[0].Priority)

				explicitReset := root.Groups[3]
				require.Len(t, explicitReset.Charts, 1)
				assert.Equal(t, 70000, explicitReset.Charts[0].Priority)
			},
		},
		"explicit empty group label promotion is inherited without collapsing to omitted": {
			input: `
version: v1
groups:
  - family: Root
    metrics: [metric]
    chart_defaults:
      label_promotion: []
    groups:
      - family: Child
        charts:
          - title: Value
            context: value
            units: value
            dimensions:
              - selector: metric
                name: value
`,
			assert: func(t *testing.T, spec *Spec) {
				t.Helper()
				promotion := spec.Groups[0].Groups[0].Charts[0].LabelPromoted
				assert.NotNil(t, promotion)
				assert.Empty(t, promotion)
			},
		},
		"rejects unknown yaml field via strict unmarshal": {
			input: `
version: v1
groups:
  - family: Database
    metrics:
      - mysql_queries_total
    charts:
      - title: Queries
        context: queries_total
        units: queries/s
        algorithm: incremental
        unknown_field: true
        dimensions:
          - selector: mysql_queries_total
            name: total
`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			spec, err := DecodeYAML([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec)
			if tc.assert != nil {
				tc.assert(t, spec)
			}
		})
	}
}

func TestDecodeYAMLEngineAutogenRules(t *testing.T) {
	const prefix = `
version: v1
engine:
  autogen:
    enabled: true
`
	const groups = `
groups:
  - family: Test
    metrics: [metric]
    charts:
      - title: Test
        context: test
        units: units
        dimensions:
          - selector: metric
            name: metric
`

	tests := map[string]struct {
		rules       string
		wantErr     string
		wantNoRules bool
	}{
		"absent rules mean no rules": {
			wantNoRules: true,
		},
		"empty list means no rules": {
			rules:       "    rules: []\n",
			wantNoRules: true,
		},
		"null list means no rules": {
			rules:       "    rules: null\n",
			wantNoRules: true,
		},
		"bare null list means no rules": {
			rules:       "    rules:\n",
			wantNoRules: true,
		},
		"escaped key empty list means no rules": {
			rules:       "    \"rule\\u0073\": []\n",
			wantNoRules: true,
		},
		"unknown autogen field remains a strict decode error": {
			rules:   "    unknown: true\n",
			wantErr: "field unknown not found",
		},
		"scoped Unicode selector is valid": {
			rules: `    rules:
      - scope: "μέτρο*"
        selector:
          allow: ['{region="west"}']
`,
		},
		"empty scope is invalid": {
			rules: `    rules:
      - scope: " "
        selector:
          deny: ["metric"]
`,
			wantErr: "engine.autogen.rules[0].scope",
		},
		"empty selector is invalid": {
			rules: `    rules:
      - scope: "metric*"
        selector: {}
`,
			wantErr: "engine.autogen.rules[0].selector",
		},
		"whitespace-only selector entry is invalid": {
			rules: `    rules:
      - scope: "metric*"
        selector:
          deny: [" "]
`,
			wantErr: "engine.autogen.rules[0].selector.deny[0]",
		},
		"invalid scope is invalid": {
			rules: `    rules:
      - scope: "["
        selector:
          deny: ["metric"]
`,
			wantErr: "engine.autogen.rules[0].scope",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			spec, validation, err := DecodeYAMLValidated([]byte(prefix + test.rules + groups))
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec.Engine)
			require.NotNil(t, spec.Engine.Autogen)
			if test.wantNoRules {
				assert.Empty(t, spec.Engine.Autogen.Rules)
				assert.Empty(t, validation.AutogenRules())
			}
			if name == "scoped Unicode selector is valid" {
				rules := validation.AutogenRules()
				require.Len(t, rules, 1)
				assert.True(t, rules[0].ScopeMatches("μέτρο_total"))
				assert.True(t, rules[0].Selects("μέτρο_total", testLabelView{
					"region": "west",
				}))
				assert.False(t, rules[0].Selects("μέτρο_total", testLabelView{
					"region": "east",
				}))
				rules[0] = ValidatedAutogenRule{}
				fresh := validation.AutogenRules()
				require.Len(t, fresh, 1)
				assert.True(t, fresh[0].ScopeMatches("μέτρο_total"))
			}
		})
	}
}

func TestEngineAutogenValidationDoesNotMutateRules(t *testing.T) {
	spec, err := DecodeYAML([]byte(`
version: v1
engine:
  autogen:
    enabled: true
    rules:
      - scope: "metric_*"
        selector:
          allow: ["metric_*"]
groups:
  - family: Test
    metrics: [metric]
    charts:
      - title: Test
        context: test
        units: units
        dimensions:
          - selector: metric
            name: metric
`))
	require.NoError(t, err)
	want := []EngineAutogenRule{{
		Scope: "metric_*",
		Selector: metrixselector.Expr{
			Allow: []string{"metric_*"},
		},
	}}
	assert.Equal(t, want, spec.Engine.Autogen.Rules)
	require.NoError(t, spec.Validate())
	assert.Equal(t, want, spec.Engine.Autogen.Rules)

	spec.Engine.Autogen.Rules[0].Scope = "["
	require.Error(t, spec.Validate())
	assert.Equal(t, "[", spec.Engine.Autogen.Rules[0].Scope)

	spec.Engine.Autogen.Rules[0].Scope = "beta*"
	require.NoError(t, spec.Validate())
	assert.Equal(t, "beta*", spec.Engine.Autogen.Rules[0].Scope)

	spec.Engine.Autogen.Rules = []EngineAutogenRule{}
	require.NoError(t, spec.Validate(), "programmatic empty rules are equivalent to no rules")
}

func TestConfigSchemaJSON(t *testing.T) {
	schema := ConfigSchemaJSON
	require.NotEmpty(t, schema)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(schema), &doc))

	defs, ok := doc["$defs"].(map[string]any)
	require.True(t, ok)

	engineAutogen, ok := defs["engine_autogen"].(map[string]any)
	require.True(t, ok)
	engineAutogenProps, ok := engineAutogen["properties"].(map[string]any)
	require.True(t, ok)
	rules, ok := engineAutogenProps["rules"].(map[string]any)
	require.True(t, ok)
	assert.ElementsMatch(t, []any{"array", "null"}, rules["type"])
	_, ok = rules["minItems"]
	assert.False(t, ok)
	assert.Len(t, engineAutogenProps, 4)

	chart, ok := defs["chart"].(map[string]any)
	require.True(t, ok)
	chartProps, ok := chart["properties"].(map[string]any)
	require.True(t, ok)
	chartLabelPromotion, ok := chartProps["label_promotion"].(map[string]any)
	require.True(t, ok)
	chartLabelPromotionItems, ok := chartLabelPromotion["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `\S`, chartLabelPromotionItems["pattern"])
	chartAggregation, ok := chartProps["aggregation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"sum", "min", "max", "avg"}, chartAggregation["enum"])

	chartDefaults, ok := defs["chart_defaults"].(map[string]any)
	require.True(t, ok)
	defaultProps, ok := chartDefaults["properties"].(map[string]any)
	require.True(t, ok)
	defaultLabelPromotion, ok := defaultProps["label_promotion"].(map[string]any)
	require.True(t, ok)
	defaultLabelPromotionItems, ok := defaultLabelPromotion["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `\S`, defaultLabelPromotionItems["pattern"])
	defaultPriority, ok := defaultProps["priority"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", defaultPriority["type"])
}

func TestConfigSchemaAutogenRuleEmptyValues(t *testing.T) {
	var schemaDoc any
	require.NoError(t, json.Unmarshal([]byte(ConfigSchemaJSON), &schemaDoc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("charttpl.schema.json", schemaDoc))
	schema, err := compiler.Compile("charttpl.schema.json")
	require.NoError(t, err)

	for name, rulesValue := range map[string]any{
		"empty rules": []any{},
		"null rules":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(richSpec())
			require.NoError(t, err)
			var instance map[string]any
			require.NoError(t, json.Unmarshal(raw, &instance))
			engine := instance["engine"].(map[string]any)
			autogen := engine["autogen"].(map[string]any)
			autogen["rules"] = rulesValue
			require.NoError(t, schema.Validate(instance))
		})
	}

	tests := map[string]struct {
		selector map[string]any
		wantErr  bool
	}{
		"empty allow with deny entry": {
			selector: map[string]any{"allow": []any{}, "deny": []any{"metric_*"}},
		},
		"allow entry with empty deny": {
			selector: map[string]any{"allow": []any{"metric_*"}, "deny": []any{}},
		},
		"both lists empty": {
			selector: map[string]any{"allow": []any{}, "deny": []any{}},
			wantErr:  true,
		},
		"both lists absent": {
			selector: map[string]any{},
			wantErr:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(richSpec())
			require.NoError(t, err)
			var instance map[string]any
			require.NoError(t, json.Unmarshal(raw, &instance))
			engine := instance["engine"].(map[string]any)
			autogen := engine["autogen"].(map[string]any)
			rules := autogen["rules"].([]any)
			rule := rules[0].(map[string]any)
			rule["selector"] = test.selector

			err = schema.Validate(instance)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfigSchemaOptionalInstanceLabels(t *testing.T) {
	var schemaDoc any
	require.NoError(t, json.Unmarshal([]byte(ConfigSchemaJSON), &schemaDoc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("charttpl.schema.json", schemaDoc))
	schema, err := compiler.Compile("charttpl.schema.json")
	require.NoError(t, err)

	tests := map[string]struct {
		instances map[string]any
		wantErr   bool
	}{
		"required only": {
			instances: map[string]any{"by_labels": []any{"deployment"}},
		},
		"optional only": {
			instances: map[string]any{"optional_by_labels": []any{"pid"}},
		},
		"required and optional": {
			instances: map[string]any{
				"by_labels":          []any{"deployment"},
				"optional_by_labels": []any{"pid"},
			},
		},
		"empty required with optional": {
			instances: map[string]any{
				"by_labels":          []any{},
				"optional_by_labels": []any{"pid"},
			},
		},
		"empty instances": {
			instances: map[string]any{},
			wantErr:   true,
		},
		"empty required only": {
			instances: map[string]any{"by_labels": []any{}},
			wantErr:   true,
		},
		"empty optional only": {
			instances: map[string]any{"optional_by_labels": []any{}},
			wantErr:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			spec := validationSpec()
			raw, err := json.Marshal(spec)
			require.NoError(t, err)
			var instance map[string]any
			require.NoError(t, json.Unmarshal(raw, &instance))
			groups := instance["groups"].([]any)
			group := groups[0].(map[string]any)
			charts := group["charts"].([]any)
			chart := charts[0].(map[string]any)
			chart["instances"] = tc.instances

			err = schema.Validate(instance)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestConfigSchemaRootGroupFamily(t *testing.T) {
	var schemaDoc any
	require.NoError(t, json.Unmarshal([]byte(ConfigSchemaJSON), &schemaDoc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("charttpl.schema.json", schemaDoc))
	schema, err := compiler.Compile("charttpl.schema.json")
	require.NoError(t, err)

	tests := map[string]struct {
		mutate  func(root map[string]any)
		wantErr bool
	}{
		"transparent root with named child": {
			mutate: func(root map[string]any) {
				delete(root, "family")
				child := map[string]any{
					"family": rootFamilyTestName,
					"charts": root["charts"],
				}
				delete(root, "charts")
				root["groups"] = []any{child}
			},
		},
		"transparent root with chart family": {
			mutate: func(root map[string]any) {
				delete(root, "family")
				charts := root["charts"].([]any)
				charts[0].(map[string]any)["family"] = rootFamilyTestName
			},
		},
		"transparent root rejects whitespace family": {
			mutate: func(root map[string]any) {
				root["family"] = " "
				child := map[string]any{
					"family": rootFamilyTestName,
					"charts": root["charts"],
				}
				delete(root, "charts")
				root["groups"] = []any{child}
			},
			wantErr: true,
		},
		"transparent root without effective chart family": {
			mutate: func(root map[string]any) {
				delete(root, "family")
			},
			wantErr: true,
		},
		"nested group without family under transparent root": {
			mutate: func(root map[string]any) {
				delete(root, "family")
				child := map[string]any{"charts": root["charts"]}
				delete(root, "charts")
				root["groups"] = []any{child}
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(validationSpec())
			require.NoError(t, err)
			var instance map[string]any
			require.NoError(t, json.Unmarshal(raw, &instance))
			groups := instance["groups"].([]any)
			root := groups[0].(map[string]any)
			tc.mutate(root)

			err = schema.Validate(instance)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

const rootFamilyTestName = "Service"

func TestDecodeYAMLFileScenarios(t *testing.T) {
	tests := map[string]struct {
		prepare func(t *testing.T) string
		wantErr bool
	}{
		"success": {
			prepare: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, "charts.yaml")
				data := []byte(`
version: v1
groups:
  - family: Root
    metrics: [metric_a]
    charts:
      - title: A
        context: a
        units: "1"
        dimensions:
          - selector: metric_a
            name: x
`)
				require.NoError(t, os.WriteFile(path, data, 0o644))
				return path
			},
		},
		"read error": {
			prepare: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(t.TempDir(), "missing.yaml")
			},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := tc.prepare(t)
			spec, err := DecodeYAMLFile(path)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec)
		})
	}
}
