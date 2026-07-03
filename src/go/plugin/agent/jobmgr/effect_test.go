// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Deadline-disabled pin: the effect path applies NO deadline and NO
// cancellation to detection - a slow detection completes and its job starts.
// The stage that activates effect deadlines must flip this pin deliberately
// together with its abandon/late-return semantics, never delete it.
func TestEffect_DeadlineDisabledPin(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"slow detection sees no deadline, no cancellation, and still starts its job": {
			run: func(t *testing.T) {
				var sawDeadline, sawCancel atomic.Bool

				reg := collectorapi.Registry{}
				reg.Register("slowok", collectorapi.Creator{
					JobConfigSchema: collectorapi.MockConfigSchema,
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							InitFunc: func(ctx context.Context) error {
								if _, ok := ctx.Deadline(); ok {
									sawDeadline.Store(true)
								}
								time.Sleep(500 * time.Millisecond)
								if ctx.Err() != nil {
									sawCancel.Store(true)
								}
								return nil
							},
							ChartsFunc: func() *collectorapi.Charts {
								return &collectorapi.Charts{&collectorapi.Chart{ID: "id", Title: "t", Units: "u", Dims: collectorapi.Dims{{ID: "d1"}}}}
							},
							CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"d1": 1} },
						}
					},
				})

				h := startCharManager(t, reg)

				h.dyncfg("1-add", []string{h.mgr.dyncfgModID("slowok"), "add", "s"}, []byte("{}"))
				h.dyncfg("2-enable", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("slowok", "s")), "enable"}, nil)

				require.Eventually(t, h.outputContains("CONFIG test:collector:slowok:s status running"), charWait, charTick,
					"slow detection must complete and start the job - no abandon path may fire")
				assert.False(t, sawDeadline.Load(), "detection effect context must carry no deadline")
				assert.False(t, sawCancel.Load(), "detection effect context must never be cancelled")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

// effectDoneCh is armed in every run-loop select state; a stray completion
// must be drained (bug-detector arm) without wedging the loop.
func TestEffect_DoneChannelLoopArms(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, h *charHarness)
	}{
		"idle select drains a completion and keeps serving": {
			run: func(t *testing.T, h *charHarness) {
				h.mgr.effectDoneCh <- effectResult{key: "stray"}

				require.Eventually(t, func() bool { return len(h.mgr.effectDoneCh) == 0 }, charWait, charTick,
					"the run loop did not drain the effect completion")

				h.dyncfg("after-idle-drain", []string{h.mgr.dyncfgJobID(prepareDyncfgCfg("success", "x")), "get"}, nil)
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN after-idle-drain"), charWait, charTick,
					"the run loop stopped serving after draining a completion")
			},
		},
		"wait-mode select drains a completion and keeps serving dyncfg": {
			run: func(t *testing.T, h *charHarness) {
				cfg := prepareUserCfg("success", "waity")
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, func() bool { return h.mgr.collectorHandler.WaitingForDecision() }, charWait, charTick)

				h.mgr.effectDoneCh <- effectResult{key: "stray"}

				require.Eventually(t, func() bool { return len(h.mgr.effectDoneCh) == 0 }, charWait, charTick,
					"the wait-mode select did not drain the effect completion")

				h.dyncfg("after-wait-drain", []string{h.mgr.dyncfgJobID(cfg), "update"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN after-wait-drain"), charWait, charTick,
					"wait mode stopped serving dyncfg after draining a completion")
				assert.True(t, h.mgr.collectorHandler.WaitingForDecision(),
					"a stray completion must not clear the wait gate")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := collectorapi.Registry{}
			reg.Register("success", charSuccessCreator())

			tc.run(t, startCharManager(t, reg))
		})
	}
}
