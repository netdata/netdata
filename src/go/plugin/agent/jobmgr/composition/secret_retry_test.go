// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

// A committed Store rotation must use normal automatic activation recovery. An
// explicit disable after the failure must cancel that desired activation.
func TestSecretReplacementTransientProviderRecovery(t *testing.T) {
	for version, v2 := range map[string]bool{"v1": false, "v2": true} {
		t.Run(version, func(t *testing.T) {
			for name, disable := range map[string]bool{"automatic retry": false, "disable cancels retry": true} {
				t.Run(name, func(t *testing.T) {
					testSecretReplacementProviderRecovery(t, secretReplacementScenario{
						v2:      v2,
						disable: disable,
					})
				})
			}
		})
	}
}

// Result-limit validation runs in the shared resolver before either collector's
// Init. One version exercises the permanent failure contract for this path.
func TestSecretReplacementPermanentFailureDoesNotRetry(t *testing.T) {
	testSecretReplacementProviderRecovery(t, secretReplacementScenario{
		oversized: true,
	})
}

type secretReplacementScenario struct {
	v2        bool
	disable   bool
	oversized bool
}

func testSecretReplacementProviderRecovery(t *testing.T, scenario secretReplacementScenario) {
	t.Helper()
	var recovered atomic.Bool
	var replacementReads atomic.Int32
	starts := make(chan string, 4)
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				collector := &collectorapi.MockCollectorV1{}
				collector.InitFunc = func(ctx context.Context) error {
					select {
					case starts <- collector.Config.OptionStr:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return collector
			},
			Config: func() any { return &collectorapi.MockConfiguration{} },
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &runTestHandler{
					cleanup: func() {},
				}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	if scenario.v2 {
		creator := modules["module"]
		creator.Create = nil
		creator.CreateV2 = func() collectorapi.CollectorV2 {
			collector := &secretRetryCollectorV2{
				store: metrix.NewCollectorStore(),
			}
			collector.InitFunc = func(ctx context.Context) error {
				select {
				case starts <- collector.Config.OptionStr:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return collector
		}
		modules["module"] = creator
	}
	jobConfig := confgroup.Config{
		"module":              "module",
		"name":                "job",
		"update_every":        1,
		"autodetection_retry": 1,
		"function_only":       true,
		"option_str":          "${store:vault:main:key}",
		"option_int":          1,
	}
	jobConfig.SetProvider(confgroup.TypeDyncfg)
	jobConfig.SetSourceType(confgroup.TypeDyncfg)
	jobConfig.SetSource("test")
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1, AutoDetectionRetry: 1},
	}
	creators, err := secretstore.NewCreatorCatalog([]secretstore.Creator{{
		Kind: secretstore.KindVault, Schema: `{}`,
		Create: func() secretstore.Store {
			return &retrySecretStore{
				replacementReads: &replacementReads,
				recovered:        &recovered,
			}
		},
	}})
	require.NoError(t, err)
	jobs.StoreCreators = creators
	reader, writer := io.Pipe()
	output := &secretRetryTickOutput{
		processSynchronizedBuffer: newProcessSynchronizedBuffer(),
	}
	process, err := newProcessCore(processCoreConfig{
		Input:           reader,
		Output:          output,
		ShutdownTimeout: time.Second,
		KeepAlive:       true,
		Modules:         modules,
		Jobs:            jobs,
		Secrets: runSecretServices{
			Initial: []secretstore.Config{{
				"name": "main", "kind": string(secretstore.KindVault), "value": "initial",
				"__source__": confgroup.TypeUser, "__source_type__": confgroup.TypeUser,
			}},
		},
		Discovery:   testRunDiscoveryServices(t, jobConfig),
		Diagnostics: testProcessDiagnostics(),
	})
	require.NoError(t, err)
	controls := newTestProcessControls(1)
	done := make(chan error, 1)
	go func() { done <- process.run(context.Background(), controls) }()
	t.Cleanup(func() {
		controls.sendTerminate(testProcessControl())
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Error("test process did not terminate")
		}
		require.NoError(t, writer.Close())
	})
	waitSecretStart(t, starts, "initial")
	output.waitContains(t, "CONFIG go.d:collector:module:job create running job")
	replacement := "replacement"
	if scenario.oversized {
		replacement = "oversized"
	}
	_, err = fmt.Fprintf(writer,
		"FUNCTION_PAYLOAD secret-rotation 30 \"config go.d:secretstore:vault:main update\" "+
			"0xFFFF \"user=test\" application/json\n{\"value\":%q}\nFUNCTION_PAYLOAD_END\n", replacement)
	require.NoError(t, err)
	output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-rotation 200 application/json")
	output.waitContains(t, "CONFIG go.d:collector:module:job status failed")
	require.GreaterOrEqual(t, replacementReads.Load(), int32(1))
	require.Contains(t, output.String(), "dependent collector restarts failed for jobs: module:job")

	if scenario.disable {
		_, err = io.WriteString(writer,
			"FUNCTION secret-disable 30 \"config go.d:collector:module:job disable\" 0xFFFF \"user=test\"\n")
		require.NoError(t, err)
		output.waitContains(t, "FUNCTION_RESULT_BEGIN secret-disable 200 application/json")
		output.waitContains(t, "CONFIG go.d:collector:module:job status disabled")
	}
	readsBeforeRecovery := replacementReads.Load()
	if scenario.oversized {
		require.EqualValues(t, 1, readsBeforeRecovery, "permanent result-limit failure must not be retried")
	}
	wireBeforeRecovery := len(output.String())
	recovered.Store(true)

	// Keepalives follow real scheduler ticks. Five ticks exceed the one-second
	// retry interval, including the retry clock's initial baseline tick.
	initialTicks := output.ticks.Load()
	require.Eventually(
		t,
		func() bool { return output.ticks.Load() >= initialTicks+5 },
		7*time.Second,
		10*time.Millisecond,
	)
	wireAfterRecovery := output.String()[wireBeforeRecovery:]
	if scenario.disable || scenario.oversized {
		require.Equal(
			t,
			readsBeforeRecovery,
			replacementReads.Load(),
			"disabled or permanently failed job must not resolve secrets again",
		)
		require.NotContains(t, wireAfterRecovery, "CONFIG go.d:collector:module:job status running")
		require.NotContains(t, wireAfterRecovery, "CONFIG go.d:collector:module:job create running job")
		select {
		case value := <-starts:
			t.Fatalf("disabled or permanently failed collector restarted with secret %q", value)
		default:
		}
		return
	}
	require.True(
		t,
		strings.Contains(wireAfterRecovery, "CONFIG go.d:collector:module:job status running") ||
			strings.Contains(wireAfterRecovery, "CONFIG go.d:collector:module:job create running job"),
		"transient provider failure did not recover after five real scheduler ticks; replacement reads: %d",
		replacementReads.Load(),
	)
	waitSecretStart(t, starts, "replacement")
	require.Greater(t, replacementReads.Load(), readsBeforeRecovery)
}

type secretRetryTickOutput struct {
	*processSynchronizedBuffer
	ticks atomic.Int32
}

func (output *secretRetryTickOutput) Write(payload []byte) (int, error) {
	count, err := output.processSynchronizedBuffer.Write(payload)
	if err == nil && len(payload) == 1 && payload[0] == '\n' {
		output.ticks.Add(1)
	}
	return count, err
}

type retrySecretStore struct {
	config struct {
		Value string `yaml:"value"`
	}
	replacementReads *atomic.Int32
	recovered        *atomic.Bool
}

func (store *retrySecretStore) Configuration() any   { return &store.config }
func (*retrySecretStore) Init(context.Context) error { return nil }
func (store *retrySecretStore) Publish() secretstore.PublishedStore {
	return retryPublishedSecret{
		value:            store.config.Value,
		replacementReads: store.replacementReads,
		recovered:        store.recovered,
	}
}

type retryPublishedSecret struct {
	value            string
	replacementReads *atomic.Int32
	recovered        *atomic.Bool
}

func (store retryPublishedSecret) Resolve(context.Context, secretstore.ResolveRequest) (string, error) {
	if store.value == "oversized" {
		store.replacementReads.Add(1)
		// A successful backend response can still violate the resolver's result
		// bound. It must become AtomicErrorResultLimit, not a transient provider error.
		return strings.Repeat("x", secretresolver.MaximumAtomicResolvedBytes+1), nil
	}
	if store.value == "replacement" {
		store.replacementReads.Add(1)
		if !store.recovered.Load() {
			return "", errors.New("synthetic temporary backend outage")
		}
	}
	return store.value, nil
}

// Reuse the V1 mock's configuration and lifecycle hooks while satisfying the
// V2 runtime contract; function-only activation does not collect metrics.
type secretRetryCollectorV2 struct {
	collectorapi.MockCollectorV1 `yaml:",inline"`
	store                        metrix.CollectorStore
}

func (*secretRetryCollectorV2) Collect(context.Context) error        { return nil }
func (c *secretRetryCollectorV2) MetricStore() metrix.CollectorStore { return c.store }
func (*secretRetryCollectorV2) ChartTemplateYAML() string            { return "" }
