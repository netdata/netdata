// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

// SecretRestartCommand serializes acknowledged dependent stop, Store commit,
// and successor start operations through their real resource lanes.
type SecretRestartCommand struct {
	mu sync.Mutex

	epoch        uint64
	dependencies *SecretDependencyIndex
	jobs         DependentJobPort
	commands     CommandPort
	nextUID      uint64
}

func NewSecretRestartCommand(
	epoch uint64,
	dependencies *SecretDependencyIndex,
	jobs DependentJobPort,
	commands CommandPort,
) (*SecretRestartCommand, error) {
	if epoch == 0 ||
		dependencies == nil ||
		jobs == nil ||
		commands == nil {
		return nil, errors.New(
			"jobmgr secrets: incomplete restart command",
		)
	}
	return &SecretRestartCommand{
		epoch: epoch, dependencies: dependencies,
		jobs: jobs, commands: commands,
	}, nil
}

func (command *SecretRestartCommand) Apply(
	ctx context.Context,
	storeKey string,
	commit func(context.Context) (
		secretstore.SecretMutationResult,
		error,
	),
) (secretstore.SecretMutationResult, string, error) {
	if command == nil ||
		ctx == nil ||
		storeKey == "" ||
		commit == nil {
		return secretstore.SecretMutationResult{}, "",
			errors.New("jobmgr secrets: invalid restart command")
	}
	refs := command.dependencies.Affected(storeKey, true)
	stopped := make([]confgroup.Config, 0, len(refs))
	for _, ref := range refs {
		plan, state, err :=
			command.jobs.PlanDependentStop(ref.ID)
		if err != nil {
			return secretstore.SecretMutationResult{}, "",
				errors.Join(err, command.restore(ctx, stopped))
		}
		if err := command.submit(
			ctx,
			ref.ID,
			"stop",
			plan,
		); err != nil {
			return secretstore.SecretMutationResult{}, "",
				errors.Join(err, command.restore(ctx, stopped))
		}
		config, didStop, err := state.Config()
		if err != nil {
			return secretstore.SecretMutationResult{}, "",
				errors.Join(err, command.restore(ctx, stopped))
		}
		if didStop {
			stopped = append(stopped, config)
		}
	}

	result, commitErr := commit(ctx)
	if !result.Applied {
		return result, "", errors.Join(
			commitErr,
			command.restore(ctx, stopped),
		)
	}

	failures, startErr := command.start(ctx, stopped)
	message := ""
	if len(failures) != 0 {
		message = "Secretstore change applied, but dependent collector restarts failed for jobs: " +
			strings.Join(failures, "; ") + "."
	}
	return result, message, errors.Join(commitErr, startErr)
}

func (command *SecretRestartCommand) restore(
	ctx context.Context,
	configs []confgroup.Config,
) error {
	_, err := command.start(ctx, configs)
	return err
}

func (command *SecretRestartCommand) start(
	ctx context.Context,
	configs []confgroup.Config,
) ([]string, error) {
	failures := make([]string, 0)
	var result error
	for _, config := range configs {
		display := config.Module() + ":" + config.Name()
		plan, state, err :=
			command.jobs.PlanDependentStart(config)
		if err != nil {
			result = errors.Join(result, err)
			failures = append(failures, display)
			continue
		}
		if err := command.submit(
			ctx,
			config.FullName(),
			"start",
			plan,
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
	id string,
	phase string,
	plan jobmgr.WorkPlan,
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
	return command.commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-secret-%d-%d",
				command.epoch,
				sequence,
			),
			LaneKey: id,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/secrets/" + phase,
		},
		plan,
	)
}
