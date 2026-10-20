// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

type recordingDiagnosticObserver struct {
	mu     sync.Mutex
	events []jobmgr.DiagnosticEvent
}

func (rdo *recordingDiagnosticObserver) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	rdo.mu.Lock()
	defer rdo.mu.Unlock()
	rdo.events = append(rdo.events, event)
}

func (rdo *recordingDiagnosticObserver) snapshot() []jobmgr.DiagnosticEvent {
	rdo.mu.Lock()
	defer rdo.mu.Unlock()
	return slices.Clone(rdo.events)
}

func TestDynCfgJobDiagnosticsFollowAppliedCommandWithoutRequestContents(t *testing.T) {
	const payloadSentinel = "job-config-payload-must-not-appear"
	diagnostics := &recordingDiagnosticObserver{}
	controller, _, supervisor, _, _ := newDynCfgJobTestHarnessWithDiagnostics(t, diagnostics)
	scope := lifecycle.ResourceTransactionScope{
		ID: "module_job",
	}
	request := DynCfgJobRequest{
		Args:         []string{"go.d:collector:module", "add", "job"},
		Payload:      []byte(`{"option":"` + payloadSentinel + `"}`),
		ContentType:  "application/json",
		CallerSource: "user=test",
		HasPayload:   true,
	}
	applyDynCfgJobDiagnosticRequest(t, controller, supervisor, request, scope, "diagnostic-add")

	events := diagnostics.snapshot()
	index := slices.IndexFunc(events, func(event jobmgr.DiagnosticEvent) bool {
		return event.Name == "job configuration command completed"
	})
	require.NotEqual(t, -1, index)
	operational := events[index]
	require.Equal(t, jobmgr.DiagnosticInfo, operational.Level)
	require.Equal(t, "module_job", operational.Resource)
	require.Equal(t, "add", operational.Command)
	require.Equal(t, "accepted", operational.State)
	require.EqualValues(t, 202, operational.ResultStatus)
	require.EqualValues(t, 9, operational.Generation)
	require.NotContains(t, fmt.Sprintf("%+v", events), payloadSentinel)
}

func TestDynCfgJobDiagnosticsIdentifyRejectedCommands(t *testing.T) {
	tests := map[string]struct {
		request      DynCfgJobRequest
		scope        lifecycle.ResourceTransactionScope
		wantResource string
		wantCommand  string
		wantStatus   int
	}{
		"missing add job name": {
			request: DynCfgJobRequest{
				Args: []string{"go.d:collector:module", "add"},
			},
			scope: lifecycle.ResourceTransactionScope{
				ID: "module",
			},
			wantResource: "module",
			wantCommand:  "add",
			wantStatus:   400,
		},
		"unknown module": {
			request: DynCfgJobRequest{
				Args: []string{"go.d:collector:unknown", "ENABLE"},
			},
			scope: lifecycle.ResourceTransactionScope{
				ID: "unknown",
			},
			wantResource: "unknown",
			wantCommand:  "enable",
			wantStatus:   404,
		},
		"internal fallback resource omitted": {
			request: DynCfgJobRequest{
				Args: []string{"invalid-config-id", "enable"},
			},
			scope: lifecycle.ResourceTransactionScope{
				ID: "\x00dyncfg-invalid",
			},
			wantCommand: "enable",
			wantStatus:  400,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			diagnostics := &recordingDiagnosticObserver{}
			controller, _, supervisor, _, _ := newDynCfgJobTestHarnessWithDiagnostics(t, diagnostics)
			applyDynCfgJobDiagnosticRequest(t, controller, supervisor, test.request, test.scope, "diagnostic-rejected")

			events := diagnostics.snapshot()
			index := slices.IndexFunc(events, func(event jobmgr.DiagnosticEvent) bool {
				return event.Name == "job configuration command failed"
			})
			require.NotEqual(t, -1, index)
			operational := events[index]
			require.Equal(t, jobmgr.DiagnosticWarning, operational.Level)
			require.Equal(t, test.wantResource, operational.Resource)
			require.Equal(t, test.wantCommand, operational.Command)
			require.Equal(t, "removed", operational.State)
			require.Equal(t, test.wantStatus, operational.ResultStatus)
			require.EqualValues(t, 9, operational.Generation)
		})
	}
}

func applyDynCfgJobDiagnosticRequest(
	t *testing.T,
	controller *DynCfgJobController,
	supervisor *lifecycle.TaskSupervisor,
	request DynCfgJobRequest,
	scope lifecycle.ResourceTransactionScope,
	uid string,
) {
	t.Helper()
	plan, err := lifecycle.NewResourceTransactionTaskPlan(
		lifecycle.SourceFunction,
		time.Time{},
		lifecycle.TransactionTaskPhases,
		nil,
		scope,
		func(
			ctx context.Context,
			current lifecycle.ReadyResource,
			taskScope lifecycle.ResourceTransactionScope,
			permit lifecycle.LongLivedPermit,
		) (lifecycle.PreparedResourceTransaction, error) {
			return controller.Prepare(ctx, request, current, taskScope, permit)
		},
	)
	require.NoError(t, err)
	applyAndEncodeDynCfgJobTestTask(t, supervisor, plan, scope, uid)
}
