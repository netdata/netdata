// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJSONParser(t *testing.T) {
	tests := map[string]struct {
		config  JSONConfig
		wantErr bool
	}{
		"empty config": {
			config:  JSONConfig{},
			wantErr: false,
		},
		"with mappings": {
			config:  JSONConfig{Mapping: map[string]string{"from_field_1": "to_field_1"}},
			wantErr: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, err := NewJSONParser(test.config, nil)

			if test.wantErr {
				assert.Error(t, err)
				assert.Nil(t, p)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

func TestJSONParser_ReadLine(t *testing.T) {
	tests := map[string]struct {
		config       JSONConfig
		input        string
		wantAssigned map[string]string
		wantErr      bool
	}{
		"string value": {
			input:   `{ "string": "example.com" }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"string": "example.com",
			},
		},
		"int value": {
			input:   `{ "int": 1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"int": "1",
			},
		},
		"float value": {
			input:   `{ "float": 1.1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"float": "1.1",
			},
		},
		"string, int, float values": {
			input:   `{ "string": "example.com", "int": 1, "float": 1.1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"string": "example.com",
				"int":    "1",
				"float":  "1.1",
			},
		},
		"string, int, float values with mappings": {
			config: JSONConfig{Mapping: map[string]string{
				"string": "STRING",
				"int":    "INT",
				"float":  "FLOAT",
			}},
			input:   `{ "string": "example.com", "int": 1, "float": 1.1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"STRING": "example.com",
				"INT":    "1",
				"FLOAT":  "1.1",
			},
		},
		"nested": {
			input: `{"one":{"two":2,"three":{"four":4}},"five":5}`,
			config: JSONConfig{Mapping: map[string]string{
				"one.two": "mapped_value",
			}},
			wantErr: false,
			wantAssigned: map[string]string{
				"mapped_value":   "2",
				"one.three.four": "4",
				"five":           "5",
			},
		},
		"nested with array": {
			input: `{"one":{"two":[2,22]},"five":5}`,
			config: JSONConfig{Mapping: map[string]string{
				"one.two.1": "mapped_value",
			}},
			wantErr: false,
			wantAssigned: map[string]string{
				"one.two.0":    "2",
				"mapped_value": "22",
				"five":         "5",
			},
		},
		"error on malformed JSON": {
			input:   `{ "host"": unquoted_string}`,
			wantErr: true,
		},
		"error on empty input": {
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			line := newLogLine()
			in := strings.NewReader(test.input)
			p, err := NewJSONParser(test.config, in)
			require.NoError(t, err)
			require.NotNil(t, p)

			err = p.ReadLine(line)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.wantAssigned, line.assigned)
			}
		})
	}
}

func TestJSONParser_Parse(t *testing.T) {
	tests := map[string]struct {
		config       JSONConfig
		input        string
		wantAssigned map[string]string
		wantErr      bool
	}{
		"string value": {
			input:   `{ "string": "example.com" }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"string": "example.com",
			},
		},
		"int value": {
			input:   `{ "int": 1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"int": "1",
			},
		},
		"float value": {
			input:   `{ "float": 1.1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"float": "1.1",
			},
		},
		"string, int, float values": {
			input:   `{ "string": "example.com", "int": 1, "float": 1.1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"string": "example.com",
				"int":    "1",
				"float":  "1.1",
			},
		},
		"string, int, float values with mappings": {
			config: JSONConfig{Mapping: map[string]string{
				"string": "STRING",
				"int":    "INT",
				"float":  "FLOAT",
			}},
			input:   `{ "string": "example.com", "int": 1, "float": 1.1 }`,
			wantErr: false,
			wantAssigned: map[string]string{
				"STRING": "example.com",
				"INT":    "1",
				"FLOAT":  "1.1",
			},
		},
		"error on malformed JSON": {
			input:   `{ "host"": unquoted_string}`,
			wantErr: true,
		},
		"error on empty input": {
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			line := newLogLine()
			p, err := NewJSONParser(test.config, nil)
			require.NoError(t, err)
			require.NotNil(t, p)

			err = p.Parse([]byte(test.input), line)

			if test.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.wantAssigned, line.assigned)
			}
		})
	}
}
