// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"math"
	"testing"
	"time"
)

func TestParseMetricFloat(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  float64
		ok    bool
	}{
		{name: "integer", value: int32(42), want: 42, ok: true},
		{name: "unsigned", value: uint64(43), want: 43, ok: true},
		{name: "float", value: 1.25, want: 1.25, ok: true},
		{name: "trimmed string", value: " 2.5 ", want: 2.5, ok: true},
		{name: "not a number", value: math.NaN()},
		{name: "infinity", value: math.Inf(1)},
		{name: "invalid string", value: "invalid"},
		{name: "unsupported", value: struct{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseMetricFloat(tt.value)
			if ok != tt.ok || ok && got != tt.want {
				t.Fatalf("ParseMetricFloat(%v) = %v/%v, want %v/%v", tt.value, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseMetricStateTTL(t *testing.T) {
	if got, err := ParseMetricStateTTL("250ms"); err != nil || got != 250*time.Millisecond {
		t.Fatalf("ParseMetricStateTTL(250ms) = %v/%v", got, err)
	}
	for _, value := range []string{"invalid", "0s", "-1s"} {
		if _, err := ParseMetricStateTTL(value); err == nil {
			t.Fatalf("ParseMetricStateTTL(%q) returned nil error", value)
		}
	}
}
