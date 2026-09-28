// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"unicode/utf8"
)

type response struct {
	Version string         `json:"version"`
	Metrics []metricSample `json:"metrics"`
	Checks  []checkSample  `json:"checks"`
}

type metricSample struct {
	Name   string            `json:"name"`
	Value  *float64          `json:"value"`
	Labels map[string]string `json:"labels"`
}

type checkSample struct {
	ID     string            `json:"id"`
	State  string            `json:"state"`
	Labels map[string]string `json:"labels"`
}

func (m manifest) decodeResponse(data []byte) (response, error) {
	var result response
	if !utf8.Valid(data) {
		return result, fmt.Errorf("response must be UTF-8")
	}
	// json.Valid bounds nesting and rejects incomplete frames before the field scan.
	if !json.Valid(data) {
		return result, fmt.Errorf("expected one complete JSON object")
	}
	fields := json.NewDecoder(bytes.NewReader(data))
	fields.UseNumber() // Type validation below reports errors without raw numeric values.
	if err := validateJSONFields(fields, "response"); err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("invalid response schema")
	}
	if result.Version != "v1" {
		return result, fmt.Errorf("unsupported response version")
	}
	seen := map[string]bool{}
	for i, sample := range result.Metrics {
		d, ok := m.metricByName[sample.Name]
		if !ok {
			return result, fmt.Errorf("metric %d is not declared", i)
		}
		if sample.Value == nil || math.IsNaN(*sample.Value) || math.IsInf(*sample.Value, 0) ||
			(d.Type == "counter" && *sample.Value < 0) {
			return result, fmt.Errorf("metric %d has an invalid value", i)
		}
		if err := validateLabels(sample.Labels); err != nil {
			return result, err
		}
		key := identity(sample.Name, sample.Labels)
		if seen[key] {
			return result, fmt.Errorf("duplicate metric identity")
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for i, sample := range result.Checks {
		d, ok := m.checkByID[sample.ID]
		if !ok {
			return result, fmt.Errorf("check %d is not declared", i)
		}
		if !slices.Contains(checkStates, sample.State) {
			return result, fmt.Errorf("check %d has an invalid state", i)
		}
		if err := validateLabels(sample.Labels); err != nil {
			return result, err
		}
		labels := make(map[string]string, len(d.ByLabels))
		for _, key := range d.ByLabels {
			if sample.Labels[key] == "" {
				return result, fmt.Errorf("check %d is missing an identity label", i)
			}
			labels[key] = sample.Labels[key]
		}
		key := identity(sample.ID, labels)
		if seen[key] {
			return result, fmt.Errorf("duplicate check identity")
		}
		seen[key] = true
	}
	return result, nil
}

func validateLabels(labels map[string]string) error {
	for key := range labels {
		if !identifier.MatchString(key) {
			return fmt.Errorf("invalid label key")
		}
	}
	return nil
}

func identity(name string, labels map[string]string) string {
	// encoding/json sorts map keys and escapes delimiters, giving an unambiguous identity.
	if len(labels) == 0 {
		return name + ":{}"
	}
	encoded, _ := json.Marshal(labels)
	return name + ":" + string(encoded)
}

// Validate field spelling and nulls before encoding/json can coerce them.
// Array items inherit their field role; label keys are arbitrary data.
func validateJSONFields(decoder *json.Decoder, role string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	switch token {
	case nil:
		return fmt.Errorf("null is not a protocol value")
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			token, err = decoder.Token()
			if err != nil {
				return err
			}
			key := token.(string)
			if !knownField(role, key) {
				return fmt.Errorf("unknown response field")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key")
			}
			seen[key] = true
			childRole := key
			if role == "labels" {
				childRole = "label_value"
			}
			if err = validateJSONFields(decoder, childRole); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case json.Delim('['):
		for decoder.More() {
			if err = validateJSONFields(decoder, role); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

func knownField(role, key string) bool {
	switch role {
	case "response":
		return key == "version" || key == "metrics" || key == "checks"
	case "metrics":
		return key == "name" || key == "value" || key == "labels"
	case "checks":
		return key == "id" || key == "state" || key == "labels"
	case "labels":
		return true
	default:
		return false
	}
}
