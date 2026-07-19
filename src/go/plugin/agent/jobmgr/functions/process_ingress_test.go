// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	functionwire "github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

func TestProcessIngressKeepsOneReaderAndLinearizesPauseAdoptFence(t *testing.T) {
	reader, writer := io.Pipe()
	ingress, err := NewProcessIngress(reader, lifecycle.NewAdmissionLedger())
	require.NoError(t, err)
	first := newTestProcessInput(1)
	second := newTestProcessInput(2)

	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{port: first, admission: ingress.admission}))

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

	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{port: second, admission: ingress.admission}))

	carriedCall := <-second.calls
	require.EqualValues(t, "carried", carriedCall.UID)
	close(second.release)
	writeFunctionLine(t, writer, "second")
	secondCall := <-second.calls
	require.EqualValues(t, "second", secondCall.UID)

	require.NoError(t, ingress.Pause(context.Background(), 0))

	require.NoError(t, ingress.Fence(context.Background()))

	ingress.boundary.mu.Lock()
	require.False(t, ingress.boundary.target != nil ||
		!ingress.boundary.contained ||
		ingress.boundary.state != ProcessIngressContained)
	ingress.boundary.mu.Unlock()
	writeFunctionLine(t, writer, "contained")

	require.NoError(t, writer.Close())

	require.NoError(t, <-done)

	census := ingress.Census()
	require.False(t, census.State != ProcessIngressContained ||
		census.ReaderStarts != 1 ||
		census.RunGeneration != 0 ||
		census.ActiveDeliveries != 0 ||
		census.PendingBody)

	require.Error(t, ingress.Run(context.Background()))
}

func TestProcessIngressTimedOutPauseCanBeRetriedWithoutSplitState(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { require.NoError(t, writer.Close()) }()
	ingress, err := NewProcessIngress(reader, lifecycle.NewAdmissionLedger())
	require.NoError(t, err)
	input := newTestProcessInput(1)

	require.NoError(t, ingress.Adopt(context.Background(), ProcessBinding{port: input, admission: ingress.admission}))

	done := make(chan error, 1)
	go func() { done <- ingress.Run(context.Background()) }()
	writeFunctionLine(t, writer, "held")
	<-input.calls
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	require.ErrorIs(t, ingress.Pause(ctx, 0), context.DeadlineExceeded)

	ingress.boundary.mu.Lock()
	boundaryState := ingress.boundary.state
	ingress.boundary.mu.Unlock()

	census := ingress.Census()
	require.False(t, census.State != ProcessIngressLive || boundaryState != ProcessIngressLive)

	close(input.release)

	require.NoError(t, ingress.Pause(context.Background(), 0))

	require.NoError(t, ingress.Fence(context.Background()))

	require.NoError(t, writer.Close())

	require.NoError(t, <-done)
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
}

func newTestProcessInput(generation uint64) *testProcessInput {
	return &testProcessInput{
		generation: generation,
		calls:      make(chan functionwire.Call, 2),
		release:    make(chan struct{}),
	}
}

func (tpi *testProcessInput) Generation() uint64 { return tpi.generation }

func (*testProcessInput) SuspendInputBody(uint64, uint64) error { return nil }
func (*testProcessInput) AdoptInputBody(uint64) error           { return nil }

func (tpi *testProcessInput) HandleCall(_ context.Context, call functionwire.Call) error {
	tpi.calls <- call
	<-tpi.release
	return nil
}

func (*testProcessInput) HandleCancel(context.Context, string) error      { return nil }
func (*testProcessInput) HandleReject(context.Context, string, int) error { return nil }
func (*testProcessInput) HandleQuit(context.Context) error                { return nil }

func (*testProcessInput) GrowInputBody(context.Context, uint64, int64) (uint64, error) {
	return 1, nil
}

func (*testProcessInput) CommitInputBodyGrowth(uint64, int64) error { return nil }
func (*testProcessInput) ReleaseInputBody(uint64) error             { return nil }
