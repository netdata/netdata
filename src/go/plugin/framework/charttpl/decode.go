// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// DecodeYAML parses YAML bytes into Spec, applies defaults, and validates it.
func DecodeYAML(data []byte) (*Spec, error) {
	spec, _, err := DecodeYAMLValidated(data)
	return spec, err
}

// DecodeYAMLValidated parses YAML and also returns its immutable validation
// artifacts so runtime consumers do not repeat derived-state construction.
func DecodeYAMLValidated(data []byte) (*Spec, Validation, error) {
	var spec Spec
	if err := yaml.UnmarshalStrict(data, &spec); err != nil {
		return nil, Validation{}, fmt.Errorf("%w: %v", errDecode, err)
	}
	if err := validateYAMLAutogenRulesPresence(data); err != nil {
		return nil, Validation{}, err
	}

	applyDefaults(&spec)
	validation, err := Validate(&spec)
	if err != nil {
		return nil, Validation{}, err
	}
	return &spec, validation, nil
}

func validateYAMLAutogenRulesPresence(data []byte) error {
	if !bytes.Contains(data, []byte("rules")) {
		return nil
	}
	var doc struct {
		Engine *struct {
			Autogen *struct {
				Rules *[]EngineAutogenRule `yaml:"rules"`
			} `yaml:"autogen"`
		} `yaml:"engine"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%w: %v", errDecode, err)
	}
	if doc.Engine == nil || doc.Engine.Autogen == nil || doc.Engine.Autogen.Rules == nil {
		return nil
	}
	if len(*doc.Engine.Autogen.Rules) == 0 {
		return semErr("engine.autogen.rules", "must contain at least one rule when configured")
	}
	return nil
}

// DecodeYAMLFile reads and parses a YAML template file.
func DecodeYAMLFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read file %q: %v", errDecode, path, err)
	}
	return DecodeYAML(data)
}
