// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

// Public UPDATE physically interrupts an enabled Accepted worker before the edit
// is admitted. Every non-adoption path must restore only its retained predecessor.
func TestCandidatePreflightResumesInterruptedAcceptedWork(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for outcome, expectedCode := range map[string]int{"rejected": 422, "canceled": 499, "deadline": 504} {
			for _, disable := range []bool{false, true} {
				name := version + "/" + outcome + "/resume"
				if disable {
					name = version + "/" + outcome + "/disable"
				}
				t.Run(name, func(t *testing.T) {
					// Fake time cannot expire UPDATE while this goroutine queues DISABLE.
					synctest.Test(t, func(t *testing.T) {
						config := continuationConfigs(false)[0]
						original := config["option_str"].(string)
						config["option_int"] = 17
						initialEntered, initialReturned := make(chan struct{}), make(chan struct{})
						editEntered, releaseEdit, editReturned := make(chan struct{}), make(chan struct{}), make(chan struct{})
						var releaseOnce sync.Once
						var originalChecks atomic.Int32
						state := &continuationCollectorState{check: func(ctx context.Context, option string) error {
							if option == original {
								if originalChecks.Add(1) == 1 {
									close(initialEntered)
									<-ctx.Done()
									close(initialReturned)
									return ctx.Err()
								}
								return nil
							}
							close(editEntered)
							defer close(editReturned)
							select {
							case <-releaseEdit:
								return collectorapi.PermanentError(errors.New("edit rejected by collector"))
							case <-ctx.Done():
								return ctx.Err()
							}
						}}
						fixture := newContinuationFixture(t, version, false, state, []confgroup.Config{config})
						t.Cleanup(func() { releaseOnce.Do(func() { close(releaseEdit) }) })
						fixture.enableIfPassive(false)
						preflightResumeWait(t, initialEntered, "initial accepted Check")
						fixture.assertExposed("before-edit", config)

						deadline := time.Now().Add(3 * time.Second)
						if outcome == "deadline" {
							deadline = time.Now().Add(500 * time.Millisecond)
						}
						const editUID = "interrupting-update"
						require.NoError(t, fixture.generation.kernel.Submit(t.Context(), jobmgr.Request{
							UID:          editUID,
							Source:       lifecycle.SourceFunction,
							Route:        "config",
							Args:         []string{"go.d:collector:module:local2", "update"},
							HasPayload:   true,
							Payload:      []byte(`{"option_str":"edited","option_int":99}`),
							ContentType:  "application/json",
							CallerSource: "user=edit",
							Deadline:     deadline,
						}))
						preflightResumeWait(t, editEntered, "replacement Check")
						preflightResumeWait(t, initialReturned, "physically interrupted predecessor Check")
						require.Equal(t, []string{original, "edited"}, state.checked())
						require.EqualValues(t, 1, originalChecks.Load())

						// Queue DISABLE before the UPDATE relinquishes its lane. Its
						// intent revocation must precede the later resume obligation.
						const disableUID = "disable-interrupted-intent"
						if disable {
							require.NoError(t, fixture.generation.kernel.Submit(t.Context(), jobmgr.Request{
								UID:      disableUID,
								Source:   lifecycle.SourceFunction,
								Route:    "config",
								Args:     []string{"go.d:collector:module:local2", "disable"},
								Deadline: time.Now().Add(3 * time.Second),
							}))
						}
						switch outcome {
						case "rejected":
							releaseOnce.Do(func() { close(releaseEdit) })
						case "canceled":
							require.NoError(t, fixture.generation.kernel.Cancel(t.Context(), editUID))
						}
						fixture.output.waitContains(t, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", editUID, expectedCode))
						preflightResumeWait(t, editReturned, "terminal replacement Check")
						if disable {
							fixture.output.waitContains(t, "FUNCTION_RESULT_BEGIN "+disableUID+" 200 ")
							fixture.waitStatus(dyncfg.StatusDisabled)
							require.Never(t, func() bool {
								record, ok := fixture.generation.vnodes.graph.Lookup(config.FullName())
								return !ok || record.Status != dyncfg.StatusDisabled.String() || originalChecks.Load() != 1 || state.maxActive.Load() != 0
							}, 100*time.Millisecond, time.Millisecond, "revoked predecessor cannot resume after the edit settles")
							require.Equal(t, []string{original, "edited"}, state.checked())
						} else {
							fixture.waitStatus(dyncfg.StatusRunning)
							require.Equal(t, []string{original, "edited", original}, state.checked(), "one fresh predecessor attempt must resume")
							require.EqualValues(t, 2, originalChecks.Load())
							if version == "v2" {
								require.EqualValues(t, 1, state.active.Load())
								require.EqualValues(t, 1, state.maxActive.Load())
							}
						}
						fixture.assertExposed("after-edit", config)
						fixture.assertLastMetadata(config)
						if !disable {
							require.Never(t, func() bool {
								return originalChecks.Load() != 2 || len(state.checked()) != 3
							}, 100*time.Millisecond, time.Millisecond, "duplicate terminal/resume delivery cannot issue another successor")
						}
						require.Equal(t, 1, strings.Count(fixture.output.String(), "FUNCTION_RESULT_BEGIN "+editUID+" "))
						require.NoError(t, fixture.generation.run.DirtyCause())
					})
				})
			}
		}
	}
}

func preflightResumeWait(t *testing.T, event <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-event:
	case <-time.After(time.Second):
		t.Fatalf("did not observe %s", description)
	}
}
