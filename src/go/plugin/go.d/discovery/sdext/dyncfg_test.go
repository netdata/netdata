// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	prepareNetListenersExecutableFixture(t)

	var httpRequests atomic.Int64
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpRequests.Add(1)
		if r.URL.Path == "/fail" {
			http.Error(w, "private backend response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `[]`)
	}))
	defer httpServer.Close()
	missingTokenFile := t.TempDir() + "/missing-token"
	missingCAFile := t.TempDir() + "/missing-ca"

	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	output := &dyncfgTestOutput{
		results: make(chan dyncfg.Result, 1),
		creates: make(chan struct{}, 16),
	}
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
	groups := make(chan []*confgroup.Group, 1)
	go func() {
		defer close(done)
		discovery.Run(ctx, groups)
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
	for range len(Registry(true).Types()) {
		select {
		case <-output.creates:
		case <-time.After(time.Second):
			require.FailNow(t, "test failed", "service discovery did not expose every DynCfg template")
		}
	}
	tests := map[string]struct {
		uid      string
		id       string
		payload  string
		message  string
		code     int
		mode     string
		requests int64
	}{
		"resource parser": {
			uid:     "test-docker-parser",
			id:      "go.d:sd:docker",
			payload: `{"discoverer":{"docker":{"timeout":"[REDACTED_SECRET]"}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery resource configuration is invalid",
			code:    400,
		},
		"resource constructor": {
			uid:     "test-http-constructor",
			id:      "go.d:sd:http",
			payload: `{"discoverer":{"http":{"url":"http://example.invalid","proxy_url":"http://%zz/[REDACTED_SECRET]"}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery resource construction failed",
			code:    400,
		},
		"HTTP TLS file construction": {
			uid:     "test-http-tls-file",
			id:      "go.d:sd:http",
			payload: fmt.Sprintf(`{"discoverer":{"http":{"url":"https://example.invalid","tls_ca":%q}},"services":[{"id":"test","match":"true"}]}`, missingCAFile),
			message: "service discovery resource construction failed: the configured HTTP credential or TLS file could not be read safely",
			code:    400,
		},
		"semantic validation": {
			uid:     "test-docker-semantics",
			id:      "go.d:sd:docker",
			payload: `{"discoverer":{"docker":{}},"services":[{"id":"[REDACTED_SECRET]","match":""}]}`,
			message: "service discovery resource configuration is invalid: service discovery service rules are invalid",
			code:    400,
		},
		"operational connection": {
			uid:     "test-docker-operational",
			id:      "go.d:sd:docker",
			payload: `{"discoverer":{"docker":{"address":"unix:///tmp/netdata-[REDACTED_SECRET].sock","timeout":"50ms"}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery operational test failed: cannot connect to the configured Docker endpoint",
			code:    422,
		},
		"net listeners operational success": {
			uid:     "test-net-listeners-operational-success",
			id:      "go.d:sd:net_listeners",
			payload: `{"discoverer":{"net_listeners":{}},"services":[{"id":"test","match":"true"}]}`,
			message: `"message":""`,
			code:    200,
		},
		"net listeners helper failure": {
			uid:     "test-net-listeners-operational-failure",
			id:      "go.d:sd:net_listeners",
			payload: `{"discoverer":{"net_listeners":{}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery operational test failed: cannot inspect local network listeners",
			code:    422,
			mode:    "failure",
		},
		"net listeners invalid output": {
			uid:     "test-net-listeners-invalid-output",
			id:      "go.d:sd:net_listeners",
			payload: `{"discoverer":{"net_listeners":{}},"services":[{"id":"test","match":"true"}]}`,
			message: "service discovery operational test failed: local listener inspection returned invalid data",
			code:    422,
			mode:    "invalid",
		},
		"http operational success": {
			uid:      "test-http-operational-success",
			id:       "go.d:sd:http",
			payload:  fmt.Sprintf(`{"discoverer":{"http":{"url":%q}},"services":[{"id":"test","match":"true"}]}`, httpServer.URL+"/ok"),
			message:  `"message":""`,
			code:     200,
			requests: 1,
		},
		"http operational failure": {
			uid:      "test-http-operational-failure",
			id:       "go.d:sd:http",
			payload:  fmt.Sprintf(`{"discoverer":{"http":{"url":%q}},"services":[{"id":"test","match":"true"}]}`, httpServer.URL+"/fail"),
			message:  "service discovery operational test failed: cannot query the configured HTTP endpoint",
			code:     422,
			requests: 1,
		},
		"http credential file failure": {
			uid:     "test-http-credential-file",
			id:      "go.d:sd:http",
			payload: fmt.Sprintf(`{"discoverer":{"http":{"url":%q,"bearer_token_file":%q}},"services":[{"id":"test","match":"true"}]}`, httpServer.URL+"/unexpected", missingTokenFile),
			message: "service discovery operational test failed: the configured HTTP credential or TLS file could not be read safely",
			code:    422,
		},
		"http unsafe method is validation only": {
			uid:     "test-http-validation-only",
			id:      "go.d:sd:http",
			payload: fmt.Sprintf(`{"discoverer":{"http":{"url":%q,"method":"POST"}},"services":[{"id":"test","match":"true"}]}`, httpServer.URL+"/unexpected"),
			message: "Configuration is valid; this discoverer does not provide an operational test.",
			code:    200,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			requestsBefore := httpRequests.Load()
			t.Setenv("NETDATA_TEST_LOCAL_LISTENER_MODE", test.mode)
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
			case <-time.After(10 * time.Second):
				require.FailNow(t, "test failed", "DynCfg test did not return a result")
			}
			select {
			case <-groups:
				require.FailNow(t, "test failed", "temporary DynCfg test published discovery groups")
			default:
			}
			select {
			case <-output.creates:
				require.FailNow(t, "test failed", "temporary DynCfg test installed configuration")
			default:
			}
			require.Equal(t, test.requests, httpRequests.Load()-requestsBefore)
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
	creates chan struct{}
}

func (o *dyncfgTestOutput) FunctionResult(result dyncfg.Result) {
	o.results <- result
}

func (o *dyncfgTestOutput) ConfigCreate(netdataapi.ConfigOpts) {
	select {
	case o.creates <- struct{}{}:
	default:
	}
}

func (*dyncfgTestOutput) ConfigStatus(string, dyncfg.Status) {
}

func (*dyncfgTestOutput) ConfigDelete(string) {
}
