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
		input any
	}{
		"duration":     {input: "300ms"},
		"string int":   {input: "1"},
		"string float": {input: "1.1"},
		"int":          {input: 2},
		"float":        {input: 2.2},
	}

	var zero Duration

	type duration struct {
		D Duration `json:"d"`
	}
	type input struct {
		D any `json:"d"`
	}

	for name, test := range tests {
		name = fmt.Sprintf("%s (%v)", name, test.input)
		t.Run(name, func(t *testing.T) {
			input := input{D: test.input}
			data, err := yaml.Marshal(input)
			require.NoError(t, err)

			var d duration
			require.NoError(t, yaml.Unmarshal(data, &d))
			assert.NotEqual(t, zero.String(), d.D.String())
		})
	}
}
