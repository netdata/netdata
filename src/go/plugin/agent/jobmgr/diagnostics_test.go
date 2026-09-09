// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

type recordingDiagnosticObserver struct {
	mu     sync.Mutex
	events []DiagnosticEvent
}

func (rdo *recordingDiagnosticObserver) ObserveDiagnostic(event DiagnosticEvent) {
	rdo.mu.Lock()
	defer rdo.mu.Unlock()
	rdo.events = append(rdo.events, event)
}

func (rdo *recordingDiagnosticObserver) snapshot() []DiagnosticEvent {
	rdo.mu.Lock()
	defer rdo.mu.Unlock()
	return slices.Clone(rdo.events)
}

func TestDiagnosticEmissionAcceptsOnlyOperationalLevels(t *testing.T) {
	tests := map[string]struct {
		level DiagnosticLevel
		want  bool
	}{
		"unset":   {},
		"info":    {level: DiagnosticInfo, want: true},
		"warning": {level: DiagnosticWarning, want: true},
		"error":   {level: DiagnosticError, want: true},
		"unknown": {level: DiagnosticLevel(99)},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			observer := &recordingDiagnosticObserver{}
			ObserveDiagnostic(observer, DiagnosticEvent{
				Level: test.level,
				Name:  name,
			})
			events := observer.snapshot()
			if !test.want {
				require.Empty(t, events)
				return
			}
			require.Len(t, events, 1)
			require.Equal(t, test.level, events[0].Level)
		})
	}
}

func TestDiagnosticResultSucceeded(t *testing.T) {
	tests := map[string]struct {
		status int
		want   bool
	}{
		"zero":                   {},
		"informational":          {status: 199},
		"successful lower bound": {status: 200, want: true},
		"successful upper bound": {status: 299, want: true},
		"redirect":               {status: 300},
		"client error":           {status: 422},
		"server error":           {status: 500},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, DiagnosticResultSucceeded(test.status))
		})
	}
}

func TestFramePoisonEmitsOperationalDiagnostic(t *testing.T) {
	writeErr := errors.New("diagnostic writer failed")
	observer := &recordingDiagnosticObserver{}
	kernel, run, uids, _ := newKernelWithPlannerWriterAndTimeout(
		t,
		stoppedKernelPlanner{},
		diagnosticErrorWriter{
			err: writeErr,
		},
		50*time.Millisecond,
	)
	require.NoError(t, kernel.BindDiagnosticObserver(observer))
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)

	terminal := make(chan error, 1)
	require.NoError(t, kernel.submit(
		context.Background(),
		Request{
			UID:     "diagnostic-frame-failure",
			LaneKey: "diagnostic-frame-failure",
			Route:   "internal/diagnostic-frame-failure",
			Source:  lifecycle.SourceJobManager,
		},
		terminal,
	))
	select {
	case <-terminal:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "frame-failure operation did not reach terminal")
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.ErrorIs(t, kernel.Wait(waitCtx), writeErr)
	closeUIDLedger(t, uids)

	require.True(t, slices.ContainsFunc(observer.snapshot(), func(event DiagnosticEvent) bool {
		return event.Name == "job manager frame owner poisoned" &&
			event.Level == DiagnosticError &&
			errors.Is(event.Err, lifecycle.ErrFrameOwnerPoisoned) &&
			errors.Is(event.Err, writeErr)
	}))
}

type diagnosticErrorWriter struct {
	err error
}

func (dew diagnosticErrorWriter) Write([]byte) (int, error) {
	return 0, errors.Join(io.ErrShortWrite, dew.err)
}
