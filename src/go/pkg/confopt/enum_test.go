// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type colorSpec struct{}

func (colorSpec) Values() []string { return []string{"red", "green", "blue"} }
func (colorSpec) Default() string  { return "green" }

type color = Enum[colorSpec]

type colorConfig struct {
	Color color `yaml:"color" json:"color"`
}

func TestEnumDecode(t *testing.T) {
	tests := map[string]struct {
		unmarshal     func([]byte, any) error
		input         string
		want          colorConfig
		wantEffective color
		wantErr       bool
	}{
		"json empty":     {unmarshal: json.Unmarshal, input: `{"color":""}`, want: colorConfig{"green"}, wantEffective: "green"},
		"json null":      {unmarshal: json.Unmarshal, input: `{"color":null}`, want: colorConfig{"green"}, wantEffective: "green"},
		"json omitted":   {unmarshal: json.Unmarshal, input: `{}`, want: colorConfig{""}, wantEffective: "green"},
		"json value":     {unmarshal: json.Unmarshal, input: `{"color":"blue"}`, want: colorConfig{"blue"}, wantEffective: "blue"},
		"json unknown":   {unmarshal: json.Unmarshal, input: `{"color":"Blue"}`, want: colorConfig{"Blue"}, wantEffective: "Blue"},
		"json no string": {unmarshal: json.Unmarshal, input: `{"color":1}`, wantErr: true},
		"yaml empty":     {unmarshal: yaml.Unmarshal, input: `color: ""`, want: colorConfig{"green"}, wantEffective: "green"},
		"yaml omitted":   {unmarshal: yaml.Unmarshal, input: `{}`, want: colorConfig{""}, wantEffective: "green"},
		"yaml value":     {unmarshal: yaml.Unmarshal, input: `color: red`, want: colorConfig{"red"}, wantEffective: "red"},
		"yaml no string": {unmarshal: yaml.Unmarshal, input: `color: [red]`, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var got colorConfig
			err := tc.unmarshal([]byte(tc.input), &got)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantEffective, got.Color.Normalized())
		})
	}
}

func TestEnumEncode(t *testing.T) {
	tests := map[string]struct {
		value    color
		wantJSON string
		wantYAML string
	}{
		"empty writes the default": {value: "", wantJSON: `{"color":"green"}`, wantYAML: "color: green\n"},
		"value":                    {value: "red", wantJSON: `{"color":"red"}`, wantYAML: "color: red\n"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			bs, err := json.Marshal(colorConfig{tc.value})
			require.NoError(t, err)
			assert.Equal(t, tc.wantJSON, string(bs))
			bs, err = yaml.Marshal(colorConfig{tc.value})
			require.NoError(t, err)
			assert.Equal(t, tc.wantYAML, string(bs))
		})
	}
}

func TestEnumValidate(t *testing.T) {
	tests := map[string]struct {
		value   color
		wantErr string
	}{
		"empty is the default": {value: ""},
		"allowed value":        {value: "blue"},
		"no case folding":      {value: "Blue", wantErr: `must be "red", "green" or "blue", got "Blue"`},
		"no trimming":          {value: " red", wantErr: `must be "red", "green" or "blue", got " red"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.value.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.wantErr)
			}
		})
	}
}
