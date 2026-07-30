// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type restartTestCommandScope struct {
	normalErr     error
	recoveryErr   error
	recovery      func(context.Context) error
	recoveryCalls int
	recoveryCtx   []context.Context
}

func (rtcs *restartTestCommandScope) SubmitPreparedAndWait(context.Context, jobmgr.Request, jobmgr.WorkPlan) error {
	return rtcs.normalErr
}

func (rtcs *restartTestCommandScope) SubmitRecoveryAndWait(
	ctx context.Context,
	_ jobmgr.Request,
	_ jobmgr.WorkPlan,
) error {
	rtcs.recoveryCalls++
	rtcs.recoveryCtx = append(rtcs.recoveryCtx, ctx)
	if rtcs.recovery != nil {
		return rtcs.recovery(ctx)
	}
	return rtcs.recoveryErr
}

type restartTestStop struct {
	stopped bool
}

func (rts restartTestStop) Stopped() (bool, error) {
	return rts.stopped, nil
}

type restartTestStart struct {
	err             error
	retainedPending int
}

func (rts *restartTestStart) Err() error {
	return rts.err
}

func (rts *restartTestStart) RetainPending() {
	rts.retainedPending++
}

type restartTestJobs struct {
	stopError    error
	restoreError error
	start        *restartTestStart
}

func (rtj restartTestJobs) PlanDependentStop(id string) (jobmgr.WorkPlan, DependentStopResult, error) {
	if id == "module_two" {
		return jobmgr.WorkPlan{}, nil, rtj.stopError
	}
	return jobmgr.WorkPlan{}, restartTestStop{
		stopped: true,
	}, nil
}

func (rtj restartTestJobs) PlanDependentStart(string) (jobmgr.WorkPlan, DependentStartResult, error) {
	if rtj.start == nil {
		rtj.start = &restartTestStart{err: rtj.restoreError}
	}
	return jobmgr.WorkPlan{}, rtj.start, nil
}

func TestSecretRestartCommandCommitsWithoutDependentsOrCompositeScope(t *testing.T) {
	command, err := NewSecretRestartCommand(1, NewSecretDependencyIndex(), restartTestJobs{}, nil)
	require.NoError(t, err)
	commits := 0
	result, message, restored, err := command.Apply(
		context.Background(),
		nil,
		"vault:main",
		func(context.Context) (secretstore.SecretMutationResult, error) {
			commits++
			return secretstore.SecretMutationResult{
				Generation: 1,
				Applied:    true,
			}, nil
		},
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, commits)
	require.False(t, !result.Applied || message != "" || !restored)
}

func TestSecretRestartCommandReportsFailedPrecommitRestoration(t *testing.T) {
	stopError := errors.New("second dependent stop failed")
	restoreError := errors.New("first dependent restore failed")
	index := NewSecretDependencyIndex()
	for _, name := range []string{"one", "two"} {
		config := confgroup.Config{
			"module": "module",
			"name":   name,
			"secret": "${store:vault:main:value}",
		}
		payload, err := yaml.Marshal(config)
		require.NoError(t, err)
		commit, err := index.PrepareJobChange(
			config.FullName(),
			&dyncfg.GraphConfig{
				ID:      config.FullName(),
				Module:  config.Module(),
				Name:    config.Name(),
				Status:  dyncfg.StatusRunning.String(),
				Payload: payload,
			},
		)
		require.NoError(t, err)
		commit()
	}
	command, err := NewSecretRestartCommand(
		1,
		index,
		restartTestJobs{
			stopError:    stopError,
			restoreError: restoreError,
		},
		nil,
	)
	require.NoError(t, err)
	commitCalled := false
	_, _, restored, err := command.Apply(
		context.Background(),
		&restartTestCommandScope{},
		"vault:main",
		func(context.Context) (secretstore.SecretMutationResult, error) {
			commitCalled = true
			return secretstore.SecretMutationResult{}, nil
		},
	)
	require.False(t, !errors.Is(err, stopError) || !errors.Is(err, restoreError))
	require.False(t, restored)
	require.False(t, commitCalled)
}

func TestSecretRestartCommandRestoresStopAcknowledgedDuringCancellation(t *testing.T) {
	index := NewSecretDependencyIndex()
	config := confgroup.Config{
		"module": "module",
		"name":   "one",
		"secret": "${store:vault:main:value}",
	}
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	commitDependency, err := index.PrepareJobChange(
		config.FullName(),
		&dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	)
	require.NoError(t, err)
	commitDependency()
	command, err := NewSecretRestartCommand(1, index, restartTestJobs{}, nil)
	require.NoError(t, err)
	scope := &restartTestCommandScope{
		normalErr: context.Canceled,
	}
	commitCalled := false
	_, _, restored, err := command.Apply(
		context.Background(),
		scope,
		"vault:main",
		func(context.Context) (secretstore.SecretMutationResult, error) {
			commitCalled = true
			return secretstore.SecretMutationResult{}, nil
		},
	)
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, restored)
	require.EqualValues(t, 1, scope.recoveryCalls)
	require.False(t, commitCalled)
}

func TestSecretRestartTimeoutAfterAppliedMutationIsOperational(t *testing.T) {
	index := NewSecretDependencyIndex()
	config := confgroup.Config{
		"module": "module",
		"name":   "one",
		"secret": "${store:vault:main:value}",
	}
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	commitDependency, err := index.PrepareJobChange(
		config.FullName(),
		&dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	)
	require.NoError(t, err)
	commitDependency()

	command, err := NewSecretRestartCommand(1, index, restartTestJobs{}, nil)
	require.NoError(t, err)
	command.childTimeout = 10 * time.Millisecond
	command.budgetLimit = 20 * time.Millisecond
	start := &restartTestStart{}
	command.jobs = restartTestJobs{start: start}
	scope := &restartTestCommandScope{
		recovery: func(ctx context.Context) error {
			<-ctx.Done()
			return context.Cause(ctx)
		},
	}
	result, message, restored, err := command.Apply(
		context.Background(),
		scope,
		"vault:main",
		func(context.Context) (secretstore.SecretMutationResult, error) {
			return secretstore.SecretMutationResult{
				Generation: 1,
				Applied:    true,
			}, nil
		},
	)

	require.NoError(t, err)
	require.True(t, result.Applied)
	require.False(t, restored)
	require.Contains(t, message, "module:one")
	require.EqualValues(t, 1, scope.recoveryCalls)
	require.ErrorIs(t, scope.recoveryCtx[0].Err(), context.DeadlineExceeded)
	require.EqualValues(t, 1, start.retainedPending)
}

func TestSecretRestartBudgetsCapAggregateAndFairShareChildren(t *testing.T) {
	command := &SecretRestartCommand{
		childTimeout: 100 * time.Millisecond,
		budgetLimit:  150 * time.Millisecond,
	}
	require.Equal(t, 100*time.Millisecond, command.aggregateRestartBudget(1))
	require.Equal(t, 150*time.Millisecond, command.aggregateRestartBudget(2))
	require.Equal(t, 150*time.Millisecond, command.aggregateRestartBudget(100))

	aggregate, cancelAggregate := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelAggregate()
	first, cancelFirst := command.childRestartContext(aggregate, 2)
	defer cancelFirst()
	second, cancelSecond := command.childRestartContext(aggregate, 1)
	defer cancelSecond()
	firstDeadline, firstOK := first.Deadline()
	secondDeadline, secondOK := second.Deadline()
	require.True(t, firstOK && secondOK)
	aggregateDeadline, aggregateOK := aggregate.Deadline()
	require.True(t, aggregateOK)
	require.WithinDuration(t, aggregateDeadline.Add(-75*time.Millisecond), firstDeadline, 10*time.Millisecond)
	require.WithinDuration(t, aggregateDeadline.Add(-50*time.Millisecond), secondDeadline, 10*time.Millisecond)
}

func TestSecretRestartCommandRedactsAppliedRestartFailure(t *testing.T) {
	sensitive := errors.New("collector initialization exposed backend-sensitive-detail")
	index := NewSecretDependencyIndex()
	config := confgroup.Config{
		"module": "module",
		"name":   "one",
		"secret": "${store:vault:main:value}",
	}
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	commitDependency, err := index.PrepareJobChange(
		config.FullName(),
		&dyncfg.GraphConfig{
			ID:      config.FullName(),
			Module:  config.Module(),
			Name:    config.Name(),
			Status:  dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	)
	require.NoError(t, err)
	commitDependency()
	diagnostics := &secretRecordingDiagnosticObserver{}
	command, err := NewSecretRestartCommand(1, index, restartTestJobs{
		restoreError: sensitive,
	}, diagnostics)
	require.NoError(t, err)
	result, message, _, err := command.Apply(
		context.Background(),
		&restartTestCommandScope{},
		"vault:main",
		func(context.Context) (secretstore.SecretMutationResult, error) {
			return secretstore.SecretMutationResult{
				Generation: 1,
				Applied:    true,
			}, nil
		},
	)
	require.False(t, !result.Applied || err != nil)
	require.False(t, strings.Contains(message, "backend-sensitive-detail") || !strings.Contains(message, "module:one"))
	events := diagnostics.snapshot()
	var warning jobmgr.DiagnosticEvent
	for _, event := range events {
		if event.Name == "secretstore dependent collector restart failed" {
			warning = event
			break
		}
	}
	require.Equal(t, jobmgr.DiagnosticWarning, warning.Level)
	require.Equal(t, "secretstore:vault_main", warning.Resource)
	require.EqualValues(t, 1, warning.Count)
	require.NotContains(t, fmt.Sprintf("%+v", events), "module:one")
	require.NotContains(t, fmt.Sprintf("%+v", events), "backend-sensitive-detail")
}

func BenchmarkBSecretRestart(b *testing.B) {
	index := NewSecretDependencyIndex()
	const dependents = 16
	for job := range dependents {
		name := fmt.Sprintf("job-%d", job)
		config := confgroup.Config{
			"module": "module",
			"name":   name,
			"secret": "${store:vault:main:value}",
		}
		payload, err := yaml.Marshal(config)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		commit, err := index.PrepareJobChange(
			config.FullName(),
			&dyncfg.GraphConfig{
				ID:      config.FullName(),
				Module:  config.Module(),
				Name:    name,
				Status:  dyncfg.StatusRunning.String(),
				Payload: payload,
			},
		)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		commit()
	}
	command, err := NewSecretRestartCommand(1, index, restartTestJobs{}, nil)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	scope := &restartTestCommandScope{}
	commit := func(context.Context) (secretstore.SecretMutationResult, error) {
		return secretstore.SecretMutationResult{
			Generation: 1,
			Applied:    true,
		}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, message, _, err := command.Apply(context.Background(), scope, "vault:main", commit)
		if err != nil || !result.Applied || message != "" {
			require.FailNowf(b, "benchmark failed", "restart result=%+v message=%q error=%v", result, message, err)
		}
	}
}
