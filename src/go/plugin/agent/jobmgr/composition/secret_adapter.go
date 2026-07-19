// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type secretDependentJobBinding struct {
	controller *joboutput.DynCfgJobController
}

func (binding secretDependentJobBinding) PlanDependentStop(
	id string,
) (
	plan jobmgr.WorkPlan,
	result secretadapter.DependentStopResult,
	err error,
) {
	plan, result, err = binding.controller.PlanSecretDependentStop(id)
	return plan, result, err
}

func (binding secretDependentJobBinding) PlanDependentStart(
	config confgroup.Config,
) (
	plan jobmgr.WorkPlan,
	result secretadapter.DependentStartResult,
	err error,
) {
	plan, result, err =
		binding.controller.PlanSecretDependentStart(config)
	return plan, result, err
}

func newSecretInitialRoute(
	epoch uint64,
	controller *secretadapter.Controller,
) (functionadapter.InitialRoute, error) {
	if epoch == 0 || controller == nil || controller.Prefix() == "" {
		return functionadapter.InitialRoute{},
			errors.New("jobmgr composition: invalid secret route")
	}
	commands := []functionadapter.ResourceTransactionCommand{
		{Name: string(dyncfg.CommandAdd), AllocateSuccessor: true},
		{Name: string(dyncfg.CommandSchema)},
		{Name: string(dyncfg.CommandGet)},
		{
			Name:              string(dyncfg.CommandUpdate),
			AllocateSuccessor: true,
			Claims: []string{
				joboutput.DynCfgJobGraphClaim,
			},
		},
		{Name: string(dyncfg.CommandTest)},
		{Name: string(dyncfg.CommandUserconfig)},
		{
			Name: string(dyncfg.CommandRemove),
			Claims: []string{
				joboutput.DynCfgJobGraphClaim,
			},
		},
	}
	prefix := controller.Prefix()
	return functionadapter.InitialRoute{
		Declaration: functionadapter.Declaration{
			ID: "dyncfg/secretstores",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: fmt.Sprintf("dyncfg/secretstores/%d", epoch),
				Handler: func(
					context.Context,
					functionadapter.HandlerInput,
				) (lifecycle.SealedResult, error) {
					return lifecycle.SealedResult{},
						errors.New(
							"jobmgr composition: secret route requires a transaction",
						)
				},
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				Prepare: func(
					ctx context.Context,
					input functionadapter.HandlerInput,
					current lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (
					lifecycle.PreparedResourceTransaction,
					error,
				) {
					return controller.Prepare(
						ctx,
						secretCommandInput(input),
						current,
						scope,
						permit,
					)
				},
				PermitFor: func(
					input functionadapter.HandlerInput,
				) (lifecycle.LongLivedPlan, error) {
					return secretadapter.MutationPermit(input.Payload)
				},
				CommandArgument: 1,
				GlobalClaim:     secretadapter.SecretGraphClaim,
				Commands:        commands,
			},
			PublicName: joboutput.DynCfgFunctionName,
			Prefix:     prefix,
			Resource: functionadapter.ScopedDynCfgJobResource(
				0,
				prefix,
				"secretstore:",
			),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
		Publication: dynCfgPublication(epoch),
	}, nil
}

func secretCommandInput(
	input functionadapter.HandlerInput,
) secretadapter.CommandInput {
	return secretadapter.CommandInput{
		Args: input.Args, Payload: input.Payload,
		ContentType: input.ContentType,
		HasPayload:  input.HasPayload,
	}
}
