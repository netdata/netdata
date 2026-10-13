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

	applyDefaults(&spec)
	validation, err := Validate(&spec)
	if err != nil {
		return nil, Validation{}, err
	}
	return &spec, validation, nil
}

// DecodeYAMLFile reads and parses a YAML template file.
func DecodeYAMLFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read file %q: %v", errDecode, path, err)
	}
	return DecodeYAML(data)
}
