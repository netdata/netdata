// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

type handoffCollector struct {
	readinessIntegrationCollector
	inits, checks *atomic.Int32
	cleanupGate   <-chan struct{}
	check         func() error
}

func (c *handoffCollector) Init(context.Context) error { c.inits.Add(1); return nil }
func (c *handoffCollector) Check(context.Context) error {
	c.checks.Add(1)
	if c.check != nil {
		return c.check()
	}
	return nil
}
func (c *handoffCollector) Run(ctx context.Context, ready func()) error {
	ready()
	<-ctx.Done()
	return ctx.Err()
}
func (c *handoffCollector) Cleanup(context.Context) {
	if c.cleanupGate != nil {
		<-c.cleanupGate
	}
}

func TestCandidateHandoffProbesOnce(t *testing.T) {
	for _, command := range []string{"update", "restart"} {
		for _, blocked := range []bool{false, true} {
			name := command + "/released"
			if blocked {
				name = command + "/retiring"
			}
			t.Run(name, func(t *testing.T) {
				var inits, checks, created atomic.Int32
				release := make(chan struct{})
				if !blocked {
					close(release)
				}
				released := !blocked
				modules := collectorapi.Registry{
					"module": {
						CreateV2: func() collectorapi.CollectorV2 {
							c := &handoffCollector{
								readinessIntegrationCollector: readinessIntegrationCollector{
									store: metrix.NewCollectorStore(),
								},
								inits:  &inits,
								checks: &checks,
							}
							if created.Add(1) == 1 {
								c.cleanupGate = release
							}
							return c
						},
						Config: func() any { return &collectorapi.MockConfiguration{} },
					},
				}
				cfg := confgroup.Config{
					"module":       "module",
					"name":         "receiver",
					"update_every": 1,
				}.
					SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
				output := newProcessSynchronizedBuffer()
				frames, err := lifecycle.NewFrameOwner(output)
				require.NoError(t, err)
				uids := lifecycle.NewUIDLedger()
				generation, err := newTestRunGeneration(t, runGenerationConfig{
					Secrets:         testRunSecrets(t),
					Generation:      1,
					ShutdownTimeout: time.Second,
					UIDs:            uids,
					Frames:          frames,
					Modules:         modules,
					Jobs:            testRunJobServices(t),
					Discovery:       testRunDiscoveryServices(t, cfg),
				})
				require.NoError(t, err)
				t.Cleanup(func() {
					if !released {
						close(release)
					}
					generation.Stop()
					require.NoError(t, generation.Wait(context.Background()))
					closeRunTestUIDs(t, uids)
				})
				require.NoError(t, generation.start(context.Background()))
				require.Eventually(t, func() bool {
					record, exists := generation.vnodes.graph.Lookup(cfg.FullName())
					return exists && record.Status == dyncfg.StatusRunning.String()
				}, time.Second, time.Millisecond)
				beforeInits, beforeChecks := inits.Load(), checks.Load()
				request := jobmgr.Request{
					UID:      "handoff",
					Source:   lifecycle.SourceFunction,
					Route:    "config",
					Args:     []string{"go.d:collector:module:receiver", command},
					Deadline: time.Now().Add(5 * time.Second),
				}
				if command == "update" {
					request.HasPayload, request.Payload, request.ContentType = true, []byte(`{"update_every":2}`), "application/json"
					request.CallerSource = "user=test"
				}
				require.NoError(t, generation.kernel.Submit(t.Context(), request))
				require.Eventually(t, func() bool { return checks.Load() > beforeChecks }, time.Second, time.Millisecond)
				if blocked {
					require.Eventually(t, func() bool {
						record, _ := generation.vnodes.graph.Lookup(cfg.FullName())
						return record.Status == dyncfg.StatusAccepted.String()
					}, time.Second, time.Millisecond)
					if command == "update" {
						output.waitContains(t, "FUNCTION_RESULT_BEGIN handoff 202 ")
					}
					if command == "restart" {
						require.NotContains(t, output.String(), "FUNCTION_RESULT_BEGIN handoff ")
					}
					close(release)
					released = true
				}
				if command == "restart" {
					output.waitContains(t, "FUNCTION_RESULT_BEGIN handoff 200 ")
				} else {
					output.waitContains(t, "FUNCTION_RESULT_BEGIN handoff 202 ")
				}
				require.Eventually(t, func() bool {
					record, _ := generation.vnodes.graph.Lookup(cfg.FullName())
					return record.Status == dyncfg.StatusRunning.String()
				}, time.Second, time.Millisecond)
				require.Equal(t, int32(1), inits.Load()-beforeInits)
				require.Equal(t, int32(1), checks.Load()-beforeChecks)
				require.NoError(t, generation.run.DirtyCause())
			})
		}
	}
}

func TestCandidateHandoffRecoversAfterRejectedSupersedingPreflight(t *testing.T) {
	var inits, checks, created atomic.Int32
	release := make(chan struct{})
	var releaseOnce sync.Once
	modules := collectorapi.Registry{
		"module": {
			CreateV2: func() collectorapi.CollectorV2 {
				c := &handoffCollector{
					readinessIntegrationCollector: readinessIntegrationCollector{
						store: metrix.NewCollectorStore(),
					},
					inits:  &inits,
					checks: &checks,
				}
				if created.Add(1) == 1 {
					c.cleanupGate = release
				}
				c.check = func() error {
					if checks.Load() == 3 {
						return collectorapi.PermanentError(errors.New("rejected later edit"))
					}
					return nil
				}
				return c
			},
			Config: func() any { return &collectorapi.MockConfiguration{} },
		},
	}
	cfg := confgroup.Config{
		"module":       "module",
		"name":         "receiver",
		"update_every": 1,
	}.
		SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets:         testRunSecrets(t),
		Generation:      1,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules:         modules,
		Jobs:            testRunJobServices(t),
		Discovery:       testRunDiscoveryServices(t, cfg),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	require.Eventually(t, func() bool {
		record, exists := generation.vnodes.graph.Lookup(cfg.FullName())
		return exists && record.Status == dyncfg.StatusRunning.String()
	}, time.Second, time.Millisecond)
	update := func(uid, payload string) {
		require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
			UID:          uid,
			Source:       lifecycle.SourceFunction,
			Route:        "config",
			Args:         []string{"go.d:collector:module:receiver", "update"},
			HasPayload:   true,
			Payload:      []byte(payload),
			ContentType:  "application/json",
			CallerSource: "user=test",
			Deadline:     time.Now().Add(5 * time.Second),
		}))
	}
	update("accepted", `{"update_every":2}`)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN accepted 202 ")
	accepted, ok := generation.vnodes.graph.Lookup(cfg.FullName())
	require.True(t, ok)
	require.Equal(t, dyncfg.StatusAccepted.String(), accepted.Status)
	require.Equal(t, int32(2), checks.Load())
	update("rejected", `{"update_every":3}`)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN rejected 422 ")
	kept, ok := generation.vnodes.graph.Lookup(cfg.FullName())
	require.True(t, ok)
	require.Equal(t, accepted, kept)
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		record, _ := generation.vnodes.graph.Lookup(cfg.FullName())
		return record.Status == dyncfg.StatusRunning.String() && record.Payload() == accepted.Payload()
	}, time.Second, time.Millisecond)
	require.Equal(t, int32(4), checks.Load(), "only the invalidated candidate must be rebuilt")
	require.NoError(t, generation.run.DirtyCause())
}
