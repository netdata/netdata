// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type runtimeFailureProjectionSnapshot struct {
	collectorapi.JobConfigLifecycleSnapshot
	failure collectorapi.JobConfigFailure
}

type runtimeFailureProjectionHook struct{ *failureLifecycleHook }

func (h *runtimeFailureProjectionHook) ProjectFailure(
	snapshot collectorapi.JobConfigLifecycleSnapshot,
	failure collectorapi.JobConfigFailure,
) collectorapi.JobConfigLifecycleSnapshot {
	return &runtimeFailureProjectionSnapshot{
		JobConfigLifecycleSnapshot: snapshot,
		failure:                    failure,
	}
}

func TestRuntimeFailureEnrichesCommittedLifecycleSnapshot(t *testing.T) {
	for _, stage := range []string{"startup", "runtime"} {
		for _, reason := range []string{"error", "unexpected_return", "panic", "startup_timeout"} {
			if stage == "runtime" && reason == "startup_timeout" {
				continue
			}
			t.Run(stage+"/"+reason, func(t *testing.T) {
				controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
				events := []string{}
				hook := &runtimeFailureProjectionHook{
					failureLifecycleHook: &failureLifecycleHook{
						recordingJobConfigLifecycle: &recordingJobConfigLifecycle{
							events: &events,
						},
					},
				}
				creator := controller.modules["module"]
				creator.JobConfigLifecycle = hook
				controller.modules["module"] = creator
				commands := &autoDetectionRetryTestCommands{}
				controller.bindRuntimeFailures(commands, 9, func(err error) { t.Errorf("dispatch: %v", err) })
				if reason == "startup_timeout" {
					controller.factory.startupTimeout = 20 * time.Millisecond
				}
				configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
					return &runtimeTestCollector{
						run: func(ctx context.Context, ready func()) error {
							if stage == "runtime" {
								ready()
							}
							switch reason {
							case "error":
								return errors.New("synthetic receiver failure")
							case "unexpected_return":
								return nil
							case "panic":
								panic("synthetic receiver panic")
							case "startup_timeout":
								<-ctx.Done()
								return ctx.Err()
							}
							return nil
						},
					}
				})
				cfg := factoryTestConfig(false).Set("autodetection_retry", 0)
				current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
				require.NotNil(t, current)
				require.Eventually(t, func() bool {
					var failure *runtimeStartupFailure
					return errors.As(current.(*JobGeneration).StartupResult(), &failure)
				}, time.Second, time.Millisecond)
				plan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
				require.Nil(t, runtimeTestApplyNotification(t, plan, current))
				requireFactoryAttemptsIdle(t, controller.factory)
				record, ok := graph.Lookup(cfg.FullName())
				require.True(t, ok)
				require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
				require.NotNil(t, hook.reconciled, "baseline lifecycle snapshot still reconciles")
				// Busy and quarantine fallbacks also prepare projections. Check the
				// committed snapshot, not the last speculative projector call.
				enriched, ok := hook.reconciled.(*runtimeFailureProjectionSnapshot)
				require.True(t, ok, "committed snapshot must contain %s/%s evidence", stage, reason)
				require.True(t, enriched.failure.Valid())
				require.Equal(t, stage, enriched.failure.Stage)
				require.Equal(t, reason, enriched.failure.Reason)
			})
		}
	}
}
