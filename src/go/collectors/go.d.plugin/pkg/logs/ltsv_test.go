// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"strings"
	"testing"

	"github.com/Wing924/ltsv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testLTSVConfig = LTSVConfig{
	FieldDelimiter: " ",
	ValueDelimiter: "=",
	Mapping:        map[string]string{"KEY": "key"},
}

func TestNewLTSVParser(t *testing.T) {
	tests := []struct {
		name    string
		config  LTSVConfig
		wantErr bool
	}{
		{name: "config", config: testLTSVConfig},
		{name: "empty config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewLTSVParser(tt.config, nil)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, p)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, p)
				if tt.config.FieldDelimiter == "" {
					assert.Equal(t, ltsv.DefaultParser.FieldDelimiter, p.parser.FieldDelimiter)
				} else {
					assert.Equal(t, tt.config.FieldDelimiter, string(p.parser.FieldDelimiter))
				}
				if tt.config.ValueDelimiter == "" {
					assert.Equal(t, ltsv.DefaultParser.ValueDelimiter, p.parser.ValueDelimiter)
				} else {
					assert.Equal(t, tt.config.ValueDelimiter, string(p.parser.ValueDelimiter))
				}
				assert.Equal(t, tt.config.Mapping, p.mapping)
			}
		})
	}
}

func TestLTSVParser_ReadLine(t *testing.T) {
	tests := []struct {
		name         string
		row          string
		wantErr      bool
		wantParseErr bool
	}{
		{name: "no error", row: "A=1 B=2 KEY=3"},
		{name: "error on parsing", row: "NO LABEL", wantErr: true, wantParseErr: true},
		{name: "error on assigning", row: "A=1 ERR=2", wantErr: true, wantParseErr: true},
		{name: "error on reading EOF", row: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var line logLine
			r := strings.NewReader(tt.row)
			p, err := NewLTSVParser(testLTSVConfig, r)
			require.NoError(t, err)

			err = p.ReadLine(&line)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantParseErr {
					assert.True(t, IsParseError(err))
				} else {
					assert.False(t, IsParseError(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLTSVParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		row     string
		wantErr bool
	}{
		{name: "no error", row: "A=1 B=2"},
		{name: "error on parsing", row: "NO LABEL", wantErr: true},
		{name: "error on assigning", row: "A=1 ERR=2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var line logLine
			p, err := NewLTSVParser(testLTSVConfig, nil)
			require.NoError(t, err)

			err = p.Parse([]byte(tt.row), &line)

			if tt.wantErr {
				require.Error(t, err)
				assert.True(t, IsParseError(err))
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLTSVParser_Info(t *testing.T) {
	p, err := NewLTSVParser(testLTSVConfig, nil)
	require.NoError(t, err)
	assert.NotZero(t, p.Info())
}
