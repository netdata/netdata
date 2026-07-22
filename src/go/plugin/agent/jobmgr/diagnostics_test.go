// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"fmt"
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
	trace  bool
	events []DiagnosticEvent
}

func (rdo *recordingDiagnosticObserver) TraceEnabled() bool {
	return rdo != nil && rdo.trace
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

func TestDiagnosticEmissionHonorsTraceAndOperationalLevels(t *testing.T) {
	tests := map[string]struct {
		trace bool
		emit  func(DiagnosticObserver)
		want  DiagnosticLevel
	}{
		"disabled trace is dropped": {
			emit: func(observer DiagnosticObserver) {
				TraceDiagnostic(observer, DiagnosticEvent{
					Name: "trace",
				})
			},
		},
		"enabled trace is normalized": {
			trace: true,
			emit: func(observer DiagnosticObserver) {
				TraceDiagnostic(observer, DiagnosticEvent{
					Level: DiagnosticError,
					Name:  "trace",
				})
			},
			want: DiagnosticTrace,
		},
		"operational event is emitted": {
			emit: func(observer DiagnosticObserver) {
				ObserveDiagnostic(observer, DiagnosticEvent{
					Level: DiagnosticWarning,
					Name:  "warning",
				})
			},
			want: DiagnosticWarning,
		},
		"invalid operational level is dropped": {
			emit: func(observer DiagnosticObserver) {
				ObserveDiagnostic(observer, DiagnosticEvent{
					Level: DiagnosticTrace,
					Name:  "invalid",
				})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			observer := &recordingDiagnosticObserver{
				trace: test.trace,
			}
			test.emit(observer)
			events := observer.snapshot()
			if test.want == 0 {
				require.Empty(t, events)
				return
			}
			require.Len(t, events, 1)
			require.Equal(t, test.want, events[0].Level)
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

func TestCommandKernelTraceCoversRealOperationWithoutRequestContents(t *testing.T) {
	const (
		argumentSentinel = "diagnostic-argument-must-not-appear"
		payloadSentinel  = "diagnostic-payload-must-not-appear"
	)
	observer := &recordingDiagnosticObserver{
		trace: true,
	}
	kernel, run, uids, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	require.NoError(t, kernel.BindDiagnosticObserver(observer))
	require.NoError(t, run.OpenAdmission())
	startKernelLoop(t, kernel)

	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	require.NoError(t, kernel.SubmitPreparedAndWait(
		context.Background(),
		Request{
			UID:        "diagnostic-operation",
			LaneKey:    "diagnostic-lane",
			Route:      "internal/diagnostic",
			Args:       []string{argumentSentinel},
			Payload:    []byte(payloadSentinel),
			Source:     lifecycle.SourceJobManager,
			HasPayload: true,
		},
		WorkPlan{
			Work: frameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
				return result, nil
			}),
		},
	))
	functionTerminal := make(chan error, 1)
	require.NoError(t, kernel.submit(
		context.Background(),
		Request{
			UID:        "diagnostic-function",
			Route:      "diagnostic-function-route",
			Args:       []string{argumentSentinel},
			Payload:    []byte(payloadSentinel),
			Source:     lifecycle.SourceFunction,
			HasPayload: true,
		},
		functionTerminal,
	))
	select {
	case err := <-functionTerminal:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "diagnostic Function did not reach terminal")
	}
	kernel.Stop()
	require.NoError(t, kernel.Wait(context.Background()))
	closeUIDLedger(t, uids)

	events := observer.snapshot()
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	for _, expected := range []string{
		"request received",
		"operation admitted",
		"operation task enqueued",
		"operation task started",
		"operation disposed",
		"kernel shutdown started",
		"kernel run stopped",
	} {
		require.Contains(t, names, expected)
	}
	require.True(t, slices.ContainsFunc(events, func(event DiagnosticEvent) bool {
		return event.UID == "diagnostic-function" && event.Source == lifecycle.SourceFunction
	}))
	encoded := fmt.Sprintf("%+v", events)
	require.NotContains(t, encoded, argumentSentinel)
	require.NotContains(t, encoded, payloadSentinel)
}

func TestFramePoisonEmitsOperationalDiagnostic(t *testing.T) {
	writeErr := errors.New("diagnostic writer failed")
	observer := &recordingDiagnosticObserver{
		trace: true,
	}
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
