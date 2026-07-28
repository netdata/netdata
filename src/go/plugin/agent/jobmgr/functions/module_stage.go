// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"unicode/utf8"

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
	plan     controllerModulePlan
	transfer *modulePlanTransfer
	err      error
}

type modulePlanTransfer struct {
	mu       sync.Mutex
	decided  chan struct{}
	accepted bool
	complete bool
}

func newModulePlanTransfer() *modulePlanTransfer {
	return &modulePlanTransfer{
		decided: make(chan struct{}),
	}
}

func (transfer *modulePlanTransfer) Accept() bool {
	return transfer.decide(true)
}

func (transfer *modulePlanTransfer) Abandon() {
	transfer.decide(false)
}

func (transfer *modulePlanTransfer) decide(accept bool) bool {
	if transfer == nil {
		return false
	}
	transfer.mu.Lock()
	defer transfer.mu.Unlock()
	if transfer.complete {
		return transfer.accepted
	}
	transfer.accepted = accept
	transfer.complete = true
	close(transfer.decided)
	return transfer.accepted
}

func (transfer *modulePlanTransfer) wasAccepted() bool {
	if transfer == nil {
		return false
	}
	<-transfer.decided
	transfer.mu.Lock()
	defer transfer.mu.Unlock()
	return transfer.accepted
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
		case settledErr := <-stage.settled:
			if settledErr != nil {
				result.err = settledErr
			} else {
				result = <-stage.result
			}
		case <-ctx.Done():
			stage.attempt.Cut(ctx.Err())
			result.err = ctx.Err()
		}
		result.err = acceptModulePlanResult(ctx, &result)
		if result.err != nil {
			cleanupErr := cleanupControllerModulePlans(plans)
			releaseModulePlanStages(stages[index:])
			return nil, errors.Join(result.err, cleanupErr)
		}
		plans[stage.module] = result.plan
	}
	return plans, nil
}

func acceptModulePlanResult(ctx context.Context, result *modulePlanResult) error {
	if ctx == nil || result == nil {
		return errors.New("jobmgr Function controller: invalid module-plan result")
	}
	if cause := context.Cause(ctx); cause != nil {
		if result.transfer != nil {
			result.transfer.Abandon()
		}
		return errors.Join(result.err, cause)
	}
	if result.err != nil {
		return result.err
	}
	if result.transfer != nil && !result.transfer.Accept() {
		return jobmgr.ErrProcessAttemptSettled
	}
	return nil
}

func startModulePlanStage(
	epoch uint64,
	attempts jobmgr.ProcessAttemptAuthority,
	module string,
	creator collectorapi.Creator,
) (modulePlanStage, error) {
	result := make(chan modulePlanResult, 1)
	settled := make(chan error, 1)
	attempt, err := attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptFunctionBundle,
			Key:       fmt.Sprintf("%d/%s/agent", epoch, module),
			Resource:  candidateFunctionResource(module),
		},
		Target: epoch,
		Work: func(ctx context.Context, admission jobmgr.ProcessAttemptAdmission) error {
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
			if admitErr := admission.Admit(); admitErr != nil {
				plan.agentBundle.retire()
				cleanupErr := plan.agentBundle.wait(context.Background())
				return errors.Join(admitErr, cleanupErr)
			}
			if plan.agentBundle.handler != nil {
				plan.owner = newModulePlanOwner()
			}
			transfer := newModulePlanTransfer()
			result <- modulePlanResult{
				plan:     plan,
				transfer: transfer,
			}
			select {
			case <-transfer.decided:
			case <-ctx.Done():
				transfer.Abandon()
				<-transfer.decided
			}
			if !transfer.wasAccepted() {
				plan.agentBundle.retire()
				return plan.agentBundle.wait(context.Background())
			}
			if plan.owner == nil {
				return nil
			}
			<-plan.owner.done
			plan.agentBundle.retire()
			return plan.agentBundle.wait(context.Background())
		},
	})
	if err != nil {
		return modulePlanStage{}, err
	}
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
		stage.attempt.Cut(context.Canceled)
		select {
		case result := <-stage.result:
			if result.transfer != nil {
				result.transfer.Abandon()
			}
		default:
		}
	}
}

func candidateFunctionResource(module string) string {
	if module == "" || len(module) > 256 || !utf8.ValidString(module) {
		return "collector module"
	}
	for _, char := range module {
		if char < ' ' || char == 0x7f {
			return "collector module"
		}
	}
	return module
}
