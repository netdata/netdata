// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/require"
)

func testProcessDiagnostics() jobmgr.DiagnosticObserver {
	return newDiagnosticLogger(io.Discard)
}

type recordingCompositionDiagnosticObserver struct {
	mu     sync.Mutex
	events []jobmgr.DiagnosticEvent
}

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

func TestDiagnosticLoggerSequencesOperationalEvents(t *testing.T) {
	var output bytes.Buffer
	diagnostics := newDiagnosticLogger(&output)
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
}
