// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

const (
	defaultDependentRestartChildTimeout = 20 * time.Second
	maximumDependentRestartBudget       = 2 * time.Minute
)

// SecretRestartCommand serializes acknowledged dependent stop, Store commit,
// and successor start operations through their real resource lanes.
type SecretRestartCommand struct {
	mu sync.Mutex // guards nextUID

	epoch        uint64                    // run generation
	dependencies *SecretDependencyIndex    // secret dependency index
	jobs         DependentJobPort          // port used to stop/start dependent jobs
	diagnostics  jobmgr.DiagnosticObserver // operational log sink
	nextUID      uint64                    // next child-command UID to assign
	childTimeout time.Duration             // maximum recovery work time for one dependent
	budgetLimit  time.Duration             // aggregate recovery budget ceiling
}

func NewSecretRestartCommand(
	epoch uint64,
	dependencies *SecretDependencyIndex,
	jobs DependentJobPort,
	diagnostics jobmgr.DiagnosticObserver,
) (*SecretRestartCommand, error) {
	if epoch == 0 || dependencies == nil || jobs == nil {
		return nil, errors.New("jobmgr secrets: incomplete restart command")
	}
	return &SecretRestartCommand{
		epoch:        epoch,
		dependencies: dependencies,
		jobs:         jobs,
		diagnostics:  diagnostics,
		childTimeout: defaultDependentRestartChildTimeout,
		budgetLimit:  maximumDependentRestartBudget,
	}, nil
}

func (src *SecretRestartCommand) Apply(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
	storeKey string,
	commit func(context.Context) (secretstore.SecretMutationResult, error),
) (secretstore.SecretMutationResult, string, bool, error) {
	if src == nil || ctx == nil || storeKey == "" || commit == nil {
		return secretstore.SecretMutationResult{}, "", false, errors.New("jobmgr secrets: invalid restart command")
	}
	refs := src.dependencies.Affected(storeKey, true)
	if len(refs) == 0 {
		result, err := commit(ctx)
		return result, "", !result.Retained, err
	}
	if commands == nil {
		return secretstore.SecretMutationResult{}, "", false,
			errors.New("jobmgr secrets: affected restart lacks composite scope")
	}
	displayByID := make(map[string]string, len(refs))
	stopped := make([]string, 0, len(refs))
	for _, ref := range refs {
		displayByID[ref.ID] = ref.Display
		plan, state, err := src.jobs.PlanDependentStop(ref.ID)
		if err != nil {
			restoreErr := src.restore(commands, stopped)
			return secretstore.SecretMutationResult{}, "", restoreErr == nil, errors.Join(err, restoreErr)
		}
		submitErr := src.submit(ctx, commands, ref.ID, "stop", plan, false)
		didStop, stateErr := state.Stopped()
		if stateErr == nil && didStop {
			stopped = append(stopped, ref.ID)
		}
		if submitErr != nil || stateErr != nil {
			restoreErr := src.restore(commands, stopped)
			return secretstore.SecretMutationResult{}, "",
				stateErr == nil && restoreErr == nil,
				errors.Join(submitErr, stateErr, restoreErr)
		}
	}

	result, commitErr := commit(ctx)
	if !result.Applied {
		restoreErr := src.restore(commands, stopped)
		return result, "", !result.Retained && restoreErr == nil, errors.Join(commitErr, restoreErr)
	}

	// Operational restart failures are surfaced to the user via message (built
	// from failures) rather than as an error: a dependent job failing to restart
	// must not fail the already-applied secretstore change. Only the integrity
	// error (startErr) propagates.
	failures, startErr, _ := src.start(commands, stopped, displayByID)
	message := ""
	if len(failures) != 0 {
		message = "Secretstore change applied, but dependent collector restarts failed for jobs: " +
			formatSecretJobNames(failures) + "."
		jobmgr.ObserveDiagnostic(src.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticWarning,
			Name:       "secretstore dependent collector restart failed",
			Resource:   secretResourceID(storeKey),
			Generation: src.epoch,
			Count:      len(failures),
			Err:        errors.Join(commitErr, startErr),
		})
	}
	return result, message, false, errors.Join(commitErr, startErr)
}

func (src *SecretRestartCommand) restore(commands jobmgr.CompositeCommandScope, ids []string) error {
	_, integrityErr, operationalErr := src.start(commands, ids, nil)
	return errors.Join(integrityErr, operationalErr)
}

func (src *SecretRestartCommand) start(
	commands jobmgr.CompositeCommandScope,
	ids []string,
	displayByID map[string]string,
) ([]string, error, error) {
	failures := make([]string, 0)
	var integrityErr error
	var operationalErr error
	if len(ids) == 0 {
		return failures, nil, nil
	}
	recoveryCtx, cancelRecovery := context.WithTimeout(
		context.Background(),
		src.aggregateRestartBudget(len(ids)),
	)
	defer cancelRecovery()
	for index, id := range ids {
		display := displayByID[id]
		if display == "" {
			display = id
		}
		plan, state, err := src.jobs.PlanDependentStart(id)
		if err != nil {
			integrityErr = errors.Join(integrityErr, err)
			failures = append(failures, display)
			continue
		}
		childCtx, cancelChild := src.childRestartContext(recoveryCtx, len(ids)-index)
		err = src.submit(childCtx, commands, id, "start", plan, true)
		cancelChild()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) &&
				!errors.Is(err, jobmgr.ErrCompositeRecoveryUnresolved) {
				state.RetainPending()
				operationalErr = errors.Join(operationalErr, err)
			} else {
				integrityErr = errors.Join(integrityErr, err)
			}
			failures = append(failures, display)
			continue
		}
		if startErr := state.Err(); startErr != nil {
			operationalErr = errors.Join(operationalErr, startErr)
			failures = append(failures, display)
		}
	}
	return failures, integrityErr, operationalErr
}

func (src *SecretRestartCommand) aggregateRestartBudget(count int) time.Duration {
	if src == nil || count <= 0 || src.childTimeout <= 0 || src.budgetLimit <= 0 {
		return 0
	}
	if count > int(src.budgetLimit/src.childTimeout) {
		return src.budgetLimit
	}
	return min(time.Duration(count)*src.childTimeout, src.budgetLimit)
}

func (src *SecretRestartCommand) childRestartContext(
	parent context.Context,
	remainingChildren int,
) (context.Context, context.CancelFunc) {
	timeout := src.childTimeout
	if deadline, ok := parent.Deadline(); ok && remainingChildren > 0 {
		fairShare := time.Until(deadline) / time.Duration(remainingChildren)
		timeout = min(timeout, fairShare)
	}
	if timeout <= 0 {
		ctx, cancel := context.WithCancel(parent)
		cancel()
		return ctx, cancel
	}
	return context.WithTimeout(parent, timeout)
}

func (src *SecretRestartCommand) submit(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
	id string,
	phase string,
	plan jobmgr.WorkPlan,
	recovery bool,
) error {
	src.mu.Lock()
	src.nextUID++
	sequence := src.nextUID
	src.mu.Unlock()
	if sequence == 0 {
		return errors.New("jobmgr secrets: restart command identity wrapped")
	}
	request := jobmgr.Request{
		UID:     fmt.Sprintf("jobmgr-secret-%d-%d", src.epoch, sequence),
		LaneKey: id,
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/secrets/" + phase,
	}
	if recovery {
		return commands.SubmitRecoveryAndWait(ctx, request, plan)
	}
	return commands.SubmitPreparedAndWait(ctx, request, plan)
}
