package confopt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AutoBool represents a tri-state boolean with explicit auto/enable/disable semantics.
// The zero value maps to Auto, which preserves historical behaviour where omitting
// a pointer boolean left feature selection up to runtime detection.
type AutoBool string

const (
	// AutoBoolAuto defers decision making to module logic (historic nil behaviour).
	AutoBoolAuto AutoBool = "auto"
	// AutoBoolEnabled forces collection on.
	AutoBoolEnabled AutoBool = "enabled"
	// AutoBoolDisabled forces collection off.
	AutoBoolDisabled AutoBool = "disabled"
)

// AutoBoolEnum exposes all valid enum values for schema/doc generation.
var AutoBoolEnum = []AutoBool{AutoBoolAuto, AutoBoolEnabled, AutoBoolDisabled}

// String returns the canonical string form (defaults to "auto" for zero values).
func (a AutoBool) String() string {
	return string(normalizeAutoBool(a))
}

// IsAuto reports whether the value is set to Auto.
func (a AutoBool) IsAuto() bool {
	return normalizeAutoBool(a) == AutoBoolAuto
}

// IsEnabled reports whether the value explicitly enables a feature.
func (a AutoBool) IsEnabled() bool {
	return normalizeAutoBool(a) == AutoBoolEnabled
}

// IsDisabled reports whether the value explicitly disables a feature.
func (a AutoBool) IsDisabled() bool {
	return normalizeAutoBool(a) == AutoBoolDisabled
}

// ToBool converts the tri-state value to a boolean pointer, matching the
// legacy pointer semantics where nil meant "auto"/unset.
func (a AutoBool) ToBool() *bool {
	switch normalizeAutoBool(a) {
	case AutoBoolEnabled:
		v := true
		return &v
	case AutoBoolDisabled:
		v := false
		return &v
	default:
		return nil
	}
}

// Bool resolves the tri-state value to a boolean using defaultValue when the
// state is Auto. This mirrors the historic pattern of dereferencing pointers with
// dynamically supplied defaults.
func (a AutoBool) Bool(defaultValue bool) bool {
	switch normalizeAutoBool(a) {
	case AutoBoolEnabled:
		return true
	case AutoBoolDisabled:
		return false
	default:
		return defaultValue
	}
}

// WithDefault resolves the auto state using the provided default and returns the
// resulting explicit enum value.
func (a AutoBool) WithDefault(defaultValue bool) AutoBool {
	if a.IsAuto() {
		return AutoBoolFromBool(defaultValue)
	}
	return normalizeAutoBool(a)
}

// AutoBoolFromBool converts a standard bool into the explicit enum form.
func AutoBoolFromBool(value bool) AutoBool {
	if value {
		return AutoBoolEnabled
	}
	return AutoBoolDisabled
}

// MarshalYAML ensures we always emit the canonical lower-case string.
func (a AutoBool) MarshalYAML() (interface{}, error) {
	return a.String(), nil
}

// UnmarshalYAML accepts literal booleans and strings (case insensitive) and
// defaults to auto when empty. Any other value results in an error to ensure
// early feedback. The signature matches the yaml.v2 marshaler interface so the
// same implementation works for both yaml.v2 and yaml.v3 consumers.
func (a *AutoBool) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if unmarshal == nil {
		*a = AutoBoolAuto
		return nil
	}

	var raw interface{}
	if err := unmarshal(&raw); err != nil {
		return err
	}

	switch v := raw.(type) {
	case nil:
		*a = AutoBoolAuto
		return nil
	case bool:
		if v {
			*a = AutoBoolEnabled
		} else {
			*a = AutoBoolDisabled
		}
		return nil
	case string:
		value := strings.TrimSpace(v)
		if value == "" {
			*a = AutoBoolAuto
			return nil
		}
		parsed, err := parseAutoBool(value)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case []byte:
		value := strings.TrimSpace(string(v))
		if value == "" {
			*a = AutoBoolAuto
			return nil
		}
		parsed, err := parseAutoBool(value)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	default:
		return fmt.Errorf("autobool: expected boolean or string value, got %T", raw)
	}
}

// MarshalJSON writes the canonical string representation.
func (a AutoBool) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.String())
}

// UnmarshalJSON accepts string values (case insensitive).
func (a *AutoBool) UnmarshalJSON(data []byte) error {
	var rawBool bool
	if err := json.Unmarshal(data, &rawBool); err == nil {
		if rawBool {
			*a = AutoBoolEnabled
		} else {
			*a = AutoBoolDisabled
		}
		return nil
	}
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("autobool: expected string value: %w", err)
	}
	parsed, err := parseAutoBool(raw)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// normalizeAutoBool coerces unknown values to Auto to retain defensive behaviour.
func normalizeAutoBool(value AutoBool) AutoBool {
	switch AutoBool(strings.ToLower(string(value))) {
	case AutoBoolEnabled:
		return AutoBoolEnabled
	case AutoBoolDisabled:
		return AutoBoolDisabled
	default:
		return AutoBoolAuto
	}
}

func parseAutoBool(value string) (AutoBool, error) {
	switch AutoBool(strings.ToLower(strings.TrimSpace(value))) {
	case AutoBoolAuto, "":
		return AutoBoolAuto, nil
	case AutoBoolEnabled:
		return AutoBoolEnabled, nil
	case AutoBoolDisabled:
		return AutoBoolDisabled, nil
	default:
		return AutoBoolAuto, fmt.Errorf("autobool: invalid value %q (expected one of auto, enabled, disabled)", value)
	}
}
