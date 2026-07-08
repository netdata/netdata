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

// INVERTED PIN (previously the deadline-disabled pin): detection now runs
// under the internal effect deadline - the context carries it - and a
// detection that finishes within the cap starts its job normally with no
// abandon path fired.
func TestEffect_DeadlineBehaviorPin(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"detection under the cap sees the deadline and still starts its job": {
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
								time.Sleep(300 * time.Millisecond)
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
					"in-deadline detection must complete and start the job")
				assert.True(t, sawDeadline.Load(), "detection context must carry the effect deadline")
				assert.False(t, sawCancel.Load(), "an in-deadline detection must not observe cancellation")
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
		"stray completion with a wait-parked key neither wedges nor unparks": {
			run: func(t *testing.T, h *charHarness) {
				cfg := prepareUserCfg("success", "waity")
				h.in <- prepareCfgGroups(cfg.Source(), cfg)
				require.Eventually(t, h.outputContains("CONFIG test:collector:success:waity create accepted"), charWait, charTick)

				h.mgr.effectDoneCh <- effectResult{key: "stray"}

				require.Eventually(t, func() bool { return len(h.mgr.effectDoneCh) == 0 }, charWait, charTick,
					"the run loop did not drain the effect completion")

				h.dyncfg("after-wait-drain", []string{h.mgr.dyncfgJobID(cfg), "update"}, []byte("{}"))
				require.Eventually(t, h.outputContains("FUNCTION_RESULT_BEGIN after-wait-drain"), charWait, charTick,
					"the loop stopped serving dyncfg after draining a completion")
				assert.False(t, h.outputContains("CONFIG test:collector:success:waity status running")(),
					"a stray completion must not unpark a waiting key")
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
