// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"

	commonmodel "github.com/prometheus/common/model"
)

type idSet map[string]struct{}

func (s idSet) add(field, id string) error {
	if !validID(id) {
		return fmt.Errorf("%s %q must be lowercase letters, digits, or underscores and start with a letter", field, id)
	}
	if _, ok := s[id]; ok {
		return fmt.Errorf("%s %q is duplicated", field, id)
	}
	s[id] = struct{}{}
	return nil
}

func validID(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, char := range value[1:] {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

func requireText(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not have leading or trailing whitespace", field)
	}
	return nil
}

func validateMetricName(field, value string) error {
	if err := requireText(field, value); err != nil {
		return err
	}
	if !commonmodel.UTF8Validation.IsValidMetricName(value) {
		return fmt.Errorf("%s %q is not a valid UTF-8 Prometheus metric name", field, value)
	}
	return nil
}

func validateLabelName(field, value string) error {
	if err := requireText(field, value); err != nil {
		return err
	}
	if !commonmodel.UTF8Validation.IsValidLabelName(value) {
		return fmt.Errorf("%s %q is not a valid UTF-8 Prometheus label name", field, value)
	}
	return nil
}

func requireList(field string, values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	return validateStringSet(field, values, false)
}

func validateStringSet(field string, values []string, allowEmpty bool) error {
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value != "" || !allowEmpty {
			if err := requireText(fmt.Sprintf("%s[%d]", field, index), value); err != nil {
				return err
			}
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s contains duplicate value %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateLabelSet(field string, values []string, allowEmpty bool) error {
	if err := validateStringSet(field, values, allowEmpty); err != nil {
		return err
	}
	for index, value := range values {
		if err := validateLabelName(fmt.Sprintf("%s[%d]", field, index), value); err != nil {
			return err
		}
	}
	return nil
}

func requireEnum(field, value string, allowed ...string) error {
	if slices.Contains(allowed, value) {
		return nil
	}
	return fmt.Errorf("%s %q must be one of %v", field, value, allowed)
}

func validComponentSets(shape string) [][]string {
	switch shape {
	case "scalar", "info":
		return [][]string{{"scalar"}}
	case "histogram":
		return [][]string{{"histogram_bucket", "histogram_count", "histogram_sum"}}
	case "summary":
		return [][]string{
			{"summary_count", "summary_sum"},
			{"summary_count", "summary_quantile", "summary_sum"},
		}
	default:
		return nil
	}
}
