// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type diagnosticJobTransaction struct {
	transaction lifecycle.PreparedResourceTransaction
	controller  *DynCfgJobController
	target      dynCfgTarget
}

func (djt *diagnosticJobTransaction) Scope() lifecycle.ResourceTransactionScope {
	return djt.transaction.Scope()
}

func (djt *diagnosticJobTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := djt.transaction.Apply(ctx)
	status := applied.ResultStatus()
	level := jobmgr.DiagnosticInfo
	name := "job configuration command completed"
	if err != nil || !jobmgr.DiagnosticResultSucceeded(status) {
		level = jobmgr.DiagnosticWarning
		name = "job configuration command failed"
	}
	state := "removed"
	if record, ok := djt.controller.graph.Lookup(djt.target.resourceID); ok {
		state = record.Status
	}
	jobmgr.ObserveDiagnostic(djt.controller.diagnostics, jobmgr.DiagnosticEvent{
		Level:        level,
		Name:         name,
		Resource:     djt.target.resourceID,
		Command:      string(djt.target.command),
		State:        state,
		Generation:   djt.controller.generation,
		ResultStatus: status,
		Err:          err,
	})
	return applied, err
}

func (djt *diagnosticJobTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	return djt.transaction.Dispose(ctx)
}

func (dcjc *DynCfgJobController) observeTransaction(
	target dynCfgTarget,
	transaction lifecycle.PreparedResourceTransaction,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	if err != nil || transaction == nil || dcjc.diagnostics == nil {
		return transaction, err
	}
	return &diagnosticJobTransaction{
		transaction: transaction,
		controller:  dcjc,
		target:      target,
	}, nil
}
