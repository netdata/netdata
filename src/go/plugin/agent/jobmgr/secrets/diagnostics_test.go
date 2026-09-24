// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestAcceptedStoreFailureDiagnosticsPreserveOnlyPublicDetail(t *testing.T) {
	for _, public := range []bool{false, true} {
		diagnostics := &secretRecordingDiagnosticObserver{}
		controller := &Controller{
			epoch:       1,
			diagnostics: diagnostics,
		}
		err := errors.New("private credential response")
		if public {
			err = dyncfg.NewPublicError("credential unavailable", err)
		}
		controller.observeActivationFailure("vault:main", err)
		events := diagnostics.snapshot()
		require.Len(t, events, 1)
		require.Equal(t, jobmgr.DiagnosticWarning, events[0].Level)
		require.Equal(t, "failed", events[0].State)
		require.Contains(t, events[0].Err.Error(), "Secretstore activation failed")
		require.NotContains(t, events[0].Err.Error(), "private credential")
		if public {
			require.Contains(t, events[0].Err.Error(), "credential unavailable")
		}
	}
}

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
