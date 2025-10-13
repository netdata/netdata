package confopt

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAutoBoolStringAndStates(t *testing.T) {
	tests := []struct {
		value        AutoBool
		expectString string
		enabled      bool
		disabled     bool
		auto         bool
	}{
		{AutoBool(""), "auto", false, false, true},
		{AutoBoolAuto, "auto", false, false, true},
		{AutoBoolEnabled, "enabled", true, false, false},
		{AutoBoolDisabled, "disabled", false, true, false},
		{AutoBool("ENABLED"), "enabled", true, false, false},
	}

	for _, tc := range tests {
		if got := tc.value.String(); got != tc.expectString {
			t.Fatalf("String() => %q, want %q", got, tc.expectString)
		}
		if tc.value.IsEnabled() != tc.enabled {
			t.Fatalf("IsEnabled() => %t, want %t for %q", tc.value.IsEnabled(), tc.enabled, tc.value)
		}
		if tc.value.IsDisabled() != tc.disabled {
			t.Fatalf("IsDisabled() => %t, want %t for %q", tc.value.IsDisabled(), tc.disabled, tc.value)
		}
		if tc.value.IsAuto() != tc.auto {
			t.Fatalf("IsAuto() => %t, want %t for %q", tc.value.IsAuto(), tc.auto, tc.value)
		}
	}
}

func TestAutoBoolToBool(t *testing.T) {
	if ptr := AutoBoolAuto.ToBool(); ptr != nil {
		t.Fatalf("AutoBoolAuto.ToBool() => %v, want nil", *ptr)
	}
	if ptr := AutoBoolEnabled.ToBool(); ptr == nil || *ptr != true {
		t.Fatalf("AutoBoolEnabled.ToBool() => %v, want true", ptr)
	}
	if ptr := AutoBoolDisabled.ToBool(); ptr == nil || *ptr != false {
		t.Fatalf("AutoBoolDisabled.ToBool() => %v, want false", ptr)
	}
}

func TestAutoBoolBool(t *testing.T) {
	if got := AutoBoolAuto.Bool(true); got != true {
		t.Fatalf("AutoBoolAuto.Bool(true) => %t, want true", got)
	}
	if got := AutoBoolAuto.Bool(false); got != false {
		t.Fatalf("AutoBoolAuto.Bool(false) => %t, want false", got)
	}
	if got := AutoBoolEnabled.Bool(false); got != true {
		t.Fatalf("AutoBoolEnabled.Bool(false) => %t, want true", got)
	}
	if got := AutoBoolDisabled.Bool(true); got != false {
		t.Fatalf("AutoBoolDisabled.Bool(true) => %t, want false", got)
	}
}

func TestAutoBoolJSONRoundTrip(t *testing.T) {
	for _, tc := range []AutoBool{AutoBoolAuto, AutoBoolEnabled, AutoBoolDisabled} {
		data, err := json.Marshal(tc)
		if err != nil {
			t.Fatalf("json.Marshal(%q) unexpected error: %v", tc, err)
		}
		var back AutoBool
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("json.Unmarshal(%q) unexpected error: %v", tc, err)
		}
		if back != tc {
			t.Fatalf("json round-trip => %q, want %q", back, tc)
		}
	}
}

func TestAutoBoolYAMLRoundTrip(t *testing.T) {
	for _, tc := range []AutoBool{AutoBoolAuto, AutoBoolEnabled, AutoBoolDisabled} {
		data, err := yaml.Marshal(tc)
		if err != nil {
			t.Fatalf("yaml.Marshal(%q) unexpected error: %v", tc, err)
		}
		var back AutoBool
		if err := yaml.Unmarshal(data, &back); err != nil {
			t.Fatalf("yaml.Unmarshal(%q) unexpected error: %v", tc, err)
		}
		if back != tc {
			t.Fatalf("yaml round-trip => %q, want %q", back, tc)
		}
	}
}

func TestAutoBoolInvalidInputs(t *testing.T) {
	var a AutoBool
	if err := json.Unmarshal([]byte(`"maybe"`), &a); err == nil {
		t.Fatalf("expected json.Unmarshal error for invalid value")
	}
	if err := yaml.Unmarshal([]byte("maybe"), &a); err == nil {
		t.Fatalf("expected yaml.Unmarshal error for invalid value")
	}
}

func TestAutoBoolWithDefault(t *testing.T) {
	if got := AutoBoolAuto.WithDefault(true); got != AutoBoolEnabled {
		t.Fatalf("AutoBoolAuto.WithDefault(true) => %q, want %q", got, AutoBoolEnabled)
	}
	if got := AutoBoolAuto.WithDefault(false); got != AutoBoolDisabled {
		t.Fatalf("AutoBoolAuto.WithDefault(false) => %q, want %q", got, AutoBoolDisabled)
	}
	if got := AutoBoolEnabled.WithDefault(false); got != AutoBoolEnabled {
		t.Fatalf("AutoBoolEnabled.WithDefault(false) => %q, want %q", got, AutoBoolEnabled)
	}
}

func TestAutoBoolFromBool(t *testing.T) {
	if got := AutoBoolFromBool(true); got != AutoBoolEnabled {
		t.Fatalf("AutoBoolFromBool(true) => %q, want %q", got, AutoBoolEnabled)
	}
	if got := AutoBoolFromBool(false); got != AutoBoolDisabled {
		t.Fatalf("AutoBoolFromBool(false) => %q, want %q", got, AutoBoolDisabled)
	}
}
