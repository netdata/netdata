// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/require"
)

func testProcessDiagnostics() jobmgr.DiagnosticObserver {
	return newDiagnosticLogger(io.Discard, false)
}

type recordingCompositionDiagnosticObserver struct {
	mu     sync.Mutex
	events []jobmgr.DiagnosticEvent
}

func (*recordingCompositionDiagnosticObserver) TraceEnabled() bool { return true }

func (rcdo *recordingCompositionDiagnosticObserver) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	rcdo.mu.Lock()
	defer rcdo.mu.Unlock()
	rcdo.events = append(rcdo.events, event)
}

func (rcdo *recordingCompositionDiagnosticObserver) snapshot() []jobmgr.DiagnosticEvent {
	rcdo.mu.Lock()
	defer rcdo.mu.Unlock()
	return slices.Clone(rcdo.events)
}

func TestJobManagerTraceBuildFlag(t *testing.T) {
	original := jobManagerTrace
	t.Cleanup(func() { jobManagerTrace = original })
	tests := map[string]struct {
		value string
		want  bool
	}{
		"unset":   {},
		"enabled": {value: jobManagerTraceEnabledValue, want: true},
		"other":   {value: "true"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			jobManagerTrace = test.value
			require.Equal(t, test.want, jobManagerTraceEnabled())
		})
	}
}

func TestDiagnosticLoggerDropsDisabledTraceAndSequencesOperationalEvents(t *testing.T) {
	var output bytes.Buffer
	diagnostics := newDiagnosticLogger(&output, false)
	diagnostics.ObserveDiagnostic(jobmgr.DiagnosticEvent{
		Level: jobmgr.DiagnosticTrace,
		Name:  "developer trace",
	})
	diagnostics.ObserveDiagnostic(jobmgr.DiagnosticEvent{
		Level:      jobmgr.DiagnosticInfo,
		Name:       "first operational event",
		Generation: 7,
	})
	diagnostics.ObserveDiagnostic(jobmgr.DiagnosticEvent{
		Level:        jobmgr.DiagnosticWarning,
		Name:         "second operational event",
		Command:      "update",
		ResultStatus: 422,
	})

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 2)
	require.Contains(t, lines[0], "msg=\"first operational event\"")
	require.Contains(t, lines[0], "component=\"job manager\"")
	require.Contains(t, lines[0], "event_sequence=1")
	require.Contains(t, lines[0], "run_generation=7")
	require.Contains(t, lines[1], "msg=\"second operational event\"")
	require.Contains(t, lines[1], "event_sequence=2")
	require.Contains(t, lines[1], "command=update")
	require.Contains(t, lines[1], "result_status=422")
	require.NotContains(t, output.String(), "developer trace")
}

func TestProcessDiagnosticLoggerEnablesDeveloperTraceAndGlobalDebug(t *testing.T) {
	original := jobManagerTrace
	jobManagerTrace = jobManagerTraceEnabledValue
	logger.Level.Set(slog.LevelInfo)
	t.Cleanup(func() {
		jobManagerTrace = original
		logger.Level.Set(slog.LevelInfo)
	})

	diagnostics := newProcessDiagnosticLogger()
	require.True(t, diagnostics.TraceEnabled())
	require.True(t, logger.Level.Enabled(slog.LevelDebug))

	var output bytes.Buffer
	diagnostics = newDiagnosticLogger(&output, true)
	diagnostics.ObserveDiagnostic(jobmgr.DiagnosticEvent{
		Level: jobmgr.DiagnosticTrace,
		Name:  "developer trace",
	})
	require.Contains(t, output.String(), "level=debug")
	require.Contains(t, output.String(), "msg=\"developer trace\"")
	require.Contains(t, output.String(), "trace=true")
}

func TestDeveloperBuildScriptMakesJobManagerTraceOptIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("developer build script requires Bash")
	}

	path := filepath.Join("..", "..", "..", "go.d", "hack", "go-build.sh")
	fakeGoDir := t.TempDir()
	fakeGo := filepath.Join(fakeGoDir, "go")
	err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >\"$TRACE_ARGS\"\n"), 0o755)
	require.NoError(t, err)
	traceLinkFlag := "-X github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition.jobManagerTrace=enabled"
	tests := map[string]struct {
		args      []string
		wantTrace bool
	}{
		"default":               {args: []string{"darwin/arm64"}},
		"flag before target":    {args: []string{"--with-traces", "darwin/arm64"}, wantTrace: true},
		"flag following target": {args: []string{"darwin/arm64", "--with-traces"}, wantTrace: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			traceArgs := filepath.Join(t.TempDir(), "go-args")
			command := exec.Command("bash", append([]string{path}, test.args...)...)
			command.Env = buildScriptTestEnvironment(fakeGoDir, traceArgs)
			output, err := command.CombinedOutput()
			require.NoError(t, err, string(output))
			payload, err := os.ReadFile(traceArgs)
			require.NoError(t, err)
			if test.wantTrace {
				require.Contains(t, string(payload), traceLinkFlag)
			} else {
				require.NotContains(t, string(payload), traceLinkFlag)
			}
		})
	}
}

func buildScriptTestEnvironment(fakeGoDir, traceArgs string) []string {
	environment := make([]string, 0, len(os.Environ())+4)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, "PATH=") ||
			strings.HasPrefix(value, "TRACE_ARGS=") ||
			strings.HasPrefix(value, "TRAVIS_TAG=") ||
			strings.HasPrefix(value, "GLDFLAGS=") {
			continue
		}
		environment = append(environment, value)
	}
	return append(
		environment,
		"PATH="+fakeGoDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"TRACE_ARGS="+traceArgs,
		"TRAVIS_TAG=test",
		"GLDFLAGS=",
	)
}
