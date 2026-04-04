// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestConfigSchemaJSON(t *testing.T) {
	schema := ConfigSchemaJSON
	require.NotEmpty(t, schema)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(schema), &doc))

	defs, ok := doc["$defs"].(map[string]any)
	require.True(t, ok)

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
