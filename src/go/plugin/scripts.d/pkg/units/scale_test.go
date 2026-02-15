package units

import "testing"

func TestNewScaleDistinguishesBitsAndBytes(t *testing.T) {
	cases := []struct {
		name          string
		unit          string
		value         float64
		canonicalUnit string
		want          int64
	}{
		{name: "megabytes per second", unit: "MBps", value: 1.5, canonicalUnit: "bytes/s", want: 1_500_000},
		{name: "megabits per second", unit: "Mbps", value: 1.5, canonicalUnit: "bits/s", want: 1_500_000},
		{name: "megabytes", unit: "MB", value: 2.5, canonicalUnit: "bytes", want: 2_500_000},
		{name: "megabits", unit: "Mb", value: 2.5, canonicalUnit: "bits", want: 2_500_000},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scale := NewScale(tc.unit)
			if scale.CanonicalUnit != tc.canonicalUnit {
				t.Fatalf("expected canonical unit %q, got %q", tc.canonicalUnit, scale.CanonicalUnit)
			}
			if got := scale.Apply(tc.value); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
