package output

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePerfdata(t *testing.T) {
	raw := []byte("OK - all good | 'time'=123ms;200;500;0;1000 'load1'=0.12;1.0;2.0\nLong output line\nAnother | ignored")
	parsed := Parse(raw)
	assert.Equal(t, "OK - all good", parsed.StatusLine())
	assert.Equal(t, "Long output line\nAnother", parsed.LongOutput())
	require.Len(t, parsed.Perfdata, 2)
	assert.Equal(t, "time", parsed.Perfdata[0].Label)
	assert.Equal(t, "ms", parsed.Perfdata[0].Unit)
	assert.Equal(t, 123.0, parsed.Perfdata[0].Value)
	require.NotNil(t, parsed.Perfdata[0].Warn)
	require.NotNil(t, parsed.Perfdata[0].Warn.High)
	assert.Equal(t, 200.0, *parsed.Perfdata[0].Warn.High)
	require.NotNil(t, parsed.Perfdata[0].Crit)
	require.NotNil(t, parsed.Perfdata[0].Crit.High)
	assert.Equal(t, 500.0, *parsed.Perfdata[0].Crit.High)
	assert.Equal(t, "load1", parsed.Perfdata[1].Label)
	assert.Equal(t, 0.12, parsed.Perfdata[1].Value)
}

func TestParseRangeVariants(t *testing.T) {
	tests := map[string]struct {
		input     string
		expectNil bool
		low       *float64
		high      *float64
		inclusive bool
	}{
		"simple":          {input: "10", low: floatPtr(0), high: floatPtr(10)},
		"range":           {input: "10:20", low: floatPtr(10), high: floatPtr(20)},
		"inclusive":       {input: "@5:15", low: floatPtr(5), high: floatPtr(15), inclusive: true},
		"lower_unbounded": {input: "~:5", low: nil, high: floatPtr(5)},
		"upper_unbounded": {input: "10:", low: floatPtr(10), high: nil},
		"default_low":     {input: ":30", low: floatPtr(0), high: floatPtr(30)},
		"unknown":         {input: "U", expectNil: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rng := parseRange(tc.input)
			if tc.expectNil {
				assert.Nil(t, rng)
				return
			}
			require.NotNil(t, rng)
			if tc.low == nil {
				assert.Nil(t, rng.Low)
			} else {
				require.NotNil(t, rng.Low)
				assert.Equal(t, *tc.low, *rng.Low)
			}
			if tc.high == nil {
				assert.Nil(t, rng.High)
			} else {
				require.NotNil(t, rng.High)
				assert.Equal(t, *tc.high, *rng.High)
			}
			assert.Equal(t, tc.inclusive, rng.Inclusive)
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
	assert.Equal(t, expected, parsed.LongOutput())
}
