// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

const (
	DynCfgJobGraphClaim     = "dyncfg:jobs"
	DefaultJobRetainedBytes = 512
)

type DiscoveredJobChange struct {
	Config  confgroup.Config
	Status  dyncfg.Status
	Remove  bool
	Restart bool
}

// PlanDiscovered builds one typed, response-free graph/job reconciliation
// plan. The caller submits it through jobmgr.PreparedCommandPort, so no
// object-token registry is needed to carry confgroup.Config through Request.
func (controller *DynCfgJobController) PlanDiscovered(
	change DiscoveredJobChange,
) (jobmgr.WorkPlan, error) {
	if controller == nil || change.Config == nil {
		return jobmgr.WorkPlan{},
			errors.New("job output: invalid discovered job change")
	}
	config, err := change.Config.Clone()
	if err != nil {
		return jobmgr.WorkPlan{}, err
	}
	creator, ok := controller.modules.Lookup(config.Module())
	if !ok {
		return jobmgr.WorkPlan{},
			errors.New("job output: discovered module is not registered")
	}
	if err := validateFactoryConfigIdentity(config, creator); err != nil {
		return jobmgr.WorkPlan{}, err
	}
	if config.FullName() == "" {
		return jobmgr.WorkPlan{},
			errors.New("job output: discovered config has no identity")
	}
	if !validDynCfgProtocolField(config.Source()) ||
		!validDynCfgProtocolField(config.SourceType()) {
		return jobmgr.WorkPlan{},
			errors.New("job output: discovered config has invalid protocol metadata")
	}
	if change.Remove {
		if change.Restart {
			return jobmgr.WorkPlan{},
				errors.New("job output: removed discovery config cannot restart")
		}
		change.Status = ""
	} else if change.Status != dyncfg.StatusAccepted &&
		change.Status != dyncfg.StatusRunning {
		return jobmgr.WorkPlan{},
			errors.New("job output: invalid discovered config status")
	}
	permit := lifecycle.LongLivedPlan{}
	if change.Restart && change.Status != dyncfg.StatusRunning {
		return jobmgr.WorkPlan{},
			errors.New("job output: only a running discovery config can restart")
	}
	if change.Status == dyncfg.StatusRunning {
		permit, err = lifecycle.NewJobLongLivedPlan(DefaultJobRetainedBytes)
		if err != nil {
			return jobmgr.WorkPlan{}, err
		}
	}
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                config.FullName(),
			AllocateSuccessor: change.Status == dyncfg.StatusRunning,
			Permit:            permit,
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return controller.prepareDiscovered(
					ctx,
					DiscoveredJobChange{
						Config:  config,
						Status:  change.Status,
						Remove:  change.Remove,
						Restart: change.Restart,
					},
					current,
					scope,
					permit,
				)
			},
		},
	}, nil
}

func (controller *DynCfgJobController) prepareDiscovered(
	ctx context.Context,
	change DiscoveredJobChange,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	record, exists := controller.graph.Lookup(scope.ID)
	if err := validateGraphResourcePair(
		record,
		exists,
		current,
		scope,
	); err != nil {
		return nil, err
	}
	result := mustDynCfgMessage(204, "")
	if change.Remove {
		disposition := lifecycle.ResourceTransactionUnchanged
		if current != nil {
			disposition = lifecycle.ResourceTransactionRemoved
		}
		return controller.prepareMutation(
			scope,
			current,
			nil,
			lifecycle.LongLivedPermit{},
			disposition,
			nil,
			result,
			controller.configDeleteCleanup(
				controller.configID(
					change.Config.Module(),
					change.Config.Name(),
				),
			),
		)
	}

	payload, err := yaml.Marshal(change.Config)
	if err != nil {
		return nil, err
	}
	postimage := dyncfg.GraphConfig{
		ID: scope.ID, Module: change.Config.Module(),
		Name: change.Config.Name(), Status: change.Status.String(),
		Payload: payload,
	}
	cleanup := controller.configCreateCleanup(
		postimage,
		change.Config.SourceType(),
		change.Config.Source(),
		controller.configType(controller.modules[change.Config.Module()]),
	)
	if change.Status == dyncfg.StatusAccepted {
		disposition := lifecycle.ResourceTransactionUnchanged
		if current != nil {
			disposition = lifecycle.ResourceTransactionRemoved
		}
		return controller.prepareMutation(
			scope,
			current,
			nil,
			lifecycle.LongLivedPermit{},
			disposition,
			&postimage,
			result,
			cleanup,
		)
	}
	if exists &&
		record.Status == dyncfg.StatusRunning.String() &&
		record.Payload() == string(payload) &&
		!change.Restart {
		return controller.noop(
			scope,
			current,
			permit,
			result,
			cleanup,
		)
	}
	successor, err := controller.factory.Prepare(
		ctx,
		change.Config,
		scope.Successor,
		permit,
	)
	if err != nil {
		return nil, err
	}
	disposition := lifecycle.ResourceTransactionInstalled
	if current != nil {
		disposition = lifecycle.ResourceTransactionReplaced
	}
	return controller.prepareMutation(
		scope,
		current,
		successor,
		lifecycle.LongLivedPermit{},
		disposition,
		&postimage,
		result,
		cleanup,
	)
}
