// SPDX-License-Identifier: GPL-3.0-or-later

package configform

import (
	"encoding/json"
	"errors"
)

// Normalize returns a deep copy of value as encoding/json would decode it:
// string-keyed objects, []any arrays and JSON scalars. It converts YAML maps
// and rejects values JSON cannot represent.
func Normalize(value any) (any, error) {
	switch v := value.(type) {
	case map[any]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			name, ok := key.(string)
			if !ok {
				return nil, errors.New("config object keys must be strings")
			}
			normalized, err := Normalize(item)
			if err != nil {
				return nil, err
			}
			result[name] = normalized
		}
		return result, nil
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			normalized, err := Normalize(item)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			normalized, err := Normalize(item)
			if err != nil {
				return nil, err
			}
			result[i] = normalized
		}
		return result, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, errors.New("config values must be JSON-compatible")
		}
		var result any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, errors.New("config values must be JSON-compatible")
		}
		return result, nil
	}
}

// applyDefaults fills omitted properties that declare a default, recursing into
// objects and homogeneous array items. present reports whether value was
// supplied; an omitted value takes the schema's own default. It mutates and
// returns value, so callers pass an owned copy.
func applyDefaults(value any, present bool, schema map[string]any) any {
	if !present {
		fallback, ok := schema["default"]
		if !ok {
			return nil
		}
		value, _ = Normalize(fallback)
	}
	switch v := value.(type) {
	case map[string]any:
		properties, _ := schema["properties"].(map[string]any)
		for name, raw := range properties {
			child, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			current, exists := v[name]
			if !exists {
				if _, hasDefault := child["default"]; !hasDefault {
					continue
				}
			}
			v[name] = applyDefaults(current, exists, child)
		}
	case []any:
		if item, ok := schema["items"].(map[string]any); ok {
			for i := range v {
				v[i] = applyDefaults(v[i], true, item)
			}
		}
	}
	return value
}
