// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
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
	if err := validateYAMLAutogenRulesPresence(&spec); err != nil {
		return nil, Validation{}, err
	}

	applyDefaults(&spec)
	validation, err := Validate(&spec)
	if err != nil {
		return nil, Validation{}, err
	}
	return &spec, validation, nil
}

func validateYAMLAutogenRulesPresence(spec *Spec) error {
	if spec.Engine == nil || spec.Engine.Autogen == nil {
		return nil
	}
	configured := spec.Engine.Autogen.yamlRulesConfigured
	spec.Engine.Autogen.yamlRulesConfigured = false
	if configured && len(spec.Engine.Autogen.Rules) == 0 {
		return semErr("engine.autogen.rules", "must contain at least one rule when configured")
	}
	return nil
}

func (a *EngineAutogen) UnmarshalYAML(unmarshal func(any) error) error {
	type plain EngineAutogen
	var decoded plain
	if err := unmarshal(&decoded); err != nil {
		return err
	}
	var fields map[string]any
	if err := unmarshal(&fields); err != nil {
		return err
	}
	*a = EngineAutogen(decoded)
	_, a.yamlRulesConfigured = fields["rules"]
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
