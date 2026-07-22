// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

func (ck *CommandKernel) trace(event DiagnosticEvent) {
	if ck == nil {
		return
	}
	if event.Generation == 0 {
		event.Generation = ck.run.Generation()
	}
	TraceDiagnostic(ck.diagnosticObserver, event)
}

func (ck *CommandKernel) traceOperation(name string, operation *commandOperation) {
	if ck == nil || operation == nil || ck.diagnosticObserver == nil || !ck.diagnosticObserver.TraceEnabled() {
		return
	}
	event := DiagnosticEvent{
		Name:       name,
		UID:        operation.UID,
		Route:      operation.request.Route,
		Lane:       operation.LaneKey,
		Generation: ck.run.Generation(),
		Operation:  operation.ID,
		Task:       operation.Task,
		Source:     operation.Source,
		Count:      len(operation.claims),
		Rollback:   operation.compositeRollback,
		Composite:  operation.parent != nil || operation.composite != nil,
	}
	if operation.plan.Transaction != nil {
		event.Resource = operation.plan.Transaction.ID
	}
	ck.trace(event)
}

func (ck *CommandKernel) observe(event DiagnosticEvent) {
	if ck == nil {
		return
	}
	if event.Generation == 0 {
		event.Generation = ck.run.Generation()
	}
	ObserveDiagnostic(ck.diagnosticObserver, event)
}
