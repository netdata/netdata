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

const secretBootResourceID = "\x00jobmgr-secret-boot"

func (c *Controller) PublishInitial(ctx context.Context, commands jobmgr.PreparedCommandPort) error {
	if c == nil || ctx == nil || commands == nil {
		return errors.New("jobmgr secrets: invalid initial publication")
	}
	c.mu.Lock()
	if c.restarts == nil || c.published {
		c.mu.Unlock()
		return errors.New("jobmgr secrets: unbound or duplicate initial publication")
	}
	initial := sortedInitialConfigs(c.initial)
	c.mu.Unlock()
	if err := c.publishTemplates(ctx, commands); err != nil {
		return err
	}
	for index, config := range initial {
		if config == nil || config.ExposedKey() == "" || config.Validate() != nil {
			return fmt.Errorf("jobmgr secrets: invalid initial configuration %d", index)
		}
		plan, err := c.planInitial(config)
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
	c.published = true
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
					lifecycle.LongLivedPermit{},
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
	key := config.ExposedKey()
	resourceID := secretResourceID(key)
	return jobmgr.WorkPlan{
		Claims:     []string{SecretGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                resourceID,
			AllocateSuccessor: true,
			Permit:            lifecycle.NewSecretStoreLongLivedPlan(),
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
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
							permit,
							mustSecretMessage(204, ""),
							nil,
							c.configCreateCleanup(existing),
						)
					}
				}
				expected := c.store.Generation(key)
				return c.prepareStoreMutation(ctx, scope, current, permit, config, expected, true)
			},
		},
		CooperativeCancel:   true,
		CooperativeDeadline: true,
	}, nil
}

func (c *Controller) Close(ctx context.Context) error {
	if c == nil || ctx == nil {
		return errors.New("jobmgr secrets: invalid controller close")
	}
	c.mu.Lock()
	c.published = false
	c.mu.Unlock()
	return c.store.Close(ctx)
}
