// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type finalizingTestService struct {
	started, joined chan struct{}
	runRelease      <-chan struct{}
	finalize        func(context.Context) error
	calls           atomic.Int32
}

func (s *finalizingTestService) Run(ctx context.Context) {
	close(s.started)
	<-ctx.Done()
	if s.runRelease != nil {
		<-s.runRelease
	}
	close(s.joined)
}

func (s *finalizingTestService) Finalize(ctx context.Context) error {
	s.calls.Add(1)
	select {
	case <-s.joined:
	default:
		return errors.New("finalization overlapped Run")
	}
	return s.finalize(ctx)
}

func TestProcessServiceFinalization(t *testing.T) {
	for name, tc := range map[string]struct {
		stop                        string
		blockRun, blockFinal, panic bool
		failure                     bool
	}{
		"termination finalizes once":             {stop: "terminate"},
		"parent cancellation":                    {stop: "cancel"},
		"input quit":                             {stop: "quit"},
		"input EOF":                              {stop: "eof"},
		"failed finalizer is reported":           {stop: "terminate", failure: true},
		"panic is isolated":                      {stop: "terminate", panic: true},
		"blocked finalizer respects wait budget": {stop: "terminate", blockFinal: true},
		"unjoined Run is never finalized":        {stop: "terminate", blockRun: true},
	} {
		t.Run(name, func(t *testing.T) {
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			release := make(chan struct{})
			finalDone := make(chan struct{})
			service := &finalizingTestService{started: make(chan struct{}), joined: make(chan struct{})}
			if tc.blockRun {
				service.runRelease = release
			}
			injected := errors.New("finalization failure")
			service.finalize = func(ctx context.Context) error {
				defer close(finalDone)
				if _, ok := ctx.Deadline(); !ok || ctx.Err() != nil {
					return errors.New("missing live shutdown budget")
				}
				if tc.blockFinal {
					<-release
				}
				if tc.panic {
					panic("injected finalizer panic")
				}
				if tc.failure {
					return injected
				}
				return nil
			}
			// A stalled service must not prevent an independent service's finalizer.
			healthyDone := make(chan struct{})
			healthy := &finalizingTestService{started: make(chan struct{}), joined: make(chan struct{}), finalize: func(context.Context) error {
				close(healthyDone)
				return nil
			}}
			config := testProductionProcessConfig(reader, io.Discard)
			config.ShutdownTimeout = 100 * time.Millisecond
			config.Services = []ProcessService{service, healthy}
			process, err := NewProcess(config)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- process.Run(ctx) }()
			<-service.started
			<-healthy.started
			require.NoError(t, process.Restart(t.Context()))
			require.Zero(t, service.calls.Load(), "restart must preserve the process service")
			switch tc.stop {
			case "terminate":
				go func() { _ = process.Terminate(t.Context()) }()
			case "cancel":
				cancel()
			case "quit":
				_, err = io.WriteString(writer, "QUIT\n")
				require.NoError(t, err)
			case "eof":
				require.NoError(t, writer.Close())
			}
			select {
			case err = <-done:
			case <-time.After(3 * time.Second):
				t.Error("process exceeded the service finalization wait budget")
			}
			close(release)
			<-service.joined
			if tc.blockRun {
				assert.Zero(t, service.calls.Load())
			} else {
				<-finalDone
				assert.EqualValues(t, 1, service.calls.Load())
			}
			<-healthyDone
			assert.EqualValues(t, 1, healthy.calls.Load())
			switch {
			case tc.blockRun || tc.blockFinal:
				assert.ErrorIs(t, err, context.DeadlineExceeded)
			case tc.failure:
				assert.ErrorIs(t, err, injected)
			case tc.panic:
				assert.ErrorContains(t, err, "injected finalizer panic")
			case tc.stop == "eof":
				assert.ErrorContains(t, err, "Function input stopped")
			case tc.stop != "cancel":
				assert.NoError(t, err)
			}
		})
	}
}

func TestProcessServiceFinalizationRetainsReadyErrors(t *testing.T) {
	for name, tc := range map[string]struct{ blockRun bool }{
		"earlier Run stalls":       {blockRun: true},
		"earlier finalizer stalls": {},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				reader, writer := io.Pipe()
				defer reader.Close()
				defer writer.Close()
				release := make(chan struct{})
				defer close(release)
				stalled := &finalizingTestService{started: make(chan struct{}), joined: make(chan struct{}), finalize: func(context.Context) error {
					<-release
					return nil
				}}
				if tc.blockRun {
					stalled.runRelease = release
				}
				injected := errors.New("independent finalization failure")
				failed := &finalizingTestService{started: make(chan struct{}), joined: make(chan struct{}), finalize: func(context.Context) error {
					return injected
				}}
				config := testProductionProcessConfig(reader, io.Discard)
				config.ShutdownTimeout = time.Second
				config.Services = []ProcessService{stalled, failed}
				process, err := NewProcess(config)
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- process.Run(ctx) }()
				<-stalled.started
				<-failed.started
				cancel()
				synctest.Wait()
				// All runnable finalizers finish before advancing the shared deadline.
				require.EqualValues(t, 1, failed.calls.Load())
				time.Sleep(config.ShutdownTimeout)
				synctest.Wait()
				select {
				case err := <-done:
					assert.ErrorIs(t, err, context.DeadlineExceeded)
					assert.ErrorIs(t, err, injected)
				default:
					t.Fatal("process exceeded its shutdown budget")
				}
			})
		})
	}
}
