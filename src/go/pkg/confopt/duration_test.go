// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v2"
)

func TestParseDuration(t *testing.T) {
	tests := map[string]struct {
		input        string
		wantDuration time.Duration
		wantErr      bool
	}{
		"nanoseconds":    {input: "10ns", wantDuration: 10 * time.Nanosecond},
		"microseconds":   {input: "10us", wantDuration: 10 * time.Microsecond},
		"milliseconds":   {input: "10ms", wantDuration: 10 * time.Millisecond},
		"seconds":        {input: "10s", wantDuration: 10 * time.Second},
		"minutes":        {input: "10m", wantDuration: 10 * time.Minute},
		"hours":          {input: "10h", wantDuration: 10 * time.Hour},
		"days":           {input: "10d", wantDuration: 10 * 24 * time.Hour},
		"weeks (w)":      {input: "10w", wantDuration: 10 * 7 * 24 * time.Hour},
		"weeks (wk)":     {input: "10wk", wantDuration: 10 * 7 * 24 * time.Hour},
		"months (mo)":    {input: "10mo", wantDuration: 10 * 30 * 24 * time.Hour},
		"months (M)":     {input: "10M", wantDuration: 10 * 30 * 24 * time.Hour},
		"years":          {input: "10y", wantDuration: 10 * 365 * 24 * time.Hour},
		"negative units": {input: "-10d", wantDuration: -10 * 24 * time.Hour},
		"mixed units": {
			input: "1y2M3w4d5h6m7s8ms9us10ns",
			wantDuration: (1 * 365 * 24 * time.Hour) +
				(2 * 30 * 24 * time.Hour) +
				(3 * 7 * 24 * time.Hour) +
				(4 * 24 * time.Hour) +
				(5 * time.Hour) +
				(6 * time.Minute) +
				(7 * time.Second) +
				(8 * time.Millisecond) +
				(9 * time.Microsecond) +
				(10 * time.Nanosecond),
		},
		"mixed units with spaces": {
			input: "1y 2M 3w 4d 5h 6m 7s 8ms 9us 10ns",
			wantDuration: (1 * 365 * 24 * time.Hour) +
				(2 * 30 * 24 * time.Hour) +
				(3 * 7 * 24 * time.Hour) +
				(4 * 24 * time.Hour) +
				(5 * time.Hour) +
				(6 * time.Minute) +
				(7 * time.Second) +
				(8 * time.Millisecond) +
				(9 * time.Microsecond) +
				(10 * time.Nanosecond),
		},
		"mixed units with decimals": {
			input: "1.5y2.25M3.75w4.5d5.5h6.5m7.5s8.5ms9.5us10.5ns",
			wantDuration: time.Duration(math.Floor(1.5*365*24*float64(time.Hour))) +
				time.Duration(math.Floor(2.25*30*24*float64(time.Hour))) +
				time.Duration(math.Floor(3.75*7*24*float64(time.Hour))) +
				time.Duration(math.Floor(4.5*24*float64(time.Hour))) +
				time.Duration(math.Floor(5.5*float64(time.Hour))) +
				time.Duration(math.Floor(6.5*float64(time.Minute))) +
				time.Duration(math.Floor(7.5*float64(time.Second))) +
				time.Duration(math.Floor(8.5*float64(time.Millisecond))) +
				time.Duration(math.Floor(9.5*float64(time.Microsecond))) +
				time.Duration(math.Floor(10.5*float64(time.Nanosecond))),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dur, err := ParseDuration(test.input)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.wantDuration, dur)
			}
		})
	}
}

func TestDuration_MarshalYAML(t *testing.T) {
	tests := map[string]struct {
		d    Duration
		want string
	}{
		"1 second":    {d: Duration(time.Second), want: "1"},
		"1.5 seconds": {d: Duration(time.Second + time.Millisecond*500), want: "1.5"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			bs, err := yaml.Marshal(&test.d)
			require.NoError(t, err)

			assert.Equal(t, test.want, strings.TrimSpace(string(bs)))
		})
	}
}

func TestDuration_MarshalJSON(t *testing.T) {
	tests := map[string]struct {
		d    Duration
		want string
	}{
		"1 second":    {d: Duration(time.Second), want: "1"},
		"1.5 seconds": {d: Duration(time.Second + time.Millisecond*500), want: "1.5"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			bs, err := json.Marshal(&test.d)
			require.NoError(t, err)

			assert.Equal(t, test.want, strings.TrimSpace(string(bs)))
		})
	}
}

func TestDuration_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input any
	}{
		"duration":     {input: "300ms"},
		"string int":   {input: "1"},
		"string float": {input: "1.1"},
		"int":          {input: 2},
		"float":        {input: 2.2},
	}

	var zero Duration

	for name, test := range tests {
		name = fmt.Sprintf("%s (%v)", name, test.input)
		t.Run(name, func(t *testing.T) {
			data, err := yaml.Marshal(test.input)
			require.NoError(t, err)

			var d Duration
			require.NoError(t, yaml.Unmarshal(data, &d))
			assert.NotEqual(t, zero.String(), d.String())
		})
	}
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// JSON numbers (interpreted as seconds)
		"json number int": {
			input:    `{"d": 30}`,
			expected: 30 * time.Second,
		},
		"json number float": {
			input:    `{"d": 1.5}`,
			expected: 1500 * time.Millisecond,
		},
		"json number zero": {
			input:    `{"d": 0}`,
			expected: 0,
		},

		// JSON strings with duration format
		"json string seconds": {
			input:    `{"d": "30s"}`,
			expected: 30 * time.Second,
		},
		"json string minutes": {
			input:    `{"d": "5m"}`,
			expected: 5 * time.Minute,
		},
		"json string hours": {
			input:    `{"d": "2h"}`,
			expected: 2 * time.Hour,
		},
		"json string milliseconds": {
			input:    `{"d": "500ms"}`,
			expected: 500 * time.Millisecond,
		},
		"json string combined": {
			input:    `{"d": "1h30m"}`,
			expected: 90 * time.Minute,
		},
		"json string days": {
			input:    `{"d": "1d"}`,
			expected: 24 * time.Hour,
		},
		"json string weeks": {
			input:    `{"d": "1w"}`,
			expected: 7 * 24 * time.Hour,
		},

		// JSON strings with numeric values (interpreted as seconds)
		"json string numeric int": {
			input:    `{"d": "30"}`,
			expected: 30 * time.Second,
		},
		"json string numeric float": {
			input:    `{"d": "1.5"}`,
			expected: 1500 * time.Millisecond,
		},

		// JSON null (results in zero value, not an error)
		"json null": {
			input:    `{"d": null}`,
			expected: 0,
		},

		// Errors
		"json string invalid": {
			input:   `{"d": "invalid"}`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var result struct {
				D Duration `json:"d"`
			}

			err := json.Unmarshal([]byte(tc.input), &result)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result.D.Duration())
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := map[string]struct {
		d    time.Duration
		want string
	}{
		"zero":             {d: 0, want: "0s"},
		"1 millisecond":    {d: time.Millisecond, want: "1ms"},
		"500 milliseconds": {d: 500 * time.Millisecond, want: "500ms"},
		"1 second":         {d: time.Second, want: "1s"},
		"30 seconds":       {d: 30 * time.Second, want: "30s"},
		"1 minute":         {d: time.Minute, want: "1m"},
		"5 minutes":        {d: 5 * time.Minute, want: "5m"},
		"30 minutes":       {d: 30 * time.Minute, want: "30m"},
		"1 hour":           {d: time.Hour, want: "1h"},
		"12 hours":         {d: 12 * time.Hour, want: "12h"},
		"1 day":            {d: 24 * time.Hour, want: "1d"},
		"7 days":           {d: 7 * 24 * time.Hour, want: "1w"},
		"14 days":          {d: 14 * 24 * time.Hour, want: "2w"},
		"30 days":          {d: 30 * 24 * time.Hour, want: "1mo"},
		"365 days":         {d: 365 * 24 * time.Hour, want: "1y"},
		"2 years":          {d: 2 * 365 * 24 * time.Hour, want: "2y"},
		"negative 1 hour":  {d: -time.Hour, want: "-1h"},
		"negative 1 day":   {d: -24 * time.Hour, want: "-1d"},
		"1.5 seconds":      {d: 1500 * time.Millisecond, want: "1.5s"},
		"90 minutes":       {d: 90 * time.Minute, want: "90m"},
		"36 hours":         {d: 36 * time.Hour, want: "36h"},
		"sub-millisecond":  {d: 100 * time.Microsecond, want: "100µs"},
		"nanoseconds":      {d: 50 * time.Nanosecond, want: "50ns"},
		"negative sub-ms":  {d: -100 * time.Microsecond, want: "-100µs"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := formatDuration(tc.d)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDurationString_MarshalJSON(t *testing.T) {
	tests := map[string]struct {
		d    LongDuration
		want string
	}{
		"1 second":    {d: LongDuration(time.Second), want: `"1s"`},
		"30 seconds":  {d: LongDuration(30 * time.Second), want: `"30s"`},
		"2 minutes":   {d: LongDuration(2 * time.Minute), want: `"2m"`},
		"12 hours":    {d: LongDuration(12 * time.Hour), want: `"12h"`},
		"1 day":       {d: LongDuration(24 * time.Hour), want: `"1d"`},
		"1 week":      {d: LongDuration(7 * 24 * time.Hour), want: `"1w"`},
		"1 month":     {d: LongDuration(30 * 24 * time.Hour), want: `"1mo"`},
		"1 year":      {d: LongDuration(365 * 24 * time.Hour), want: `"1y"`},
		"1.5 seconds": {d: LongDuration(1500 * time.Millisecond), want: `"1.5s"`},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			bs, err := json.Marshal(&tc.d)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(bs))
		})
	}
}

func TestDurationString_MarshalYAML(t *testing.T) {
	tests := map[string]struct {
		d    LongDuration
		want string
	}{
		"1 second": {d: LongDuration(time.Second), want: "1s"},
		"12 hours": {d: LongDuration(12 * time.Hour), want: "12h"},
		"1 day":    {d: LongDuration(24 * time.Hour), want: "1d"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			bs, err := yaml.Marshal(&tc.d)
			require.NoError(t, err)
			assert.Equal(t, tc.want, strings.TrimSpace(string(bs)))
		})
	}
}

func TestDurationString_UnmarshalJSON(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		// JSON numbers (interpreted as seconds)
		"json number int": {
			input:    `{"d": 30}`,
			expected: 30 * time.Second,
		},
		"json number float": {
			input:    `{"d": 1.5}`,
			expected: 1500 * time.Millisecond,
		},

		// JSON strings with duration format
		"json string seconds": {
			input:    `{"d": "30s"}`,
			expected: 30 * time.Second,
		},
		"json string hours": {
			input:    `{"d": "12h"}`,
			expected: 12 * time.Hour,
		},
		"json string days": {
			input:    `{"d": "1d"}`,
			expected: 24 * time.Hour,
		},
		"json string weeks": {
			input:    `{"d": "1w"}`,
			expected: 7 * 24 * time.Hour,
		},
		"json string months": {
			input:    `{"d": "1mo"}`,
			expected: 30 * 24 * time.Hour,
		},
		"json string years": {
			input:    `{"d": "1y"}`,
			expected: 365 * 24 * time.Hour,
		},

		// JSON strings with numeric values (interpreted as seconds)
		"json string numeric": {
			input:    `{"d": "120"}`,
			expected: 120 * time.Second,
		},

		// Errors
		"json string invalid": {
			input:   `{"d": "invalid"}`,
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var result struct {
				D LongDuration `json:"d"`
			}

			err := json.Unmarshal([]byte(tc.input), &result)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result.D.Duration())
		})
	}
}

func TestDurationString_UnmarshalYAML(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected time.Duration
	}{
		"duration string": {input: "d: 12h", expected: 12 * time.Hour},
		"duration days":   {input: "d: 1d", expected: 24 * time.Hour},
		"numeric int":     {input: "d: 120", expected: 120 * time.Second},
		"numeric float":   {input: "d: 1.5", expected: 1500 * time.Millisecond},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var result struct {
				D LongDuration `yaml:"d"`
			}

			err := yaml.Unmarshal([]byte(tc.input), &result)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result.D.Duration())
		})
	}
}
