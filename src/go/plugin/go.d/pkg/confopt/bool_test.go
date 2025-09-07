// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"testing"

	"github.com/goccy/go-yaml"
)

func TestFlexBool_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input     string
		want      FlexBool
		wantError bool
	}{
		// Native boolean values
		"native_true":  {input: "value: true", want: true},
		"native_false": {input: "value: false", want: false},

		// Yes/No variants
		"yes_lower": {input: "value: yes", want: true},
		"no_lower":  {input: "value: no", want: false},
		"yes_upper": {input: "value: YES", want: true},
		"no_upper":  {input: "value: NO", want: false},
		"yes_mixed": {input: "value: Yes", want: true},
		"no_mixed":  {input: "value: No", want: false},
		"y_lower":   {input: "value: y", want: true},
		"n_lower":   {input: "value: n", want: false},
		"y_upper":   {input: "value: Y", want: true},
		"n_upper":   {input: "value: N", want: false},

		// On/Off variants
		"on_lower":  {input: "value: on", want: true},
		"off_lower": {input: "value: off", want: false},
		"on_upper":  {input: "value: ON", want: true},
		"off_upper": {input: "value: OFF", want: false},
		"on_mixed":  {input: "value: On", want: true},
		"off_mixed": {input: "value: Off", want: false},

		// T/F variants
		"t_lower": {input: "value: t", want: true},
		"f_lower": {input: "value: f", want: false},
		"t_upper": {input: "value: T", want: true},
		"f_upper": {input: "value: F", want: false},

		// Numeric values
		"number_0":    {input: "value: 0", want: false},
		"number_1":    {input: "value: 1", want: true},
		"string_0":    {input: "value: '0'", want: false},
		"string_1":    {input: "value: '1'", want: true},
		"string_0_dq": {input: "value: \"0\"", want: false},
		"string_1_dq": {input: "value: \"1\"", want: true},

		// Quoted string values
		"quoted_true_sq":  {input: "value: 'true'", want: true},
		"quoted_false_sq": {input: "value: 'false'", want: false},
		"quoted_true_dq":  {input: "value: \"true\"", want: true},
		"quoted_false_dq": {input: "value: \"false\"", want: false},
		"quoted_yes_sq":   {input: "value: 'yes'", want: true},
		"quoted_no_sq":    {input: "value: 'no'", want: false},
		"quoted_yes_dq":   {input: "value: \"yes\"", want: true},
		"quoted_no_dq":    {input: "value: \"no\"", want: false},

		// Whitespace handling
		"whitespace_true":   {input: "value: ' true '", want: true},
		"whitespace_false":  {input: "value: ' false '", want: false},
		"whitespace_yes":    {input: "value: ' yes '", want: true},
		"whitespace_no":     {input: "value: ' no '", want: false},
		"whitespace_quoted": {input: "value: ' \"yes\" '", want: true},
		"tab_true":          {input: "value: '\ttrue\t'", want: true},
		"newline_true":      {input: "value: '\ntrue\n'", want: true},

		// Error cases
		"invalid_string":      {input: "value: maybe", wantError: true},
		"invalid_number_2":    {input: "value: 2", wantError: true},
		"invalid_number_neg1": {input: "value: -1", wantError: true},
		"invalid_number_10":   {input: "value: 10", wantError: true},
		"empty_string":        {input: "value: ''", wantError: true},
		"empty_string_dq":     {input: "value: \"\"", wantError: true},
		"random_text":         {input: "value: random", wantError: true},
		"partial_true":        {input: "value: tru", wantError: true},
		"partial_false":       {input: "value: fals", wantError: true},
		"yess":                {input: "value: yess", wantError: true},
		"noo":                 {input: "value: noo", wantError: true},

		// Edge cases with different YAML structures
		"flow_style_true":   {input: "{value: true}", want: true},
		"flow_style_yes":    {input: "{value: yes}", want: true},
		"multiline_literal": {input: "value: |\n  yes", want: true},
		"multiline_folded":  {input: "value: >\n  no", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var s struct {
				Value FlexBool `yaml:"value"`
			}

			err := yaml.Unmarshal([]byte(tc.input), &s)

			if tc.wantError {
				if err == nil {
					t.Errorf("expected error but got none, value: %v", s.Value)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if s.Value != tc.want {
					t.Errorf("got %v, want %v", s.Value, tc.want)
				}
			}
		})
	}
}

func TestFlexBool_MarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input FlexBool
		want  string
	}{
		"marshal_true":  {input: true, want: "value: true\n"},
		"marshal_false": {input: false, want: "value: false\n"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := struct {
				Value FlexBool `yaml:"value"`
			}{
				Value: tc.input,
			}

			got, err := yaml.Marshal(&s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(got) != tc.want {
				t.Errorf("got %q, want %q", string(got), tc.want)
			}
		})
	}
}

func TestFlexBool_ComplexStructures(t *testing.T) {
	tests := map[string]struct {
		input     string
		wantDebug FlexBool
		wantProd  FlexBool
		wantError bool
	}{
		"mixed_values": {
			input: `
debug: yes
prod: false`,
			wantDebug: true,
			wantProd:  false,
		},
		"all_different_formats": {
			input: `
debug: 1
prod: off`,
			wantDebug: true,
			wantProd:  false,
		},
		"quoted_mixed": {
			input: `
debug: "yes"
prod: 'no'`,
			wantDebug: true,
			wantProd:  false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var config struct {
				Debug FlexBool `yaml:"debug"`
				Prod  FlexBool `yaml:"prod"`
			}

			err := yaml.Unmarshal([]byte(tc.input), &config)

			if tc.wantError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if config.Debug != tc.wantDebug {
					t.Errorf("debug: got %v, want %v", config.Debug, tc.wantDebug)
				}
				if config.Prod != tc.wantProd {
					t.Errorf("prod: got %v, want %v", config.Prod, tc.wantProd)
				}
			}
		})
	}
}

func TestFlexBool_RoundTrip(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string // Expected output after marshal (always true/false)
	}{
		"roundtrip_yes":   {input: "value: yes", want: "value: true\n"},
		"roundtrip_no":    {input: "value: no", want: "value: false\n"},
		"roundtrip_on":    {input: "value: on", want: "value: true\n"},
		"roundtrip_off":   {input: "value: off", want: "value: false\n"},
		"roundtrip_1":     {input: "value: 1", want: "value: true\n"},
		"roundtrip_0":     {input: "value: 0", want: "value: false\n"},
		"roundtrip_true":  {input: "value: true", want: "value: true\n"},
		"roundtrip_false": {input: "value: false", want: "value: false\n"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var s struct {
				Value FlexBool `yaml:"value"`
			}

			// Unmarshal
			if err := yaml.Unmarshal([]byte(tc.input), &s); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}

			// Marshal back
			got, err := yaml.Marshal(&s)
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			if string(got) != tc.want {
				t.Errorf("got %q, want %q", string(got), tc.want)
			}
		})
	}
}
