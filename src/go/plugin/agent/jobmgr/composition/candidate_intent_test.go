// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

type candidateIntentDependencyOwner struct {
	source    *testDependencySource
	entered   chan struct{}
	release   <-chan struct{}
	committed atomic.Int32
}

type candidateIntentDependencyReservation struct {
	owner *candidateIntentDependencyOwner
	next  collectorapi.JobConfigIdentity
}

func (o *candidateIntentDependencyOwner) Prepare(_, next collectorapi.JobConfigIdentity, _ map[string]any) (collectorapi.PreparedJobConfigIntent, error) {
	if next.Valid() {
		o.entered <- struct{}{}
		<-o.release
	}
	return &candidateIntentDependencyReservation{owner: o, next: next}, nil
}

func (r *candidateIntentDependencyReservation) Commit() {
	if r.next.Valid() {
		r.owner.committed.Add(1)
		r.owner.source.publish()
	}
}

func (*candidateIntentDependencyReservation) Abort() error { return nil }

// Automatic admission must bootstrap its own dependency without monopolizing
// the shared graph claim. The same job's lane remains held during reservation.
func TestProcessAutomaticCandidateIntentReservation(t *testing.T) {
	for name, disable := range map[string]bool{"dependency bootstrap": false, "same-name serialization": true} {
		t.Run(name, func(t *testing.T) {
			source := &testDependencySource{callbacks: make(map[int]func(string, string))}
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			owner := &candidateIntentDependencyOwner{source: source, entered: make(chan struct{}, 4), release: release}
			state := &continuationCollectorState{}
			batches := make(chan []*confgroup.Group, 1)
			reader, writer := io.Pipe()
			output := newProcessSynchronizedBuffer()
			cfg := testProductionProcessConfig(reader, output)
			cfg.AutoEnable = true
			cfg.Services = []ProcessService{source}
			cfg.Modules["test"] = collectorapi.Creator{
				JobConfigSchema: collectorapi.MockConfigSchema,
				Config:          func() any { return &collectorapi.MockConfiguration{} },
				CreateV2: func() collectorapi.CollectorV2 {
					return &continuationV2Collector{
						readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
						state:                         state,
					}
				},
				JobConfigIntent: owner,
				ActivationDependencies: func(map[string]any) ([]collectorapi.ActivationDependency, error) {
					return []collectorapi.ActivationDependency{{Kind: "test-runtime", Name: "browser", Ready: func() error {
						if source.ready.Load() {
							return nil
						}
						return errors.New("runtime missing")
					}}}, nil
				},
			}
			cfg.DiscoveryProviders = []agentdiscovery.ProviderFactory{
				agentdiscovery.NewProviderFactory("test", func(agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
					return continuationDiscoverer{batches: batches}, true, nil
				}),
			}
			process, err := NewProcess(cfg)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			done := make(chan error, 1)
			go func() { done <- process.Run(ctx) }()
			t.Cleanup(func() {
				unblock()
				shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				defer cancel()
				defer reader.Close()
				defer writer.Close()
				require.NoError(t, process.Terminate(shutdown))
				select {
				case err := <-done:
					require.NoError(t, err)
				case <-shutdown.Done():
					t.Error("process did not join")
				}
				require.Zero(t, source.count())
				if t.Failed() {
					t.Log(output.String())
				}
			})

			// ADD stores the unrelated config without enabling resource demand.
			_, err = fmt.Fprintln(writer, "FUNCTION_PAYLOAD other-add 8 \"config go.d:collector:test add other\" 0xFFFF \"user=test\" application/json\n{\"option_str\":\"other\"}\nFUNCTION_PAYLOAD_END")
			require.NoError(t, err)
			output.waitContains(t, "FUNCTION_RESULT_BEGIN other-add 202 ")
			require.Zero(t, owner.committed.Load())
			batches <- []*confgroup.Group{{Source: "test", Configs: []confgroup.Config{
				{"module": "test", "name": "browser", "option_str": "browser", "__source_type__": "user", "__source__": "test", "__provider__": "test"},
			}}}
			select {
			case <-owner.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("automatic candidate did not reserve dependency demand")
			}
			require.Zero(t, owner.committed.Load())
			require.Zero(t, state.inits.Load(), "unavailable dependency must prevent collector Init")
			require.NotContains(t, output.String(), "CONFIG go.d:collector:test:browser create accepted")

			_, err = fmt.Fprintln(writer, `FUNCTION other-disable 8 "config go.d:collector:test:other disable" 0xFFFF "user=test"`)
			require.NoError(t, err)
			output.waitContains(t, "FUNCTION_RESULT_BEGIN other-disable 200 ")
			if disable {
				_, err = fmt.Fprintln(writer, `FUNCTION browser-disable 8 "config go.d:collector:test:browser disable" 0xFFFF "user=test"`)
				require.NoError(t, err)
				require.Never(t, func() bool {
					return strings.Contains(output.String(), "FUNCTION_RESULT_BEGIN browser-disable ")
				}, 50*time.Millisecond, time.Millisecond, "same-name mutation bypassed the reservation lane")
			}
			unblock()
			if disable {
				output.waitContains(t, "FUNCTION_RESULT_BEGIN browser-disable 200 ")
				output.waitContains(t, "CONFIG go.d:collector:test:browser status disabled")
			} else {
				output.waitContains(t, "CONFIG go.d:collector:test:browser status running")
				require.EqualValues(t, 1, state.active.Load())
				require.Equal(t, []string{"browser"}, state.checked())
			}
			require.EqualValues(t, 1, owner.committed.Load(), "dependency demand must commit exactly once")
		})
	}
}
