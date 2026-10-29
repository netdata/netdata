// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/logs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const diagnosticMarker = "SYNTHETIC_SOURCE_MARKER"

func TestAssignmentDiagnostics(t *testing.T) {
	for name, test := range map[string]struct {
		config logs.ParserConfig
		row    string
	}{
		"CSV": {config: logs.ParserConfig{
			LogType: logs.TypeCSV,
			CSV: logs.CSVConfig{
				CheckField: checkCSVFormatField,
				Format:     "$status",
			},
		}, row: diagnosticMarker},
		"regexp": {config: logs.ParserConfig{
			LogType: logs.TypeRegExp,
			RegExp: logs.RegExpConfig{
				Pattern: "(?P<status>.*)",
			},
		}, row: diagnosticMarker},
		"LTSV": {config: logs.ParserConfig{
			LogType: logs.TypeLTSV,
		}, row: "status:" + diagnosticMarker},
		"JSON": {config: logs.ParserConfig{
			LogType: logs.TypeJSON,
		}, row: `{"status":"` + diagnosticMarker + `"}`},
	} {
		t.Run(name, func(t *testing.T) {
			parser, err := logs.NewParser(test.config, strings.NewReader(""))
			require.NoError(t, err)
			line := newEmptyLogLine()
			err = parser.Parse([]byte(test.row), line)
			require.Error(t, err)
			assert.True(t, logs.IsParseError(err))
			assert.ErrorIs(t, err, errBadRespCode)
			assertSafeDiagnosticChain(t, err)
		})
	}
}

func TestVerificationDiagnostics(t *testing.T) {
	line := newEmptyLogLine()
	require.NoError(t, line.Assign("host", diagnosticMarker+" invalid"))
	err := line.verify()
	require.Error(t, err)
	assert.ErrorIs(t, err, errBadVhost)
	assertSafeDiagnosticChain(t, err)
}

func TestCollectorSourceDiagnostics(t *testing.T) {
	previous := slog.LevelDebug
	for !logger.Level.Enabled(previous) {
		previous++
	}
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(previous) })
	for name, test := range map[string]struct {
		kind                                string
		initial, appended, wantLog          string
		wantNil, wantUnmatched, rejectedDim bool
	}{
		"startup unused sample value": {
			kind:    logs.TypeJSON,
			initial: `{"status":200,"host":"example.test","ignored":"SYNTHETIC_SOURCE_MARKER"}` + "\n",
			wantLog: "starting parser creating",
		},
		"startup assignment": {
			kind:    logs.TypeJSON,
			initial: `{"status":"` + diagnosticMarker + `"}` + "\n" + `{"status":200,"host":"example.test"}` + "\n",
			wantLog: "failed to parse line",
		},
		"startup verification": {
			kind:    logs.TypeJSON,
			initial: `{"status":200,"host":"SYNTHETIC_SOURCE_MARKER invalid"}` + "\n" + `{"status":200,"host":"example.test"}` + "\n",
			wantLog: "failed to verify line",
		},
		"later assignment": {
			kind:          logs.TypeJSON,
			initial:       `{"status":200,"host":"example.test"}` + "\n",
			appended:      `{"status":"` + diagnosticMarker + `"}` + "\n",
			wantLog:       "unmatched line",
			wantUnmatched: true,
		},
		"later JSON syntax": {
			kind:     logs.TypeJSON,
			initial:  `{"status":200,"host":"example.test"}` + "\n",
			appended: `{"status":` + diagnosticMarker + `}` + "\n",
			wantLog:  "json parse",
			wantNil:  true,
		},
		"later LTSV syntax": {
			kind:          logs.TypeLTSV,
			initial:       "status:200\n",
			appended:      diagnosticMarker + "\n",
			wantLog:       "unmatched line",
			wantUnmatched: true,
		},
		"later rejected dimension": {
			kind:        logs.TypeJSON,
			initial:     `{"status":200,"host":"example.test"}` + "\n",
			appended:    `{"status":200,"host":"SYNTHETIC_SOURCE_MARKER invalid"}` + "\n",
			wantLog:     "level=warn",
			rejectedDim: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "access.log")
			require.NoError(t, os.WriteFile(path, []byte(test.initial), 0600))
			var output bytes.Buffer
			collector := New()
			collector.Logger = logger.NewWithWriter(&output)
			collector.Path = path
			collector.ParserConfig.LogType = test.kind
			t.Cleanup(func() { collector.Cleanup(context.Background()) })
			require.NoError(t, collector.Init(context.Background()))
			require.NoError(t, collector.Check(context.Background()))
			if test.appended != "" {
				output.Reset()
				file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
				require.NoError(t, err)
				_, err = file.WriteString(test.appended)
				require.NoError(t, err)
				require.NoError(t, file.Close())
				metrics := collector.Collect(context.Background())
				if test.wantNil {
					assert.Nil(t, metrics)
				} else {
					require.NotNil(t, metrics)
					assert.Equal(t, int64(1), metrics["requests"])
					if test.wantUnmatched {
						assert.Equal(t, int64(1), metrics["req_unmatched"])
					}
				}
				if test.rejectedDim {
					chart := collector.Charts().Get(reqByVhost.ID)
					require.NotNil(t, chart)
					assert.Nil(t, chart.GetDim("req_vhost_"+diagnosticMarker+" invalid"))
				}
			}
			assert.Contains(t, output.String(), test.wantLog)
			assert.NotContains(t, output.String(), diagnosticMarker)
		})
	}
}

func assertSafeDiagnosticChain(t *testing.T, err error) {
	t.Helper()
	for cause := err; cause != nil; cause = errors.Unwrap(cause) {
		assert.NotContains(t, cause.Error(), diagnosticMarker)
	}
}

func BenchmarkLogLineDiagnostics(b *testing.B) {
	for name, value := range map[string]string{"valid": "200", "invalid": diagnosticMarker} {
		b.Run(name, func(b *testing.B) {
			line := newEmptyLogLine()
			b.ReportAllocs()
			for b.Loop() {
				_ = line.Assign("status", value)
			}
		})
	}
}
