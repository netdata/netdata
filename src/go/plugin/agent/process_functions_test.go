// SPDX-License-Identifier: GPL-3.0-or-later

package agent

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

type agentProcessFunctionHandler struct{ cleanups *atomic.Int32 }

func (*agentProcessFunctionHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}
func (*agentProcessFunctionHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{
		Status: 200,
	}
}
func (h *agentProcessFunctionHandler) Cleanup(context.Context) { h.cleanups.Add(1) }

type agentProcessFunctionOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *agentProcessFunctionOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}
func (o *agentProcessFunctionOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

func TestAgentProcessFunctionsIndependentOfCollectorSelection(t *testing.T) {
	tests := map[string]struct {
		config, runModule              string
		withModule, provider, disabled bool
	}{
		"no registered modules":        {provider: true},
		"collector disabled":           {provider: true, withModule: true, config: "modules:\n  module: no\n"},
		"collector filtered":           {provider: true, withModule: true, runModule: "another"},
		"plugin disabled":              {provider: true, config: "enabled: no\n", disabled: true},
		"legacy empty modules disable": {disabled: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var creates, cleanups, discoveryCalls atomic.Int32
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "test.conf"), []byte(test.config), 0600))
			cfg := Config{
				Name:            "test",
				PluginConfigDir: []string{dir},
				RunModule:       test.runModule,
				ShutdownTimeout: time.Second,
			}
			if test.withModule {
				cfg.ModuleRegistry = collectorapi.Registry{
					"module": {
						AgentFunctions: func() []funcapi.FunctionConfig { t.Error("filtered collector declarations called"); return nil },
					},
				}
			}
			if test.provider {
				cfg.ProcessFunctions = []funcapi.ProcessFunctionProvider{
					{
						ID:        "history",
						Functions: func() []funcapi.FunctionConfig { return []funcapi.FunctionConfig{{ID: "read"}} },
						NewHandler: func() funcapi.MethodHandler {
							creates.Add(1)
							return &agentProcessFunctionHandler{
								cleanups: &cleanups,
							}
						},
					},
				}
			}
			cfg.DiscoveryProviders = []discovery.ProviderFactory{
				discovery.NewProviderFactory(
					"sentinel",
					func(discovery.BuildContext) (discovery.Discoverer, bool, error) {
						discoveryCalls.Add(1)
						return nil, false, nil
					},
				),
			}
			a := New(cfg)
			output := &agentProcessFunctionOutput{}
			a.Out = output
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			a.In = reader
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- a.RunContext(ctx) }()
			if test.disabled {
				require.NoError(t, <-done)
				require.Contains(t, output.String(), "DISABLE")
				require.Zero(t, creates.Load())
			} else {
				require.Eventually(t, func() bool { return strings.Contains(output.String(), `FUNCTION GLOBAL "history:read"`) }, 3*time.Second, time.Millisecond)
				stopCtx, stopCancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer stopCancel()
				require.NoError(t, a.Terminate(stopCtx))
				require.NoError(t, <-done)
				require.EqualValues(t, 1, creates.Load())
				require.EqualValues(t, 1, cleanups.Load())
				require.NotContains(t, output.String(), "DISABLE")
			}
			require.Zero(t, discoveryCalls.Load())
		})
	}
}
