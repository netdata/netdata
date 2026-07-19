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

				cfg, err := cb.ParseAndValidate(restart.ctx(key), addFn, name)
				require.NoError(t, err)
				assert.Empty(t, restart.calls)

				require.NoError(t, cb.Start(restart.ctx(key), cfg))
				assert.Equal(t, []string{key}, restart.calls)
				assert.Equal(t, "restart:"+key, restart.takeMessage())
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

				updatedCfg, err := cb.ParseAndValidate(restart.ctx(key), updateFn, "")
				require.NoError(t, err)
				assert.Len(t, restart.calls, 1)

				require.NoError(t, cb.Update(restart.ctx(key), cfg, updatedCfg))
				assert.Equal(t, []string{key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, restart.takeMessage())
				assert.Equal(t, "two", resolveTestStoreValue(t, svc, key))

				cb.Stop(restart.ctx(key), updatedCfg)
				assert.Equal(t, []string{key, key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, restart.takeMessage())
				_, ok = svc.GetStatus(key)
				assert.False(t, ok)
			},
		},
		"restart seam called only on mutation": {
			run: func(t *testing.T, cb *secretStoreCallbacks, _ secretstore.Service, restart *secretStoreCallbackRestartRecorder) {
				addFn := newSecretStoreCallbackFunction(t, "ss-mutate-add", testSecretStoreTemplateID(secretstore.KindVault), dyncfg.CommandAdd, "vault_prod", map[string]any{"value": "one"})
				key, name, ok := cb.ExtractKey(addFn)
				require.True(t, ok)

				cfg, err := cb.ParseAndValidate(restart.ctx(key), addFn, name)
				require.NoError(t, err)
				assert.Empty(t, restart.calls)

				assert.Equal(t, testSecretStoreConfigID(key), cb.ConfigID(cfg))
				cb.OnStatusChange(nil, dyncfg.StatusAccepted, addFn)
				assert.Empty(t, restart.calls)

				require.NoError(t, cb.Start(restart.ctx(key), cfg))
				assert.Equal(t, []string{key}, restart.calls)
				assert.Equal(t, "restart:"+key, restart.takeMessage())

				updateFn := newSecretStoreCallbackFunction(t, "ss-mutate-update", testSecretStoreConfigID(key), dyncfg.CommandUpdate, "", map[string]any{"value": "two"})
				updatedCfg, err := cb.ParseAndValidate(restart.ctx(key), updateFn, "")
				require.NoError(t, err)
				assert.Len(t, restart.calls, 1)

				require.NoError(t, cb.Update(restart.ctx(key), cfg, updatedCfg))
				assert.Equal(t, []string{key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, restart.takeMessage())

				cb.Stop(restart.ctx(key), updatedCfg)
				assert.Equal(t, []string{key, key, key}, restart.calls)
				assert.Equal(t, "restart:"+key, restart.takeMessage())
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

// A deadline-cut restart sequence must fail the mutating callbacks: the
// commands' classification cannot depend on whether the effect's return or
// the worker's abandon wins their select race, so the cut error is returned
// (Start/Update), never swallowed into the message alone. Stop keeps the
// handler's void contract - the message still reports the skips.
func TestSecretStoreCallbacks_DeadlineCutRestartsFailMutations(t *testing.T) {
	cb, _, restart := newSecretStoreCallbacksTestSubject()
	cutErr := fmt.Errorf("the store operation timed out before all dependent restarts started")
	restart.runErr = cutErr

	addFn := newSecretStoreCallbackFunction(t, "ss-add", testSecretStoreTemplateID(secretstore.KindVault), dyncfg.CommandAdd, "vault_prod", map[string]any{"value": "one"})
	key, name, ok := cb.ExtractKey(addFn)
	require.True(t, ok)
	cfg, err := cb.ParseAndValidate(restart.ctx(key), addFn, name)
	require.NoError(t, err)

	err = cb.Start(restart.ctx(key), cfg)
	assert.ErrorIs(t, err, cutErr, "Start must return the cut error, not swallow it into the message")
	assert.Equal(t, "restart:"+key, restart.takeMessage(), "the skip report still reaches the terminal message")

	updateFn := newSecretStoreCallbackFunction(t, "ss-update", testSecretStoreConfigID(key), dyncfg.CommandUpdate, "", map[string]any{"value": "two"})
	updatedCfg, err := cb.ParseAndValidate(restart.ctx(key), updateFn, "")
	require.NoError(t, err)
	err = cb.Update(restart.ctx(key), cfg, updatedCfg)
	assert.ErrorIs(t, err, cutErr, "Update must return the cut error, not swallow it into the message")

	cb.Stop(restart.ctx(key), updatedCfg)
	assert.Equal(t, "restart:"+key, restart.takeMessage(), "Stop keeps its void contract but still reports through the message")
}

// The custom commit paths (add, conversion) classify plain failures by
// PHASE via the run box's activation marker: coded errors keep their codes;
// a plain error AFTER activation is the applied-but-degraded partial
// outcome (200, like the shared update path - 400 would misreport an
// applied mutation as a bad request); a plain error BEFORE activation is
// validation-class.
func TestStoreCommandRun_CommandCodeByPhase(t *testing.T) {
	tests := map[string]struct {
		err       error
		activated bool
		want      int
	}{
		"coded error keeps its code regardless of phase": {
			err:       &codedError{err: fmt.Errorf("nope"), code: 409},
			activated: true,
			want:      409,
		},
		"plain error after activation is the applied-but-degraded 200": {
			err:       fmt.Errorf("the store operation timed out during the dependent restarts"),
			activated: true,
			want:      200,
		},
		"plain error before activation is validation-class 400": {
			err:       fmt.Errorf("value is required"),
			activated: false,
			want:      400,
		},
		"store-not-found before activation maps to 404": {
			err:       secretstore.ErrStoreNotFound,
			activated: false,
			want:      404,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := &storeCommandRun{}
			if tc.activated {
				r.markActivated()
			}
			assert.Equal(t, tc.want, r.commandCode(tc.err))
		})
	}
}

// The race-independent classification is a TOTAL post-condition at the
// restart seam, not only a per-checkpoint duty inside the plan: a restart
// sequence reporting NO error after cancellation fired during its last step
// must still fail the mutating callbacks, or the closure-wins arm of the
// completion race would classify the same physical timeout as success.
func TestSecretStoreCallbacks_CancellationDuringRestartFailsMutationsWithoutRunError(t *testing.T) {
	cb, _, restart := newSecretStoreCallbacksTestSubject()

	addFn := newSecretStoreCallbackFunction(t, "ss-add", testSecretStoreTemplateID(secretstore.KindVault), dyncfg.CommandAdd, "vault_prod", map[string]any{"value": "one"})
	key, name, ok := cb.ExtractKey(addFn)
	require.True(t, ok)
	cfg, err := cb.ParseAndValidate(restart.ctx(key), addFn, name)
	require.NoError(t, err)

	err = cb.Start(restart.ctxCancelledDuringRun(key), cfg)
	require.Error(t, err, "cancellation during the final restart must fail the mutation even when the restart run reports no error")
	assert.Contains(t, err.Error(), "timed out")
	assert.Equal(t, "restart:"+key, restart.takeMessage(), "the per-dependent report still reaches the terminal message")

	updateFn := newSecretStoreCallbackFunction(t, "ss-update", testSecretStoreConfigID(key), dyncfg.CommandUpdate, "", map[string]any{"value": "two"})
	updatedCfg, err := cb.ParseAndValidate(restart.ctx(key), updateFn, "")
	require.NoError(t, err)
	err = cb.Update(restart.ctxCancelledDuringRun(key), cfg, updatedCfg)
	require.Error(t, err, "Update must reclassify cancellation during the final restart the same way")
	assert.Contains(t, err.Error(), "timed out")
}

// secretStoreCallbackRestartRecorder plays the command-run box: it records
// restart-seam invocations and captures the message the callbacks store for
// the terminal. runErr, when set, plays a deadline-cut restart sequence.
type secretStoreCallbackRestartRecorder struct {
	calls       []string
	box         *storeCommandRun
	runErr      error
	cancelOnRun context.CancelFunc
}

// ctx returns an effect context carrying a command run whose staged restart
// records the given store key.
func (r *secretStoreCallbackRestartRecorder) ctx(storeKey string) context.Context {
	return r.ctxOn(context.Background(), storeKey)
}

func (r *secretStoreCallbackRestartRecorder) ctxCancelledDuringRun(
	storeKey string,
) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancelOnRun = cancel
	return r.ctxOn(ctx, storeKey)
}

// ctxOn is ctx on an arbitrary parent.
func (r *secretStoreCallbackRestartRecorder) ctxOn(parent context.Context, storeKey string) context.Context {
	r.box = &storeCommandRun{
		staged: &StagedRestarts{
			Run: func(context.Context) (string, error) {
				r.calls = append(r.calls, storeKey)
				if r.cancelOnRun != nil {
					r.cancelOnRun()
					r.cancelOnRun = nil
				}
				return "restart:" + storeKey, r.runErr
			},
			Flush: func() {},
		},
	}
	return withStoreCommandRun(parent, r.box)
}

func (r *secretStoreCallbackRestartRecorder) takeMessage() string {
	if r.box == nil {
		return ""
	}
	return r.box.takeMessage()
}

func newSecretStoreCallbacksTestSubject() (*secretStoreCallbacks, secretstore.Service, *secretStoreCallbackRestartRecorder) {
	svc := newTestSecretStoreService()
	restart := &secretStoreCallbackRestartRecorder{}
	cb := newSecretStoreCallbacks(secretStoreCallbackDeps{
		pluginName: testPluginName,
		service:    svc,
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
