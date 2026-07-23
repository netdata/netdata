// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

func (ck *CommandKernel) observe(event DiagnosticEvent) {
	if ck == nil {
		return
	}
	if event.Generation == 0 {
		event.Generation = ck.run.Generation()
	}
	ObserveDiagnostic(ck.diagnosticObserver, event)
}
