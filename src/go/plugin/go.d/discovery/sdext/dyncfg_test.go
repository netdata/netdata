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

func TestShippedRegistryDyncfgTestSanitizesResourceFailures(t *testing.T) {
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
	tests := []struct {
		name    string
		uid     string
		id      string
		payload string
		message string
		code    int
	}{
		{
			name:    "resource parser",
			uid:     "test-docker-parser",
			id:      "go.d:sd:docker",
			payload: `{"discoverer":{"docker":{"timeout":"[REDACTED_SECRET]"}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery resource configuration is invalid",
			code:    400,
		},
		{
			name:    "resource constructor",
			uid:     "test-http-constructor",
			id:      "go.d:sd:http",
			payload: `{"discoverer":{"http":{"url":"http://example.invalid","proxy_url":"http://user:[REDACTED_SECRET]@%zz"}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery resource construction failed",
			code:    400,
		},
		{
			name:    "semantic validation",
			uid:     "test-docker-semantics",
			id:      "go.d:sd:docker",
			payload: `{"discoverer":{"docker":{}},"services":[{"id":"[REDACTED_SECRET]","match":""}]}`,
			message: "service discovery resource configuration is invalid: service discovery service rules are invalid",
			code:    400,
		},
		{
			name:    "operational connection",
			uid:     "test-docker-operational",
			id:      "go.d:sd:docker",
			payload: `{"discoverer":{"docker":{"address":"unix:///tmp/netdata-[REDACTED_SECRET].sock","timeout":"50ms"}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery operational test failed: cannot connect to the configured Docker endpoint",
			code:    422,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler(t.Context(), functions.Function{
				UID:         test.uid,
				Args:        []string{test.id, "test", "job"},
				Payload:     []byte(test.payload),
				Source:      "user=test",
				ContentType: "application/json",
			})

			select {
			case result := <-output.results:
				require.Equal(t, test.code, result.Code)
				require.Contains(t, result.Payload, test.message)
				require.NotContains(t, result.Payload, "[REDACTED_SECRET]")
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "DynCfg test did not return a result")
			}
		})
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
