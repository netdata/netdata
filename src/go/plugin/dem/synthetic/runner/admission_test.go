// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slotRequest is one caller blocked in acquire. Tests run in a synctest
// bubble, so synctest.Wait returns once every caller is granted or queued.
type slotRequest struct {
	t      *testing.T
	owner  *executionOwner
	cancel context.CancelFunc
	done   chan struct{}
	job    *admissionJob
	err    error
}

func requestSlot(t *testing.T, o *executionOwner, key executionKey) *slotRequest {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	r := &slotRequest{t: t, owner: o, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.job, r.err = o.acquire(ctx, key)
	}()
	synctest.Wait()
	return r
}

func jobKey(name string) executionKey { return executionKey{jobID: name} }

func (r *slotRequest) finished() bool {
	select {
	case <-r.done:
		return true
	default:
		return false
	}
}

func (r *slotRequest) release() {
	r.t.Helper()
	require.True(r.t, r.finished(), "released a request that was never granted")
	require.NoError(r.t, r.err)
	r.owner.release(r.job)
	synctest.Wait()
}

func (r *slotRequest) withdraw() error {
	r.cancel()
	synctest.Wait()
	require.True(r.t, r.finished())
	return r.err
}

// granted reports which requests hold a slot, in argument order.
func granted(requests ...*slotRequest) []bool {
	held := make([]bool, len(requests))
	for i, r := range requests {
		held[i] = r.finished() && r.err == nil
	}
	return held
}

func assertIdle(t *testing.T, o *executionOwner) {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	assert.Zero(t, o.active)
	assert.Empty(t, o.jobs)
	assert.Empty(t, o.ready)
}

func TestAdmissionLimitsActiveAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := newExecutionOwner(2)
		a, b, c := requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("b")), requestSlot(t, o, jobKey("c"))
		assert.Equal(t, []bool{true, true, false}, granted(a, b, c))
		a.release()
		assert.Equal(t, []bool{true}, granted(c))
		b.release()
		c.release()
		assertIdle(t, o)
	})
}

func TestAdmissionRunsOneAttemptPerJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := newExecutionOwner(2)
		monitor, rehearsal, other := requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("b"))
		assert.Equal(t, []bool{true, false, true}, granted(monitor, rehearsal, other),
			"a second request of a running job must not take a free slot")
		monitor.release()
		assert.Equal(t, []bool{true}, granted(rehearsal))
		rehearsal.release()
		other.release()
		assertIdle(t, o)
	})
}

func TestAdmissionGivesReadyJobsTurns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := newExecutionOwner(1)
		a1, a2, a3 := requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("a"))
		b1, c1 := requestSlot(t, o, jobKey("b")), requestSlot(t, o, jobKey("c"))
		require.Equal(t, []bool{true}, granted(a1))
		a1.release()
		assert.Equal(t, []bool{false, false, true, false}, granted(a2, a3, b1, c1), "b's turn comes before a's backlog")
		b1.release()
		assert.Equal(t, []bool{true}, granted(c1))
		c1.release()
		assert.Equal(t, []bool{true}, granted(a2))
		b2 := requestSlot(t, o, jobKey("b"))
		a2.release()
		assert.Equal(t, []bool{false, true}, granted(a3, b2), "a requeues at the tail after its turn")
		b2.release()
		assert.Equal(t, []bool{true}, granted(a3))
		a3.release()
		assertIdle(t, o)
	})
}

func TestAdmissionCancellationKeepsOtherPositions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := newExecutionOwner(1)
		hold, cancelled := requestSlot(t, o, jobKey("hold")), requestSlot(t, o, jobKey("cancel"))
		a1, a2, b := requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("a")), requestSlot(t, o, jobKey("b"))
		assert.ErrorIs(t, a1.withdraw(), context.Canceled)
		assert.ErrorIs(t, cancelled.withdraw(), context.Canceled)
		o.mu.Lock()
		assert.NotContains(t, o.jobs, jobKey("cancel"), "a job without requests leaves the owner")
		o.mu.Unlock()
		hold.release()
		assert.Equal(t, []bool{true, false}, granted(a2, b), "a keeps its place with its remaining request")
		a2.release()
		assert.Equal(t, []bool{true}, granted(b))
		b.release()
		assertIdle(t, o)
	})
}

func TestAdmissionPoisonStopsWaitingAndFutureRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := newExecutionOwner(1)
		held, waiting := requestSlot(t, o, jobKey("held")), requestSlot(t, o, jobKey("waiting"))
		close(o.poisoned)
		synctest.Wait()
		require.True(t, waiting.finished())
		assert.ErrorIs(t, waiting.err, ndexec.ErrTreeNotDrained)
		_, err := o.acquire(t.Context(), jobKey("future"))
		assert.ErrorIs(t, err, ndexec.ErrTreeNotDrained)
		held.release()
		assertIdle(t, o)
	})
}

func TestAdmissionQualificationDoesNotQueueBehindUserJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		o := newExecutionOwner(2)
		user := requestSlot(t, o, jobKey("runtime-qualification"))
		qualification := requestSlot(t, o, executionKey{jobID: "runtime-qualification", qualification: true})
		assert.Equal(t, []bool{true, true}, granted(user, qualification))
		user.release()
		qualification.release()
		assertIdle(t, o)
	})
}

// TestAdmissionReleasesGrantRacingWithdrawal covers a grant that lands while
// its caller stops: withdrawing the granted request releases its slot.
func TestAdmissionReleasesGrantRacingWithdrawal(t *testing.T) {
	o := newExecutionOwner(1)
	key := jobKey("a")
	job := &admissionJob{key: key}
	waiter := &admissionWaiter{ready: make(chan struct{}), job: job}
	job.waiters = []*admissionWaiter{waiter}
	o.jobs[key] = job
	o.ready = []*admissionJob{job}
	o.mu.Lock()
	o.dispatch()
	o.mu.Unlock()
	require.True(t, waiter.granted)
	o.remove(waiter)
	assertIdle(t, o)
}
