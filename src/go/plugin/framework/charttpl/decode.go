// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v2"
)

// DecodeYAML parses YAML bytes into Spec, applies defaults, and validates it.
func DecodeYAML(data []byte) (*Spec, error) {
	var spec Spec
	if err := yaml.UnmarshalStrict(data, &spec); err != nil {
		return nil, fmt.Errorf("%w: %v", errDecode, err)
	}

	applyDefaults(&spec)
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	return &spec, nil
}

// DecodeYAMLFile reads and parses a YAML template file.
func DecodeYAMLFile(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read file %q: %v", errDecode, path, err)
	}
	return DecodeYAML(data)
}
