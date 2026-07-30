// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/require"
)

func TestHTTPRegistryDyncfgTestSanitizesResourceConstructionFailure(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	output := &dyncfgTestOutput{results: make(chan dyncfg.Result, 1)}
	registry := &dyncfgTestRegistry{registered: make(chan functions.Handler, 1)}
	discovery, err := sd.NewServiceDiscovery(sd.Config{
		Epoch:        1,
		Attempts:     attempts,
		PluginName:   "go.d",
		DyncfgOutput: output,
		FnReg:        registry,
		Discoverers:  Registry(true),
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		discovery.Run(ctx, make(chan []*confgroup.Group))
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "service discovery did not stop")
		}
		attempts.BeginShutdown()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		defer shutdownCancel()
		require.NoError(t, attempts.Shutdown(shutdownCtx))
	})

	var handler functions.Handler
	select {
	case handler = <-registry.registered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "service discovery did not register DynCfg")
	}
	handler(t.Context(), functions.Function{
		UID:         "test-http",
		Args:        []string{"go.d:sd:http", "test", "job"},
		Payload:     []byte(`{"discoverer":{"http":{"url":"http://example.invalid","proxy_url":"http://user:[REDACTED_SECRET]@%zz"}},"services":[{"id":"test","match":"true"}]}`),
		Source:      "user=test",
		ContentType: "application/json",
	})

	select {
	case result := <-output.results:
		require.Equal(t, 400, result.Code)
		require.Contains(t, result.Payload, "service discovery resource construction failed")
		require.NotContains(t, result.Payload, "[REDACTED_SECRET]")
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "DynCfg test did not return a result")
	}
}

type dyncfgTestRegistry struct {
	registered chan functions.Handler
}

func (r *dyncfgTestRegistry) RegisterPrefix(_ string, _ string, handler functions.Handler) {
	r.registered <- handler
}

func (*dyncfgTestRegistry) UnregisterPrefix(string, string) {
}

type dyncfgTestOutput struct {
	results chan dyncfg.Result
}

func (o *dyncfgTestOutput) FunctionResult(result dyncfg.Result) {
	o.results <- result
}

func (*dyncfgTestOutput) ConfigCreate(netdataapi.ConfigOpts) {
}

func (*dyncfgTestOutput) ConfigStatus(string, dyncfg.Status) {
}

func (*dyncfgTestOutput) ConfigDelete(string) {
}
