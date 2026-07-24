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
      label_promotion: [resource_name, region]
      instances:
        by_labels: [resource_uid]
    groups:
      - family: Child
        chart_defaults:
          instances:
            by_labels: [resource_uid, region]
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
                context: overrides
                units: queries/s
                label_promotion: []
                instances:
                  by_labels: [region]
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

				leaf := child.Groups[0]
				require.Len(t, leaf.Charts, 1)
				assert.Equal(t, "line", leaf.Charts[0].Type)
				assert.Empty(t, leaf.Charts[0].LabelPromoted)
				require.NotNil(t, leaf.Charts[0].Instances)
				assert.Equal(t, []string{"region"}, leaf.Charts[0].Instances.ByLabels)
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
				assert.True(t, rules[0].Selects("μέτρο_total", testLabelView{"region": "west"}))
				assert.False(t, rules[0].Selects("μέτρο_total", testLabelView{"region": "east"}))
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

	chartDefaults, ok := defs["chart_defaults"].(map[string]any)
	require.True(t, ok)
	defaultProps, ok := chartDefaults["properties"].(map[string]any)
	require.True(t, ok)
	defaultLabelPromotion, ok := defaultProps["label_promotion"].(map[string]any)
	require.True(t, ok)
	defaultLabelPromotionItems, ok := defaultLabelPromotion["items"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `\S`, defaultLabelPromotionItems["pattern"])
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
