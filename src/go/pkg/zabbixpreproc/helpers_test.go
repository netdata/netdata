package zabbixpreproc

import (
	"testing"
)

// Unit tests for helper functions (separate from integration tests)

func TestFormatFloat(t *testing.T) {
	tests := []struct {
		name  string
		input float64
		want  string
	}{
		{"whole number", 42.0, "42"},
		{"simple decimal", 3.14, "3.14"},
		{"small decimal", 0.001, "0.001"},
		{"large number", 1234567.89, "1.23456789e+06"},
		{"negative whole", -100.0, "-100"},
		{"negative decimal", -0.5, "-0.5"},
		{"zero", 0.0, "0"},
		{"very small", 0.0000001, "1e-07"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFloat(tt.input)
			if got != tt.want {
				t.Errorf("formatFloat(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestInterpretEscapeSequences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no escapes", "hello", "hello"},
		{"newline", "line1\\nline2", "line1\nline2"},
		{"tab", "col1\\tcol2", "col1\tcol2"},
		{"carriage return", "text\\rmore", "text\rmore"},
		{"backslash", "path\\\\file", "path\\file"},
		{"space", "a\\sb", "a b"},
		{"multiple escapes", "a\\nb\\tc", "a\nb\tc"},
		{"double backslash s", "trim\\\\sthis", "trim \t\r\n\\this"}, // \\s expands to full whitespace class
		{"unknown escape", "\\x41BC", "\\x41BC"},                     // Unknown escapes preserved
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := interpretEscapeSequences(tt.input)
			if got != tt.want {
				t.Errorf("interpretEscapeSequences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestConvertBackreferences(t *testing.T) {
	tests := []struct {
		name        string
		replacement string
		want        string
	}{
		{"no backrefs", "literal", "literal"},
		{"\\1 to ${1}", "prefix\\1suffix", "prefix${1}suffix"},
		{"\\2 to ${2}", "value\\2", "value${2}"},
		{"multiple backrefs", "\\1 and \\2", "${1} and ${2}"},
		{"\\\\1 literal backslash", "\\\\1", "\\1"},
		{"mixed", "a\\1b\\\\2c\\3", "a${1}b\\2c${3}"},
		{"\\0 full match", "result\\0", "result${0}"},
		{"multi-digit", "\\10", "${10}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertBackreferences(tt.replacement)
			if got != tt.want {
				t.Errorf("convertBackreferences(%q) = %q, want %q", tt.replacement, got, tt.want)
			}
		})
	}
}

func TestParseHexBytes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []byte
		wantErr bool
	}{
		{"simple hex", "48656C6C6F", []byte("Hello"), false},
		{"with spaces", "48 65 6C 6C 6F", []byte("Hello"), false},
		{"lowercase", "68656c6c6f", []byte("hello"), false},
		{"mixed case", "48656C6c6F", []byte("Hello"), false},
		{"empty", "", []byte{}, false},
		{"invalid hex", "ZZZZ", nil, true},
		{"odd length", "48656C6C6", nil, true},
		{"null bytes", "00410042", []byte("\x00A\x00B"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHexBytes(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseHexBytes(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseHexBytes(%q) unexpected error: %v", tt.input, err)
				}
				if string(got) != string(tt.want) {
					t.Errorf("parseHexBytes(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestConvertBITSToInteger(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"single byte", "05", "5", false},
		{"two bytes hex", "01 02", "513", false},             // Little-endian: reversed to 02 01 = 0x0201 = 513
		{"four bytes hex", "01 02 03 04", "67305985", false}, // Little-endian: reversed to 04 03 02 01 = 0x04030201 = 67305985
		{"all zeros", "00 00", "0", false},
		{"all ones", "FF FF", "65535", false},
		{"invalid hex", "ZZ", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertBITSToInteger(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("convertBITSToInteger(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("convertBITSToInteger(%q) unexpected error: %v", tt.input, err)
				}
				if got != tt.want {
					t.Errorf("convertBITSToInteger(%q) = %q, want %q", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestFormatSTRING(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		formatMode int
		want       string
		wantErr    bool
	}{
		{"plain text - value only", "hello", 1, "hello", false},
		{"with quotes - value only", `"quoted"`, 1, `quoted`, false}, // formatSTRING strips outer quotes
		{"empty - value only", "", 1, "", false},
		{"plain text - oid and value", "hello", 0, "hello", false},
		{"with quotes - oid and value", `"quoted"`, 0, `quoted`, false}, // formatSTRING strips outer quotes
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := formatSTRING(tt.input, tt.formatMode)
			if tt.wantErr {
				if err == nil {
					t.Errorf("formatSTRING(%q, %d) expected error, got nil", tt.input, tt.formatMode)
				}
			} else {
				if err != nil {
					t.Errorf("formatSTRING(%q, %d) unexpected error: %v", tt.input, tt.formatMode, err)
				}
				if got != tt.want {
					t.Errorf("formatSTRING(%q, %d) = %q, want %q", tt.input, tt.formatMode, got, tt.want)
				}
			}
		})
	}
}

func TestValueTypeToString(t *testing.T) {
	tests := []struct {
		name  string
		input ValueType
		want  string
	}{
		{"string type", ValueTypeStr, "ITEM_VALUE_TYPE_STR"},
		{"uint64 type", ValueTypeUint64, "ITEM_VALUE_TYPE_UINT64"},
		{"float type", ValueTypeFloat, "ITEM_VALUE_TYPE_FLOAT"},
		{"invalid type", ValueType(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valueTypeToString(tt.input)
			if got != tt.want {
				t.Errorf("valueTypeToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseValueType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ValueType
		wantErr bool
	}{
		{"string type", "ITEM_VALUE_TYPE_STR", ValueTypeStr, false},
		{"uint64 type", "ITEM_VALUE_TYPE_UINT64", ValueTypeUint64, false},
		{"float type", "ITEM_VALUE_TYPE_FLOAT", ValueTypeFloat, false},
		{"invalid type", "INVALID", ValueTypeStr, true},
		{"empty", "", ValueTypeStr, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseValueType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseValueType(%q) expected error, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("parseValueType(%q) unexpected error: %v", tt.input, err)
				}
				if got != tt.want {
					t.Errorf("parseValueType(%q) = %v, want %v", tt.input, got, tt.want)
				}
			}
		})
	}
}

func TestNoopLogger(t *testing.T) {
	// Ensure NoopLogger doesn't panic and accepts all methods
	logger := NoopLogger{}
	logger.Debug("test", "key", "value")
	logger.Info("test", "key", "value")
	logger.Warn("test", "key", "value")
	logger.Error("test", "key", "value")

	// If we get here without panic, test passes
}

func TestErrorHandlerValidation(t *testing.T) {
	p := NewPreprocessor("test")

	tests := []struct {
		name    string
		handler ErrorHandler
		origErr error
		wantErr bool
		wantVal string
	}{
		{
			name:    "default - returns original error",
			handler: ErrorHandler{Action: ErrorActionDefault},
			origErr: errTestError,
			wantErr: true,
		},
		{
			name:    "discard - returns empty value",
			handler: ErrorHandler{Action: ErrorActionDiscard},
			origErr: errTestError,
			wantErr: false,
			wantVal: "",
		},
		{
			name:    "set value",
			handler: ErrorHandler{Action: ErrorActionSetValue, Params: "fallback"},
			origErr: errTestError,
			wantErr: false,
			wantVal: "fallback",
		},
		{
			name:    "set error - custom message",
			handler: ErrorHandler{Action: ErrorActionSetError, Params: "Custom error"},
			origErr: errTestError,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := p.handleError(tt.origErr, tt.handler)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && val.Data != tt.wantVal {
				t.Errorf("got value %q, want %q", val.Data, tt.wantVal)
			}
		})
	}
}

// Helper error for tests
var errTestError = &testError{msg: "test error"}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
