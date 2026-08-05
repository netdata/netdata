// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type diagnosticSecretTransaction struct {
	transaction lifecycle.PreparedResourceTransaction
	composite   jobmgr.PreparedCompositeResourceTransaction
	controller  *Controller
	target      secretTarget
}

func (dst *diagnosticSecretTransaction) Scope() lifecycle.ResourceTransactionScope {
	return dst.transaction.Scope()
}

func (dst *diagnosticSecretTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := dst.transaction.Apply(ctx)
	dst.observeApplied(applied, err)
	return applied, err
}

func (dst *diagnosticSecretTransaction) ApplyComposite(
	ctx context.Context,
	commands jobmgr.CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := dst.composite.ApplyComposite(ctx, commands)
	dst.observeApplied(applied, err)
	return applied, err
}

func (dst *diagnosticSecretTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	return dst.transaction.Dispose(ctx)
}

func (dst *diagnosticSecretTransaction) observeApplied(
	applied lifecycle.AppliedResourceTransaction,
	err error,
) {
	if !secretMutationCommand(dst.target.command) {
		return
	}
	status := applied.ResultStatus()
	level := jobmgr.DiagnosticInfo
	name := "secretstore configuration command completed"
	if err != nil || !jobmgr.DiagnosticResultSucceeded(status) {
		level = jobmgr.DiagnosticWarning
		name = "secretstore configuration command failed"
	}
	state := "removed"
	if entry, ok := dst.controller.entry(dst.target.key); ok {
		state = entry.status.String()
	}
	jobmgr.ObserveDiagnostic(dst.controller.diagnostics, jobmgr.DiagnosticEvent{
		Level:        level,
		Name:         name,
		Resource:     secretDiagnosticResource(dst.target),
		Command:      string(dst.target.command),
		State:        state,
		Generation:   dst.controller.epoch,
		ResultStatus: status,
		Err:          err,
	})
}

func (c *Controller) observeTransaction(
	target secretTarget,
	transaction lifecycle.PreparedResourceTransaction,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	if err != nil || transaction == nil || c.diagnostics == nil {
		return transaction, err
	}
	composite, ok := transaction.(jobmgr.PreparedCompositeResourceTransaction)
	if !ok {
		return transaction, err
	}
	return &diagnosticSecretTransaction{
		transaction: transaction,
		composite:   composite,
		controller:  c,
		target:      target,
	}, nil
}

func secretDiagnosticResource(target secretTarget) string {
	if resource := secretResourceID(target.key); resource != "" {
		return resource
	}
	return target.id
}

func secretMutationCommand(command dyncfg.Command) bool {
	switch command {
	case dyncfg.CommandAdd, dyncfg.CommandUpdate, dyncfg.CommandRemove:
		return true
	default:
		return false
	}
}
