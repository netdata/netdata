// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestAutoDetectionRetryIndexDispatchesOnlyCurrentDueEntries(t *testing.T) {
	tests := map[string]struct {
		arrange      func(*autoDetectionRetryIndex, confgroup.Config) confgroup.Config
		clocks       []int
		wantDispatch int
	}{
		"due retry dispatches": {
			arrange: func(index *autoDetectionRetryIndex, config confgroup.Config) confgroup.Config {
				index.schedule(config, 2)
				return config
			},
			clocks:       []int{0, 1, 2},
			wantDispatch: 1,
		},
		"replacement invalidates older token": {
			arrange: func(index *autoDetectionRetryIndex, config confgroup.Config) confgroup.Config {
				index.schedule(config, 1)
				replacement, err := config.Clone()
				if err != nil {
					panic(err)
				}
				replacement.Set("option", "replacement")
				index.schedule(replacement, 2)
				return replacement
			},
			clocks:       []int{0, 1, 2},
			wantDispatch: 1,
		},
		"cancel removes exact pending entry": {
			arrange: func(index *autoDetectionRetryIndex, config confgroup.Config) confgroup.Config {
				index.schedule(config, 1)
				index.cancel(config.FullName())
				return config
			},
			clocks: []int{0, 1, 2},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			commands := &autoDetectionRetryTestCommands{}
			var planned []autoDetectionRetryToken
			require.NoError(t, index.bind(
				commands,
				func(_ confgroup.Config, token autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
					planned = append(planned, token)
					return jobmgr.WorkPlan{}, nil
				},
				7,
				func(error) {},
			))
			config := autoDetectionRetryTestConfig("job")
			expected := test.arrange(index, config)
			for _, clock := range test.clocks {
				index.advance(clock)
			}

			commands.waitForSubmissions(t, test.wantDispatch)
			submitted, plans, waited := commands.snapshot()
			require.Len(t, submitted, test.wantDispatch)
			require.Len(t, plans, test.wantDispatch)
			require.False(t, waited)
			if test.wantDispatch != 0 {
				require.Equal(t, expected.UID(), planned[0].uid)
			}
			index.stopWorker()
			require.NoError(t, index.wait(context.Background()))
		})
	}
}

func TestAutoDetectionRetryIndexHasNoFixedPopulationLimit(t *testing.T) {
	tests := map[string]struct {
		population int
	}{
		"former active-job limit":          {population: 256},
		"former active-job limit plus one": {population: 257},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			index := newAutoDetectionRetryIndex()
			commands := &autoDetectionRetryTestCommands{}
			require.NoError(t, index.bind(
				commands,
				func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
					return jobmgr.WorkPlan{}, nil
				},
				1,
				func(error) {},
			))
			for number := range test.population {
				index.schedule(autoDetectionRetryTestConfig(fmt.Sprintf("job-%03d", number)), 1)
			}
			index.advance(0)
			index.advance(1)
			commands.waitForSubmissions(t, test.population)
			submitted, _, _ := commands.snapshot()
			require.Len(t, submitted, test.population)
			index.stopWorker()
			require.NoError(t, index.wait(context.Background()))
		})
	}
}

func TestAutoDetectionRetryUsesLogicalClockFromFirstRunTick(t *testing.T) {
	index := newAutoDetectionRetryIndex()
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, index.bind(
		commands,
		func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
		1,
		func(error) {},
	))
	index.schedule(autoDetectionRetryTestConfig("job"), 2)

	index.advance(100)
	index.advance(101)
	commands.waitForSubmissions(t, 0)
	index.advance(102)
	commands.waitForSubmissions(t, 1)

	index.stopWorker()
	require.NoError(t, index.wait(context.Background()))
}

func TestAutoDetectionRetryRetainsExactAuthorityWhileDispatching(t *testing.T) {
	release := make(chan struct{})
	index := newAutoDetectionRetryIndex()
	commands := &autoDetectionRetryTestCommands{block: release}
	require.NoError(t, index.bind(
		commands,
		func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
		1,
		func(error) {},
	))
	config := autoDetectionRetryTestConfig("job")
	index.schedule(config, 1)
	index.mu.Lock()
	original := index.entries[config.FullName()].token
	index.mu.Unlock()
	index.advance(0)
	index.advance(1)
	commands.waitForSubmissions(t, 1)
	require.True(t, index.isCurrent(config.FullName(), original))

	replacement, err := config.Clone()
	require.NoError(t, err)
	replacement.Set("option", "replacement")
	index.schedule(replacement, 1)
	index.mu.Lock()
	current := index.entries[config.FullName()].token
	index.mu.Unlock()
	require.False(t, index.isCurrent(config.FullName(), original))
	require.True(t, index.isCurrent(config.FullName(), current))
	index.cancelToken(config.FullName(), original)
	require.True(t, index.isCurrent(config.FullName(), current))
	index.cancelToken(config.FullName(), current)
	require.False(t, index.isCurrent(config.FullName(), current))

	close(release)
	index.stopWorker()
	require.NoError(t, index.wait(context.Background()))
}

func TestSchedulerTickDoesNotBlockOnRetryAdmission(t *testing.T) {
	release := make(chan struct{})
	commands := &autoDetectionRetryTestCommands{block: release}
	scheduler, err := NewScheduler(testModuleReconciler{})
	require.NoError(t, err)
	require.NoError(t, scheduler.bindAutoDetectionRetries(
		commands,
		func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
		1,
		func(error) {},
	))
	require.False(t, scheduler.AutoDetectionRetriesJoined())
	scheduler.retries.schedule(autoDetectionRetryTestConfig("job"), 1)
	require.NoError(t, scheduler.Tick(context.Background(), 0))

	tickDone := make(chan error, 1)
	go func() {
		tickDone <- scheduler.Tick(context.Background(), 1)
	}()
	select {
	case err := <-tickDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		close(release)
		require.FailNow(t, "test failed", "scheduler tick blocked on retry admission")
	}
	commands.waitForSubmissions(t, 1)
	close(release)
	scheduler.StopAutoDetectionRetries()
	require.NoError(t, scheduler.WaitAutoDetectionRetries(context.Background()))
	require.True(t, scheduler.AutoDetectionRetriesJoined())
}

func TestAutoDetectionRetryReportsStructuralDispatchFailure(t *testing.T) {
	tests := map[string]struct {
		planningErr error
		submitErr   error
	}{
		"planning failure":   {planningErr: errors.New("retry planning failed")},
		"submission failure": {submitErr: errors.New("retry submission failed")},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			failed := make(chan error, 1)
			index := newAutoDetectionRetryIndex()
			require.NoError(t, index.bind(
				&autoDetectionRetryTestCommands{submitErr: test.submitErr},
				func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
					return jobmgr.WorkPlan{}, test.planningErr
				},
				1,
				func(err error) {
					failed <- err
				},
			))
			index.schedule(autoDetectionRetryTestConfig("job"), 1)
			index.advance(0)
			index.advance(1)

			wantErr := test.planningErr
			if wantErr == nil {
				wantErr = test.submitErr
			}
			select {
			case err := <-failed:
				require.ErrorIs(t, err, wantErr)
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "retry dispatch failure was not reported")
			}
			require.ErrorIs(t, index.wait(context.Background()), wantErr)
			index.stopWorker()
		})
	}
}

func TestAutoDetectionRetryWaitRequiresWorkerJoin(t *testing.T) {
	release := make(chan struct{})
	commands := &autoDetectionRetryTestCommands{block: release}
	index := newAutoDetectionRetryIndex()
	require.NoError(t, index.bind(
		commands,
		func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
			return jobmgr.WorkPlan{}, nil
		},
		1,
		func(error) {},
	))
	index.schedule(autoDetectionRetryTestConfig("job"), 1)
	index.advance(0)
	index.advance(1)
	commands.waitForSubmissions(t, 1)
	index.stopWorker()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, index.wait(ctx), context.Canceled)
	select {
	case <-index.done:
		require.FailNow(t, "test failed", "retry worker joined while blocked")
	default:
	}

	close(release)
	require.NoError(t, index.wait(context.Background()))
	select {
	case <-index.done:
	default:
		require.FailNow(t, "test failed", "retry worker did not join")
	}
}

func TestAutoDetectionRetryClassifiesStoppingSubmission(t *testing.T) {
	sentinel := errors.New("structural failure")
	tests := map[string]struct {
		submitErr   error
		wantFailure bool
	}{
		"exact current stopping token is clean": {submitErr: &lifecycle.StoppingRejection{Generation: 1}},
		"wrong generation is structural": {
			submitErr:   &lifecycle.StoppingRejection{Generation: 2},
			wantFailure: true,
		},
		"joined structural error is not hidden": {
			submitErr:   errors.Join(&lifecycle.StoppingRejection{Generation: 1}, sentinel),
			wantFailure: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			failed := make(chan error, 1)
			index := newAutoDetectionRetryIndex()
			require.NoError(t, index.bind(
				&autoDetectionRetryTestCommands{submitErr: test.submitErr},
				func(confgroup.Config, autoDetectionRetryToken) (jobmgr.WorkPlan, error) {
					return jobmgr.WorkPlan{}, nil
				},
				1,
				func(err error) {
					failed <- err
				},
			))
			index.schedule(autoDetectionRetryTestConfig("job"), 1)
			index.advance(0)
			index.advance(1)

			waitErr := index.wait(context.Background())
			if test.wantFailure {
				require.ErrorIs(t, waitErr, test.submitErr)
				select {
				case err := <-failed:
					require.ErrorIs(t, err, test.submitErr)
				case <-time.After(time.Second):
					require.FailNow(t, "test failed", "retry failure was not reported")
				}
			} else {
				require.NoError(t, waitErr)
				select {
				case err := <-failed:
					require.FailNowf(t, "test failed", "stopping submission was reported as a failure: %v", err)
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

func (artc *autoDetectionRetryTestCommands) SubmitPrepared(
	_ context.Context,
	request jobmgr.Request,
	plan jobmgr.WorkPlan,
) error {
	artc.mu.Lock()
	artc.submitted = append(artc.submitted, request)
	artc.plans = append(artc.plans, plan)
	if artc.notify == nil {
		artc.notify = make(chan struct{}, 1)
	}
	notify := artc.notify
	block := artc.block
	err := artc.submitErr
	artc.mu.Unlock()
	select {
	case notify <- struct{}{}:
	default:
	}
	if block != nil {
		<-block
	}
	return err
}

func (artc *autoDetectionRetryTestCommands) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	artc.mu.Lock()
	artc.waited = true
	artc.mu.Unlock()
	return nil
}

func (artc *autoDetectionRetryTestCommands) snapshot() ([]jobmgr.Request, []jobmgr.WorkPlan, bool) {
	artc.mu.Lock()
	defer artc.mu.Unlock()
	return append([]jobmgr.Request(nil), artc.submitted...), append([]jobmgr.WorkPlan(nil), artc.plans...), artc.waited
}

func (artc *autoDetectionRetryTestCommands) waitForSubmissions(t *testing.T, want int) {
	t.Helper()
	if want == 0 {
		select {
		case <-artc.notification():
			require.FailNow(t, "test failed", "unexpected retry submission")
		case <-time.After(20 * time.Millisecond):
		}
		return
	}
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		artc.mu.Lock()
		got := len(artc.submitted)
		artc.mu.Unlock()
		if got >= want {
			return
		}
		select {
		case <-artc.notification():
		case <-timeout.C:
			require.FailNowf(t, "test failed", "retry submissions=%d want=%d", got, want)
		}
	}
}

func (artc *autoDetectionRetryTestCommands) notification() <-chan struct{} {
	artc.mu.Lock()
	defer artc.mu.Unlock()
	if artc.notify == nil {
		artc.notify = make(chan struct{}, 1)
	}
	return artc.notify
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
