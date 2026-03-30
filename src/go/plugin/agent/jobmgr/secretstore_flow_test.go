// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestStoreConfig struct {
	Value string `yaml:"value" json:"value"`
}

type testStore struct {
	TestStoreConfig `yaml:",inline" json:""`
}

func (s *testStore) Configuration() any { return &s.TestStoreConfig }

func (s *testStore) Init(context.Context) error {
	if s.TestStoreConfig.Value == "" {
		return fmt.Errorf("value is required")
	}
	return nil
}

func (s *testStore) Publish() secretstore.PublishedStore {
	return &testPublishedStore{value: s.TestStoreConfig.Value}
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

type secretAwareCollector struct {
	collectorapi.Base
	Config collectorapi.MockConfiguration `yaml:",inline" json:""`
}

func (c *secretAwareCollector) Configuration() any           { return c.Config }
func (c *secretAwareCollector) Check(context.Context) error  { return nil }
func (c *secretAwareCollector) Cleanup(context.Context)      {}
func (c *secretAwareCollector) Charts() *collectorapi.Charts { return &collectorapi.Charts{} }
func (c *secretAwareCollector) Collect(context.Context) map[string]int64 {
	return map[string]int64{"value": 1}
}

func (c *secretAwareCollector) Init(context.Context) error {
	if c.Config.OptionStr != "good" {
		return fmt.Errorf("secret is not usable: %s", c.Config.OptionStr)
	}
	return nil
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

func TestApplyConfig_ResolvesStoreReferenceWithKindAndName(t *testing.T) {
	svc := newTestSecretStoreService()
	raw := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "resolved-secret"}, confgroup.TypeDyncfg, confgroup.TypeDyncfg)
	require.NoError(t, svc.Add(context.Background(), raw))

	cfg := prepareDyncfgCfg("success", "secret-job").
		Set("option_str", "${store:vault:vault_prod:value}").
		Set("option_int", 7)

	module := &collectorapi.MockCollectorV1{}
	err := applyConfig(t.Context(), cfg, module, secretresolver.New(), svc, svc.Capture())
	require.NoError(t, err)

	assert.Equal(t, "resolved-secret", module.Config.OptionStr)
	assert.Equal(t, 7, module.Config.OptionInt)
}

func TestRun_StartupLoadedSecretStoreIsAvailableToFirstCollectorStart(t *testing.T) {
	initial := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "good"}, "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser)
	mgr := New(Config{
		PluginName:         testPluginName,
		SecretStores:       []secretstore.Config{initial},
		SecretStoreService: newTestSecretStoreService(),
	})
	mgr.modules = collectorapi.Registry{
		"gated": {
			Create: func() collectorapi.CollectorV1 { return &secretAwareCollector{} },
		},
	}

	var out bytes.Buffer
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))
	mgr.runModePolicy.AutoEnableDiscovered = true

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []*confgroup.Group)
	done := make(chan struct{})
	go func() {
		defer close(done)
		mgr.Run(ctx, in)
	}()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	require.True(t, mgr.WaitStarted(waitCtx), "manager did not report started")

	key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	entry, ok := mgr.lookupSecretStoreEntry(key)
	require.True(t, ok)
	assert.Equal(t, dyncfg.StatusRunning, entry.Status)
	_, ok = mustSecretStoreService(t, mgr).GetStatus(key)
	assert.True(t, ok)

	cfg := prepareDyncfgCfg("gated", "startup").
		Set("option_str", "${store:vault:vault_prod:value}").
		Set("option_int", 1)
	mgr.addConfig(cfg)

	jobEntry, ok := mgr.lookupExposedByFullName(cfg.FullName())
	require.True(t, ok)
	assert.Equal(t, dyncfg.StatusRunning, jobEntry.Status)
	require.Len(t, mgr.runningJobs.snapshot(), 1)
	assert.Equal(t, cfg.FullName(), mgr.runningJobs.snapshot()[0].FullName())

	cancel()
	close(in)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("manager did not stop after cancel")
	}
}

func TestDyncfgSecretStoreUpdate_DependentRestartBehavior(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, out *bytes.Buffer)
	}{
		"restarts failed dependent after store is fixed": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				mgr.modules["gated"] = collectorapi.Creator{
					Create: func() collectorapi.CollectorV1 { return &secretAwareCollector{} },
				}

				key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", map[string]any{"value": "good"}, dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("gated", "mysql").
					Set("option_str", "${store:vault:vault_prod:value}").
					Set("option_int", 1)
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.syncSecretStoreDepsForConfig(cfg)
				require.NoError(t, mgr.collectorCallbacks.Start(cfg))

				_, running := mgr.secretStoreDeps.Impacted(key)
				require.Len(t, running, 1)
				assert.Equal(t, cfg.FullName(), running[0].ID)

				badFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-bad",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "bad"}),
					Args: []string{
						mgr.dyncfgSecretStoreID(key),
						string(dyncfg.CommandUpdate),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(badFn)

				var badResp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-update-bad", &badResp)
				assert.Equal(t, float64(200), badResp["status"])
				assert.Contains(t, badResp["message"], "Secretstore change applied, but dependent collector restarts failed")
				assert.Contains(t, badResp["message"], "gated:mysql")

				entry, ok := mgr.lookupExposedByFullName(cfg.FullName())
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusFailed, entry.Status)

				_, running = mgr.secretStoreDeps.Impacted(key)
				assert.Empty(t, running)

				goodFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-good",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "good"}),
					Args: []string{
						mgr.dyncfgSecretStoreID(key),
						string(dyncfg.CommandUpdate),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(goodFn)

				var goodResp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-update-good", &goodResp)
				assert.Equal(t, float64(200), goodResp["status"])
				assert.Equal(t, "", goodResp["message"])

				entry, ok = mgr.lookupExposedByFullName(cfg.FullName())
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning, entry.Status)

				_, running = mgr.secretStoreDeps.Impacted(key)
				require.Len(t, running, 1)
				assert.Equal(t, cfg.FullName(), running[0].ID)
			},
		},
		"ignores accepted and disabled dependents": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", map[string]any{"value": "good"}, dyncfg.StatusRunning)

				acceptedCfg := prepareDyncfgCfg("success", "accepted")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    acceptedCfg,
					Status: dyncfg.StatusAccepted,
				})
				mgr.secretStoreDeps.SetActiveJobStores(acceptedCfg.FullName(), "success:accepted", []string{key})

				disabledCfg := prepareDyncfgCfg("success", "disabled")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    disabledCfg,
					Status: dyncfg.StatusDisabled,
				})
				mgr.secretStoreDeps.SetActiveJobStores(disabledCfg.FullName(), "success:disabled", []string{key})

				updateFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-ignored",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "better"}),
					Args: []string{
						mgr.dyncfgSecretStoreID(key),
						string(dyncfg.CommandUpdate),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(updateFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-update-ignored", &resp)
				assert.Equal(t, float64(200), resp["status"])
				assert.Equal(t, "", resp["message"])

				acceptedEntry, ok := mgr.lookupExposedByFullName(acceptedCfg.FullName())
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusAccepted, acceptedEntry.Status)

				disabledEntry, ok := mgr.lookupExposedByFullName(disabledCfg.FullName())
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusDisabled, disabledEntry.Status)

				_, running := mgr.secretStoreDeps.Impacted(key)
				assert.Empty(t, running)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newDyncfgSecretStoreTestManagerWithService(newTestSecretStoreService())
			tc.run(t, mgr, out)
		})
	}
}

func TestDyncfgSecretStoreGet_CanonicalJSONDoesNotExposeUnknownFields(t *testing.T) {
	mgr, out := newDyncfgSecretStoreTestManagerWithService(newTestSecretStoreService())

	cfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{
		"value":   "resolved-secret",
		"ignored": "drop-me",
	}, confgroup.TypeDyncfg, confgroup.TypeDyncfg)
	_, changed, err := mgr.rememberSecretStoreConfig(cfg)
	require.NoError(t, err)
	require.True(t, changed)

	getFn := dyncfg.NewFunction(functions.Function{
		UID:  "ss-get-canonical",
		Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandGet)},
	})
	mgr.dyncfgSecretStoreSeqExec(getFn)

	var got map[string]any
	mustDecodeFunctionPayload(t, out.String(), "ss-get-canonical", &got)
	assert.Equal(t, "resolved-secret", got["value"])
	_, ok := got["ignored"]
	assert.False(t, ok)
}

func TestSecretStoreConfigFromPayload_PreservesKindAndNameForStoreSyntax(t *testing.T) {
	mgr, _ := newDyncfgSecretStoreTestManagerWithService(newTestSecretStoreService())

	fn := dyncfg.NewFunction(functions.Function{
		ContentType: "application/json",
		Payload:     mustJSON(t, map[string]any{"value": "resolved-secret"}),
	})
	cfg, err := mgr.secretStoreConfigFromPayload(fn, "vault_prod", secretstore.KindVault)
	require.NoError(t, err)

	assert.Equal(t, "vault_prod", cfg.Name())
	assert.Equal(t, secretstore.KindVault, cfg.Kind())
	assert.Equal(t, "resolved-secret", cfg["value"])
}
