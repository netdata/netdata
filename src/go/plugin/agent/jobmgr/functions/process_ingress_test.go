// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

func TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence(t *testing.T) {
	reader, writer := io.Pipe()
	ingress, err := NewProcessIngress(reader)
	require.NoError(t, err)
	first := newTestProcessInput(1)
	second := newTestProcessInput(2)

	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
		port: first,
	}))

	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()

	writeFunctionLine(t, writer, "first")
	firstCall := <-first.calls
	require.EqualValues(t, "first", firstCall.UID)

	require.NoError(t, ingress.SealPause())

	paused := make(chan error, 1)
	go func() { paused <- ingress.DrainPause(context.Background(), 2) }()
	select {
	case err := <-paused:
		require.FailNowf(t, "test failed", "pause bypassed an acquired delivery: %v", err)
	default:
	}
	close(first.release)

	require.NoError(t, <-paused)

	writeFunctionLine(t, writer, "carried")
	select {
	case call := <-first.calls:
		require.FailNowf(t, "test failed", "paused call reached old run: %+v", call)
	case call := <-second.calls:
		require.FailNowf(t, "test failed", "paused call reached successor before adopt: %+v", call)
	default:
	}

	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
		port: second,
	}))

	carriedCall := <-second.calls
	require.EqualValues(t, "carried", carriedCall.UID)
	close(second.release)
	writeFunctionLine(t, writer, "second")
	secondCall := <-second.calls
	require.EqualValues(t, "second", secondCall.UID)

	require.NoError(t, ingress.SealPause())
	require.NoError(t, ingress.DrainPause(context.Background(), 0))
	writeFunctionLine(t, writer, "fenced")
	select {
	case call := <-second.calls:
		require.FailNowf(t, "test failed", "fenced call reached retired run: %+v", call)
	default:
	}

	require.NoError(t, ingress.Fence(context.Background()))

	writeFunctionLine(t, writer, "contained")

	require.NoError(t, writer.Close())

	require.NoError(t, <-done)

	ingress.mu.Lock()
	state, readerStarted, deliveries := ingress.state, ingress.readerStarted, ingress.deliveries
	ingress.mu.Unlock()
	require.Equal(t, ProcessIngressContained, state)
	require.True(t, readerStarted)
	require.Zero(t, deliveries)

	require.Error(t, ingress.Run(context.Background()))
}

func TestProcessIngressCarriesPartialBodyAcrossAdopt(t *testing.T) {
	reader, writer := io.Pipe()
	ingress, err := NewProcessIngress(reader)
	require.NoError(t, err)
	first := newTestProcessInput(1)
	second := newTestProcessInput(2)
	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
		port: first,
	}))

	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()
	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD carried 30 \"test:work\" 0xFFFF \"source\" application/octet-stream\npartial\n",
	)
	require.NoError(t, err)

	require.NoError(t, ingress.SealPause())
	require.NoError(t, ingress.DrainPause(context.Background(), 2))
	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
		port: second,
	}))

	_, err = io.WriteString(writer, "successor\nFUNCTION_PAYLOAD_END\n")
	require.NoError(t, err)
	call := <-second.calls
	require.Equal(t, "carried", call.UID)
	require.Equal(t, "partial\nsuccessor", string(call.Payload))
	close(second.release)

	require.NoError(t, ingress.SealPause())
	require.NoError(t, ingress.DrainPause(context.Background(), 0))
	require.NoError(t, ingress.Fence(context.Background()))
	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
}

func TestProcessIngressTimedOutDrainRetainsSealedStateForRetry(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	ingress, err := NewProcessIngress(reader)
	require.NoError(t, err)
	input := newTestProcessInput(1)

	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
		port: input,
	}))

	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()
	writeFunctionLine(t, writer, "held")
	<-input.calls
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	require.NoError(t, ingress.SealPause())
	require.ErrorIs(t, ingress.DrainPause(ctx, 0), context.DeadlineExceeded)

	ingress.mu.Lock()
	pauseSealed := ingress.pauseSealed
	ingress.mu.Unlock()

	require.Equal(t, ProcessIngressLive, ingress.State())
	require.True(t, pauseSealed)

	close(input.release)

	require.NoError(t, ingress.DrainPause(context.Background(), 0))

	require.NoError(t, ingress.Fence(context.Background()))

	require.NoError(t, writer.Close())

	require.NoError(t, <-done)
}

func TestProcessIngressDiscardsPartialBodyOnFence(t *testing.T) {
	reader, writer := io.Pipe()
	ingress, err := NewProcessIngress(reader)
	require.NoError(t, err)
	input := newTestProcessInput(1)
	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
		port: input,
	}))

	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()
	_, err = io.WriteString(
		writer,
		"FUNCTION_PAYLOAD fenced 30 \"test:work\" 0xFFFF \"source\" application/octet-stream\npartial\n",
	)
	require.NoError(t, err)

	require.NoError(t, ingress.SealPause())
	require.NoError(t, ingress.DrainPause(context.Background(), 0))
	require.NoError(t, ingress.Fence(context.Background()))

	require.NoError(t, writer.Close())
	require.NoError(t, <-done)
}

func TestProcessIngressSealedPauseClassifiesStoppingDelivery(t *testing.T) {
	structuralErr := errors.New("structural delivery failure")
	tests := map[string]struct {
		deliveryErr    error
		wantSuppressed bool
	}{
		"current generation stopping rejection": {
			deliveryErr: &lifecycle.StoppingRejection{
				Generation: 1,
			},
			wantSuppressed: true,
		},
		"wrapped current generation stopping rejection": {
			deliveryErr: fmt.Errorf("wrapped: %w", &lifecycle.StoppingRejection{
				Generation: 1,
			}),
			wantSuppressed: true,
		},
		"joined current generation stopping rejections": {
			deliveryErr: errors.Join(
				&lifecycle.StoppingRejection{
					Generation: 1,
				},
				&lifecycle.StoppingRejection{
					Generation: 1,
				},
			),
			wantSuppressed: true,
		},
		"legacy exact stopped rejection": {
			deliveryErr:    jobmgr.ErrStopped,
			wantSuppressed: true,
		},
		"wrong generation stopping rejection": {
			deliveryErr: &lifecycle.StoppingRejection{
				Generation: 2,
			},
		},
		"joined generation stopping rejection": {
			deliveryErr: errors.Join(&lifecycle.StoppingRejection{
				Generation: 1,
			}, structuralErr),
		},
		"joined legacy stopped rejection": {
			deliveryErr: errors.Join(jobmgr.ErrStopped, structuralErr),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ingress, err := NewProcessIngress(strings.NewReader(""))
			require.NoError(t, err)
			input := newTestProcessInput(1)
			input.callErr = test.deliveryErr
			require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{
				port: input,
			}))

			result := make(chan error, 1)
			go func() {
				result <- ingress.HandleCall(context.Background(), functionwire.Call{
					UID: "during-pause",
				})
			}()
			<-input.calls
			require.NoError(t, ingress.SealPause())
			close(input.release)

			deliveryErr := <-result
			if test.wantSuppressed {
				require.NoError(t, deliveryErr)
			} else {
				require.Error(t, deliveryErr)
			}
			require.NoError(t, ingress.DrainPause(context.Background(), 0))
		})
	}
}

func writeFunctionLine(t *testing.T, writer io.Writer, uid string) {
	t.Helper()
	line := "FUNCTION " + uid + " 30 \"test:work\" 0xFFFF \"method=api,role=test\"\n"

	_, err := io.WriteString(writer, line)
	require.NoError(t, err)
}

type testProcessInput struct {
	generation uint64
	calls      chan functionwire.Call
	release    chan struct{}
	callErr    error
}

func newTestProcessInput(generation uint64) *testProcessInput {
	return &testProcessInput{
		generation: generation,
		calls:      make(chan functionwire.Call, 2),
		release:    make(chan struct{}),
	}
}

func (tpi *testProcessInput) Generation() uint64 { return tpi.generation }

func (tpi *testProcessInput) HandleCall(_ context.Context, call functionwire.Call) error {
	tpi.calls <- call
	<-tpi.release
	return tpi.callErr
}

func (*testProcessInput) HandleCancel(context.Context, string) error      { return nil }
func (*testProcessInput) HandleReject(context.Context, string, int) error { return nil }
func (*testProcessInput) HandleQuit(context.Context) error                { return nil }
