// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/charttpl"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v2"
)

var (
	chartTemplateSchemaOnce sync.Once
	chartTemplateSchema     *jsonschema.Schema
	chartTemplateSchemaErr  error
)

// AssertChartTemplateSchema validates chart-template YAML against
// charttpl JSON schema and fails the test on error.
func AssertChartTemplateSchema(t testing.TB, templateYAML string) {
	t.Helper()
	if err := ValidateChartTemplateSchema(templateYAML); err != nil {
		t.Fatalf("collecttest: chart template schema validation failed: %v", err)
	}
}

// ValidateChartTemplateSchema validates chart-template YAML against charttpl JSON schema.
func ValidateChartTemplateSchema(templateYAML string) error {
	schema, err := loadChartTemplateSchema()
	if err != nil {
		return err
	}

	doc, err := decodeYAMLToGenericJSONValue([]byte(templateYAML))
	if err != nil {
		return err
	}

	if err := schema.Validate(doc); err != nil {
		return fmt.Errorf("schema validate: %w", err)
	}
	return nil
}

func loadChartTemplateSchema() (*jsonschema.Schema, error) {
	chartTemplateSchemaOnce.Do(func() {
		var doc any
		if err := json.Unmarshal([]byte(charttpl.ConfigSchemaJSON), &doc); err != nil {
			chartTemplateSchemaErr = fmt.Errorf("decode embedded charttpl schema JSON: %w", err)
			return
		}

		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource("charttpl.schema.json", doc); err != nil {
			chartTemplateSchemaErr = fmt.Errorf("add schema resource: %w", err)
			return
		}

		chartTemplateSchema, chartTemplateSchemaErr = compiler.Compile("charttpl.schema.json")
		if chartTemplateSchemaErr != nil {
			chartTemplateSchemaErr = fmt.Errorf("compile schema: %w", chartTemplateSchemaErr)
			return
		}
	})

	return chartTemplateSchema, chartTemplateSchemaErr
}

func decodeYAMLToGenericJSONValue(rawYAML []byte) (any, error) {
	var doc any
	if err := yaml.UnmarshalStrict(rawYAML, &doc); err != nil {
		return nil, fmt.Errorf("decode template YAML: %w", err)
	}
	return normalizeYAML(doc), nil
}

func normalizeYAML(v any) any {
	switch tv := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(tv))
		for key, val := range tv {
			out[key] = normalizeYAML(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(tv))
		for key, val := range tv {
			ks, ok := key.(string)
			if !ok {
				continue
			}
			out[ks] = normalizeYAML(val)
		}
		return out
	case []any:
		out := make([]any, len(tv))
		for i, val := range tv {
			out[i] = normalizeYAML(val)
		}
		return out
	default:
		return v
	}
}
