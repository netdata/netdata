// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"slices"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

type secretRecordingDiagnosticObserver struct {
	mu     sync.Mutex
	events []jobmgr.DiagnosticEvent
}

func (srdo *secretRecordingDiagnosticObserver) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	srdo.mu.Lock()
	defer srdo.mu.Unlock()
	srdo.events = append(srdo.events, event)
}

func (srdo *secretRecordingDiagnosticObserver) snapshot() []jobmgr.DiagnosticEvent {
	srdo.mu.Lock()
	defer srdo.mu.Unlock()
	return slices.Clone(srdo.events)
}
