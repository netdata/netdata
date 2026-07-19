// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

type restartTestCommandPort struct{}

func (restartTestCommandPort) SubmitPrepared(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}

func (restartTestCommandPort) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}

type restartTestStop struct {
	config confgroup.Config
}

func (stop restartTestStop) Config() (
	confgroup.Config,
	bool,
	error,
) {
	return stop.config, true, nil
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
	return jobmgr.WorkPlan{}, restartTestStop{
		config: confgroup.Config{
			"module": "module",
			"name":   "one",
		},
	}, nil
}

func (jobs restartTestJobs) PlanDependentStart(
	confgroup.Config,
) (jobmgr.WorkPlan, DependentStartResult, error) {
	return jobmgr.WorkPlan{},
		restartTestStart{err: jobs.restoreError},
		nil
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
		restartTestCommandPort{},
	)
	if err != nil {
		t.Fatal(err)
	}
	commitCalled := false
	_, _, err = command.Apply(
		context.Background(),
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
	if commitCalled {
		t.Fatal("Store commit ran after dependent stop failure")
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
		restartTestCommandPort{},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, message, err := command.Apply(
		context.Background(),
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
	if !result.Applied || !errors.Is(err, sensitive) {
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
