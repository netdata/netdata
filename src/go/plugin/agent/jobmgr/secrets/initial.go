// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
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
	for index, config := range initial {
		plan, err := c.planInitial(config)
		if err != nil {
			return err
		}
		if err := commands.SubmitPreparedAndWait(ctx, jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-secrets-%d-%d", c.epoch, index+1),
			LaneKey: secretResourceID(config.ExposedKey()),
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/secrets/publish",
		}, plan); err != nil {
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

func (c *Controller) planInitial(config secretstore.Config) (jobmgr.WorkPlan, error) {
	version, err := c.allocateDesiredVersion()
	if err != nil {
		return jobmgr.WorkPlan{}, err
	}
	structuralErr := c.store.ValidateStructure(c.creators, config)
	key := config.ExposedKey()
	resourceID := secretResourceID(key)
	return jobmgr.WorkPlan{
		Claims:     []string{SecretGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: resourceID,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				if scope.ID != resourceID || permit.Valid() {
					return nil, errors.New("jobmgr secrets: invalid initial Store scope")
				}
				if existing, ok := c.entry(key); ok &&
					existing.config.SourceTypePriority() >= config.SourceTypePriority() {
					return c.noop(scope, current, mustSecretMessage(204, ""), nil, nil)
				}
				if current != nil || c.store.Generation(key) != 0 {
					return nil, errors.New("jobmgr secrets: initial publication found a live Store")
				}
				return c.prepareAccepted(scope, current, storeOperationResult{
					config:         config,
					desiredVersion: version,
					err:            structuralErr,
				})
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
		state.cancel()
	}
	c.mu.Unlock()
	if closeContext != nil {
		closeContext()
	}
	return nil
}
