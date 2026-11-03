// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestInitialStoreStructuralFailureUsesValidationDiagnostic(t *testing.T) {
	for name, value := range map[string]any{
		"invalid type":    map[string]any{"bad": "type"},
		"store reference": "${store:vault:other:key}",
	} {
		t.Run(name, func(t *testing.T) {
			config := secretTestConfig(confgroup.TypeUser, "unused")
			config["value"] = value
			controller, store := newSecretControllerTestHarness(t, []secretstore.Config{config})
			diagnostics := &secretRecordingDiagnosticObserver{}
			controller.diagnostics = diagnostics
			require.NoError(t, controller.Bind(restartTestJobs{}))
			commands := &applyingInitialStoreTestCommands{
				publishTemplates: controller.templateCleanup(),
			}
			t.Cleanup(func() {
				require.NoError(t, controller.CloseProjection())
				require.NoError(t, commands.Finalize())
				require.NoError(t, store.Close(context.Background()))
			})
			require.NoError(t, controller.PublishInitial(t.Context(), commands))
			entry, ok := controller.entry("vault:main")
			require.True(t, ok)
			require.Equal(t, dyncfg.StatusFailed, entry.status)
			require.Equal(t, []jobmgr.DiagnosticEvent{{
				Level:      jobmgr.DiagnosticWarning,
				Name:       "secretstore configuration validation failed",
				Resource:   "secretstore:vault_main",
				State:      "failed",
				Generation: 1,
				Err:        errors.New(msgSecretStoreValidationFailed),
			}}, diagnostics.snapshot())
		})
	}
}

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
