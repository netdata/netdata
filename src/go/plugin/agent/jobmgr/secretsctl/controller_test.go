// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerSeqExec(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams)
	}{
		"schema dispatch": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				fn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-schema",
					Args: []string{ctl.templateID(secretstore.KindVault), string(dyncfg.CommandSchema)},
				})
				ctl.SeqExec(fn)

				var payload any
				mustDecodeFunctionPayload(t, out.String(), "ss-schema", &payload)
				assert.NotNil(t, payload)
				assert.Empty(t, seams.affectedJobsCalls)
				assert.Empty(t, seams.restartCalls)
			},
		},
		"test preview uses affected jobs seam": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				addFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-add",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "one"}),
					Args:        []string{ctl.templateID(secretstore.KindVault), string(dyncfg.CommandAdd), "vault_prod"},
				})
				ctl.SeqExec(addFn)

				key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
				seams.affectedJobs[key] = []secretstore.JobRef{{ID: "mysql:prod", Display: "mysql:prod"}}
				seams.restartableJobs[key] = []secretstore.JobRef{{ID: "mysql:prod", Display: "mysql:prod"}}

				testFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-test",
					Args: []string{ctl.configID(key), string(dyncfg.CommandTest)},
				})
				ctl.SeqExec(testFn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test", &payload)
				assert.Equal(t, float64(202), payload["status"])
				assert.Contains(t, payload["message"], "This secretstore is used by jobs: mysql:prod.")
				assert.Contains(t, payload["message"], "Running or failed jobs that would be restarted automatically by a change: mysql:prod.")
				assert.Equal(t, []string{key}, seams.affectedJobsCalls)
				assert.Equal(t, []string{key}, seams.restartableJobsCalls)
			},
		},
		"remove blocks when dependent jobs exist": {
			run: func(t *testing.T, ctl *Controller, out *bytes.Buffer, seams *controllerSeams) {
				addFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-add-remove-blocked",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"value": "one"}),
					Args:        []string{ctl.templateID(secretstore.KindVault), string(dyncfg.CommandAdd), "vault_prod"},
				})
				ctl.SeqExec(addFn)

				key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
				seams.affectedJobs[key] = []secretstore.JobRef{{ID: "mysql:prod", Display: "mysql:prod"}, {ID: "nginx:prod", Display: "nginx:prod"}}

				removeFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-remove-blocked",
					Args: []string{ctl.configID(key), string(dyncfg.CommandRemove)},
				})
				ctl.SeqExec(removeFn)

				var payload map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-remove-blocked", &payload)
				assert.Equal(t, float64(409), payload["status"])
				assert.Equal(t, "The specified secretstore 'vault:vault_prod' is used by jobs (mysql:prod, nginx:prod).", payload["errorMessage"])
				_, ok := ctl.Lookup(key)
				assert.True(t, ok)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctl, out, seams := newControllerTestSubject()
			tc.run(t, ctl, out, seams)
		})
	}
}

func TestControllerSetAPI_NilPreservesResponder(t *testing.T) {
	tests := map[string]struct {
		uid string
	}{
		"nil SetAPI keeps existing responder for schema responses": {
			uid: "ss-schema-nil-rebind",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctl, out, _ := newControllerTestSubject()
			ctl.SetAPI(nil)

			fn := dyncfg.NewFunction(functions.Function{
				UID:  tc.uid,
				Args: []string{ctl.templateID(secretstore.KindVault), string(dyncfg.CommandSchema)},
			})
			ctl.SeqExec(fn)

			var payload any
			mustDecodeFunctionPayload(t, out.String(), tc.uid, &payload)
			assert.NotNil(t, payload)
		})
	}
}

func TestControllerSeqExec_TestStoredWithoutService_Returns400(t *testing.T) {
	tests := map[string]struct {
		storeKey string
	}{
		"stored validation without service returns controlled 400": {
			storeKey: secretstore.StoreKey(secretstore.KindVault, "vault_prod"),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			ctl := New(Options{
				Logger: logger.New(),
				API:    dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))),
				Plugin: testPluginName,
			})

			cfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "one"}, confgroup.TypeDyncfg, confgroup.TypeDyncfg)
			ctl.exposed.Add(&dyncfg.Entry[secretstore.Config]{
				Cfg:    cfg,
				Status: dyncfg.StatusRunning,
			})

			fn := dyncfg.NewFunction(functions.Function{
				UID:  "ss-test-nil-service",
				Args: []string{ctl.configID(tc.storeKey), string(dyncfg.CommandTest)},
			})

			ctl.SeqExec(fn)

			var payload map[string]any
			mustDecodeFunctionPayload(t, out.String(), "ss-test-nil-service", &payload)
			assert.Equal(t, float64(400), payload["status"])
			assert.Contains(t, payload["errorMessage"], "secretstore service is not available")
		})
	}
}

func TestRememberDiscoveredConfig_DrainsRestartFailureMessageFromNonHandlerStop(t *testing.T) {
	ctl, _, seams := newControllerTestSubject()
	existing := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "one"}, "/etc/netdata/secretstores.yaml", confgroup.TypeUser)
	require.NoError(t, ctl.Service().Add(context.Background(), existing))
	ctl.seen.Add(existing)
	ctl.exposed.Add(&dyncfg.Entry[secretstore.Config]{
		Cfg:    existing,
		Status: dyncfg.StatusRunning,
	})

	key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")
	seams.restartMessages[key] = "restart failed"

	replacement := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "two"}, confgroup.TypeDyncfg, confgroup.TypeDyncfg)
	entry, changed, err := ctl.RememberDiscoveredConfig(replacement)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, dyncfg.StatusAccepted, entry.Status)
	assert.Equal(t, []string{key}, seams.restartCalls)
	_, ok := ctl.Service().GetStatus(key)
	assert.False(t, ok)
	assert.Equal(t, "", ctl.cb.TakeCommandMessage())
}

func TestControllerPublishExisting(t *testing.T) {
	t.Run("initial valid config publishes running without enable disable commands", func(t *testing.T) {
		cfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "one"}, "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser)

		var out bytes.Buffer
		ctl := New(Options{
			Logger:  logger.New(),
			API:     dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))),
			Plugin:  testPluginName,
			Service: newTestSecretStoreService(),
			Initial: []secretstore.Config{cfg},
		})

		ctl.PublishExisting()

		entry, ok := ctl.Lookup(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
		require.True(t, ok)
		assert.Equal(t, dyncfg.StatusRunning, entry.Status)
		_, ok = ctl.Service().GetStatus(entry.Cfg.ExposedKey())
		assert.True(t, ok)
		assert.Contains(t, out.String(), "schema get update test")
		assert.NotContains(t, out.String(), "enable")
		assert.NotContains(t, out.String(), "disable")
	})

	t.Run("keyable startup failure publishes failed", func(t *testing.T) {
		cfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{}, "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser)

		ctl, _, _ := newControllerTestSubjectWithOptions(Options{
			Initial: []secretstore.Config{cfg},
		})
		ctl.PublishExisting()

		entry, ok := ctl.Lookup(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
		require.True(t, ok)
		assert.Equal(t, dyncfg.StatusFailed, entry.Status)
		_, ok = ctl.Service().GetStatus(entry.Cfg.ExposedKey())
		assert.False(t, ok)
	})

	t.Run("failed higher priority startup config shadows lower priority running config", func(t *testing.T) {
		stockCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "stock"}, "file=/usr/lib/netdata/conf.d/go.d/ss/vault.conf", confgroup.TypeStock)
		userCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{}, "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser)

		ctl, _, _ := newControllerTestSubjectWithOptions(Options{
			Initial: []secretstore.Config{stockCfg, userCfg},
		})
		ctl.PublishExisting()

		entry, ok := ctl.Lookup(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
		require.True(t, ok)
		assert.Equal(t, dyncfg.StatusFailed, entry.Status)
		assert.Equal(t, confgroup.TypeUser, entry.Cfg.SourceType())
		assert.Equal(t, "file=/etc/netdata/go.d/ss/vault.conf", entry.Cfg.Source())
		_, ok = ctl.Service().GetStatus(entry.Cfg.ExposedKey())
		assert.False(t, ok)
	})

	t.Run("later equal priority valid config replaces earlier failed config", func(t *testing.T) {
		failedCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{}, "file=/etc/netdata/go.d/ss/01-vault.conf", confgroup.TypeUser)
		validCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"value": "good"}, "file=/etc/netdata/go.d/ss/02-vault.conf", confgroup.TypeUser)

		ctl, _, _ := newControllerTestSubjectWithOptions(Options{
			Initial: []secretstore.Config{failedCfg, validCfg},
		})
		ctl.PublishExisting()

		entry, ok := ctl.Lookup(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
		require.True(t, ok)
		assert.Equal(t, dyncfg.StatusRunning, entry.Status)
		assert.Equal(t, "file=/etc/netdata/go.d/ss/02-vault.conf", entry.Cfg.Source())
		_, ok = ctl.Service().GetStatus(entry.Cfg.ExposedKey())
		assert.True(t, ok)
	})
}

type controllerSeams struct {
	affectedJobs         map[string][]secretstore.JobRef
	affectedJobsCalls    []string
	restartableJobs      map[string][]secretstore.JobRef
	restartableJobsCalls []string
	restartMessages      map[string]string
	restartCalls         []string
}

func newControllerTestSubject() (*Controller, *bytes.Buffer, *controllerSeams) {
	return newControllerTestSubjectWithOptions(Options{})
}

func newControllerTestSubjectWithOptions(opts Options) (*Controller, *bytes.Buffer, *controllerSeams) {
	var out bytes.Buffer
	seams := &controllerSeams{
		affectedJobs:    make(map[string][]secretstore.JobRef),
		restartableJobs: make(map[string][]secretstore.JobRef),
		restartMessages: make(map[string]string),
	}
	svc := opts.Service
	if svc == nil {
		svc = newTestSecretStoreService()
	}
	log := opts.Logger
	if log == nil {
		log = logger.New()
	}
	api := opts.API
	if api == nil {
		api = dyncfg.NewResponder(netdataapi.New(safewriter.New(&out)))
	}
	plugin := opts.Plugin
	if plugin == "" {
		plugin = testPluginName
	}
	affectedJobs := opts.AffectedJobs
	if affectedJobs == nil {
		affectedJobs = func(storeKey string) []secretstore.JobRef {
			seams.affectedJobsCalls = append(seams.affectedJobsCalls, storeKey)
			return seams.affectedJobs[storeKey]
		}
	}
	restartableJobs := opts.RestartableAffectedJobs
	if restartableJobs == nil {
		restartableJobs = func(storeKey string) []secretstore.JobRef {
			seams.restartableJobsCalls = append(seams.restartableJobsCalls, storeKey)
			return seams.restartableJobs[storeKey]
		}
	}
	restartDependentJobs := opts.RestartDependentJobs
	if restartDependentJobs == nil {
		restartDependentJobs = func(storeKey string) string {
			seams.restartCalls = append(seams.restartCalls, storeKey)
			return seams.restartMessages[storeKey]
		}
	}
	ctl := New(Options{
		Logger:                  log,
		API:                     api,
		Plugin:                  plugin,
		Service:                 svc,
		AffectedJobs:            affectedJobs,
		RestartableAffectedJobs: restartableJobs,
		RestartDependentJobs:    restartDependentJobs,
		Initial:                 opts.Initial,
		Seen:                    opts.Seen,
		Exposed:                 opts.Exposed,
	})
	return ctl, &out, seams
}

func newSecretStoreConfigWithSource(t *testing.T, kind secretstore.StoreKind, name string, cfg map[string]any, source, sourceType string) secretstore.Config {
	t.Helper()
	bs, err := json.Marshal(cfg)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(bs, &payload))
	out := secretstore.Config(payload)
	out.SetName(name)
	out.SetKind(kind)
	out.SetSource(source)
	out.SetSourceType(sourceType)
	return out
}

func mustDecodeFunctionPayload(t *testing.T, output, uid string, dst any) {
	t.Helper()

	re := regexp.MustCompile(`(?s)FUNCTION_RESULT_BEGIN ` + regexp.QuoteMeta(uid) + ` [^\n]+\n(.*?)\nFUNCTION_RESULT_END`)
	match := re.FindStringSubmatch(output)
	require.Len(t, match, 2, "function result for uid '%s' not found in output:\n%s", uid, output)
	require.NoError(t, json.Unmarshal([]byte(match[1]), dst))
}
