// SPDX-License-Identifier: GPL-3.0-or-later

package logs

import (
	"errors"
	"strings"
	"testing"

	"github.com/Wing924/ltsv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserSyntaxDiagnostics(t *testing.T) {
	const marker = "SYNTHETIC_SOURCE_MARKER"
	tests := map[string]struct {
		kind       string
		row        string
		parseError bool
		cause      error
	}{
		"JSON":               {kind: TypeJSON, row: `{"status":` + marker + `}`},
		"LTSV missing label": {kind: TypeLTSV, row: marker, parseError: true, cause: ltsv.ErrMissingLabel},
		"LTSV empty label":   {kind: TypeLTSV, row: ":" + marker, parseError: true, cause: ltsv.ErrEmptyLabel},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			parser, err := NewParser(ParserConfig{
				LogType: test.kind,
			}, strings.NewReader(""))
			require.NoError(t, err)
			err = parser.Parse([]byte(test.row), newLogLine())
			require.Error(t, err)
			assert.Equal(t, test.parseError, IsParseError(err))
			if test.cause != nil {
				assert.ErrorIs(t, err, test.cause)
			}
			for cause := err; cause != nil; cause = errors.Unwrap(cause) {
				assert.NotContains(t, cause.Error(), marker)
			}
		})
	}
}

type benchmarkDiagnosticLine struct{}

func (benchmarkDiagnosticLine) Assign(string, string) error { return nil }

// These cases measure one parser call, including its diagnostic on failure.
// Successful calls retain no input and use no collector state.
func BenchmarkParserDiagnostics(b *testing.B) {
	for name, test := range map[string]struct {
		kind, row string
		wantError bool
	}{
		"JSON valid":   {kind: TypeJSON, row: `{"status":200,"host":"example.test"}`},
		"JSON invalid": {kind: TypeJSON, row: `{"status":SYNTHETIC_SOURCE_MARKER}`, wantError: true},
		"LTSV valid":   {kind: TypeLTSV, row: "status:200\thost:example.test"},
		"LTSV invalid": {kind: TypeLTSV, row: "SYNTHETIC_SOURCE_MARKER", wantError: true},
	} {
		b.Run(name, func(b *testing.B) {
			parser, err := NewParser(ParserConfig{
				LogType: test.kind,
			}, strings.NewReader(""))
			require.NoError(b, err)
			row := []byte(test.row)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				err := parser.Parse(row, benchmarkDiagnosticLine{})
				if (err != nil) != test.wantError {
					b.Fatal("unexpected parser result")
				}
			}
		})
	}
}
