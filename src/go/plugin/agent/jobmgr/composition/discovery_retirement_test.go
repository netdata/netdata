// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryPreparationProcessRetirement(t *testing.T) {
	tests := map[string]struct {
		cut func(*containment.Authority)
	}{
		"process shutdown": {cut: func(a *containment.Authority) { a.BeginShutdown() }},
		"run rotation":     {cut: func(a *containment.Authority) { a.CutTarget(1) }},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			entered, returned := make(chan struct{}), make(chan struct{})
			cfg := confgroup.Config{
				"module": "module", "name": "job", "update_every": 1,
			}.SetSourceType(confgroup.TypeUser).SetProvider(confgroup.TypeUser).SetSource("file=test")
			modules := collectorapi.Registry{
				"module": {
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							CheckFunc: func(ctx context.Context) error {
								close(entered)
								defer close(returned)
								<-ctx.Done()
								return ctx.Err()
							},
						}
					},
					Config: func() any { return &collectorapi.MockConfiguration{} },
				},
			}
			frames, err := lifecycle.NewFrameOwner(newProcessSynchronizedBuffer())
			require.NoError(t, err)
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			uids := lifecycle.NewUIDLedger()
			generation, err := newTestRunGeneration(t, runGenerationConfig{
				Secrets:         testRunSecrets(t),
				Generation:      1,
				ShutdownTimeout: time.Second,
				UIDs:            uids,
				Frames:          frames,
				Attempts:        attempts,
				Modules:         modules,
				Jobs:            testRunJobServices(t),
				Discovery:       testRunDiscoveryServices(t, cfg),
			})
			require.NoError(t, err)
			t.Cleanup(func() {
				attempts.BeginShutdown()
				generation.Stop()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, generation.Wait(ctx))
				require.NoError(t, attempts.Shutdown(ctx))
				closeRunTestUIDs(t, uids)
			})
			require.NoError(t, generation.start(context.Background()))
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("discovered collector did not enter Check")
			}
			// Inventory application returns before the probe. The real discovery
			// supervisor must remain live while process retirement cuts its worker.
			test.cut(attempts)
			select {
			case <-returned:
			case <-ctx.Done():
				t.Fatal("retired discovery preparation did not settle")
			}
			require.NoError(t, ctx.Err())
			require.NoError(t, generation.run.DirtyCause())
			require.Never(t, func() bool {
				_, exists := generation.vnodes.graph.Lookup(cfg.FullName())
				return exists || generation.run.DirtyCause() != nil
			}, 100*time.Millisecond, time.Millisecond, "retired preparation cannot publish or dirty the run")
		})
	}
}
