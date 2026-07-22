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
	dvt.binding.traceCommand("vnode configuration transaction applied", dvt.command, dvt.resource, err)
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
	current, err := dvt.transaction.Dispose(ctx)
	dvt.binding.traceCommand("vnode configuration transaction disposed", dvt.command, dvt.resource, err)
	return current, err
}

func (vb *vnodeBinding) observeTransaction(
	command dyncfg.Command,
	resource string,
	transaction lifecycle.PreparedResourceTransaction,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	if err != nil || transaction == nil {
		vb.traceCommand("vnode configuration transaction preparation failed", command, resource, err)
		return transaction, err
	}
	vb.traceCommand("vnode configuration transaction prepared", command, resource, nil)
	if vb.diagnostics == nil {
		return transaction, nil
	}
	return &diagnosticVNodeTransaction{
		transaction: transaction,
		binding:     vb,
		command:     command,
		resource:    resource,
	}, nil
}

func (vb *vnodeBinding) traceCommand(name string, command dyncfg.Command, resource string, err error) {
	if vb == nil {
		return
	}
	jobmgr.TraceDiagnostic(vb.diagnostics, jobmgr.DiagnosticEvent{
		Name:       name,
		Resource:   resource,
		Command:    string(command),
		Generation: vb.epoch,
		Err:        err,
	})
}

func vnodeConfigNameFromResource(resource string) (string, bool) {
	return strings.CutPrefix(resource, "vnode:")
}
