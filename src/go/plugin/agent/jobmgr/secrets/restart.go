// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

// SecretRestartCommand serializes acknowledged dependent stop, Store commit,
// and successor start operations through their real resource lanes.
type SecretRestartCommand struct {
	mu sync.Mutex

	epoch        uint64
	dependencies *SecretDependencyIndex
	jobs         DependentJobPort
	nextUID      uint64
}

func NewSecretRestartCommand(
	epoch uint64,
	dependencies *SecretDependencyIndex,
	jobs DependentJobPort,
) (*SecretRestartCommand, error) {
	if epoch == 0 ||
		dependencies == nil ||
		jobs == nil {
		return nil, errors.New(
			"jobmgr secrets: incomplete restart command",
		)
	}
	return &SecretRestartCommand{
		epoch: epoch, dependencies: dependencies,
		jobs: jobs,
	}, nil
}

func (command *SecretRestartCommand) HasAffectedJobs(
	storeKey string,
) bool {
	return command != nil &&
		len(command.dependencies.Affected(storeKey, true)) != 0
}

func (command *SecretRestartCommand) Apply(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
	storeKey string,
	commit func(context.Context) (
		secretstore.SecretMutationResult,
		error,
	),
) (secretstore.SecretMutationResult, string, bool, error) {
	if command == nil ||
		ctx == nil ||
		commands == nil ||
		storeKey == "" ||
		commit == nil {
		return secretstore.SecretMutationResult{}, "", false,
			errors.New("jobmgr secrets: invalid restart command")
	}
	refs := command.dependencies.Affected(storeKey, true)
	displayByID := make(map[string]string, len(refs))
	stopped := make([]string, 0, len(refs))
	for _, ref := range refs {
		displayByID[ref.ID] = ref.Display
		plan, state, err :=
			command.jobs.PlanDependentStop(ref.ID)
		if err != nil {
			restoreErr := command.restore(commands, stopped)
			return secretstore.SecretMutationResult{}, "",
				restoreErr == nil,
				errors.Join(err, restoreErr)
		}
		submitErr := command.submit(
			ctx,
			commands,
			ref.ID,
			"stop",
			plan,
			false,
		)
		didStop, stateErr := state.Stopped()
		if stateErr == nil && didStop {
			stopped = append(stopped, ref.ID)
		}
		if submitErr != nil || stateErr != nil {
			restoreErr := command.restore(commands, stopped)
			return secretstore.SecretMutationResult{}, "",
				stateErr == nil && restoreErr == nil,
				errors.Join(submitErr, stateErr, restoreErr)
		}
	}

	result, commitErr := commit(ctx)
	if !result.Applied {
		restoreErr := command.restore(commands, stopped)
		return result, "",
			!result.Retained && restoreErr == nil,
			errors.Join(commitErr, restoreErr)
	}

	failures, startErr := command.start(
		commands,
		stopped,
		displayByID,
	)
	message := ""
	if len(failures) != 0 {
		message = "Secretstore change applied, but dependent collector restarts failed for jobs: " +
			formatSecretJobNames(failures) + "."
	}
	return result, message, false,
		errors.Join(commitErr, startErr)
}

func (command *SecretRestartCommand) restore(
	commands jobmgr.CompositeCommandScope,
	ids []string,
) error {
	_, err := command.start(commands, ids, nil)
	return err
}

func (command *SecretRestartCommand) start(
	commands jobmgr.CompositeCommandScope,
	ids []string,
	displayByID map[string]string,
) ([]string, error) {
	failures := make([]string, 0)
	var result error
	for _, id := range ids {
		display := displayByID[id]
		if display == "" {
			display = id
		}
		plan, state, err :=
			command.jobs.PlanDependentStart(id)
		if err != nil {
			result = errors.Join(result, err)
			failures = append(failures, display)
			continue
		}
		if err := command.submit(
			context.Background(),
			commands,
			id,
			"start",
			plan,
			true,
		); err != nil {
			result = errors.Join(result, err)
			failures = append(failures, display)
			continue
		}
		if startErr := state.Err(); startErr != nil {
			result = errors.Join(result, startErr)
			failures = append(failures, display)
		}
	}
	return failures, result
}

func (command *SecretRestartCommand) submit(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
	id string,
	phase string,
	plan jobmgr.WorkPlan,
	rollback bool,
) error {
	command.mu.Lock()
	command.nextUID++
	sequence := command.nextUID
	command.mu.Unlock()
	if sequence == 0 {
		return errors.New(
			"jobmgr secrets: restart command identity wrapped",
		)
	}
	request := jobmgr.Request{
		UID: fmt.Sprintf(
			"jobmgr-secret-%d-%d",
			command.epoch,
			sequence,
		),
		LaneKey: id,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/secrets/" + phase,
	}
	if rollback {
		return commands.SubmitRollbackAndWait(request, plan)
	}
	return commands.SubmitPreparedAndWait(ctx, request, plan)
}
