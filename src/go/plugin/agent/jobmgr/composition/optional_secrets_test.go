// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	filediscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	secretconfig "github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func withSecretsModes(t *testing.T, test func(*testing.T, *SecretsConfig)) {
	t.Helper()
	t.Run("enabled", func(t *testing.T) { test(t, testRunSecrets(t)) })
	t.Run("absent", func(t *testing.T) { test(t, nil) })
}

func TestProductionProcessSecretCapabilityOwnership(t *testing.T) {
	for name, invalid := range map[string]*SecretsConfig{
		"empty": {},
		"resolver only": {Providers: secretconfig.Config{
			Resolver: testRunSecrets(t).Providers.Resolver,
		}},
		"catalog only": {Providers: secretconfig.Config{
			Creators: testRunSecrets(t).Providers.Creators,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
			config.Secrets = invalid
			_, err := NewProcess(config)
			require.Error(t, err)
		})
	}
	withSecretsModes(t, func(t *testing.T, secrets *SecretsConfig) {
		config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
		config.Secrets = secrets
		if secrets != nil {
			secrets.Initial = []secretstore.Config{{"name": "original", "nested": map[string]any{"value": "original"}}}
		}
		process, err := NewProcess(config)
		require.NoError(t, err)
		if secrets == nil {
			require.Nil(t, process.core.storeEpochs)
			return
		}
		require.NotNil(t, process.core.storeEpochs, "empty providers/catalog is still enabled")
		original := secrets.Providers
		secrets.Providers = secretconfig.Config{}
		secrets.Initial[0]["name"] = "changed"
		secrets.Initial[0]["nested"].(map[string]any)["value"] = "changed"
		owned := process.core.config.Secrets
		require.Equal(t, original, owned.Providers)
		require.Equal(t, "original", owned.Initial[0]["name"])
		require.Equal(t, map[any]any{"value": "original"}, owned.Initial[0]["nested"])
	})
}

func TestProcessWithoutSecretsPreservesFileAndDynCfgLiterals(t *testing.T) {
	const literal = "${env:MISSING} ${file:/missing} ${cmd:/missing} ${store:vault:missing:key} ${store:invalid} $${env:ESCAPED} $$ ${"
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	observed := make(chan string, 16)
	config := testProductionProcessConfig(reader, output)
	config.Secrets = nil
	config.AutoEnable = true
	config.Modules["test"] = collectorapi.Creator{
		Config: func() any { return &collectorapi.MockConfiguration{} },
		Create: func() collectorapi.CollectorV1 {
			c := &collectorapi.MockCollectorV1{}
			c.InitFunc = func(context.Context) error { observed <- c.Config.OptionStr; return nil }
			c.ChartsFunc = func() *collectorapi.Charts {
				return &collectorapi.Charts{
					&collectorapi.Chart{
						ID:    "chart",
						Title: "test",
						Units: "value",
						Dims: collectorapi.Dims{&collectorapi.Dim{
							ID: "value",
						}},
					},
				}
			}
			c.CollectFunc = func(context.Context) map[string]int64 { return map[string]int64{"value": 1} }
			return c
		},
	}
	path := filepath.Join(t.TempDir(), "test.conf")
	require.NoError(
		t,
		os.WriteFile(
			path,
			[]byte(fmt.Sprintf("jobs:\n - name: file\n   update_every: 1\n   option_str: %q\n", literal)),
			0600,
		),
	)
	config.DiscoveryProviders = []agentdiscovery.ProviderFactory{
		agentdiscovery.NewProviderFactory(
			"file",
			func(build agentdiscovery.BuildContext) (agentdiscovery.Discoverer, bool, error) {
				discovery, err := filediscovery.NewDiscovery(
					filediscovery.Config{
						Registry: build.Registry,
						Read:     []string{path},
					},
				)
				return discovery, true, err
			},
		),
	}
	process, err := NewProcess(config)
	require.NoError(t, err)
	require.Nil(t, process.core.storeEpochs)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() {
		defer cancel()
		defer reader.Close()
		defer writer.Close()
		shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, process.Terminate(shutdown))
		require.NoError(t, <-done)
	})
	expect := func(value string) {
		t.Helper()
		select {
		case got := <-observed:
			require.Equal(t, value, got)
		case <-time.After(3 * time.Second):
			t.Fatal("collector did not receive literal config")
		}
	}
	call := func(uid, command, payload string, status int) string {
		t.Helper()
		return callProcessFunction(t, writer, output, uid, "config "+command, payload, status)
	}
	payload := func(value, vnode string) string {
		b, err := json.Marshal(map[string]any{"option_str": value, "option_int": 1, "update_every": 1, "vnode": vnode})
		require.NoError(t, err)
		return string(b)
	}
	expect(literal)
	output.waitContains(t, "CONFIG go.d:collector:test:file status running")
	require.NotContains(t, output.String(), "go.d:secretstore")
	call("store-unavailable", "go.d:secretstore:vault schema", "", 404)
	call("test-literal", "go.d:collector:test test", payload(literal, ""), 200)
	expect(literal)
	call("update-literal", "go.d:collector:test:file update", payload(literal+" updated", ""), 202)
	expect(literal + " updated")
	require.Eventually(t, func() bool {
		return strings.Count(output.String(), "CONFIG go.d:collector:test:file status running") == 2
	}, time.Second, time.Millisecond)
	call("restart-job", "go.d:collector:test:file restart", "", 200)
	expect(literal + " updated")
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(call("get", "go.d:collector:test:file get", "", 200)), &got))
	require.Equal(t, literal+" updated", got["option_str"])
	call("add-vnode-job", "go.d:collector:test add dynamic", payload(literal, "later"), 202)
	call("enable-vnode-job", "go.d:collector:test:dynamic enable", "", 202)
	require.Empty(t, observed, "a missing vnode must still delay activation")
	call("add-vnode", "go.d:vnode add later", `{"hostname":"later","guid":"22222222-2222-2222-2222-222222222222"}`, 202)
	expect(literal)
	output.waitContains(t, "CONFIG go.d:collector:test:dynamic status running")
	restart, cancelRestart := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancelRestart()
	require.NoError(t, process.Restart(restart))
	expect(literal)
	require.NotContains(t, output.String(), "CONFIG go.d:secretstore")
}
