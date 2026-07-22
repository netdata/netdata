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

func (*recordingDiagnosticObserver) TraceEnabled() bool { return true }

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
	applyAndEncodeDynCfgJobTestTask(t, supervisor, plan, scope, "diagnostic-add")

	events := diagnostics.snapshot()
	names := make([]string, 0, len(events))
	for _, event := range events {
		names = append(names, event.Name)
	}
	require.Contains(t, names, "job configuration transaction prepared")
	require.Contains(t, names, "job configuration transaction applied")
	require.Contains(t, names, "job configuration command completed")
	operational := events[slices.IndexFunc(events, func(event jobmgr.DiagnosticEvent) bool {
		return event.Name == "job configuration command completed"
	})]
	require.Equal(t, jobmgr.DiagnosticInfo, operational.Level)
	require.Equal(t, "module_job", operational.Resource)
	require.Equal(t, "add", operational.Command)
	require.Equal(t, "accepted", operational.State)
	require.EqualValues(t, 202, operational.ResultStatus)
	require.EqualValues(t, 9, operational.Generation)
	require.NotContains(t, fmt.Sprintf("%+v", events), payloadSentinel)
}
