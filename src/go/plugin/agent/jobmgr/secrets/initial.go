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
	"gopkg.in/yaml.v2"
)

const secretBootResourceID = "\x00jobmgr-secret-boot"

func (controller *Controller) PublishInitial(
	ctx context.Context,
	commands CommandPort,
) error {
	if controller == nil || ctx == nil || commands == nil {
		return errors.New(
			"jobmgr secrets: invalid initial publication",
		)
	}
	controller.mu.Lock()
	if controller.restarts == nil || controller.published {
		controller.mu.Unlock()
		return errors.New(
			"jobmgr secrets: unbound or duplicate initial publication",
		)
	}
	initial := sortedInitialConfigs(controller.initial)
	controller.mu.Unlock()
	if err := controller.publishTemplates(ctx, commands); err != nil {
		return err
	}
	for index, config := range initial {
		if config == nil ||
			config.ExposedKey() == "" ||
			config.Validate() != nil {
			return fmt.Errorf(
				"jobmgr secrets: invalid initial configuration %d",
				index,
			)
		}
		plan, err := controller.planInitial(config)
		if err != nil {
			return err
		}
		if err := commands.SubmitPreparedAndWait(
			ctx,
			jobmgr.Request{
				UID: fmt.Sprintf(
					"jobmgr-secrets-%d-%d",
					controller.epoch,
					index+1,
				),
				LaneKey: secretResourceID(config.ExposedKey()),
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/secrets/publish",
			},
			plan,
		); err != nil {
			return err
		}
	}
	controller.mu.Lock()
	controller.initial = nil
	controller.published = true
	controller.mu.Unlock()
	return nil
}

func (controller *Controller) publishTemplates(
	ctx context.Context,
	commands CommandPort,
) error {
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
					return nil, errors.New(
						"jobmgr secrets: invalid template publication scope",
					)
				}
				return controller.noop(
					scope,
					nil,
					lifecycle.LongLivedPermit{},
					mustSecretMessage(204, ""),
					nil,
					controller.templateCleanup(),
				)
			},
		},
	}
	return commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-secret-templates-%d",
				controller.epoch,
			),
			LaneKey: secretBootResourceID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/secrets/templates",
		},
		plan,
	)
}

func (controller *Controller) planInitial(
	config secretstore.Config,
) (jobmgr.WorkPlan, error) {
	payload, err := yaml.Marshal(config)
	if err != nil {
		return jobmgr.WorkPlan{}, err
	}
	permit, err := MutationPermit(payload)
	if err != nil {
		return jobmgr.WorkPlan{}, err
	}
	key := config.ExposedKey()
	resourceID := secretResourceID(key)
	return jobmgr.WorkPlan{
		Claims:     []string{SecretGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                resourceID,
			AllocateSuccessor: true,
			Permit:            permit,
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if scope.ID != resourceID {
					return nil, errors.New(
						"jobmgr secrets: initial Store scope differs",
					)
				}
				if existing, ok := controller.entry(key); ok {
					existingPriority :=
						existing.config.SourceTypePriority()
					nextPriority := config.SourceTypePriority()
					if existingPriority > nextPriority ||
						existingPriority == nextPriority &&
							existing.status ==
								dyncfg.StatusRunning {
						return controller.noop(
							scope,
							current,
							permit,
							mustSecretMessage(204, ""),
							nil,
							controller.configCreateCleanup(existing),
						)
					}
				}
				expected := controller.store.Generation(key)
				return controller.prepareStoreMutation(
					ctx,
					scope,
					current,
					permit,
					config,
					expected,
					true,
				)
			},
		},
		CooperativeCancel:   true,
		CooperativeDeadline: true,
	}, nil
}

func (controller *Controller) Close(ctx context.Context) error {
	if controller == nil || ctx == nil {
		return errors.New("jobmgr secrets: invalid controller close")
	}
	controller.mu.Lock()
	controller.published = false
	controller.mu.Unlock()
	return controller.store.Close(ctx)
}

func (controller *Controller) ConfigStatus(
	key string,
) (secretstore.Config, dyncfg.Status, bool) {
	entry, ok := controller.entry(key)
	return entry.config, entry.status, ok
}
