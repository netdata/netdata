// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (adri *autoDetectionRetryIndex) joined() bool {
	if adri == nil {
		return true
	}
	adri.mu.Lock()
	bound, done := adri.bound, adri.done
	adri.mu.Unlock()
	if !bound {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func retryTestWake(schedule uint64, due int64) candidateWake {
	return candidateWake{
		Token: candidateWakeToken{
			Run:      1,
			Intent:   2,
			Schedule: schedule,
		},
		Due: due,
	}
}

func TestAutoDetectionRetryDispatchesOnlyCurrentDueCapabilities(t *testing.T) {
	tests := map[string]struct {
		arrange func(*autoDetectionRetryIndex)
		want    int
		token   candidateWakeToken
	}{
		"due": {
			arrange: func(i *autoDetectionRetryIndex) { i.schedule("job", retryTestWake(1, 2)) },
			want:    1,
			token:   retryTestWake(1, 2).Token,
		},
		"replacement": {arrange: func(i *autoDetectionRetryIndex) {
			i.schedule("job", retryTestWake(1, 1))
			i.schedule("job", retryTestWake(2, 2))
		}, want: 1, token: retryTestWake(2, 2).Token},
		"cancel": {
			arrange: func(i *autoDetectionRetryIndex) { i.schedule("job", retryTestWake(1, 1)); i.cancel("job") },
		},
		"wrong run": {
			arrange: func(i *autoDetectionRetryIndex) { w := retryTestWake(1, 1); w.Token.Run = 2; i.schedule("job", w) },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			commands := &autoDetectionRetryTestCommands{}
			var planned []candidateWakeToken
			require.NoError(t, index.bind(commands, func(id string, token candidateWakeToken) (jobmgr.WorkPlan, error) {
				assert.Equal(t, "job", id)
				planned = append(planned, token)
				return jobmgr.WorkPlan{}, nil
			}, 1, func(error) {}))
			test.arrange(index)
			for _, clock := range []int{100, 101, 102} {
				index.advance(clock)
			}
			commands.waitForSubmissions(t, test.want)
			index.stopWorker()
			require.NoError(t, index.wait(t.Context()))
			submitted, plans, waited := commands.snapshot()
			require.Len(t, submitted, test.want)
			require.Len(t, plans, test.want)
			assert.Equal(t, test.want > 0, waited)
			if test.want > 0 {
				assert.Equal(t, []candidateWakeToken{test.token}, planned)
			}
		})
	}
}

func TestAutoDetectionRetryLogicalClockAndRegression(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	failures := make(chan error, 1)
	require.NoError(
		t,
		index.bind(
			&autoDetectionRetryTestCommands{},
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(err error) { failures <- err },
		),
	)
	index.advance(100)
	assert.Equal(t, int64(0), index.now())
	index.advance(103)
	assert.Equal(t, int64(3), index.now())
	index.advance(102)
	select {
	case err := <-failures:
		require.ErrorContains(t, err, "process clock regressed")
	case <-time.After(time.Second):
		t.Fatal("clock regression not reported")
	}
	index.schedule("job", retryTestWake(1, 3))
	index.advance(105)
	assert.Equal(t, int64(3), index.now())
	index.stopWorker()
	require.ErrorContains(t, index.wait(t.Context()), "process clock regressed")
}

func TestAutoDetectionRetryRetainsExactCapabilityWhileDispatching(t *testing.T) {
	release := make(chan struct{})
	index := newAutoDetectionRetryIndex()
	commands := &autoDetectionRetryTestCommands{
		block: release,
	}
	require.NoError(
		t,
		index.bind(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(error) {},
		),
	)
	original := retryTestWake(1, 1)
	index.schedule("job", original)
	index.advance(0)
	index.advance(1)
	commands.waitForSubmissions(t, 1)
	require.True(t, index.isCurrent("job", original.Token))
	replacement := retryTestWake(2, 2)
	index.schedule("job", replacement)
	require.False(t, index.isCurrent("job", original.Token))
	require.True(t, index.isCurrent("job", replacement.Token))
	index.cancelToken("job", original.Token)
	require.True(t, index.isCurrent("job", replacement.Token))
	index.cancelToken("job", replacement.Token)
	require.False(t, index.isCurrent("job", replacement.Token))
	close(release)
	index.stopWorker()
	require.NoError(t, index.wait(t.Context()))
}

func TestAutoDetectionRetryAlreadyDueAndPopulation(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(
		t,
		index.bind(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(error) {},
		),
	)
	index.advance(10)
	index.advance(13)
	for n := range 257 {
		index.schedule(fmt.Sprintf("job-%03d", n), retryTestWake(uint64(n+1), 3))
	}
	commands.waitForSubmissions(t, 257)
	index.stopWorker()
	require.NoError(t, index.wait(t.Context()))
	submitted, _, _ := commands.snapshot()
	require.Len(t, submitted, 257)
}

func TestSchedulerTickDoesNotBlockOnRetryAcknowledgement(t *testing.T) {
	release := make(chan struct{})
	commands := &autoDetectionRetryTestCommands{
		block: release,
	}
	scheduler, err := NewScheduler(testModuleReconciler{})
	require.NoError(t, err)
	require.NoError(
		t,
		scheduler.bindBackgroundWorkers(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(error) {},
		),
	)
	require.False(t, scheduler.retries.joined())
	scheduler.retries.schedule("job", retryTestWake(1, 1))
	require.NoError(t, scheduler.Tick(t.Context(), 0))
	tickDone := make(chan error, 1)
	go func() { tickDone <- scheduler.Tick(t.Context(), 1) }()
	select {
	case err := <-tickDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		close(release)
		t.Fatal("tick blocked on retry acknowledgement")
	}
	commands.waitForSubmissions(t, 1)
	close(release)
	scheduler.StopBackgroundWorkers()
	require.NoError(t, scheduler.WaitBackgroundWorkers(t.Context()))
	require.True(t, scheduler.retries.joined())
	require.NoError(t, scheduler.accepted.wait(t.Context()))
}

func TestAutoDetectionRetryReportsStructuralDispatchFailure(t *testing.T) {
	tests := map[string]struct {
		plan   error
		submit error
	}{"planning": {plan: errors.New("plan failed")}, "acknowledgement": {submit: errors.New("apply failed")}}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			failed := make(chan error, 1)
			require.NoError(
				t,
				index.bind(
					&autoDetectionRetryTestCommands{
						submitErr: test.submit,
					},
					func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, test.plan },
					1,
					func(err error) { failed <- err },
				),
			)
			index.schedule("job", retryTestWake(1, 0))
			want := test.plan
			if want == nil {
				want = test.submit
			}
			select {
			case err := <-failed:
				require.ErrorIs(t, err, want)
			case <-time.After(time.Second):
				t.Fatal("dispatch failure not supervised")
			}
			require.ErrorIs(t, index.wait(t.Context()), want)
			index.stopWorker()
		})
	}
}

func TestAutoDetectionRetryWaitRequiresWorkerJoin(t *testing.T) {
	release := make(chan struct{})
	commands := &autoDetectionRetryTestCommands{
		block: release,
	}
	index := newAutoDetectionRetryIndex()
	require.NoError(
		t,
		index.bind(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(error) {},
		),
	)
	index.schedule("job", retryTestWake(1, 0))
	commands.waitForSubmissions(t, 1)
	index.stopWorker()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, index.wait(ctx), context.Canceled)
	require.False(t, index.joined())
	close(release)
	require.NoError(t, index.wait(t.Context()))
	require.True(t, index.joined())
}

func TestAutoDetectionRetryClassifiesStoppingAcknowledgement(t *testing.T) {
	tests := map[string]struct {
		err   error
		fatal bool
	}{
		"current": {err: &lifecycle.StoppingRejection{
			Generation: 1,
		}},
		"wrapped current": {err: fmt.Errorf("apply: %w", &lifecycle.StoppingRejection{
			Generation: 1,
		})},
		"joined current": {
			err: errors.Join(&lifecycle.StoppingRejection{
				Generation: 1,
			}, &lifecycle.StoppingRejection{
				Generation: 1,
			}),
		},
		"wrong run": {err: &lifecycle.StoppingRejection{
			Generation: 2,
		}, fatal: true},
		"mixed": {
			err: errors.Join(&lifecycle.StoppingRejection{
				Generation: 1,
			}, errors.New("structural")),
			fatal: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			failed := make(chan error, 1)
			require.NoError(
				t,
				index.bind(
					&autoDetectionRetryTestCommands{
						submitErr: test.err,
					},
					func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
					1,
					func(err error) { failed <- err },
				),
			)
			index.schedule("job", retryTestWake(1, 0))
			err := index.wait(t.Context())
			if test.fatal {
				require.ErrorIs(t, err, test.err)
				require.ErrorIs(t, <-failed, test.err)
			} else {
				require.NoError(t, err)
				select {
				case err := <-failed:
					t.Fatalf("current stopping failed: %v", err)
				default:
				}
			}
			index.stopWorker()
		})
	}
}

type autoDetectionRetryTestCommands struct {
	mu        sync.Mutex
	submitted []jobmgr.Request
	plans     []jobmgr.WorkPlan
	waited    bool
	block     <-chan struct{}
	submitErr error
	notify    chan struct{}
}

func (adrtc *autoDetectionRetryTestCommands) SubmitPrepared(
	_ context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	return adrtc.submit(request, plan, false)
}

func (adrtc *autoDetectionRetryTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	return adrtc.submit(request, plan, true)
}

func (adrtc *autoDetectionRetryTestCommands) submit(
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
	waited bool,
) error {
	adrtc.mu.Lock()
	adrtc.submitted = append(adrtc.submitted, request)
	adrtc.plans = append(adrtc.plans, plan)
	adrtc.waited = adrtc.waited || waited
	if adrtc.notify == nil {
		adrtc.notify = make(chan struct{}, 1)
	}
	notify := adrtc.notify
	block := adrtc.block
	err := adrtc.submitErr
	adrtc.mu.Unlock()
	select {
	case notify <- struct{}{}:
	default:
	}
	if block != nil {
		<-block
	}
	return err
}

func (adrtc *autoDetectionRetryTestCommands) snapshot() ([]jobmgr.Request, []jobmgr.WorkPlan, bool) {
	adrtc.mu.Lock()
	defer adrtc.mu.Unlock()
	return append(
			[]jobmgr.Request(nil),
			adrtc.submitted...), append(
			[]jobmgr.WorkPlan(nil),
			adrtc.plans...), adrtc.waited
}

func (adrtc *autoDetectionRetryTestCommands) waitForSubmissions(t *testing.T, want int) {
	t.Helper()
	if want == 0 {
		select {
		case <-adrtc.notification():
			require.FailNow(t, "test failed", "unexpected retry submission")
		case <-time.After(20 * time.Millisecond):
		}
		return
	}
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		adrtc.mu.Lock()
		got := len(adrtc.submitted)
		adrtc.mu.Unlock()
		if got >= want {
			return
		}
		select {
		case <-adrtc.notification():
		case <-timeout.C:
			require.FailNowf(t, "test failed", "retry submissions=%d want=%d", got, want)
		}
	}
}

func (adrtc *autoDetectionRetryTestCommands) notification() <-chan struct{} {
	adrtc.mu.Lock()
	defer adrtc.mu.Unlock()
	if adrtc.notify == nil {
		adrtc.notify = make(chan struct{}, 1)
	}
	return adrtc.notify
}

func autoDetectionRetryTestConfig(name string) confgroup.Config {
	return confgroup.Config{
		"module":              "module",
		"name":                name,
		"update_every":        1,
		"autodetection_retry": 1,
		"__source_type__":     confgroup.TypeDyncfg,
		"__source__":          "user=test",
		"__provider__":        "test",
	}
}

func TestAutoDetectionRetryCoalescesSameWakeWhileAcknowledgementPending(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	release := make(chan struct{})
	commands := &concurrentRetryTestCommands{
		release:   release,
		submitted: make(chan jobmgr.Request, 2),
	}
	require.NoError(
		t,
		index.bind(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(err error) { t.Errorf("dispatch failure: %v", err) },
		),
	)
	wake := retryTestWake(1, 0)
	index.schedule("a", wake)
	require.Equal(t, "a", (<-commands.submitted).LaneKey)
	for range 3 {
		index.schedule("a", wake)
	}
	close(release)
	select {
	case <-commands.submitted:
		t.Error("identical in-flight wake was dispatched again")
	case <-time.After(30 * time.Millisecond):
	}
	index.stopWorker()
	require.NoError(t, index.wait(t.Context()))
}

func TestAutoDetectionRetryConflictingDeadlineFailsClosed(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	failed := make(chan error, 1)
	require.NoError(
		t,
		index.bind(
			&autoDetectionRetryTestCommands{},
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(err error) { failed <- err },
		),
	)
	wake := retryTestWake(1, 2)
	index.schedule("job", wake)
	wake.Due = 3
	index.schedule("job", wake)
	select {
	case err := <-failed:
		require.ErrorContains(t, err, "conflicting wake deadline")
	case <-time.After(time.Second):
		t.Error("same capability accepted a different deadline")
	}
	index.stopWorker()
	require.ErrorContains(t, index.wait(t.Context()), "conflicting wake deadline")
}

func TestAutoDetectionRetryDifferentNamesDoNotWaitForAnotherAcknowledgement(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	release := make(chan struct{})
	commands := &concurrentRetryTestCommands{
		release:   release,
		submitted: make(chan jobmgr.Request, 2),
	}
	require.NoError(
		t,
		index.bind(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(err error) { t.Errorf("retry dispatch: %v", err) },
		),
	)
	index.schedule("a", retryTestWake(1, 0))
	require.Equal(t, "a", (<-commands.submitted).LaneKey)
	index.schedule("b", retryTestWake(2, 0))
	select {
	case request := <-commands.submitted:
		require.Equal(t, "b", request.LaneKey)
	case <-time.After(time.Second):
		t.Error("one job's pending acknowledgement blocked another due wake")
	}
	index.stopWorker()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, index.wait(ctx), context.Canceled, "stop must retain ownership of the pending dispatch")
	close(release)
	require.NoError(t, index.wait(t.Context()))
}

type concurrentRetryTestCommands struct {
	release   <-chan struct{}
	submitted chan jobmgr.Request
}

func (*concurrentRetryTestCommands) SubmitPrepared(context.Context, jobmgr.Request, jobmgr.WorkPlan) error {
	return errors.New("test: retry delivery lost its acknowledgement")
}

func (c *concurrentRetryTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	c.submitted <- request
	if request.LaneKey == "a" {
		<-c.release
	}
	return nil
}

func TestAutoDetectionRetryTransportIdentityAcceptsLongJobNames(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	commands := &validatingRetryTestCommands{
		submitted: make(chan jobmgr.Request, 1),
	}
	failed := make(chan error, 1)
	require.NoError(
		t,
		index.bind(
			commands,
			func(string, candidateWakeToken) (jobmgr.WorkPlan, error) { return jobmgr.WorkPlan{}, nil },
			1,
			func(err error) { failed <- err },
		),
	)
	id := "module_" + strings.Repeat("job", 100)
	index.schedule(id, retryTestWake(1, 0))
	select {
	case request := <-commands.submitted:
		require.Equal(t, id, request.LaneKey)
		require.NotContains(t, request.UID, id)
	case err := <-failed:
		t.Errorf("valid long name failed transport validation: %v", err)
	case <-time.After(time.Second):
		t.Error("long job wake was not delivered")
	}
	index.stopWorker()
	require.NoError(t, index.wait(t.Context()))
}

type validatingRetryTestCommands struct{ submitted chan jobmgr.Request }

func (*validatingRetryTestCommands) SubmitPrepared(context.Context, jobmgr.Request, jobmgr.WorkPlan) error {
	return errors.New("test: retry delivery lost acknowledgement")
}

func (c *validatingRetryTestCommands) SubmitPreparedAndWait(
	_ context.Context,
	request jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	if err := request.Validate(); err != nil {
		return err
	}
	c.submitted <- request
	return nil
}
