// SPDX-License-Identifier: GPL-3.0-or-later

package sdext

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/pipeline"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/dockersd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/httpsd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/discovery/sdext/discoverer/netlistensd"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_DockerInclusion(t *testing.T) {
	withDocker := Registry(true)
	assert.Contains(t, withDocker.Types(), discovererHTTP)
	assert.Contains(t, withDocker.Types(), discovererDocker)
	assert.Contains(t, withDocker.Types(), discovererRedfish)

	withoutDocker := Registry(false)
	assert.Contains(t, withoutDocker.Types(), discovererHTTP)
	assert.Contains(t, withoutDocker.Types(), discovererRedfish)
	assert.NotContains(t, withoutDocker.Types(), discovererDocker)
}

func TestRegistry_DockerOperationalTestRejectsUnreachableEndpoint(t *testing.T) {
	raw, err := json.Marshal(dockersd.Config{
		Address: fmt.Sprintf("unix:///tmp/netdata-sd-%d.sock", time.Now().UnixNano()),
		Timeout: confopt.Duration(50 * time.Millisecond),
	})
	require.NoError(t, err)
	registry := Registry(true)

	candidate, err := pipeline.New(
		pipeline.Config{
			Name: "docker-test",
			Discoverer: pipeline.DiscovererPayload{
				Kind:   discovererDocker,
				Config: raw,
			},
			Services: []pipeline.ServiceRuleConfig{{
				ID:    "test",
				Match: "true",
			}},
		},
		func(payload pipeline.DiscovererPayload, source string) ([]model.Discoverer, error) {
			descriptor, ok := registry.Get(payload.Type())
			require.True(t, ok)
			config, err := descriptor.ParseJSONConfig(payload.Config)
			if err != nil {
				return nil, err
			}
			return descriptor.NewDiscoverers(config, source)
		},
	)
	require.NoError(t, err)

	fullyTested, err := candidate.Test(t.Context())

	require.False(t, fullyTested)
	require.Error(t, err)
	require.ErrorContains(t, err, "cannot connect to the configured Docker endpoint")
}

func TestRegistry_HTTPOperationalTest(t *testing.T) {
	tests := map[string]struct {
		method            string
		handler           http.HandlerFunc
		wantFullyTested   bool
		wantRequests      int64
		wantPublicMessage string
		wantAbsent        string
	}{
		"uses the shipped construction path": {
			method: http.MethodGet,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `[]`)
			},
			wantFullyTested: true,
			wantRequests:    1,
		},
		"unsafe method is validation only": {
			method: http.MethodPost,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			},
		},
		"operational failure is public": {
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "private backend response", http.StatusInternalServerError)
			},
			wantRequests:      1,
			wantPublicMessage: "cannot query the configured HTTP endpoint",
			wantAbsent:        "private backend response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int64
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				tc.handler(w, r)
			}))
			defer srv.Close()

			raw, err := json.Marshal(httpsd.Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL:    srv.URL,
						Method: tc.method,
					},
				},
			})
			require.NoError(t, err)
			registry := Registry(true)
			descriptor, ok := registry.Get(discovererHTTP)
			require.True(t, ok)
			config, err := descriptor.ParseJSONConfig(raw)
			require.NoError(t, err)
			discoverers, err := descriptor.NewDiscoverers(config, "dyncfg=user=test")
			require.NoError(t, err)
			require.Len(t, discoverers, 1)
			_, ok = discoverers[0].(dyncfg.Testable)
			require.True(t, ok)

			candidate := newHTTPPipelineCandidate(t, registry, raw)
			fullyTested, err := candidate.Test(t.Context())

			require.Equal(t, tc.wantFullyTested, fullyTested)
			if tc.wantPublicMessage == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				message, ok := dyncfg.PublicMessage(err)
				require.True(t, ok)
				assert.Equal(t, tc.wantPublicMessage, message)
				assert.NotContains(t, err.Error(), tc.wantAbsent)
			}
			assert.Equal(t, tc.wantRequests, requests.Load())
		})
	}
}

func newHTTPPipelineCandidate(t *testing.T, registry sd.Registry, raw json.RawMessage) *pipeline.Pipeline {
	t.Helper()
	candidate, err := pipeline.New(
		pipeline.Config{
			Name: "http-test",
			Discoverer: pipeline.DiscovererPayload{
				Kind:   discovererHTTP,
				Config: raw,
			},
			Services: []pipeline.ServiceRuleConfig{{
				ID:    "test",
				Match: "true",
			}},
		},
		func(payload pipeline.DiscovererPayload, source string) ([]model.Discoverer, error) {
			descriptor, ok := registry.Get(payload.Type())
			require.True(t, ok)
			config, err := descriptor.ParseJSONConfig(payload.Config)
			if err != nil {
				return nil, err
			}
			return descriptor.NewDiscoverers(config, source)
		},
	)
	require.NoError(t, err)
	return candidate
}

func TestRegistry_NetListenersOperationalTestUsesShippedConstructionPath(t *testing.T) {
	fixture := prepareNetListenersExecutableFixture(t)

	raw, err := json.Marshal(netlistensd.Config{
		Timeout: confopt.Duration(5 * time.Second),
	})
	require.NoError(t, err)
	registry := Registry(true)
	descriptor, ok := registry.Get(discovererNetListeners)
	require.True(t, ok)
	config, err := descriptor.ParseJSONConfig(raw)
	require.NoError(t, err)
	discoverers, err := descriptor.NewDiscoverers(config, "dyncfg=user=test")
	require.NoError(t, err)
	require.Len(t, discoverers, 1)
	_, ok = discoverers[0].(dyncfg.Testable)
	require.True(t, ok)

	candidate, err := pipeline.New(
		pipeline.Config{
			Name: "net-listeners-test",
			Discoverer: pipeline.DiscovererPayload{
				Kind:   discovererNetListeners,
				Config: raw,
			},
			Services: []pipeline.ServiceRuleConfig{{
				ID:    "test",
				Match: "true",
			}},
		},
		func(payload pipeline.DiscovererPayload, source string) ([]model.Discoverer, error) {
			descriptor, ok := registry.Get(payload.Type())
			require.True(t, ok)
			config, err := descriptor.ParseJSONConfig(payload.Config)
			if err != nil {
				return nil, err
			}
			return descriptor.NewDiscoverers(config, source)
		},
	)
	require.NoError(t, err)

	fullyTested, err := candidate.Test(t.Context())

	require.NoError(t, err)
	require.True(t, fullyTested)
	invocations, err := os.ReadFile(fixture.invocationsPath)
	require.NoError(t, err)
	require.Equal(t, "nd-run\ninvoked\n", string(invocations))
	args, err := os.ReadFile(fixture.argsPath)
	require.NoError(t, err)
	require.Equal(
		t,
		"no-udp6 no-local no-inbound no-outbound no-namespaces",
		strings.TrimSpace(string(args)),
	)
}

func TestRegistry_NetListenersOperationalTestHonorsConfiguredTimeout(t *testing.T) {
	prepareNetListenersExecutableFixture(t)
	t.Setenv("NETDATA_TEST_LOCAL_LISTENER_MODE", "timeout")

	raw, err := json.Marshal(netlistensd.Config{
		Timeout: confopt.Duration(100 * time.Millisecond),
	})
	require.NoError(t, err)
	descriptor, ok := Registry(true).Get(discovererNetListeners)
	require.True(t, ok)
	config, err := descriptor.ParseJSONConfig(raw)
	require.NoError(t, err)
	discoverers, err := descriptor.NewDiscoverers(config, "dyncfg=user=test")
	require.NoError(t, err)
	require.Len(t, discoverers, 1)
	testable, ok := discoverers[0].(dyncfg.Testable)
	require.True(t, ok)

	err = testable.Test(t.Context())

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, "local listener inspection did not complete before the timeout", err.Error())
}

type netListenersExecutableFixture struct {
	invocationsPath string
	argsPath        string
}

func prepareNetListenersExecutableFixture(t *testing.T) netListenersExecutableFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses shell executable fixtures")
	}

	tmp := t.TempDir()
	fixture := netListenersExecutableFixture{
		invocationsPath: filepath.Join(tmp, "invocations"),
		argsPath:        filepath.Join(tmp, "args"),
	}
	t.Setenv("NETDATA_PLUGINS_DIR", tmp)
	t.Setenv("NETDATA_TEST_LOCAL_LISTENER_INVOCATIONS", fixture.invocationsPath)
	t.Setenv("NETDATA_TEST_LOCAL_LISTENER_ARGS", fixture.argsPath)

	localListeners := filepath.Join(tmp, "local-listeners")
	require.NoError(t, os.WriteFile(localListeners, []byte(`#!/bin/sh
printf 'invoked\n' >> "$NETDATA_TEST_LOCAL_LISTENER_INVOCATIONS"
printf '%s\n' "$*" >> "$NETDATA_TEST_LOCAL_LISTENER_ARGS"
case "$NETDATA_TEST_LOCAL_LISTENER_MODE" in
failure)
	printf 'private helper failure [REDACTED_SECRET]\n' >&2
	exit 1
	;;
invalid)
	printf 'invalid [REDACTED_SECRET]\n'
	;;
timeout)
	sleep 30
	;;
*)
	printf 'TCP|127.0.0.1|19999|/usr/bin/example\n'
	;;
esac
`), 0o755))

	ndRun := filepath.Join(tmp, "nd-run")
	require.NoError(t, os.WriteFile(ndRun, []byte(`#!/bin/sh
printf 'nd-run\n' >> "$NETDATA_TEST_LOCAL_LISTENER_INVOCATIONS"
exec "$@"
`), 0o755))
	t.Cleanup(ndexec.SetRunnerPathsForTests(ndRun, ""))

	return fixture
}

func TestRedfishSchemaLeavesPositiveScanConcurrencyOperatorOwned(t *testing.T) {
	var schema struct {
		JSONSchema struct {
			Properties struct {
				Discoverer struct {
					Properties struct {
						Redfish struct {
							Properties map[string]map[string]any `json:"properties"`
						} `json:"redfish"`
					} `json:"properties"`
				} `json:"discoverer"`
			} `json:"properties"`
		} `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal([]byte(schemaRedfish), &schema))
	property := schema.JSONSchema.Properties.Discoverer.Properties.Redfish.Properties["max_concurrent_scans"]
	require.EqualValues(t, 1, property["minimum"])
	assert.NotContains(t, property, "maximum")
}

func TestRedfishSchemaRejectsUnknownNetworkAndServiceKeys(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(schemaRedfish), &schema))
	properties := schema["jsonSchema"].(map[string]any)["properties"].(map[string]any)
	discoverer := properties["discoverer"].(map[string]any)["properties"].(map[string]any)
	redfish := discoverer["redfish"].(map[string]any)["properties"].(map[string]any)
	networkItems := redfish["networks"].(map[string]any)["items"].(map[string]any)
	serviceItems := properties["services"].(map[string]any)["items"].(map[string]any)
	require.Equal(t, false, networkItems["additionalProperties"])
	require.Equal(t, false, serviceItems["additionalProperties"])
}

func TestRedfishDiscoveryProfileSchemaMatchesRuntime(t *testing.T) {
	const trimSpaceCharacters = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"

	tests := map[string]struct {
		profile map[string]any
		valid   bool
	}{
		"none without credentials": {
			profile: map[string]any{"auth_method": "none"}, valid: true,
		},
		"none with runtime-empty credentials": {
			profile: map[string]any{"auth_method": "none", "username": trimSpaceCharacters, "password": ""}, valid: true,
		},
		"none with username": {
			profile: map[string]any{"auth_method": "none", "username": "user"},
		},
		"none with whitespace password": {
			profile: map[string]any{"auth_method": "none", "password": " "},
		},
		"auto with credentials": {
			profile: map[string]any{"auth_method": "auto", "username": "user", "password": "secret"}, valid: true,
		},
		"auto with empty credentials": {
			profile: map[string]any{"auth_method": "auto", "username": "", "password": ""},
		},
		"session with runtime-empty username": {
			profile: map[string]any{"auth_method": "session", "username": trimSpaceCharacters, "password": "secret"},
		},
		"omitted authentication method with credentials": {
			profile: map[string]any{"username": "user", "password": "secret"}, valid: true,
		},
		"omitted authentication method and credentials": {
			profile: map[string]any{},
		},
		"both TLS identity fields runtime-empty": {
			profile: map[string]any{"auth_method": "none", "tls_cert": trimSpaceCharacters, "tls_key": " \t"}, valid: true,
		},
		"both TLS identity fields configured": {
			profile: map[string]any{"auth_method": "none", "tls_cert": " certificate ", "tls_key": " key "}, valid: true,
		},
		"TLS certificate only": {
			profile: map[string]any{"auth_method": "none", "tls_cert": "certificate"},
		},
		"TLS key only": {
			profile: map[string]any{"auth_method": "none", "tls_key": "key"},
		},
		"TLS certificate with runtime-empty key": {
			profile: map[string]any{"auth_method": "none", "tls_cert": "certificate", "tls_key": trimSpaceCharacters},
		},
		"configured log backend": {
			profile: map[string]any{"auth_method": "none", "logs": map[string]any{"backend": " \u00a0isolated\u3000 "}}, valid: true,
		},
		"empty log backend": {
			profile: map[string]any{"auth_method": "none", "logs": map[string]any{"backend": ""}},
		},
		"runtime-empty log backend": {
			profile: map[string]any{"auth_method": "none", "logs": map[string]any{"backend": trimSpaceCharacters}},
		},
	}

	schema := compileRedfishDiscoverySchema(t)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			profile := maps.Clone(test.profile)
			profile["name"] = "profile"
			schemaErr := schema.Validate(redfishDiscoveryDocument(profile))
			_, runtimeErr := redfish.PrepareDiscoveryProfile(test.profile, "https")
			if test.valid {
				require.NoError(t, schemaErr)
				require.NoError(t, runtimeErr)
			} else {
				require.Error(t, schemaErr)
				require.Error(t, runtimeErr)
			}
		})
	}
}

func TestRedfishDiscoverySchemaLeavesLogsBackendByteLimitToRuntime(t *testing.T) {
	tests := map[string]struct {
		backend      string
		runtimeValid bool
	}{
		"257 ASCII bytes": {
			backend: strings.Repeat("x", 257),
		},
		"258 multibyte UTF-8 bytes": {
			backend: strings.Repeat("é", 129),
		},
		"256 normalized bytes with surrounding whitespace": {
			backend: " \u00a0" + strings.Repeat("x", 256) + "\u3000 ", runtimeValid: true,
		},
	}

	schema := compileRedfishDiscoverySchema(t)
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			profile := map[string]any{
				"name": "profile", "auth_method": "none", "logs": map[string]any{"backend": test.backend},
			}
			// Draft-07 maxLength counts pre-normalization code points, not normalized UTF-8 bytes.
			require.NoError(t, schema.Validate(redfishDiscoveryDocument(profile)))
			delete(profile, "name")
			_, runtimeErr := redfish.PrepareDiscoveryProfile(profile, "https")
			if test.runtimeValid {
				require.NoError(t, runtimeErr)
			} else {
				require.ErrorContains(t, runtimeErr, "must not exceed 256 bytes")
			}
		})
	}
}

func compileRedfishDiscoverySchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	var document struct {
		JSONSchema any `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal([]byte(schemaRedfish), &document))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("redfish-discovery-schema.json", document.JSONSchema))
	schema, err := compiler.Compile("redfish-discovery-schema.json")
	require.NoError(t, err)
	return schema
}

func redfishDiscoveryDocument(profile map[string]any) map[string]any {
	return map[string]any{
		"discoverer": map[string]any{
			"redfish": map[string]any{
				"profiles": []any{profile},
				"networks": []any{map[string]any{
					"subnet": "192.0.2.1", "ports": []any{443}, "profile": "profile",
				}},
			},
		},
		"services": []any{map[string]any{"id": "redfish", "match": "{{ true }}"}},
	}
}
