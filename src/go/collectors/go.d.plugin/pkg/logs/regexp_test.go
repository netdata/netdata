// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRegExpParser(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{name: "valid pattern", pattern: `(?P<A>\d+) (?P<B>\d+)`},
		{name: "no names subgroups in pattern", pattern: `(?:\d+) (?:\d+)`, wantErr: true},
		{name: "invalid pattern", pattern: `(((?P<A>\d+) (?P<B>\d+)`, wantErr: true},
		{name: "empty pattern", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewRegExpParser(RegExpConfig{Pattern: tt.pattern}, nil)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, p)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

func TestRegExpParser_ReadLine(t *testing.T) {
	tests := []struct {
		name         string
		row          string
		pattern      string
		wantErr      bool
		wantParseErr bool
	}{
		{name: "match and no error", row: "1 2", pattern: `(?P<A>\d+) (?P<B>\d+)`},
		{name: "match but error on assigning", row: "1 2", pattern: `(?P<A>\d+) (?P<ERR>\d+)`, wantErr: true, wantParseErr: true},
		{name: "not match", row: "A B", pattern: `(?P<A>\d+) (?P<B>\d+)`, wantErr: true, wantParseErr: true},
		{name: "not match multiline", row: "a b\n3 4", pattern: `(?P<A>\d+) (?P<B>\d+)`, wantErr: true, wantParseErr: true},
		{name: "error on reading EOF", row: "", pattern: `(?P<A>\d+) (?P<B>\d+)`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var line logLine
			r := strings.NewReader(tt.row)
			p, err := NewRegExpParser(RegExpConfig{Pattern: tt.pattern}, r)
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

func TestRegExpParser_Parse(t *testing.T) {
	tests := []struct {
		name    string
		row     string
		pattern string
		wantErr bool
	}{
		{name: "match and no error", row: "1 2", pattern: `(?P<A>\d+) (?P<B>\d+)`},
		{name: "match but error on assigning", row: "1 2", pattern: `(?P<A>\d+) (?P<ERR>\d+)`, wantErr: true},
		{name: "not match", row: "A B", pattern: `(?P<A>\d+) (?P<B>\d+)`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var line logLine
			p, err := NewRegExpParser(RegExpConfig{Pattern: tt.pattern}, nil)
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

func TestRegExpParser_Info(t *testing.T) {
	p, err := NewRegExpParser(RegExpConfig{Pattern: `(?P<A>\d+) (?P<B>\d+)`}, nil)
	require.NoError(t, err)
	assert.NotZero(t, p.Info())
}

type logLine struct {
	assigned map[string]string
}

func newLogLine() *logLine {
	return &logLine{
		assigned: make(map[string]string),
	}
}

func (l *logLine) Assign(name, val string) error {
	switch name {
	case "$ERR", "ERR":
		return errors.New("assign error")
	}
	if l.assigned != nil {
		l.assigned[name] = val
	}
	return nil
}
