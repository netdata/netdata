// SPDX-License-Identifier: GPL-3.0-or-later

// Package configform owns native package configuration forms: a restricted
// Draft 7 JSON Schema plus its UI schema. It validates the form, applies its
// defaults to job configuration and embeds it into a job's DynCfg form.
//
// Errors are fixed text and never contain schema or configuration values.
package configform

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/strictjson"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const draft7 = "http://json-schema.org/draft-07/schema#"

// Form is a validated configuration form. It is immutable after parsing.
type Form struct {
	schema   *jsonschema.Schema
	document map[string]any
	ui       map[string]any
}

// The envelope is exact; the schema and UI documents are arbitrary JSON.
var formShape = strictjson.Object(strictjson.Fields{
	"jsonSchema": strictjson.Any(),
	"uiSchema":   strictjson.Any(),
})

// Parse validates and compiles a JSON form document.
func Parse(data []byte) (*Form, error) {
	var form struct {
		Schema map[string]any `json:"jsonSchema"`
		UI     map[string]any `json:"uiSchema"`
	}
	if err := strictjson.Decode(data, formShape, &form); err != nil {
		return nil, errors.New("invalid package config schema JSON")
	}
	if form.Schema == nil || form.UI == nil {
		return nil, errors.New("config schema requires jsonSchema and uiSchema objects")
	}
	if form.Schema["$schema"] != draft7 || form.Schema["type"] != "object" {
		return nil, errors.New("config schema must be a draft-07 object")
	}
	if err := checkSchema(form.Schema); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil) // Only this document and the compiler's built-in metaschemas.
	compiler.DefaultDraft(jsonschema.Draft7)
	const resource = "urn:netdata:native:config"
	if compiler.AddResource(resource, form.Schema) != nil {
		return nil, errors.New("invalid package config schema")
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, errors.New("invalid package config schema")
	}
	if err := checkDefaults(form.Schema, compiler, resource, ""); err != nil {
		return nil, err
	}
	return &Form{
		schema:   compiled,
		document: form.Schema,
		ui:       form.UI,
	}, nil
}

// ParseValue parses a form that is already decoded from JSON or YAML.
func ParseValue(value any) (*Form, error) {
	normalized, err := Normalize(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, errors.New("config schema cannot be encoded")
	}
	return Parse(data)
}

// Validate checks configuration, with defaults applied, against the form schema.
func (f *Form) Validate(config map[string]any) error {
	if f.schema.Validate(config) != nil {
		return errors.New("config does not satisfy package schema")
	}
	return nil
}

// WithDefaults returns a copy of config with the form defaults applied. Omitted
// configuration uses the root default, otherwise an empty object.
func (f *Form) WithDefaults(config map[string]any) map[string]any {
	var value any
	if config == nil {
		value = applyDefaults(nil, false, f.document)
		if value == nil {
			value = map[string]any{}
		}
	} else {
		value, _ = Normalize(config)
	}
	result, _ := applyDefaults(value, true, f.document).(map[string]any)
	return result
}

// Embed returns copies of the schema and UI documents for embedding at pointer
// in a larger form. Local references are rebased onto pointer.
func (f *Form) Embed(pointer string) (schema, ui map[string]any) {
	document, _ := Normalize(f.document)
	schema = document.(map[string]any)
	_ = walkSchema(schema, "", true, func(value any, _ string, _ bool) error {
		if node, ok := value.(map[string]any); ok {
			if ref, ok := node["$ref"].(string); ok {
				node["$ref"] = pointer + strings.TrimPrefix(ref, "#")
			}
		}
		return nil
	})
	uiDocument, _ := Normalize(f.ui)
	return schema, uiDocument.(map[string]any)
}
