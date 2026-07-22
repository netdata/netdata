// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// dynCfgJobBinding seals the construction cycle between the immutable Function
// catalog and the DynCfg controller whose factory consumes Function job hooks.
type dynCfgJobBinding struct {
	controller atomic.Pointer[joboutput.DynCfgJobController]
}

func (dcjb *dynCfgJobBinding) bind(controller *joboutput.DynCfgJobController) error {
	if dcjb == nil || controller == nil {
		return errors.New("jobmgr composition: invalid DynCfg job binding")
	}
	if !dcjb.controller.CompareAndSwap(nil, controller) {
		return errors.New("jobmgr composition: duplicate DynCfg job binding")
	}
	return nil
}

func (dcjb *dynCfgJobBinding) handle(
	ctx context.Context,
	input functionadapter.HandlerInput,
) (lifecycle.SealedResult, error) {
	controller := dcjb.controller.Load()
	if controller == nil {
		return lifecycle.SealedResult{}, errors.New("jobmgr composition: unbound DynCfg job handler")
	}
	return controller.Handle(ctx, dynCfgJobRequest(input))
}

func (dcjb *dynCfgJobBinding) prepare(
	ctx context.Context,
	input functionadapter.HandlerInput,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	controller := dcjb.controller.Load()
	if controller == nil {
		return nil, errors.New("jobmgr composition: unbound DynCfg job transaction")
	}
	return controller.Prepare(ctx, dynCfgJobRequest(input), current, scope, permit)
}

func dynCfgJobRequest(input functionadapter.HandlerInput) joboutput.DynCfgJobRequest {
	return joboutput.DynCfgJobRequest{
		Args:         input.Args,
		Payload:      input.Payload,
		ContentType:  input.ContentType,
		CallerSource: input.CallerSource,
		HasPayload:   input.HasPayload,
	}
}

func newDynCfgJobInitialRoute(
	epoch uint64,
	prefix string,
	binding *dynCfgJobBinding,
) (functionadapter.InitialRoute, error) {
	if epoch == 0 || prefix == "" || binding == nil {
		return functionadapter.InitialRoute{}, errors.New("jobmgr composition: invalid DynCfg job route")
	}
	permit := lifecycle.NewJobLongLivedPlan()
	return functionadapter.InitialRoute{
		Declaration: functionadapter.Declaration{
			ID: "dyncfg/jobs",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID:      fmt.Sprintf("dyncfg/jobs/%d", epoch),
				Handler: binding.handle,
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				Prepare:         binding.prepare,
				Permit:          permit,
				CommandArgument: 1,
				GlobalClaim:     joboutput.DynCfgJobGraphClaim,
				Commands: []functionadapter.ResourceTransactionCommand{
					{Name: string(dyncfg.CommandAdd)},
					{Name: string(dyncfg.CommandUpdate), AllocateSuccessor: true},
					{Name: string(dyncfg.CommandEnable), AllocateSuccessor: true},
					{Name: string(dyncfg.CommandRestart), AllocateSuccessor: true},
					{Name: string(dyncfg.CommandDisable)},
					{Name: string(dyncfg.CommandRemove)},
				},
			},
			PublicName:          joboutput.DynCfgFunctionName,
			Prefix:              prefix,
			Resource:            functionadapter.DynCfgJobResource(0, prefix),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
	}, nil
}
