// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type modulePlanOwner struct {
	once sync.Once
	done chan struct{}
}

func newModulePlanOwner() *modulePlanOwner {
	return &modulePlanOwner{
		done: make(chan struct{}),
	}
}

func (owner *modulePlanOwner) Release() {
	if owner != nil {
		owner.once.Do(func() {
			close(owner.done)
		})
	}
}

type modulePlanStage struct {
	module  string
	attempt jobmgr.ProcessAttempt
	result  chan modulePlanResult
	settled chan error
}

type modulePlanResult struct {
	plan controllerModulePlan
	err  error
}

func prepareContainedModulePlans(
	ctx context.Context,
	epoch uint64,
	attempts jobmgr.ProcessAttemptAuthority,
	modules collectorapi.Registry,
) (map[string]controllerModulePlan, error) {
	if ctx == nil || epoch == 0 || attempts == nil || modules == nil {
		return nil, errors.New("jobmgr Function controller: invalid contained module preparation")
	}
	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	slices.Sort(names)
	stages := make([]modulePlanStage, 0, len(names))
	for _, module := range names {
		stage, err := startModulePlanStage(epoch, attempts, module, modules[module])
		if err != nil {
			releaseModulePlanStages(stages)
			return nil, err
		}
		stages = append(stages, stage)
	}

	plans := make(map[string]controllerModulePlan, len(stages))
	for index := range stages {
		stage := &stages[index]
		var result modulePlanResult
		select {
		case result = <-stage.result:
		case err := <-stage.settled:
			result.err = err
		case <-ctx.Done():
			stage.attempt.Cut(ctx.Err())
			result.err = ctx.Err()
		}
		if result.err != nil {
			for _, plan := range plans {
				plan.owner.Release()
			}
			releaseModulePlanStages(stages[index:])
			return nil, result.err
		}
		plans[stage.module] = result.plan
	}
	return plans, nil
}

func startModulePlanStage(
	epoch uint64,
	attempts jobmgr.ProcessAttemptAuthority,
	module string,
	creator collectorapi.Creator,
) (modulePlanStage, error) {
	result := make(chan modulePlanResult, 1)
	settled := make(chan error, 1)
	attemptReady := make(chan jobmgr.ProcessAttempt, 1)
	attempt, err := attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionBundle,
			Key:       fmt.Sprintf("%d/%s/agent", epoch, module),
			Resource:  candidateFunctionResource(module),
		},
		Target: epoch,
		Work: func(context.Context) error {
			owned := <-attemptReady
			plan, buildErr := buildControllerModulePlan(module, creator)
			if buildErr != nil {
				result <- modulePlanResult{err: buildErr}
				return buildErr
			}
			if bindErr := plan.agentBundle.bindContainment(
				attempts,
				epoch,
				fmt.Sprintf("%d/%s/agent", epoch, module),
				candidateFunctionResource(module),
			); bindErr != nil {
				plan.agentBundle.retire()
				cleanupErr := plan.agentBundle.wait(context.Background())
				result <- modulePlanResult{err: errors.Join(bindErr, cleanupErr)}
				return errors.Join(bindErr, cleanupErr)
			}
			if admitErr := owned.Admit(); admitErr != nil {
				plan.agentBundle.retire()
				cleanupErr := plan.agentBundle.wait(context.Background())
				return errors.Join(admitErr, cleanupErr)
			}
			if plan.agentBundle.handler == nil {
				result <- modulePlanResult{plan: plan}
				return nil
			}
			owner := newModulePlanOwner()
			plan.owner = owner
			result <- modulePlanResult{plan: plan}
			<-owner.done
			plan.agentBundle.retire()
			return plan.agentBundle.wait(context.Background())
		},
	})
	if err != nil {
		return modulePlanStage{}, err
	}
	attemptReady <- attempt
	go func() {
		settled <- attempt.Await(context.Background())
	}()
	return modulePlanStage{
		module:  module,
		attempt: attempt,
		result:  result,
		settled: settled,
	}, nil
}

func buildControllerModulePlan(
	module string,
	creator collectorapi.Creator,
) (controllerModulePlan, error) {
	var plan controllerModulePlan
	var err error
	if creator.AgentFunctions != nil {
		plan.agent, err = callModuleFunctions("AgentFunctions", creator.AgentFunctions)
		if err != nil {
			return controllerModulePlan{}, err
		}
		plan.agent, err = validateConfiguredMethods(module, plan.agent)
		if err != nil {
			return controllerModulePlan{}, err
		}
	}
	if creator.SharedFunctions != nil {
		plan.shared, err = callModuleFunctions("SharedFunctions", creator.SharedFunctions)
		if err != nil {
			return controllerModulePlan{}, err
		}
		plan.shared, err = validateConfiguredMethods(module, plan.shared)
		if err != nil {
			return controllerModulePlan{}, err
		}
	}
	plan.agentBundle, err = newAgentFunctionBundle(module, creator, plan.agent)
	if err != nil {
		return controllerModulePlan{}, err
	}
	return plan, nil
}

func releaseModulePlanStages(stages []modulePlanStage) {
	for index := range stages {
		stage := &stages[index]
		select {
		case result := <-stage.result:
			if result.plan.owner != nil {
				result.plan.owner.Release()
			}
		default:
			stage.attempt.Cut(context.Canceled)
		}
	}
}

func candidateFunctionResource(module string) string {
	if module == "" || len(module) > 256 {
		return "collector module"
	}
	for _, char := range module {
		if char < ' ' || char == 0x7f {
			return "collector module"
		}
	}
	return module
}
