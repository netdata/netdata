// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

// The leading NUL reserves an internal-only identity that cannot collide with
// DynCfg-valid module or module_name job resource IDs.
const dynCfgJobBootResourceID = "\x00jobmgr-job-boot"

// PublishInitial publishes deterministic module templates and reconciles every
// configured initial job through the same transaction path used by
// discovery. The graph starts empty, so a running initial record cannot exist
// without its matching current resource.
func (dcjc *DynCfgJobController) PublishInitial(
	ctx context.Context,
	commands jobmgr.PreparedCommandPort,
	epoch uint64,
	initial []dyncfg.GraphConfig,
) error {
	if dcjc == nil || ctx == nil || commands == nil || epoch == 0 {
		return errors.New("job output: invalid initial DynCfg publication")
	}
	if err := dcjc.publishInitialTemplates(
		ctx,
		commands,
		epoch,
	); err != nil {
		return err
	}
	configs := slices.Clone(initial)
	slices.SortFunc(configs, func(a, b dyncfg.GraphConfig) int {
		return cmp.Compare(a.ID, b.ID)
	})
	for index, record := range configs {
		config, status, err := dcjc.initialConfiguration(record)
		if err != nil {
			return err
		}
		plan, err := dcjc.PlanDiscovered(DiscoveredJobChange{
			Config: config,
			Status: status,
		})
		if err != nil {
			return err
		}
		if err := commands.SubmitPreparedAndWait(
			ctx,
			jobmgr.Request{
				UID: fmt.Sprintf(
					"jobmgr-jobs-%d-%d",
					epoch,
					index+1,
				),
				LaneKey: config.FullName(),
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/jobs/publish",
			},
			plan,
		); err != nil {
			return err
		}
	}
	return nil
}

func (dcjc *DynCfgJobController) publishInitialTemplates(
	ctx context.Context,
	commands jobmgr.PreparedCommandPort,
	epoch uint64,
) error {
	result, err := lifecycle.NewSealedResult(
		204,
		"application/json",
		nil,
	)
	if err != nil {
		return err
	}
	plan := jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: dynCfgJobBootResourceID,
			Prepare: func(
				_ context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil ||
					scope.ID != dynCfgJobBootResourceID ||
					scope.Current.Valid() ||
					scope.Successor.Valid() ||
					permit.Valid() {
					return nil, errors.New(
						"job output: invalid initial template scope",
					)
				}
				return PrepareNoopResourceTransaction(
					scope,
					nil,
					lifecycle.LongLivedPermit{},
					result,
					dcjc.templatePublicationCleanup(),
					nil,
				)
			},
		},
	}
	return commands.SubmitPreparedAndWait(
		ctx,
		jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-job-templates-%d", epoch),
			LaneKey: dynCfgJobBootResourceID,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/jobs/templates",
		},
		plan,
	)
}

func (dcjc *DynCfgJobController) templatePublicationCleanup() lifecycle.TaskCleanup {
	names := make([]string, 0, len(dcjc.modules))
	for name, creator := range dcjc.modules {
		if creator.InstancePolicy == collectorapi.InstancePolicyPerJob {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return func() error { return nil }
	}
	var payload bytes.Buffer
	api := netdataapi.New(&payload)
	for _, name := range names {
		api.CONFIGCREATE(netdataapi.ConfigOpts{
			ID:         dcjc.prefix + name,
			Status:     dyncfg.StatusAccepted.String(),
			ConfigType: dyncfg.ConfigTypeTemplate.String(),
			Path:       dcjc.path, SourceType: "internal",
			Source: "internal",
			SupportedCommands: dyncfg.JoinCommands(
				dyncfg.CommandAdd,
				dyncfg.CommandSchema,
				dyncfg.CommandEnable,
				dyncfg.CommandDisable,
				dyncfg.CommandTest,
				dyncfg.CommandUserconfig,
			),
		})
	}
	prepared, err := lifecycle.PrepareProtocolFrame(payload.Bytes())
	if err != nil {
		return func() error { return err }
	}
	return func() error {
		return dcjc.frames.CommitPreparedProtocolFrame(prepared)
	}
}

func (dcjc *DynCfgJobController) initialConfiguration(
	record dyncfg.GraphConfig,
) (confgroup.Config, dyncfg.Status, error) {
	status := dyncfg.Status(record.Status)
	if status != dyncfg.StatusAccepted &&
		status != dyncfg.StatusRunning {
		return nil, "", fmt.Errorf(
			"job output: unsupported initial DynCfg status %q",
			record.Status,
		)
	}
	var config confgroup.Config
	if err := yaml.Unmarshal(record.Payload, &config); err != nil {
		return nil, "", fmt.Errorf(
			"job output: invalid initial DynCfg payload: %w",
			err,
		)
	}
	if config == nil ||
		config.FullName() != record.ID ||
		config.Module() != record.Module ||
		config.Name() != record.Name {
		return nil, "", errors.New(
			"job output: initial DynCfg identity differs from payload",
		)
	}
	return config, status, nil
}
