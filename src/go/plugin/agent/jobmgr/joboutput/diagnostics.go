// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type diagnosticJobTransaction struct {
	transaction lifecycle.PreparedResourceTransaction
	controller  *DynCfgJobController
	command     dyncfg.Command
	resource    string
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
	if record, ok := djt.controller.graph.Lookup(djt.resource); ok {
		state = record.Status
	}
	jobmgr.ObserveDiagnostic(djt.controller.diagnostics, jobmgr.DiagnosticEvent{
		Level:        level,
		Name:         name,
		Resource:     djt.resource,
		Command:      string(djt.command),
		State:        state,
		Generation:   djt.controller.generation,
		ResultStatus: status,
		Err:          err,
	})
	return applied, err
}

func (djt *diagnosticJobTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	// Disposal is an unapplied rollback; only the kernel knows whether cancellation, deadline, or shutdown caused it.
	return djt.transaction.Dispose(ctx)
}

func (dcjc *DynCfgJobController) observeTransaction(
	command dyncfg.Command,
	resource string,
	transaction lifecycle.PreparedResourceTransaction,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	if err != nil || transaction == nil || dcjc.diagnostics == nil {
		return transaction, err
	}
	return &diagnosticJobTransaction{
		transaction: transaction,
		controller:  dcjc,
		command:     command,
		resource:    resource,
	}, nil
}

func dynCfgRequestDiagnosticIdentity(
	request DynCfgJobRequest,
	scope lifecycle.ResourceTransactionScope,
) (dyncfg.Command, string) {
	command := dyncfg.CommandFromArgs(request.Args)
	resource := scope.ID
	// Leading NUL identifies an internal fallback scope, not an operator-visible resource.
	if strings.HasPrefix(resource, "\x00") {
		resource = ""
	}
	return command, resource
}
