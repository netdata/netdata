// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestAvailabilityAttemptDiagnosticsDistinguishControlAndMixedFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		cause  error
		report bool
	}{
		{name: "canceled", cause: context.Canceled},
		{name: "retired", cause: jobmgr.ErrProcessAttemptRetired},
		{name: "stopped", cause: jobmgr.ErrProcessAttemptStopped},
		{name: "superseded", cause: jobmgr.ErrProcessAttemptSuperseded},
		{name: "joined control", cause: errors.Join(context.Canceled, jobmgr.ErrProcessAttemptRetired)},
		{name: "mixed failure", cause: errors.Join(context.Canceled, errors.New("synthetic-password")), report: true},
		{name: "deadline", cause: jobmgr.ErrProcessAttemptDeadline, report: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			diagnostics := &availabilityDiagnosticRecorder{}
			controller, _, err := NewContainedController(
				context.Background(), 1, attempts, diagnostics, collectorapi.Registry{},
			)
			require.NoError(t, err)
			var blocking atomic.Bool
			entered := make(chan struct{})
			release := make(chan struct{})
			bundle, err := newAgentFunctionBundle("module", collectorapi.Creator{
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &controllerTestHandler{} },
			}, []funcapi.FunctionConfig{{
				ID: "method",
				Available: func() bool {
					if blocking.Load() {
						close(entered)
						<-release
					}
					return true
				},
			}})
			require.NoError(t, err)
			require.NoError(t, bundle.bindContainment(attempts, 1, "module-poll", "module"))
			t.Cleanup(func() {
				close(release)
				bundle.retire()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, bundle.wait(ctx))
				require.NoError(t, controller.AbortConstruction(ctx))
				require.NoError(t, attempts.Shutdown(ctx))
			})
			blocking.Store(true)
			poll, err := bundle.startAvailabilityPoll()
			require.NoError(t, err)
			waitBundleContainmentTestValue(t, entered, "availability callback")
			require.True(t, poll.attempt.Cut(test.cause))
			finished := make(chan struct{})
			go func() {
				controller.finishAvailabilityPoll("module", collectorapi.Creator{}, poll)
				close(finished)
			}()
			waitBundleContainmentTestValue(t, finished, "availability failure diagnostic")
			if !test.report {
				require.Empty(t, diagnostics.events)
				return
			}
			require.Equal(t, []jobmgr.DiagnosticEvent{{
				Name:       DiagnosticAvailabilityAttemptFailed,
				Resource:   "module",
				Generation: 1,
				State:      "failed",
				Level:      jobmgr.DiagnosticWarning,
			}}, diagnostics.events)
		})
	}
}

type availabilityDiagnosticRecorder struct {
	events []jobmgr.DiagnosticEvent
}

func (r *availabilityDiagnosticRecorder) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	r.events = append(r.events, event)
}
