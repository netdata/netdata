// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPluginName = "go.d"

func TestSecretStoreCallbacks(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, cb *secretStoreCallbacks, svc secretstore.Service, restart *secretStoreCallbackRestartRecorder)
	}{
		"isolation without manager": {
			run: func(t *testing.T, cb *secretStoreCallbacks, svc secretstore.Service, restart *secretStoreCallbackRestartRecorder) {
				addFn := newSecretStoreCallbackFunction(t, "ss-add", testSecretStoreTemplateID(secretstore.KindVault), dyncfg.CommandAdd, "vault_prod", map[string]any{"value": "one"})
				key, name, ok := cb.ExtractKey(addFn)
				require.True(t, ok)
				assert.Equal(t, secretstore.StoreKey(secretstore.KindVault, "vault_prod"), key)
				assert.Equal(t, "vault_prod", name)

				cfg, err := cb.ParseAndValidate(addFn, name)
				require.NoError(t, err)
				assert.Empty(t, restart.calls)
				assert.Equal(t, "", cb.TakeCommandMessage())

				require.NoError(t, cb.Start(cfg))
				assert.Equal(t, []string{key}, restart.calls)
				assert.Equal(t, "restart:"+key, cb.TakeCommandMessage())
				assert.Equal(t, testSecretStoreConfigID(key), cb.ConfigID(cfg))

				status, ok := svc.GetStatus(key)
				require.True(t, ok)
				assert.Equal(t, secretstore.KindVault, status.Kind)
				assert.Equal(t, "vault_prod", status.Name)
				assert.Equal(t, "one", resolveTestStoreValue(t, svc, key))

				updateFn := newSecretStoreCallbackFunction(t, "ss-update", testSecretStoreConfigID(key), dyncfg.CommandUpdate, "", map[string]any{"value": "two"})
				updateKey, updateName, ok := cb.ExtractKey(updateFn)
				require.True(t, ok)
				assert.Equal(t, key, updateKey)
				assert.Equal(t, name, updateName)

				updatedCfg, err := cb.ParseAndValidate(updateFn, "")
				require.NoError(t, err)
				assert.Len(t, restart.calls, 1)

				require.NoError(t, cb.Update(cfg, updatedCfg))
				assert.Equal(t, []string{key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, cb.TakeCommandMessage())
				assert.Equal(t, "two", resolveTestStoreValue(t, svc, key))

				cb.Stop(updatedCfg)
				assert.Equal(t, []string{key, key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, cb.TakeCommandMessage())
				_, ok = svc.GetStatus(key)
				assert.False(t, ok)
			},
		},
		"restart seam called only on mutation": {
			run: func(t *testing.T, cb *secretStoreCallbacks, _ secretstore.Service, restart *secretStoreCallbackRestartRecorder) {
				addFn := newSecretStoreCallbackFunction(t, "ss-mutate-add", testSecretStoreTemplateID(secretstore.KindVault), dyncfg.CommandAdd, "vault_prod", map[string]any{"value": "one"})
				key, name, ok := cb.ExtractKey(addFn)
				require.True(t, ok)

				cfg, err := cb.ParseAndValidate(addFn, name)
				require.NoError(t, err)
				assert.Empty(t, restart.calls)

				assert.Equal(t, testSecretStoreConfigID(key), cb.ConfigID(cfg))
				assert.Equal(t, "", cb.TakeCommandMessage())
				cb.OnStatusChange(nil, dyncfg.StatusAccepted, addFn)
				assert.Empty(t, restart.calls)

				require.NoError(t, cb.Start(cfg))
				assert.Equal(t, []string{key}, restart.calls)
				assert.Equal(t, "restart:"+key, cb.TakeCommandMessage())

				updateFn := newSecretStoreCallbackFunction(t, "ss-mutate-update", testSecretStoreConfigID(key), dyncfg.CommandUpdate, "", map[string]any{"value": "two"})
				updatedCfg, err := cb.ParseAndValidate(updateFn, "")
				require.NoError(t, err)
				assert.Len(t, restart.calls, 1)

				require.NoError(t, cb.Update(cfg, updatedCfg))
				assert.Equal(t, []string{key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, cb.TakeCommandMessage())

				cb.Stop(updatedCfg)
				assert.Equal(t, []string{key, key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, cb.TakeCommandMessage())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cb, svc, restart := newSecretStoreCallbacksTestSubject()
			tc.run(t, cb, svc, restart)
		})
	}
}

type secretStoreCallbackRestartRecorder struct {
	calls []string
}

func newSecretStoreCallbacksTestSubject() (*secretStoreCallbacks, secretstore.Service, *secretStoreCallbackRestartRecorder) {
	svc := newTestSecretStoreService()
	restart := &secretStoreCallbackRestartRecorder{}
	cb := newSecretStoreCallbacks(secretStoreCallbackDeps{
		pluginName: testPluginName,
		service:    svc,
		restartDependentJobs: func(storeKey string) string {
			restart.calls = append(restart.calls, storeKey)
			return "restart:" + storeKey
		},
	})
	return cb, svc, restart
}

func newSecretStoreCallbackFunction(t *testing.T, uid, id string, cmd dyncfg.Command, jobName string, payload any) dyncfg.Function {
	t.Helper()

	args := []string{id, string(cmd)}
	if jobName != "" {
		args = append(args, jobName)
	}

	fn := functions.Function{
		UID:  uid,
		Args: args,
	}
	if payload != nil {
		fn.ContentType = "application/json"
		fn.Payload = mustJSON(t, payload)
	}
	return dyncfg.NewFunction(fn)
}

func testSecretStoreTemplateID(kind secretstore.StoreKind) string {
	return fmt.Sprintf("%s%s", fmt.Sprintf(dyncfgSecretStorePrefixf, testPluginName), kind)
}

func testSecretStoreConfigID(key string) string {
	return fmt.Sprintf("%s%s", fmt.Sprintf(dyncfgSecretStorePrefixf, testPluginName), key)
}

func resolveTestStoreValue(t *testing.T, svc secretstore.Service, key string) string {
	t.Helper()

	kind, name, err := secretstore.ParseStoreKey(key)
	require.NoError(t, err)

	ref := fmt.Sprintf("%s:%s:value", kind, name)
	original := fmt.Sprintf("${store:%s}", ref)
	value, err := svc.Resolve(context.Background(), svc.Capture(), ref, original)
	require.NoError(t, err)
	return value
}

type testStoreConfig struct {
	Value string `yaml:"value" json:"value"`
}

type testStore struct {
	testStoreConfig `yaml:",inline" json:""`
}

func (s *testStore) Configuration() any { return &s.testStoreConfig }

func (s *testStore) Init(context.Context) error {
	if s.testStoreConfig.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func (s *testStore) Publish() secretstore.PublishedStore {
	return &testPublishedStore{value: s.testStoreConfig.Value}
}

type testPublishedStore struct {
	value string
}

func (s *testPublishedStore) Resolve(_ context.Context, req secretstore.ResolveRequest) (string, error) {
	if req.Operand != "value" {
		return "", fmt.Errorf("unexpected operand %q", req.Operand)
	}
	return s.value, nil
}

func newTestSecretStoreService() secretstore.Service {
	return secretstore.NewService(secretstore.Creator{
		Kind:        secretstore.KindVault,
		DisplayName: "Vault",
		Schema:      `{"jsonSchema":{"type":"object","properties":{"value":{"type":"string"}}},"uiSchema":[]}`,
		Create: func() secretstore.Store {
			return &testStore{}
		},
	})
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	bs, err := json.Marshal(v)
	require.NoError(t, err)
	return bs
}
