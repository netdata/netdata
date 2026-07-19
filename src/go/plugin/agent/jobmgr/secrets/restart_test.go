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
	"gopkg.in/yaml.v2"
)

type restartTestCommandScope struct {
	normalErr     error
	rollbackErr   error
	rollbackCalls int
}

func (scope *restartTestCommandScope) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return scope.normalErr
}

func (scope *restartTestCommandScope) SubmitRollbackAndWait(
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	scope.rollbackCalls++
	return scope.rollbackErr
}

func (*restartTestCommandScope) RollbackContext() (
	context.Context,
	error,
) {
	return context.Background(), nil
}

type restartTestStop struct {
	stopped bool
}

func (stop restartTestStop) Stopped() (bool, error) {
	return stop.stopped, nil
}

type restartTestStart struct {
	err error
}

func (start restartTestStart) Err() error {
	return start.err
}

type restartTestJobs struct {
	stopError    error
	restoreError error
}

func (jobs restartTestJobs) PlanDependentStop(
	id string,
) (jobmgr.WorkPlan, DependentStopResult, error) {
	if id == "module_two" {
		return jobmgr.WorkPlan{}, nil, jobs.stopError
	}
	return jobmgr.WorkPlan{},
		restartTestStop{stopped: true},
		nil
}

func (jobs restartTestJobs) PlanDependentStart(
	string,
) (jobmgr.WorkPlan, DependentStartResult, error) {
	return jobmgr.WorkPlan{},
		restartTestStart{err: jobs.restoreError},
		nil
}

func TestSecretRestartCommandCommitsWithoutDependentsOrCompositeScope(
	t *testing.T,
) {
	command, err := NewSecretRestartCommand(
		1,
		NewSecretDependencyIndex(),
		restartTestJobs{},
	)
	if err != nil {
		t.Fatal(err)
	}
	commits := 0
	result, message, restored, err := command.Apply(
		context.Background(),
		nil,
		"vault:main",
		func(context.Context) (
			secretstore.SecretMutationResult,
			error,
		) {
			commits++
			return secretstore.SecretMutationResult{
				Generation: 1,
				Applied:    true,
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if commits != 1 {
		t.Fatalf("Store commits=%d want=1", commits)
	}
	if !result.Applied || message != "" || !restored {
		t.Fatalf(
			"no-dependent result=%+v message=%q restored=%v",
			result,
			message,
			restored,
		)
	}
}

func TestSecretRestartCommandReportsFailedPrecommitRestoration(
	t *testing.T,
) {
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
		if err != nil {
			t.Fatal(err)
		}
		commit, err := index.PrepareJobChange(
			config.FullName(),
			&dyncfg.GraphConfig{
				ID: config.FullName(), Module: config.Module(),
				Name: config.Name(), Status: dyncfg.StatusRunning.String(),
				Payload: payload,
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		commit()
	}
	command, err := NewSecretRestartCommand(
		1,
		index,
		restartTestJobs{
			stopError: stopError, restoreError: restoreError,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	commitCalled := false
	_, _, restored, err := command.Apply(
		context.Background(),
		&restartTestCommandScope{},
		"vault:main",
		func(context.Context) (
			secretstore.SecretMutationResult,
			error,
		) {
			commitCalled = true
			return secretstore.SecretMutationResult{}, nil
		},
	)
	if !errors.Is(err, stopError) ||
		!errors.Is(err, restoreError) {
		t.Fatalf("restart error=%v", err)
	}
	if restored {
		t.Fatal("failed dependent restoration was reported as complete")
	}
	if commitCalled {
		t.Fatal("Store commit ran after dependent stop failure")
	}
}

func TestSecretRestartCommandRestoresStopAcknowledgedDuringCancellation(
	t *testing.T,
) {
	index := NewSecretDependencyIndex()
	config := confgroup.Config{
		"module": "module",
		"name":   "one",
		"secret": "${store:vault:main:value}",
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	commitDependency, err := index.PrepareJobChange(
		config.FullName(),
		&dyncfg.GraphConfig{
			ID: config.FullName(), Module: config.Module(),
			Name: config.Name(), Status: dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	commitDependency()
	command, err := NewSecretRestartCommand(
		1,
		index,
		restartTestJobs{},
	)
	if err != nil {
		t.Fatal(err)
	}
	scope := &restartTestCommandScope{
		normalErr: context.Canceled,
	}
	commitCalled := false
	_, _, restored, err := command.Apply(
		context.Background(),
		scope,
		"vault:main",
		func(context.Context) (
			secretstore.SecretMutationResult,
			error,
		) {
			commitCalled = true
			return secretstore.SecretMutationResult{}, nil
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("restart error=%v", err)
	}
	if !restored {
		t.Fatal("acknowledged stop was not restored")
	}
	if scope.rollbackCalls != 1 {
		t.Fatalf(
			"rollback starts=%d want=1",
			scope.rollbackCalls,
		)
	}
	if commitCalled {
		t.Fatal("Store commit ran after cancelled dependent stop")
	}
}

func TestSecretRestartCommandRedactsAppliedRestartFailure(
	t *testing.T,
) {
	sensitive := errors.New(
		"collector initialization exposed backend-sensitive-detail",
	)
	index := NewSecretDependencyIndex()
	config := confgroup.Config{
		"module": "module",
		"name":   "one",
		"secret": "${store:vault:main:value}",
	}
	payload, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	commitDependency, err := index.PrepareJobChange(
		config.FullName(),
		&dyncfg.GraphConfig{
			ID: config.FullName(), Module: config.Module(),
			Name: config.Name(), Status: dyncfg.StatusRunning.String(),
			Payload: payload,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	commitDependency()
	command, err := NewSecretRestartCommand(
		1,
		index,
		restartTestJobs{restoreError: sensitive},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, message, _, err := command.Apply(
		context.Background(),
		&restartTestCommandScope{},
		"vault:main",
		func(context.Context) (
			secretstore.SecretMutationResult,
			error,
		) {
			return secretstore.SecretMutationResult{
				Generation: 1,
				Applied:    true,
			}, nil
		},
	)
	if !result.Applied || err != nil {
		t.Fatalf(
			"restart result=%+v error=%v",
			result,
			err,
		)
	}
	if strings.Contains(message, "backend-sensitive-detail") ||
		!strings.Contains(message, "module:one") {
		t.Fatalf("public restart message=%q", message)
	}
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
			b.Fatal(err)
		}
		commit, err := index.PrepareJobChange(
			config.FullName(),
			&dyncfg.GraphConfig{
				ID: config.FullName(), Module: config.Module(),
				Name: name, Status: dyncfg.StatusRunning.String(),
				Payload: payload,
			},
		)
		if err != nil {
			b.Fatal(err)
		}
		commit()
	}
	command, err := NewSecretRestartCommand(
		1,
		index,
		restartTestJobs{},
	)
	if err != nil {
		b.Fatal(err)
	}
	scope := &restartTestCommandScope{}
	commit := func(context.Context) (
		secretstore.SecretMutationResult,
		error,
	) {
		return secretstore.SecretMutationResult{
			Generation: 1,
			Applied:    true,
		}, nil
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, message, _, err := command.Apply(
			context.Background(),
			scope,
			"vault:main",
			commit,
		)
		if err != nil || !result.Applied || message != "" {
			b.Fatalf(
				"restart result=%+v message=%q error=%v",
				result,
				message,
				err,
			)
		}
	}
}
