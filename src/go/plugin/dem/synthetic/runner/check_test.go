// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeProbe stands in for the runtime contract probe. Each run blocks until
// released or until its context stops, and records how it ended.
type fakeProbe struct {
	runs     atomic.Int32
	stopped  atomic.Int32 // runs that ended because their context stopped
	inFlight atomic.Bool
	release  chan error
}

func newFakeProbe() *fakeProbe { return &fakeProbe{release: make(chan error)} }

func (p *fakeProbe) probe(ctx context.Context) error {
	p.runs.Add(1)
	p.inFlight.Store(true)
	defer p.inFlight.Store(false)
	select {
	case err := <-p.release:
		return err
	case <-ctx.Done():
		p.stopped.Add(1)
		return ctx.Err()
	}
}

// checkCall is one caller of checkOwner.do running in the synctest bubble.
type checkCall struct {
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func callCheck(ctx context.Context, o *checkOwner, poisoned <-chan struct{}, probe func(context.Context) error) *checkCall {
	ctx, cancel := context.WithCancel(ctx)
	c := &checkCall{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(c.done)
		c.err = o.do(ctx, poisoned, probe)
	}()
	synctest.Wait()
	return c
}

func (c *checkCall) returned() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func TestCheckCallersShareOneProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, p, poisoned := &checkOwner{}, newFakeProbe(), make(chan struct{})
		calls := make([]*checkCall, 8)
		for i := range calls {
			calls[i] = callCheck(t.Context(), o, poisoned, p.probe)
		}
		assert.Equal(t, int32(1), p.runs.Load())
		failure := errors.New("contract mismatch")
		p.release <- failure
		synctest.Wait()
		for _, c := range calls {
			require.True(t, c.returned())
			assert.Equal(t, failure, c.err)
		}
		later := callCheck(t.Context(), o, poisoned, p.probe)
		assert.Equal(t, int32(2), p.runs.Load(), "a finished probe is not cached")
		p.release <- nil
		synctest.Wait()
		assert.NoError(t, later.err)
	})
}

func TestCheckLastCallerCancelsAndJoinsBeforeReplacement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, poisoned := &checkOwner{}, make(chan struct{})
		joined := make(chan struct{})
		var runs atomic.Int32
		var stopped atomic.Bool
		probe := func(ctx context.Context) error {
			if runs.Add(1) == 1 {
				<-ctx.Done()
				stopped.Store(true)
				<-joined // its process tree has not joined yet
				return ctx.Err()
			}
			return nil
		}
		first, second := callCheck(t.Context(), o, poisoned, probe), callCheck(t.Context(), o, poisoned, probe)
		first.cancel()
		synctest.Wait()
		require.True(t, first.returned(), "a caller leaves on its own stop")
		assert.ErrorIs(t, first.err, context.Canceled)
		assert.False(t, stopped.Load(), "the probe keeps running for the remaining caller, detached from the first")
		second.cancel()
		synctest.Wait()
		assert.True(t, stopped.Load(), "the last caller cancels the probe")
		assert.False(t, second.returned(), "the last caller waits for the cancelled probe to join")
		replacement := callCheck(t.Context(), o, poisoned, probe)
		expired, expire := context.WithCancel(t.Context())
		expire()
		assert.ErrorIs(t, o.do(expired, poisoned, probe), context.Canceled, "an expired caller starts nothing")
		assert.Equal(t, int32(1), runs.Load(), "no replacement probe starts before the join")
		close(joined)
		synctest.Wait()
		require.True(t, second.returned())
		assert.ErrorIs(t, second.err, context.Canceled)
		require.True(t, replacement.returned())
		assert.NoError(t, replacement.err)
		assert.Equal(t, int32(2), runs.Load())
	})
}

func TestCheckCallerDeadlineStopsLoneProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, p, poisoned := &checkOwner{}, newFakeProbe(), make(chan struct{})
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
		defer cancel()
		c := callCheck(ctx, o, poisoned, p.probe)
		time.Sleep(30 * time.Millisecond)
		synctest.Wait()
		require.True(t, c.returned())
		assert.ErrorIs(t, c.err, context.DeadlineExceeded)
		assert.Equal(t, int32(1), p.stopped.Load())
		assert.False(t, p.inFlight.Load())
	})
}

func TestCheckProbeHasItsOwnTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, poisoned := &checkOwner{}, make(chan struct{})
		var deadline time.Time
		start := time.Now()
		err := o.do(t.Context(), poisoned, func(ctx context.Context) error {
			deadline, _ = ctx.Deadline()
			return nil
		})
		require.NoError(t, err)
		assert.Equal(t, start.Add(checkTimeout), deadline, "the probe outlives a caller without a deadline only up to checkTimeout")
	})
}

func TestCheckPoisonReleasesCallers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, p, poisoned := &checkOwner{}, newFakeProbe(), make(chan struct{})
		first, second := callCheck(t.Context(), o, poisoned, p.probe), callCheck(t.Context(), o, poisoned, p.probe)
		close(poisoned)
		synctest.Wait()
		for _, c := range []*checkCall{first, second} {
			require.True(t, c.returned())
			assert.ErrorIs(t, c.err, ndexec.ErrTreeNotDrained)
		}
		assert.Equal(t, int32(1), p.stopped.Load(), "the last caller cancels the probe")
		assert.ErrorIs(t, o.do(t.Context(), poisoned, p.probe), ndexec.ErrTreeNotDrained)
		assert.Equal(t, int32(1), p.runs.Load())
	})
}

// TestCheckLastCallerReportsUndrainedProbe keeps an undrained probe's error for
// the caller that cancelled and joined it, so the caller does not release the
// runtime as if it were idle.
func TestCheckLastCallerReportsUndrainedProbe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o, poisoned := &checkOwner{}, make(chan struct{})
		probe := func(ctx context.Context) error {
			<-ctx.Done()
			return ndexec.ErrTreeNotDrained
		}
		c := callCheck(t.Context(), o, poisoned, probe)
		c.cancel()
		synctest.Wait()
		require.True(t, c.returned())
		assert.ErrorIs(t, c.err, context.Canceled)
		assert.ErrorIs(t, c.err, ndexec.ErrTreeNotDrained)
	})
}
