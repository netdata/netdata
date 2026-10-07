// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package streamexec

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec/streamexectest"
)

// newUnsignalableSource returns a source that starts the fake through ndsudo, as the ndsudo command "igt" with the
// argument "--test", and cannot signal it, like a root command of an unprivileged caller.
func newUnsignalableSource(t *testing.T, timing Timing) *Source[string] {
	t.Helper()
	t.Cleanup(ndexec.DenyNDSudoSignalsForTests())
	s, err := New(Config[string]{
		Name: "fake",
		Start: func(ctx context.Context, stdout *os.File) (*ndexec.Process, error) {
			return ndexec.StartNDSudoProcess(ctx, ndexec.ProcessOptions{
				Stdout: stdout,
			}, "igt", "--test")
		},
		NewDecoder: recordDecoder,
		Timing:     timing,
	})
	require.NoError(t, err)
	return s
}

func TestUnsignalableWriterEndsAtNextWrite(t *testing.T) {
	fake := streamexectest.NewFake(t)
	s := newUnsignalableSource(t, testTiming)
	stop := startRun(t, s)
	conn := fake.Accept(t)
	assert.Equal(t, []string{"igt", "--test"}, conn.Args)
	conn.Line(t, "record 1")
	waitLatest(t, s, "record 1")
	conn.Noise(t, "noise")

	began := time.Now()
	stop()
	assert.Less(t, time.Since(began), 2*time.Second, "a writing command outlived the close of its output")
	conn.RequireExited(t)
}

func TestUnsignalableSilentCommandBoundsStop(t *testing.T) {
	fake := streamexectest.NewFake(t)
	timing := testTiming
	timing.StallTimeout = time.Second
	s := newUnsignalableSource(t, timing)
	stop := startRun(t, s)
	conn := fake.Accept(t)
	conn.Line(t, "record 1")
	waitLatest(t, s, "record 1")

	began := time.Now()
	stop()
	elapsed := time.Since(began)
	assert.GreaterOrEqual(t, elapsed, timing.StallTimeout, "stop did not wait for the exit")
	assert.Less(t, elapsed, 3*timing.StallTimeout, "stop waited beyond the bound")
	assert.Nil(t, s.latest.Load())

	// The command was left running, so the next Run does not start another instance until its next write ends it.
	conn.RequireRunning(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started, done := goRun(ctx, s)
	fake.RequireNoStart(t, 500*time.Millisecond)
	conn.Line(t, "late")
	next := fake.Accept(t)
	conn.RequireExited(t)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the next Run did not start")
	}
	next.Line(t, "record 2")
	waitLatest(t, s, "record 2")
	cancel()
	next.Noise(t, "noise") // a writing command ends at once, so Run returns without the bound
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the next Run did not return")
	}
}

func TestUnsignalableStalledCommandDelaysReplacement(t *testing.T) {
	fake := streamexectest.NewFake(t)
	timing := testTiming
	// Long enough for a slow fake startup to deliver its first record before the stall timer fires.
	timing.StallTimeout = time.Second
	s := newUnsignalableSource(t, timing)
	startRun(t, s)
	conn := fake.Accept(t)
	conn.Line(t, "record 1")
	waitLatest(t, s, "record 1")

	// The stalled instance writes nothing more, so it keeps running, and no replacement starts: neither while its close
	// waits for the exit nor after that wait.
	waitWithdrawn(t, s)
	fake.RequireNoStart(t, 2*timing.StallTimeout)
	conn.RequireRunning(t)

	conn.Line(t, "late")
	replacement := fake.Accept(t)
	conn.RequireExited(t) // the replacement started after the exit
	replacement.Line(t, "record 2")
	waitLatest(t, s, "record 2")
}
