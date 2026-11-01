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

// Automatic admission yields the shared graph claim during validation.
// The same job's lane remains held until the validation result is adopted.
func TestProcessAutomaticCandidateAdmission(t *testing.T) {
	for name, disable := range map[string]bool{"dependency activation": false, "same-name serialization": true} {
		t.Run(name, func(t *testing.T) {
			source := &testDependencySource{callbacks: make(map[int]func(string, string))}
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			entered := make(chan struct{}, 4)
			var validate atomic.Bool
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
					if validate.Load() {
						entered <- struct{}{}
						<-release
					}
					return &continuationV2Collector{
						readinessIntegrationCollector: readinessIntegrationCollector{store: metrix.NewCollectorStore()},
						state:                         state,
					}
				},
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

			// ADD stores the unrelated config without enabling it.
			_, err = fmt.Fprintln(writer, "FUNCTION_PAYLOAD other-add 8 \"config go.d:collector:test add other\" 0xFFFF \"user=test\" application/json\n{\"option_str\":\"other\"}\nFUNCTION_PAYLOAD_END")
			require.NoError(t, err)
			output.waitContains(t, "FUNCTION_RESULT_BEGIN other-add 202 ")
			validate.Store(true)
			batches <- []*confgroup.Group{{Source: "test", Configs: []confgroup.Config{
				{"module": "test", "name": "browser", "option_str": "browser", "__source_type__": "user", "__source__": "test", "__provider__": "test"},
			}}}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("automatic candidate did not validate configuration")
			}
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
				}, 50*time.Millisecond, time.Millisecond, "same-name mutation bypassed the admission lane")
			}
			unblock()
			source.publish()
			if disable {
				output.waitContains(t, "FUNCTION_RESULT_BEGIN browser-disable 200 ")
				output.waitContains(t, "CONFIG go.d:collector:test:browser status disabled")
			} else {
				output.waitContains(t, "CONFIG go.d:collector:test:browser status running")
				require.EqualValues(t, 1, state.active.Load())
				require.Equal(t, []string{"browser"}, state.checked())
			}
		})
	}
}
