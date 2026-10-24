// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

type processFunctionTestHandler struct {
	assemblyTestHandler
	requests chan funcapi.RawMethodRequest
	cleanup  func()
	handle   func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse
}

func (h *processFunctionTestHandler) HandleRaw(
	ctx context.Context,
	req funcapi.RawMethodRequest,
) *funcapi.FunctionResponse {
	if h.requests != nil {
		h.requests <- req
	}
	if h.handle != nil {
		return h.handle(ctx, req)
	}
	return &funcapi.FunctionResponse{
		Status: 200,
		Data:   [][]any{{"retained"}},
	}
}
func (h *processFunctionTestHandler) Cleanup(context.Context) {
	if h.cleanup != nil {
		h.cleanup()
	}
}

func testProcessProvider(
	id string,
	methods []funcapi.FunctionConfig,
	create func() funcapi.MethodHandler,
) funcapi.ProcessFunctionProvider {
	return funcapi.ProcessFunctionProvider{
		ID:         id,
		Functions:  func() []funcapi.FunctionConfig { return methods },
		NewHandler: create,
	}
}

func startProviderProcess(
	t *testing.T,
	cfg Config,
) (*Process, *io.PipeWriter, *processSynchronizedBuffer, <-chan error) {
	t.Helper()
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	cfg.Input, cfg.Output = reader, output
	cfg.PluginName = "test"
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = time.Second
	}
	process, err := NewProcess(cfg)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() { cancel(); _ = writer.Close(); _ = reader.Close() })
	return process, writer, output, done
}

func TestProcessFunctionsWithoutCollectors(t *testing.T) {
	var created, cleaned, serviceStarts, serviceStops atomic.Int32
	requests := make(chan funcapi.RawMethodRequest, 4)
	provider := testProcessProvider("history", []funcapi.FunctionConfig{{
		ID: "read", Aliases: []string{"history-alias"}, RawRequest: true, ManagedInfo: true,
		HasHistory: true, AcceptedParams: []string{"after", "before"}, RequireCloud: true,
	}}, func() funcapi.MethodHandler {
		created.Add(1)
		return &processFunctionTestHandler{
			requests: requests,
			cleanup:  func() { cleaned.Add(1) },
		}
	})
	// Invalid discovery factories are ignored in a Functions-only process.
	process, writer, output, done := startProviderProcess(t, Config{
		ProcessFunctions: []funcapi.ProcessFunctionProvider{provider},
		Services: []ProcessService{
			processServiceFunc(func(ctx context.Context) { serviceStarts.Add(1); <-ctx.Done(); serviceStops.Add(1) }),
		},
		DiscoveryProviders: []agentdiscovery.ProviderFactory{nil},
	})
	output.waitContains(t, `FUNCTION GLOBAL "history:read"`)
	output.waitContains(t, `FUNCTION GLOBAL "history-alias"`)
	require.Contains(t, output.String(), `"top" 0x0013 100 3`)
	info := callProcessFunction(t, writer, output, "info", "history-alias info", "", 200)
	require.Contains(t, info, `"has_history":true`)
	require.Contains(t, info, `"after"`)
	require.NotContains(t, info, "__job")
	require.True(t, (<-requests).Info)
	payload := `{"after":17}`
	callProcessFunction(t, writer, output, "data", "history:read before:20", payload, 200)
	require.Equal(t, funcapi.RawMethodRequest{
		Method:      "read",
		Args:        []string{"before:20"},
		Payload:     []byte(payload),
		ContentType: "application/json",
		Timeout:     10 * time.Second,
		Permissions: "0xFFFF",
		Source:      "user=test",
	}, <-requests)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, process.Restart(ctx))
	require.EqualValues(t, 2, created.Load())
	require.EqualValues(t, 1, cleaned.Load())
	require.NoError(t, process.Terminate(ctx))
	require.NoError(t, <-done)
	require.EqualValues(t, 2, cleaned.Load())
	require.EqualValues(t, 1, serviceStarts.Load())
	require.EqualValues(t, 1, serviceStops.Load())
	require.Equal(t, 2, strings.Count(output.String(), `FUNCTION_DEL GLOBAL "history:read"`))
}

func TestProcessFunctionAvailabilityWithoutJobs(t *testing.T) {
	var available atomic.Bool
	var unavailablePolls atomic.Int32
	var calls atomic.Int32
	provider := testProcessProvider(
		"history",
		[]funcapi.FunctionConfig{{ID: "read", RawRequest: true, Available: func() bool {
			ready := available.Load()
			if !ready {
				unavailablePolls.Add(1)
			}
			return ready
		}}},
		func() funcapi.MethodHandler {
			return &processFunctionTestHandler{
				handle: func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					calls.Add(1)
					return &funcapi.FunctionResponse{
						Status: 200,
					}
				},
			}
		},
	)
	process, writer, output, done := startProviderProcess(
		t,
		Config{
			ProcessFunctions: []funcapi.ProcessFunctionProvider{provider},
		},
	)
	// An admitted control round trip proves startup completed without a public history route.
	callProcessFunction(t, writer, output, "unknown", "history:read", "", 404)
	require.NotContains(t, output.String(), `FUNCTION GLOBAL "history:read"`)
	available.Store(true)
	output.waitContains(t, `FUNCTION GLOBAL "history:read"`)
	callProcessFunction(t, writer, output, "available", "history:read", "", 200)
	unavailableBefore := unavailablePolls.Load()
	available.Store(false)
	// Available is monotonic publication gating, preserving AgentFunctions semantics.
	require.Eventually(t, func() bool { return unavailablePolls.Load() > unavailableBefore }, 3*time.Second, time.Millisecond)
	callProcessFunction(t, writer, output, "retained", "history:read", "", 200)
	require.NotContains(t, output.String(), `FUNCTION_DEL GLOBAL "history:read"`)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, process.Terminate(ctx))
	require.NoError(t, <-done)
	require.EqualValues(t, 2, calls.Load())
}

func TestProcessProviderConfigurationRejectsInvalidIdentity(t *testing.T) {
	valid := testProcessProvider(
		"history",
		[]funcapi.FunctionConfig{{ID: "read"}},
		func() funcapi.MethodHandler { return &assemblyTestHandler{} },
	)
	for name, providers := range map[string][]funcapi.ProcessFunctionProvider{
		"missing ID":           {{Functions: valid.Functions, NewHandler: valid.NewHandler}},
		"missing declarations": {{ID: "history", NewHandler: valid.NewHandler}},
		"missing factory":      {{ID: "history", Functions: valid.Functions}},
		"duplicate ID":         {valid, valid},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewProcess(
				Config{
					PluginName:       "test",
					Input:            strings.NewReader("QUIT\n"),
					Output:           io.Discard,
					ProcessFunctions: providers,
				},
			)
			require.Error(t, err)
		})
	}
	_, err := NewProcess(Config{
		PluginName: "test",
		Input:      strings.NewReader("QUIT\n"),
		Output:     io.Discard,
	})
	require.ErrorContains(t, err, "incomplete production configuration")
}

func TestProcessProviderPublicNameCollisions(t *testing.T) {
	for name, test := range map[string]struct {
		other  string
		module bool
	}{
		"provider primary":      {other: "history:read"},
		"provider alias":        {other: "history-alias"},
		"private config route":  {other: "config"},
		"collector declaration": {other: "module:read", module: true},
	} {
		t.Run(name, func(t *testing.T) {
			var cleaned atomic.Int32
			create := func() funcapi.MethodHandler {
				return &processFunctionTestHandler{
					cleanup: func() { cleaned.Add(1) },
				}
			}
			provider := testProcessProvider(
				"history",
				[]funcapi.FunctionConfig{
					{ID: "read", Aliases: []string{"history-alias"}, Available: func() bool { return false }},
				},
				create,
			)
			cfg := Config{
				PluginName:      "test",
				Input:           strings.NewReader("QUIT\n"),
				Output:          io.Discard,
				ShutdownTimeout: time.Second,
			}
			cfg.ProcessFunctions = []funcapi.ProcessFunctionProvider{provider}
			if test.other == "config" || test.module {
				cfg.ProcessFunctions[0].Functions = func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{
						{ID: "read", FunctionName: test.other, Available: func() bool { return false }},
					}
				}
			} else {
				cfg.ProcessFunctions = append(cfg.ProcessFunctions, testProcessProvider("second", []funcapi.FunctionConfig{{ID: "read", FunctionName: test.other}}, create))
			}
			if test.module {
				cfg = func(c Config) Config {
					base := testProductionProcessConfig(c.Input, c.Output)
					base.ProcessFunctions = c.ProcessFunctions
					base.Modules = collectorapi.Registry{
						"module": {
							AgentFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "read"}} },
							MethodHandler:  func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &assemblyTestHandler{} },
						},
					}
					return base
				}(cfg)
			}
			process, err := NewProcess(cfg)
			require.NoError(t, err)
			err = process.Run(t.Context())
			require.Error(t, err)
			require.True(
				t,
				strings.Contains(err.Error(), "collid") ||
					strings.Contains(err.Error(), "duplicate process Function public name"),
				"%v",
				err,
			)
			require.EqualValues(t, len(cfg.ProcessFunctions), cleaned.Load())
		})
	}
}

func TestProcessProviderRestartJoinsPhysicalCleanup(t *testing.T) {
	var created, cleaned atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	provider := testProcessProvider("history", []funcapi.FunctionConfig{{ID: "read"}}, func() funcapi.MethodHandler {
		generation := created.Add(1)
		return &processFunctionTestHandler{
			cleanup: func() {
				if generation == 1 {
					close(entered)
					<-release
				}
				cleaned.Add(1)
			},
		}
	})
	process, writer, output, done := startProviderProcess(
		t,
		Config{
			ProcessFunctions: []funcapi.ProcessFunctionProvider{provider},
		},
	)
	output.waitContains(t, `FUNCTION GLOBAL "history:read"`)
	callProcessFunction(t, writer, output, "ready", "history:read", "", 200)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	restarted := make(chan error, 1)
	go func() { restarted <- process.Restart(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("cleanup not entered")
	}
	require.EqualValues(t, 1, created.Load())
	require.Contains(t, output.String(), `FUNCTION_DEL GLOBAL "history:read"`)
	select {
	case err := <-restarted:
		t.Fatalf("restart returned before cleanup: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-restarted)
	require.EqualValues(t, 2, created.Load())
	require.EqualValues(t, 1, cleaned.Load())
	require.NoError(t, process.Terminate(ctx))
	require.NoError(t, <-done)
	require.EqualValues(t, 2, cleaned.Load())
}

func TestProcessProviderTerminationContainsNoncooperativeInvocation(t *testing.T) {
	entered, release, cleaned := make(chan struct{}), make(chan struct{}), make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	var cleanupCount atomic.Int32
	provider := testProcessProvider(
		"history",
		[]funcapi.FunctionConfig{{ID: "read", RawRequest: true}},
		func() funcapi.MethodHandler {
			return &processFunctionTestHandler{
				handle: func(context.Context, funcapi.RawMethodRequest) *funcapi.FunctionResponse {
					close(entered)
					<-release
					return &funcapi.FunctionResponse{
						Status: 200,
					}
				},
				cleanup: func() { cleanupCount.Add(1); close(cleaned) },
			}
		},
	)
	process, writer, output, done := startProviderProcess(
		t,
		Config{
			ShutdownTimeout:  50 * time.Millisecond,
			ProcessFunctions: []funcapi.ProcessFunctionProvider{provider},
		},
	)
	output.waitContains(t, `FUNCTION GLOBAL "history:read"`)
	_, err := io.WriteString(writer, "FUNCTION held 10 \"history:read\" 0xFFFF \"user=test\"\n")
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("invocation not entered")
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err = process.Terminate(ctx)
	require.Error(t, err)
	require.Error(t, <-done)
	require.Zero(t, cleanupCount.Load(), "handler cleanup must not overlap its physical invocation")
	require.Contains(t, output.String(), `FUNCTION_DEL GLOBAL "history:read"`)
	close(release)
	select {
	case <-cleaned:
	case <-time.After(time.Second):
		t.Fatal("late invocation did not release handler cleanup")
	}
	require.EqualValues(t, 1, cleanupCount.Load())
	require.ErrorIs(t, process.Restart(ctx), ErrProcessStopped)
}

func TestProcessProviderAndCollectorMayShareID(t *testing.T) {
	var processCleanups, collectorCleanups atomic.Int32
	cfg := testProductionProcessConfig(nil, nil)
	cfg.Modules = collectorapi.Registry{
		"history": {
			AgentFunctions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "collector"}} },
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &processFunctionTestHandler{
					cleanup: func() { collectorCleanups.Add(1) },
				}
			},
		},
	}
	cfg.ProcessFunctions = []funcapi.ProcessFunctionProvider{
		testProcessProvider("history", []funcapi.FunctionConfig{{ID: "process"}}, func() funcapi.MethodHandler {
			return &processFunctionTestHandler{
				cleanup: func() { processCleanups.Add(1) },
			}
		}),
	}
	process, writer, output, done := startProviderProcess(t, cfg)
	output.waitContains(t, `FUNCTION GLOBAL "history:collector"`)
	output.waitContains(t, `FUNCTION GLOBAL "history:process"`)
	callProcessFunction(t, writer, output, "collector", "history:collector", "", 200)
	callProcessFunction(t, writer, output, "process", "history:process", "", 200)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	require.NoError(t, process.Restart(ctx))
	require.EqualValues(t, 1, processCleanups.Load())
	require.EqualValues(t, 1, collectorCleanups.Load())
	require.NoError(t, process.Terminate(ctx))
	require.NoError(t, <-done)
	require.EqualValues(t, 2, processCleanups.Load())
	require.EqualValues(t, 2, collectorCleanups.Load())
}

func TestProcessProviderPreparationRemainsContained(t *testing.T) {
	for _, stage := range []string{"declarations", "factory"} {
		t.Run(stage, func(t *testing.T) {
			entered, release, cleaned := make(chan struct{}), make(chan struct{}), make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			provider := funcapi.ProcessFunctionProvider{
				ID: "history",
				Functions: func() []funcapi.FunctionConfig {
					if stage == "declarations" {
						close(entered)
						<-release
					}
					return []funcapi.FunctionConfig{{ID: "read"}}
				},
				NewHandler: func() funcapi.MethodHandler {
					if stage == "factory" {
						close(entered)
						<-release
					}
					return &processFunctionTestHandler{
						cleanup: func() { close(cleaned) },
					}
				},
			}
			process, _, output, done := startProviderProcess(
				t,
				Config{
					ShutdownTimeout:  50 * time.Millisecond,
					ProcessFunctions: []funcapi.ProcessFunctionProvider{provider},
				},
			)
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("preparation not entered")
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			require.Error(t, process.Terminate(ctx))
			require.Error(t, <-done)
			require.NotContains(t, output.String(), `FUNCTION GLOBAL "history:read"`)
			close(release)
			select {
			case <-cleaned:
			case <-time.After(time.Second):
				t.Fatal("late preparation did not clean handler")
			}
			require.NotContains(t, output.String(), `FUNCTION GLOBAL "history:read"`)
		})
	}
}

func TestProcessProviderCleanupFailureRequiresFreshProcess(t *testing.T) {
	var created, cleaned atomic.Int32
	provider := testProcessProvider("history", []funcapi.FunctionConfig{{ID: "read"}}, func() funcapi.MethodHandler {
		created.Add(1)
		return &processFunctionTestHandler{
			cleanup: func() { cleaned.Add(1); panic("cleanup failed") },
		}
	})
	process, writer, output, done := startProviderProcess(
		t,
		Config{
			ProcessFunctions: []funcapi.ProcessFunctionProvider{provider},
		},
	)
	output.waitContains(t, `FUNCTION GLOBAL "history:read"`)
	callProcessFunction(t, writer, output, "ready", "history:read", "", 200)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	require.ErrorIs(t, process.Restart(ctx), ErrProcessRestartRequired)
	require.Error(t, <-done)
	require.EqualValues(t, 1, created.Load(), "quarantined owner must not construct a successor")
	require.EqualValues(t, 1, cleaned.Load())
}
