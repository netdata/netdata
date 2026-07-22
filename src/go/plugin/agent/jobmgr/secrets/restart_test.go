// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type restartTestCommandScope struct {
	normalErr          error
	rollbackErr        error
	rollbackContextErr error
	rollbackCalls      int
}

func (rtcs *restartTestCommandScope) SubmitPreparedAndWait(context.Context, jobmgr.Request, jobmgr.WorkPlan) error {
	return rtcs.normalErr
}

func (rtcs *restartTestCommandScope) SubmitRollbackAndWait(jobmgr.Request, jobmgr.WorkPlan) error {
	rtcs.rollbackCalls++
	return rtcs.rollbackErr
}

func (rtcs *restartTestCommandScope) RollbackContext() (context.Context, error) {
	if rtcs.rollbackContextErr != nil {
		return nil, rtcs.rollbackContextErr
	}
	return context.Background(), nil
}

type restartTestStop struct {
	stopped bool
}

func (rts restartTestStop) Stopped() (bool, error) {
	return rts.stopped, nil
}

type restartTestStart struct {
	err error
}

func (rts restartTestStart) Err() error {
	return rts.err
}

type restartTestJobs struct {
	stopError    error
	restoreError error
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
	return jobmgr.WorkPlan{}, restartTestStart{
		err: rtj.restoreError,
	}, nil
}

func TestSecretRestartCommandCommitsWithoutDependentsOrCompositeScope(t *testing.T) {
	command, err := NewSecretRestartCommand(1, NewSecretDependencyIndex(), restartTestJobs{})
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
	command, err := NewSecretRestartCommand(1, index, restartTestJobs{})
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
	require.EqualValues(t, 1, scope.rollbackCalls)
	require.False(t, commitCalled)
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
	command, err := NewSecretRestartCommand(1, index, restartTestJobs{
		restoreError: sensitive,
	})
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
	command, err := NewSecretRestartCommand(1, index, restartTestJobs{})
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
