// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var sourceEvidenceLocationPattern = regexp.MustCompile(`^(.+):([1-9][0-9]*)(?:-([1-9][0-9]*))?$`)

func validateIdentity(field, version, wantVersion, profile string) error {
	if version != wantVersion {
		return fmt.Errorf("%s.version: got %q, want %q", field, version, wantVersion)
	}
	if !validID(profile) {
		return fmt.Errorf("%s.profile %q must be lowercase letters, digits, or underscores and start with a letter", field, profile)
	}
	return nil
}

func validateIDMap[V any](field string, values map[string]V, required bool) error {
	if err := validateMap(field, values, required); err != nil {
		return err
	}
	for id := range values {
		if !validID(id) {
			return fmt.Errorf("%s key %q must be lowercase letters, digits, or underscores and start with a letter", field, id)
		}
	}
	return nil
}

func validateMap[K comparable, V any](field string, values map[K]V, required bool) error {
	if values == nil {
		if required {
			return fmt.Errorf("%s must be present", field)
		}
		return nil
	}
	if required && len(values) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	return nil
}

func validateRelativeContext(field, value string) error {
	if err := requireText(field, value); err != nil {
		return err
	}
	for segment := range strings.SplitSeq(value, ".") {
		if !validID(segment) {
			return fmt.Errorf("%s %q must contain dot-separated lowercase context segments", field, value)
		}
	}
	return nil
}

func validateStringMap(field string, values map[string]string, required bool) error {
	if values == nil {
		if required {
			return fmt.Errorf("%s must be present", field)
		}
		return nil
	}
	if required && len(values) == 0 {
		return fmt.Errorf("%s must not be empty", field)
	}
	for key, value := range values {
		if err := requireText(field+"."+key, value); err != nil {
			return err
		}
	}
	return nil
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func validateRepositoryRelativePath(field, value string) error {
	if err := requireText(field, value); err != nil {
		return err
	}
	if strings.Contains(value, "\\") || path.IsAbs(value) || path.Clean(value) != value ||
		value == "." || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("%s %q must be a canonical repository-relative slash path", field, value)
	}
	return nil
}

func validateEvidenceLocation(field, value string) error {
	match := sourceEvidenceLocationPattern.FindStringSubmatch(value)
	if match == nil {
		return fmt.Errorf("%s %q must be path:line or path:start-end", field, value)
	}
	if err := validateRepositoryRelativePath(field, match[1]); err != nil {
		return err
	}
	start, _ := strconv.Atoi(match[2])
	if match[3] != "" {
		end, _ := strconv.Atoi(match[3])
		if end < start {
			return fmt.Errorf("%s %q ends before it starts", field, value)
		}
	}
	return nil
}

func validatePrometheusContract(field string, contract PrometheusContract) error {
	if err := requireEnum(field+".type", contract.Type, "gauge", "counter", "histogram", "summary", "untyped"); err != nil {
		return err
	}
	if err := requireEnum(field+".shape", contract.Shape, "scalar", "histogram", "summary", "info"); err != nil {
		return err
	}
	valid := contract.Type == "counter" && contract.Shape == "scalar" ||
		contract.Type == "histogram" && contract.Shape == "histogram" ||
		contract.Type == "summary" && contract.Shape == "summary" ||
		contract.Type == "gauge" && (contract.Shape == "scalar" || contract.Shape == "info") ||
		contract.Type == "untyped" && contract.Shape == "scalar"
	if !valid {
		return fmt.Errorf("%s type %q and shape %q are incompatible", field, contract.Type, contract.Shape)
	}
	if contract.Type == "untyped" {
		if err := requireEnum(field+".classification", contract.Classification, "gauge", "counter"); err != nil {
			return err
		}
	} else if contract.Classification != "" {
		return fmt.Errorf("%s.classification is allowed only for untyped scalar registrations", field)
	}
	return nil
}

func validateFamilySelector(field string, selector FamilySelector, inline bool) error {
	if selector.Exact == "" {
		if inline {
			return fmt.Errorf("%s.exact must not be empty for an inline registration", field)
		}
		if err := requireText(field+".grammar", selector.Grammar); err != nil {
			return err
		}
		if err := requireText(field+".form", selector.Form); err != nil {
			return err
		}
		return nil
	}
	if selector.Grammar != "" || selector.Form != "" {
		return fmt.Errorf("%s must declare exactly exact or grammar+form", field)
	}
	return validateMetricName(field+".exact", selector.Exact)
}

func validateLabelCondition(field string, condition *LabelCondition) error {
	if condition == nil {
		return nil
	}
	if len(condition.Any) == 0 {
		return fmt.Errorf("%s.any must not be empty", field)
	}
	for clauseIndex, clause := range condition.Any {
		clauseField := fmt.Sprintf("%s.any[%d]", field, clauseIndex)
		if len(clause.All) == 0 {
			return fmt.Errorf("%s.all must not be empty", clauseField)
		}
		for predicateIndex, predicate := range clause.All {
			predicateField := fmt.Sprintf("%s.all[%d]", clauseField, predicateIndex)
			if err := validateLabelName(predicateField+".label", predicate.Label); err != nil {
				return err
			}
			if err := requireEnum(predicateField+".op", predicate.Op, "present", "nonblank", "absent", "eq", "in"); err != nil {
				return err
			}
			switch predicate.Op {
			case "present", "nonblank", "absent":
				if predicate.Value != nil || len(predicate.Values) != 0 {
					return fmt.Errorf("%s presence operation accepts no value", predicateField)
				}
			case "eq":
				if predicate.Value == nil || len(predicate.Values) != 0 {
					return fmt.Errorf("%s eq requires exactly value", predicateField)
				}
			case "in":
				if predicate.Value != nil || len(predicate.Values) == 0 {
					return fmt.Errorf("%s in requires exactly nonempty values", predicateField)
				}
				if err := validateStringSet(predicateField+".values", predicate.Values, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func validateConditionUse(
	field string,
	condition ConditionUse,
	axes map[string]EnvironmentAxis,
	policies map[string]EnvironmentPolicy,
	required bool,
) error {
	if condition.IsZero() {
		if required {
			return fmt.Errorf("%s must be present", field)
		}
		return nil
	}
	if condition.Policy != "" && condition.Inline != nil {
		return fmt.Errorf("%s cannot combine a policy and inline condition", field)
	}
	if condition.Policy != "" {
		if _, ok := policies[condition.Policy]; !ok {
			return fmt.Errorf("%s references unknown environment policy %q", field, condition.Policy)
		}
		return nil
	}
	return validateEnvironmentCondition(field, *condition.Inline, axes)
}

func validateEnvironmentCondition(
	field string,
	condition EnvironmentCondition,
	axes map[string]EnvironmentAxis,
) error {
	if len(condition.Any) == 0 {
		return fmt.Errorf("%s.any must not be empty", field)
	}
	seenClauses := make(map[string]struct{}, len(condition.Any))
	for clauseIndex, clause := range condition.Any {
		clauseField := fmt.Sprintf("%s.any[%d]", field, clauseIndex)
		if len(clause.All) == 0 {
			return fmt.Errorf("%s.all must not be empty", clauseField)
		}
		canonical := make([]string, 0, len(clause.All))
		seenPredicates := make(map[string]struct{}, len(clause.All))
		for predicateIndex, predicate := range clause.All {
			predicateField := fmt.Sprintf("%s.all[%d]", clauseField, predicateIndex)
			axis, ok := axes[predicate.Axis]
			if !ok {
				return fmt.Errorf("%s.axis references unknown axis %q", predicateField, predicate.Axis)
			}
			allowed := map[string][]string{
				"enum":         {"eq", "in"},
				"ordered_enum": {"eq", "in", "at_least", "at_most"},
				"integer":      {"eq", "in", "min", "max"},
				"enum_set":     {"contains", "contains_all", "contains_any", "excludes"},
			}[axis.Kind]
			if !slices.Contains(allowed, predicate.Op) {
				return fmt.Errorf("%s.op %q is not valid for %s", predicateField, predicate.Op, axis.Kind)
			}
			usesValues := predicate.Op == "in" || predicate.Op == "contains_all" || predicate.Op == "contains_any"
			if usesValues {
				if predicate.Value != nil || len(predicate.Values) == 0 {
					return fmt.Errorf("%s requires exactly nonempty values", predicateField)
				}
			} else if predicate.Value == nil || len(predicate.Values) != 0 {
				return fmt.Errorf("%s requires exactly value", predicateField)
			}
			values := predicate.Values
			if predicate.Value != nil {
				values = []AxisValue{*predicate.Value}
			}
			for valueIndex, value := range values {
				if err := validateAxisValue(
					fmt.Sprintf("%s.values[%d]", predicateField, valueIndex),
					value,
					axis,
				); err != nil {
					return err
				}
				if axis.Kind == "enum_set" && value.String == nil {
					return fmt.Errorf("%s values must be individual strings for operation %q", predicateField, predicate.Op)
				}
			}
			valueKeys := make([]string, 0, len(values))
			for _, value := range values {
				valueKeys = append(valueKeys, axisValueKey(value))
			}
			slices.Sort(valueKeys)
			predicateKey := predicate.Axis + ":" + predicate.Op + ":" + strings.Join(valueKeys, ",")
			if _, ok := seenPredicates[predicateKey]; ok {
				return fmt.Errorf("%s duplicates a previous predicate", predicateField)
			}
			seenPredicates[predicateKey] = struct{}{}
			canonical = append(canonical, predicateKey)
		}
		if !environmentClauseSatisfiable(clause, axes) {
			return fmt.Errorf("%s contains contradictory predicates", clauseField)
		}
		slices.Sort(canonical)
		key := strings.Join(canonical, "|")
		if _, ok := seenClauses[key]; ok {
			return fmt.Errorf("%s duplicates a previous canonical clause", clauseField)
		}
		seenClauses[key] = struct{}{}
	}
	return nil
}

func environmentClauseSatisfiable(clause EnvironmentClause, axes map[string]EnvironmentAxis) bool {
	byAxis := make(map[string][]EnvironmentPredicate)
	for _, predicate := range clause.All {
		byAxis[predicate.Axis] = append(byAxis[predicate.Axis], predicate)
	}
	for axisID, predicates := range byAxis {
		axis := axes[axisID]
		switch axis.Kind {
		case "enum", "ordered_enum":
			allowed := make(map[string]struct{}, len(axis.Values))
			for _, value := range axis.Values {
				allowed[value] = struct{}{}
			}
			for _, predicate := range predicates {
				switch predicate.Op {
				case "eq":
					intersectStringSet(allowed, []string{*predicate.Value.String})
				case "in":
					values := make([]string, 0, len(predicate.Values))
					for _, value := range predicate.Values {
						values = append(values, *value.String)
					}
					intersectStringSet(allowed, values)
				case "at_least", "at_most":
					index := slices.Index(axis.Values, *predicate.Value.String)
					values := axis.Values[index:]
					if predicate.Op == "at_most" {
						values = axis.Values[:index+1]
					}
					intersectStringSet(allowed, values)
				}
			}
			if len(allowed) == 0 {
				return false
			}
		case "integer":
			minimum, maximum := *axis.Min, *axis.Max
			var allowed map[int]struct{}
			for _, predicate := range predicates {
				switch predicate.Op {
				case "min":
					minimum = max(minimum, *predicate.Value.Integer)
				case "max":
					maximum = min(maximum, *predicate.Value.Integer)
				case "eq":
					values := map[int]struct{}{*predicate.Value.Integer: {}}
					allowed = intersectIntSets(allowed, values)
				case "in":
					values := make(map[int]struct{}, len(predicate.Values))
					for _, value := range predicate.Values {
						values[*value.Integer] = struct{}{}
					}
					allowed = intersectIntSets(allowed, values)
				}
			}
			if minimum > maximum {
				return false
			}
			if allowed != nil {
				found := false
				for value := range allowed {
					if value >= minimum && value <= maximum {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			}
		case "enum_set":
			required := make(map[string]struct{})
			excluded := make(map[string]struct{})
			var anyGroups [][]string
			for _, predicate := range predicates {
				switch predicate.Op {
				case "contains":
					required[*predicate.Value.String] = struct{}{}
				case "contains_all":
					for _, value := range predicate.Values {
						required[*value.String] = struct{}{}
					}
				case "contains_any":
					group := make([]string, 0, len(predicate.Values))
					for _, value := range predicate.Values {
						group = append(group, *value.String)
					}
					anyGroups = append(anyGroups, group)
				case "excludes":
					excluded[*predicate.Value.String] = struct{}{}
				}
			}
			for value := range required {
				if _, ok := excluded[value]; ok {
					return false
				}
			}
			for _, group := range anyGroups {
				possible := false
				for _, value := range group {
					if _, excluded := excluded[value]; !excluded {
						possible = true
						break
					}
				}
				if !possible {
					return false
				}
			}
		}
	}
	return true
}

func intersectStringSet(target map[string]struct{}, values []string) {
	keep := make(map[string]struct{}, len(values))
	for _, value := range values {
		keep[value] = struct{}{}
	}
	for value := range target {
		if _, ok := keep[value]; !ok {
			delete(target, value)
		}
	}
}

func intersectIntSets(left, right map[int]struct{}) map[int]struct{} {
	if left == nil {
		return right
	}
	result := make(map[int]struct{})
	for value := range left {
		if _, ok := right[value]; ok {
			result[value] = struct{}{}
		}
	}
	return result
}

func axisValueKey(value AxisValue) string {
	switch {
	case value.String != nil:
		return "s:" + *value.String
	case value.Integer != nil:
		return "i:" + strconv.Itoa(*value.Integer)
	default:
		return "set:" + strings.Join(value.Strings, ",")
	}
}

func validateAxisValue(field string, value AxisValue, axis EnvironmentAxis) error {
	switch axis.Kind {
	case "enum", "ordered_enum":
		if value.String == nil || value.Integer != nil || value.Strings != nil {
			return fmt.Errorf("%s must be a string for %s", field, axis.Kind)
		}
		if !slices.Contains(axis.Values, *value.String) {
			return fmt.Errorf("%s value %q is outside the axis domain", field, *value.String)
		}
	case "integer":
		if value.Integer == nil || value.String != nil || value.Strings != nil {
			return fmt.Errorf("%s must be an integer", field)
		}
		if value.Integer == nil || *value.Integer < *axis.Min || *value.Integer > *axis.Max {
			return fmt.Errorf("%s value is outside the axis bounds", field)
		}
	case "enum_set":
		var values []string
		switch {
		case value.String != nil && value.Integer == nil && value.Strings == nil:
			values = []string{*value.String}
		case value.String == nil && value.Integer == nil && value.Strings != nil:
			values = value.Strings
		default:
			return fmt.Errorf("%s must be a string or string sequence for enum_set", field)
		}
		if err := validateStringSet(field, values, false); err != nil {
			return err
		}
		for _, item := range values {
			if !slices.Contains(axis.Values, item) {
				return fmt.Errorf("%s value %q is outside the axis domain", field, item)
			}
		}
	}
	return nil
}
