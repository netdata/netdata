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
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type secretDependentJobBinding struct {
	controller *joboutput.DynCfgJobController
}

func (sdjb secretDependentJobBinding) PlanDependentStop(
	id string,
) (
	plan jobmgr.WorkPlan,
	result secretadapter.DependentStopResult,
	err error,
) {
	plan, result, err = sdjb.controller.PlanSecretDependentStop(id)
	return plan, result, err
}

func (sdjb secretDependentJobBinding) PlanDependentStart(
	id string,
) (
	plan jobmgr.WorkPlan,
	result secretadapter.DependentStartResult,
	err error,
) {
	plan, result, err = sdjb.controller.PlanSecretDependentStart(id)
	return plan, result, err
}

func newSecretInitialRoute(
	epoch uint64,
	controller *secretadapter.Controller,
) (functionadapter.InitialRoute, error) {
	if epoch == 0 || controller == nil || controller.Prefix() == "" {
		return functionadapter.InitialRoute{}, errors.New("jobmgr composition: invalid secret route")
	}
	commands := []functionadapter.ResourceTransactionCommand{
		{
			Name:              string(dyncfg.CommandAdd),
			AllocateSuccessor: true,
			Stage:             true,
			Claims:            []string{joboutput.DynCfgJobGraphClaim},
		},
		{Name: string(dyncfg.CommandSchema)},
		{Name: string(dyncfg.CommandGet)},
		{
			Name:              string(dyncfg.CommandUpdate),
			AllocateSuccessor: true,
			Stage:             true,
			Claims:            []string{joboutput.DynCfgJobGraphClaim},
		},
		{Name: string(dyncfg.CommandTest), Stage: true},
		{Name: string(dyncfg.CommandUserconfig)},
		{
			Name:   string(dyncfg.CommandRemove),
			Stage:  true,
			Claims: []string{joboutput.DynCfgJobGraphClaim},
		},
	}
	prefix := controller.Prefix()
	return functionadapter.InitialRoute{
		Declaration: functionadapter.Declaration{
			ID: "dyncfg/secretstores",
			Generation: &functionadapter.HandlerGenerationDeclaration{
				ID: fmt.Sprintf("dyncfg/secretstores/%d", epoch),
				Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
					return lifecycle.SealedResult{},
						errors.New("jobmgr composition: secret route requires a transaction")
				},
			},
			Transaction: &functionadapter.ResourceTransactionDeclaration{
				CompositeChildLaneConflict: func(
					input functionadapter.HandlerInput,
					lane string,
				) bool {
					return controller.CompositeChildLaneConflict(secretCommandInput(input), lane)
				},
				PrepareComposite: func(
					ctx context.Context,
					input functionadapter.HandlerInput,
					current lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (
					jobmgr.PreparedCompositeResourceTransaction,
					error,
				) {
					prepared, err := controller.Prepare(ctx, secretCommandInput(input), current, scope, permit)
					if prepared == nil {
						return nil, err
					}
					composite, ok := prepared.(jobmgr.PreparedCompositeResourceTransaction)
					if !ok {
						return nil, errors.Join(
							err,
							errors.New("jobmgr composition: secret transaction is not composite"),
						)
					}
					return composite, err
				},
				StageComposite: func(
					input functionadapter.HandlerInput,
				) (functionadapter.CompositeResourceTransactionStage, error) {
					command := secretCommandInput(input)
					stage, err := controller.Stage(command)
					if err != nil {
						return nil, err
					}
					return &stagedSecretTransaction{
						controller: controller,
						input:      command,
						stage:      stage,
					}, nil
				},
				CommandArgument: 1,
				GlobalClaim:     secretadapter.SecretGraphClaim,
				Commands:        commands,
			},
			PublicName:          joboutput.DynCfgFunctionName,
			Prefix:              prefix,
			Resource:            functionadapter.ScopedDynCfgJobResource(0, prefix, "secretstore:"),
			CooperativeCancel:   true,
			CooperativeDeadline: true,
			RawPayload:          true,
		},
	}, nil
}

type stagedSecretTransaction struct {
	controller *secretadapter.Controller
	input      secretadapter.CommandInput
	stage      *secretadapter.PreparedStoreOperation
}

func (transaction *stagedSecretTransaction) Start() {
	transaction.stage.Start()
}

func (transaction *stagedSecretTransaction) Ready() <-chan struct{} {
	return transaction.stage.Ready()
}

func (transaction *stagedSecretTransaction) Cancel(cause error) {
	transaction.stage.Cancel(cause)
}

func (transaction *stagedSecretTransaction) Release() {
	transaction.stage.Release()
	transaction.controller = nil
	transaction.input = secretadapter.CommandInput{}
	transaction.stage = nil
}

func (transaction *stagedSecretTransaction) PrepareComposite(
	ctx context.Context,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (jobmgr.PreparedCompositeResourceTransaction, error) {
	if transaction == nil ||
		transaction.controller == nil ||
		transaction.stage == nil {
		return nil, errors.New("jobmgr composition: invalid staged secret transaction")
	}
	prepared, err := transaction.controller.PrepareStaged(
		ctx,
		transaction.input,
		current,
		scope,
		permit,
		transaction.stage,
	)
	if prepared == nil {
		return nil, err
	}
	composite, ok := prepared.(jobmgr.PreparedCompositeResourceTransaction)
	if !ok {
		return nil, errors.Join(
			err,
			errors.New("jobmgr composition: staged secret transaction is not composite"),
		)
	}
	return composite, err
}

func secretCommandInput(input functionadapter.HandlerInput) secretadapter.CommandInput {
	return secretadapter.CommandInput{
		Args:        input.Args,
		Payload:     input.Payload,
		ContentType: input.ContentType,
		HasPayload:  input.HasPayload,
	}
}
