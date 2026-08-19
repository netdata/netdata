// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// The leading NUL reserves an internal-only identity that cannot collide with
// DynCfg-valid secretstore resource IDs.
const secretBootResourceID = "\x00jobmgr-secret-boot"

func (c *Controller) PublishInitial(ctx context.Context, commands jobmgr.PreparedCommandPort) error {
	if c == nil || ctx == nil || commands == nil {
		return errors.New("jobmgr secrets: invalid initial publication")
	}
	c.mu.Lock()
	if c.restarts == nil || c.commandsReady || c.commands != nil {
		c.mu.Unlock()
		return errors.New("jobmgr secrets: unbound or duplicate initial publication")
	}
	c.commands = commands
	initial := selectInitialConfigs(c.initial)
	c.mu.Unlock()
	for index, config := range initial {
		if config == nil || config.ExposedKey() == "" || config.Validate() != nil {
			return fmt.Errorf("jobmgr secrets: invalid initial configuration %d", index)
		}
	}
	if err := c.publishTemplates(ctx, commands); err != nil {
		return err
	}
	stages := make([]*PreparedStoreOperation, len(initial))
	defer func() {
		for _, stage := range stages {
			stage.Release()
		}
	}()
	for index, config := range initial {
		desiredVersion, err := c.allocateDesiredVersion()
		if err != nil {
			return err
		}
		target := secretTarget{
			key:     config.ExposedKey(),
			kind:    config.Kind(),
			name:    config.Name(),
			command: dyncfg.CommandAdd,
		}
		stage, err := c.operations.prepare(storeOperationSpec{
			target:         target,
			config:         config,
			expected:       c.store.Generation(target.key),
			mode:           storeOperationMutation,
			supersede:      true,
			desiredVersion: desiredVersion,
		})
		if err != nil {
			return err
		}
		stages[index] = stage
		stage.Start()
	}
	for _, stage := range stages {
		select {
		case <-stage.Ready():
		case <-ctx.Done():
			for _, pending := range stages {
				pending.Cancel(context.Cause(ctx))
			}
			return context.Cause(ctx)
		}
	}
	for index, config := range initial {
		plan, err := c.planInitial(config, stages[index])
		if err != nil {
			return err
		}
		if err := commands.SubmitPreparedAndWait(
			ctx,
			jobmgr.Request{
				UID:     fmt.Sprintf("jobmgr-secrets-%d-%d", c.epoch, index+1),
				LaneKey: secretResourceID(config.ExposedKey()),
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/secrets/publish",
			},
			plan,
		); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.initial = nil
	c.mu.Unlock()
	return nil
}

func (c *Controller) publishTemplates(ctx context.Context, commands jobmgr.PreparedCommandPort) error {
	plan := jobmgr.WorkPlan{
		Claims:     []string{SecretGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: secretBootResourceID,
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil ||
					scope.ID != secretBootResourceID ||
					scope.Current.Valid() ||
					scope.Successor.Valid() ||
					permit.Valid() {
					return nil, errors.New("jobmgr secrets: invalid template publication scope")
				}
				return c.noop(
					scope,
					nil,
					mustSecretMessage(204, ""),
					nil,
					c.templateCleanup(),
				)
			},
		},
	}
	return commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-secret-templates-%d", c.epoch),
			LaneKey: secretBootResourceID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/secrets/templates",
		},
		plan,
	)
}

func (c *Controller) planInitial(
	config secretstore.Config,
	stage *PreparedStoreOperation,
) (jobmgr.WorkPlan, error) {
	key := config.ExposedKey()
	resourceID := secretResourceID(key)
	return jobmgr.WorkPlan{
		Claims:     []string{SecretGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                resourceID,
			AllocateSuccessor: true,
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (
				transaction lifecycle.PreparedResourceTransaction,
				resultErr error,
			) {
				if scope.ID != resourceID {
					return nil, errors.New("jobmgr secrets: initial Store scope differs")
				}
				if existing, ok := c.entry(key); ok {
					existingPriority := existing.config.SourceTypePriority()
					nextPriority := config.SourceTypePriority()
					if existingPriority > nextPriority ||
						existingPriority == nextPriority &&
							existing.status ==
								dyncfg.StatusRunning {
						return c.noop(
							scope,
							current,
							mustSecretMessage(204, ""),
							nil,
							c.configCreateCleanup(existing),
						)
					}
				}
				operation, err := takeStoreOperation(stage)
				if err != nil {
					return nil, err
				}
				defer operation.releaseUntransferred(&transaction, &resultErr)
				materialized := operation.result
				expected := c.store.Generation(key)
				if materialized.expected != expected {
					materialized.retryable = true
					materialized.err = errors.New(
						"jobmgr secrets: initial Store changed while preparation was staged",
					)
					return c.prepareRetryableResult(scope, current, materialized, expected == 0)
				}
				if materialized.retryable {
					return c.prepareRetryableResult(scope, current, materialized, expected == 0)
				}
				return c.prepareStoreMutation(scope, current, operation, true)
			},
		},
		CooperativeCancel:   true,
		CooperativeDeadline: true,
	}, nil
}

func (c *Controller) CloseProjection() error {
	if c == nil {
		return errors.New("jobmgr secrets: invalid controller projection close")
	}
	c.mu.Lock()
	c.commandsReady = false
	closeContext := c.closeContext
	c.closeContext = nil
	c.commands = nil
	for key, state := range c.pending {
		delete(c.pending, key)
		select {
		case state.update <- struct{}{}:
		default:
		}
	}
	c.mu.Unlock()
	if closeContext != nil {
		closeContext()
	}
	return nil
}
