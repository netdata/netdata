// SPDX-License-Identifier: GPL-3.0-or-later

package configform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

const testForm = `{
 "jsonSchema": {
  "$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "title": "Fixture",
  "additionalProperties": false,
  "properties": {
   "text": {"$ref": "#/definitions/text"},
   "count": {"type": "integer", "minimum": 0, "default": 17},
   "optional": {"type": ["string", "null"], "default": "fallback"},
   "nested": {"type": "object", "default": {}, "properties": {"value": {"type": "integer", "default": 3}}},
   "items": {"type": "array", "items": {"type": "object", "properties": {"id": {"type": "string", "default": "x"}}}}
  },
  "required": ["text"], "definitions": {"text": {"type": "string", "minLength": 1}}
 },
 "uiSchema": {"text": {"ui:widget": "password"}}
}`

func TestParse(t *testing.T) {
	external := filepath.Join(t.TempDir(), "external.json")
	require.NoError(
		t,
		os.WriteFile(external, []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"string"}`), 0644),
	)
	withSchema := func(extra string) string {
		return `{"jsonSchema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",` + extra + `},"uiSchema":{}}`
	}
	tests := map[string]struct {
		data    string
		wantErr bool
	}{
		"valid": {data: testForm},
		"invalid default": {
			data:    strings.Replace(testForm, `"default": 17`, `"default": -1`, 1),
			wantErr: true,
		},
		"external reference": {
			data:    strings.Replace(testForm, "#/definitions/text", "https://invalid.example/schema", 1),
			wantErr: true,
		},
		"file reference": {
			data:    strings.Replace(testForm, "#/definitions/text", "file:///private/secret", 1),
			wantErr: true,
		},
		"secret reference default": {
			data:    strings.Replace(testForm, `"default": "fallback"`, `"default": "${env:SYNTHETIC}"`, 1),
			wantErr: true,
		},
		"default outside properties": {
			data:    strings.Replace(testForm, `"minLength": 1`, `"minLength": 1, "default":"bad"`, 1),
			wantErr: true,
		},
		"resource ID": {
			data:    strings.Replace(testForm, `"title": "Fixture"`, `"title": "Fixture", "$id":"urn:other"`, 1),
			wantErr: true,
		},
		"duplicate key": {
			data:    strings.Replace(testForm, `"minLength": 1`, `"minLength": 1, "minLength":2`, 1),
			wantErr: true,
		},
		"unknown envelope field": {
			data:    strings.Replace(testForm, `"uiSchema":`, `"unexpected":true,"uiSchema":`, 1),
			wantErr: true,
		},
		"case-folded envelope field": {
			data:    strings.Replace(testForm, `"jsonSchema":`, `"JsonSchema":`, 1),
			wantErr: true,
		},
		"missing UI schema": {
			data: strings.Replace(
				testForm,
				`"uiSchema": {"text": {"ui:widget": "password"}}`,
				`"uiSchema": null`,
				1,
			),
			wantErr: true,
		},
		"trailing document": {data: testForm + "{}", wantErr: true},
		"not draft-07": {
			data:    strings.Replace(testForm, "draft-07", "draft-04", 1),
			wantErr: true,
		},
		"hidden external reference": {
			data: withSchema(
				fmt.Sprintf(`"properties":{"text":{"$ref":"#/x-hidden"}},"x-hidden":{"$ref":%q}`, "file://"+external),
			),
			wantErr: true,
		},
		"hidden local reference": {
			data: withSchema(
				`"properties":{"text":{"$ref":"#/x-hidden"}},"x-hidden":{"$ref":"#/definitions/text"},"definitions":{"text":{"type":"string"}}`,
			),
			wantErr: true,
		},
		"external nested dialect": {
			data: withSchema(
				fmt.Sprintf(`"properties":{"text":{"type":"string","$schema":%q}}`, "file://"+external),
			),
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			form, err := Parse([]byte(tc.data))
			if tc.wantErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "SYNTHETIC", "errors never echo schema values")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, form)
		})
	}
}

// YAML-decoded forms (nested map[any]any) parse like their JSON equivalent.
func TestParseValue(t *testing.T) {
	var value map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(testForm), &value))
	form, err := ParseValue(value)
	require.NoError(t, err)
	assert.Equal(t, float64(17), form.WithDefaults(nil)["count"])
}

func TestForm_WithDefaults(t *testing.T) {
	form, err := Parse([]byte(testForm))
	require.NoError(t, err)
	tests := map[string]struct {
		config map[string]any
		want   map[string]any
	}{
		"omitted config": {
			config: nil,
			want: map[string]any{
				"count":    float64(17),
				"optional": "fallback",
				"nested":   map[string]any{"value": float64(3)},
			},
		},
		"explicit zero and null are kept": {
			config: map[string]any{"text": "x", "count": float64(0), "optional": nil},
			want: map[string]any{
				"text":     "x",
				"count":    float64(0),
				"optional": nil,
				"nested":   map[string]any{"value": float64(3)},
			},
		},
		"array items": {
			config: map[string]any{
				"items":  []any{map[string]any{}, map[string]any{"id": "y"}},
				"nested": map[string]any{},
			},
			want: map[string]any{
				"count":    float64(17),
				"optional": "fallback",
				"nested":   map[string]any{"value": float64(3)},
				"items":    []any{map[string]any{"id": "x"}, map[string]any{"id": "y"}},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			input, err := Normalize(tc.config)
			require.NoError(t, err)
			assert.Equal(t, tc.want, form.WithDefaults(tc.config))
			if tc.config != nil {
				assert.Equal(t, input, any(tc.config), "WithDefaults must not modify its input")
			}
		})
	}
}

func TestForm_Validate(t *testing.T) {
	form, err := Parse([]byte(testForm))
	require.NoError(t, err)
	require.NoError(t, form.Validate(form.WithDefaults(map[string]any{"text": "x"})))
	require.Error(t, form.Validate(form.WithDefaults(map[string]any{"text": ""})))
	require.Error(t, form.Validate(form.WithDefaults(map[string]any{"text": "x", "unknown": true})))
}

func TestForm_Embed(t *testing.T) {
	form, err := Parse([]byte(`{"jsonSchema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",` +
		`"properties":{"a":{"$ref":"#/definitions/a"},"root":{"$ref":"#"}},"definitions":{"a":{"type":"string"}},` +
		`"examples":[{"$ref":"#/annotation-is-data"}]},"uiSchema":{"a":{"ui:widget":"password"}}}`))
	require.NoError(t, err)
	schema, ui := form.Embed("#/properties/config")
	properties := schema["properties"].(map[string]any)
	assert.Equal(t, "#/properties/config/definitions/a", properties["a"].(map[string]any)["$ref"])
	assert.Equal(t, "#/properties/config", properties["root"].(map[string]any)["$ref"])
	assert.Equal(
		t,
		"#/annotation-is-data",
		schema["examples"].([]any)[0].(map[string]any)["$ref"],
		"annotations are data",
	)
	assert.Equal(t, map[string]any{"a": map[string]any{"ui:widget": "password"}}, ui)
	again, _ := form.Embed("#/properties/config")
	assert.Equal(
		t,
		"#/properties/config/definitions/a",
		again["properties"].(map[string]any)["a"].(map[string]any)["$ref"],
		"embedding does not modify the form",
	)
}

func TestNormalize(t *testing.T) {
	tests := map[string]struct {
		value   any
		want    any
		wantErr bool
	}{
		"YAML map": {
			value: map[any]any{"a": []any{map[any]any{"b": 1}}},
			want:  map[string]any{"a": []any{map[string]any{"b": float64(1)}}},
		},
		"non-string key":    {value: map[any]any{1: "x"}, wantErr: true},
		"unsupported value": {value: map[string]any{"x": func() {}}, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := Normalize(tc.value)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
