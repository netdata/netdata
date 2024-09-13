// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/yaml.v2"
)

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
