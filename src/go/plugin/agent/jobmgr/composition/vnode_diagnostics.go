// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type diagnosticVNodeTransaction struct {
	transaction lifecycle.PreparedResourceTransaction
	binding     *vnodeBinding
	command     dyncfg.Command
	resource    string
}

func (dvt *diagnosticVNodeTransaction) Scope() lifecycle.ResourceTransactionScope {
	return dvt.transaction.Scope()
}

func (dvt *diagnosticVNodeTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	applied, err := dvt.transaction.Apply(ctx)
	status := applied.ResultStatus()
	level := jobmgr.DiagnosticInfo
	name := "vnode configuration command completed"
	if err != nil || !jobmgr.DiagnosticResultSucceeded(status) {
		level = jobmgr.DiagnosticWarning
		name = "vnode configuration command failed"
	}
	state := "removed"
	if name, ok := vnodeConfigNameFromResource(dvt.resource); ok {
		if _, exists := dvt.binding.config.Lookup(name); exists {
			state = "configured"
		}
	}
	jobmgr.ObserveDiagnostic(dvt.binding.diagnostics, jobmgr.DiagnosticEvent{
		Level:        level,
		Name:         name,
		Resource:     dvt.resource,
		Command:      string(dvt.command),
		State:        state,
		Generation:   dvt.binding.epoch,
		ResultStatus: status,
		Err:          err,
	})
	return applied, err
}

func (dvt *diagnosticVNodeTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	return dvt.transaction.Dispose(ctx)
}

func (vb *vnodeBinding) observeTransaction(
	command dyncfg.Command,
	resource string,
	transaction lifecycle.PreparedResourceTransaction,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	if err != nil || transaction == nil || vb.diagnostics == nil {
		return transaction, err
	}
	return &diagnosticVNodeTransaction{
		transaction: transaction,
		binding:     vb,
		command:     command,
		resource:    resource,
	}, nil
}

func vnodeConfigNameFromResource(resource string) (string, bool) {
	return strings.CutPrefix(resource, "vnode:")
}
