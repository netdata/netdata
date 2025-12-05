package output

import "testing"

func TestParsePerfdata(t *testing.T) {
	raw := []byte("OK - all good | 'time'=123ms;200;500;0;1000 'load1'=0.12;1.0;2.0\nLong output line\nAnother | ignored")
	parsed := Parse(raw)
	if parsed.StatusLine != "OK - all good" {
		t.Fatalf("unexpected status line: %s", parsed.StatusLine)
	}
	if parsed.LongOutput != "Long output line\nAnother" {
		t.Fatalf("unexpected long output: %s", parsed.LongOutput)
	}
	if len(parsed.Perfdata) != 2 {
		t.Fatalf("expected 2 perfdata entries, got %d", len(parsed.Perfdata))
	}
	if parsed.Perfdata[0].Label != "time" || parsed.Perfdata[0].Unit != "ms" || parsed.Perfdata[0].Value != 123 {
		t.Fatalf("unexpected first perfdatum: %+v", parsed.Perfdata[0])
	}
	if parsed.Perfdata[0].Warn == nil || parsed.Perfdata[0].Warn.High == nil || *parsed.Perfdata[0].Warn.High != 200 {
		t.Fatalf("expected warn high=200: %+v", parsed.Perfdata[0].Warn)
	}
	if parsed.Perfdata[0].Crit == nil || parsed.Perfdata[0].Crit.High == nil || *parsed.Perfdata[0].Crit.High != 500 {
		t.Fatalf("expected crit high=500: %+v", parsed.Perfdata[0].Crit)
	}
	if parsed.Perfdata[1].Label != "load1" || parsed.Perfdata[1].Value != 0.12 {
		t.Fatalf("unexpected second perfdatum: %+v", parsed.Perfdata[1])
	}
}

func TestParseRangeVariants(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expectNil bool
		low       *float64
		high      *float64
		inclusive bool
	}{
		{name: "simple", input: "10", low: floatPtr(0), high: floatPtr(10)},
		{name: "range", input: "10:20", low: floatPtr(10), high: floatPtr(20)},
		{name: "inclusive", input: "@5:15", low: floatPtr(5), high: floatPtr(15), inclusive: true},
		{name: "lower_unbounded", input: "~:5", low: nil, high: floatPtr(5)},
		{name: "upper_unbounded", input: "10:", low: floatPtr(10), high: nil},
		{name: "default_low", input: ":30", low: floatPtr(0), high: floatPtr(30)},
		{name: "unknown", input: "U", expectNil: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rng := parseRange(tc.input)
			if tc.expectNil {
				if rng != nil {
					t.Fatalf("expected nil range, got %+v", rng)
				}
				return
			}
			if rng == nil {
				t.Fatalf("expected non-nil range for %q", tc.input)
			}
			if tc.low == nil {
				if rng.Low != nil {
					t.Fatalf("expected nil low, got %+v", *rng.Low)
				}
			} else if rng.Low == nil || *rng.Low != *tc.low {
				t.Fatalf("unexpected low: got %v want %v", rng.Low, *tc.low)
			}
			if tc.high == nil {
				if rng.High != nil {
					t.Fatalf("expected nil high, got %+v", *rng.High)
				}
			} else if rng.High == nil || *rng.High != *tc.high {
				t.Fatalf("unexpected high: got %v want %v", rng.High, *tc.high)
			}
			if rng.Inclusive != tc.inclusive {
				t.Fatalf("unexpected inclusive flag: got %v want %v", rng.Inclusive, tc.inclusive)
			}
		})
	}
}

func floatPtr(v float64) *float64 {
	return &v
}

func TestParseLongOutputPreservesLeadingWhitespace(t *testing.T) {
	raw := []byte("WARNING something broke\n  first line\n\tsecond line  \n")
	parsed := Parse(raw)
	expected := "  first line\n\tsecond line"
	if parsed.LongOutput != expected {
		t.Fatalf("expected long output %q, got %q", expected, parsed.LongOutput)
	}
}
