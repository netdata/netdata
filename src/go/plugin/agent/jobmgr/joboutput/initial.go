// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// The leading NUL reserves an internal-only identity that cannot collide with
// DynCfg-valid module or module_name job resource IDs.
const dynCfgJobBootResourceID = "\x00jobmgr-job-boot"

// PublishInitial publishes deterministic module templates.
func (dcjc *DynCfgJobController) PublishInitial(
	ctx context.Context,
	commands jobmgr.PreparedCommandPort,
	epoch uint64,
) error {
	if dcjc == nil || ctx == nil || commands == nil || epoch == 0 {
		return errors.New("job output: invalid initial DynCfg publication")
	}
	return dcjc.publishInitialTemplates(ctx, commands, epoch)
}

func (dcjc *DynCfgJobController) publishInitialTemplates(
	ctx context.Context,
	commands jobmgr.PreparedCommandPort,
	epoch uint64,
) error {
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
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
					return nil, errors.New("job output: invalid initial template scope")
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
			Path:       dcjc.path,
			SourceType: "internal",
			Source:     "internal",
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
