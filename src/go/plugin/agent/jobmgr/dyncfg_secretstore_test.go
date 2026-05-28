// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestDyncfgSecretStoreSeqExec(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, mgr *Manager, out *bytes.Buffer)
	}{
		"add and get": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				addFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-add",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfig()),
					Args: []string{
						mgr.dyncfgSecretStoreTemplateID(secretstore.KindVault),
						string(dyncfg.CommandAdd),
						"vault_prod",
					},
				})
				mgr.dyncfgSecretStoreSeqExec(addFn)

				var addResp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-add", &addResp)
				assert.Equal(t, float64(200), addResp["status"])
				assert.Equal(t, "", addResp["message"])
				entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning, entry.Status)
				_, ok = mustSecretStoreService(t, mgr).GetStatus(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.True(t, ok)
				assert.Contains(t, out.String(), "schema get update test userconfig remove")
				assert.NotContains(t, out.String(), "enable")
				assert.NotContains(t, out.String(), "disable")

				getFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-get",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandGet)},
				})
				mgr.dyncfgSecretStoreSeqExec(getFn)

				var cfg map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-get", &cfg)
				_, ok = cfg["name"]
				assert.False(t, ok)
				_, ok = cfg["kind"]
				assert.False(t, ok)
				assert.Equal(t, "token", cfg["mode"])
			},
		},
		"add and get from yaml payload": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				addFn := dyncfg.NewFunction(functions.Function{
					UID: "ss-add-yaml",
					Payload: []byte(`
mode: token
mode_token:
  token: vault-token
addr: https://vault.example
`),
					Args: []string{
						mgr.dyncfgSecretStoreTemplateID(secretstore.KindVault),
						string(dyncfg.CommandAdd),
						"vault_prod",
					},
				})
				mgr.dyncfgSecretStoreSeqExec(addFn)

				var addResp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-add-yaml", &addResp)
				assert.Equal(t, float64(200), addResp["status"])

				entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning, entry.Status)

				getFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-get-yaml",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandGet)},
				})
				mgr.dyncfgSecretStoreSeqExec(getFn)

				var cfg map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-get-yaml", &cfg)
				_, ok = cfg["name"]
				assert.False(t, ok)
				assert.Equal(t, "token", cfg["mode"])

				modeToken, ok := cfg["mode_token"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "vault-token", modeToken["token"])
			},
		},
		"add activation failure publishes failed store": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				addFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-add-failed",
					ContentType: "application/json",
					Payload:     mustJSON(t, map[string]any{"mode": "token"}),
					Args: []string{
						mgr.dyncfgSecretStoreTemplateID(secretstore.KindVault),
						string(dyncfg.CommandAdd),
						"vault_prod",
					},
				})
				mgr.dyncfgSecretStoreSeqExec(addFn)

				var addResp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-add-failed", &addResp)
				assert.Equal(t, float64(400), addResp["status"])
				assert.Contains(t, addResp["errorMessage"], "mode_token")

				entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusFailed, entry.Status)
				assert.Equal(t, confgroup.TypeDyncfg, entry.Cfg.SourceType())
				_, ok = mustSecretStoreService(t, mgr).GetStatus(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.False(t, ok)
			},
		},
		"duplicate add is rejected": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})
				mgr.secretStoreDeps.setRunning(cfg.FullName(), true)

				addFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-add-duplicate",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfigTokenFile()),
					Args: []string{
						mgr.dyncfgSecretStoreTemplateID(secretstore.KindVault),
						string(dyncfg.CommandAdd),
						"vault_prod",
					},
				})
				mgr.dyncfgSecretStoreSeqExec(addFn)

				var addResp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-add-duplicate", &addResp)
				assert.Equal(t, float64(409), addResp["status"])
				assert.Contains(t, addResp["errorMessage"], "already exists")
				assert.NotContains(t, out.String(), "CONFIG test:collector:success:mysql status running")

				getFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-get-after-duplicate",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandGet)},
				})
				mgr.dyncfgSecretStoreSeqExec(getFn)

				var got map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-get-after-duplicate", &got)
				assert.Equal(t, "token", got["mode"])

				_, ok := mustSecretStoreService(t, mgr).GetStatus(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.True(t, ok)
			},
		},
		"runtime-affecting update succeeds for running store": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})
				mgr.secretStoreDeps.setRunning(cfg.FullName(), true)

				updateFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfigTokenFile()),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandUpdate),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(updateFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-update", &resp)
				assert.Equal(t, float64(200), resp["status"])
				assert.Equal(t, "", resp["message"])

				getFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-get-updated",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandGet)},
				})
				mgr.dyncfgSecretStoreSeqExec(getFn)

				var got map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-get-updated", &got)
				assert.Equal(t, "token_file", got["mode"])
			},
		},
		"unknown-field update is preserved in raw config but hidden from get": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})
				mgr.secretStoreDeps.setRunning(cfg.FullName(), true)

				updateCfg := testVaultConfig()
				updateCfg["ui_note"] = "updated description"

				updateFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-update-metadata",
					ContentType: "application/json",
					Payload:     mustJSON(t, updateCfg),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandUpdate),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(updateFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-update-metadata", &resp)
				assert.Equal(t, float64(200), resp["status"])

				getFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-get-metadata",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandGet)},
				})
				mgr.dyncfgSecretStoreSeqExec(getFn)

				var got map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-get-metadata", &got)
				_, ok := got["ui_note"]
				assert.False(t, ok)

				entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				assert.Equal(t, "updated description", entry.Cfg["ui_note"])
			},
		},
		"test command reports affected jobs": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})
				testFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-test-affected",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfigTokenFile()),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandTest),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-affected", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, "Updated configuration is used by jobs: success:mysql. Running or failed jobs that would be restarted automatically: success:mysql.", resp["message"])
			},
		},
		"test command reports no-op for unchanged payload": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})

				testFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-test-noop",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfig()),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandTest),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-noop", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, "Submitted configuration does not change the active secretstore.", resp["message"])
				assert.NotContains(t, out.String(), "CONFIG test:collector:success:mysql status running")
			},
		},
		"test command does not mutate generation": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)
				before := mustSecretStoreService(t, mgr).Capture().Generation()

				testCfg := testVaultConfig()
				testCfg["mode_token"].(map[string]any)["extra"] = "ignored"

				testFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-test",
					ContentType: "application/json",
					Payload:     mustJSON(t, testCfg),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandTest),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, before, mustSecretStoreService(t, mgr).Capture().Generation())
			},
		},
		"test command with empty payload validates stored config": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)
				before := mustSecretStoreService(t, mgr).Capture().Generation()

				testFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-test-empty",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandTest)},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-empty", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, "Stored configuration is valid. No jobs are currently using this secretstore.", resp["message"])

				status, ok := mustSecretStoreService(t, mgr).GetStatus(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				require.NotNil(t, status.LastValidation)
				assert.True(t, status.LastValidation.OK)
				assert.Equal(t, before, mustSecretStoreService(t, mgr).Capture().Generation())
			},
		},
		"test command with empty payload reports affected jobs": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				cfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    cfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(cfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})

				testFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-test-empty-affected",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandTest)},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-empty-affected", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, "Stored configuration is valid. This secretstore is used by jobs: success:mysql. Running or failed jobs that would be restarted automatically by a change: success:mysql.", resp["message"])
			},
		},
		"test command reports all dependent jobs and restartable subset": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				runningCfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    runningCfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(runningCfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})

				acceptedCfg := prepareDyncfgCfg("success", "nginx")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    acceptedCfg,
					Status: dyncfg.StatusAccepted,
				})
				mgr.secretStoreDeps.SetActiveJobStores(acceptedCfg.FullName(), "success:nginx", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})

				testFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-test-all-deps",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfigTokenFile()),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandTest),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-all-deps", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, "Updated configuration is used by jobs: success:mysql, success:nginx. Running or failed jobs that would be restarted automatically: success:mysql.", resp["message"])
			},
		},
		"test command reports no affected jobs for changed payload": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				testFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-test-no-affected",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfigTokenFile()),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandTest),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-no-affected", &resp)
				assert.Equal(t, float64(202), resp["status"])
				assert.Equal(t, "No jobs currently use this secretstore.", resp["message"])
			},
		},
		"enable is unsupported": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				enableFn := dyncfg.NewFunction(functions.Function{
					UID: "ss-enable-unsupported",
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandEnable),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(enableFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-enable-unsupported", &resp)
				assert.Equal(t, float64(501), resp["status"])

				entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning, entry.Status)
			},
		},
		"disable is unsupported": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				disableFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-disable-unsupported",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandDisable)},
				})
				mgr.dyncfgSecretStoreSeqExec(disableFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-disable-unsupported", &resp)
				assert.Equal(t, float64(501), resp["status"])

				entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				require.True(t, ok)
				assert.Equal(t, dyncfg.StatusRunning, entry.Status)
			},
		},
		"remove deletes store": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				removeFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-remove",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandRemove)},
				})
				mgr.dyncfgSecretStoreSeqExec(removeFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-remove", &resp)
				assert.Equal(t, float64(200), resp["status"])
				_, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.False(t, ok)
				_, ok = mustSecretStoreService(t, mgr).GetStatus(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.False(t, ok)
			},
		},
		"remove blocks when dependent jobs use the store": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusRunning)

				runningCfg := prepareDyncfgCfg("success", "mysql")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    runningCfg,
					Status: dyncfg.StatusRunning,
				})
				mgr.secretStoreDeps.SetActiveJobStores(runningCfg.FullName(), "success:mysql", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})

				disabledCfg := prepareDyncfgCfg("success", "nginx")
				mgr.collectorExposed.Add(&dyncfg.Entry[confgroup.Config]{
					Cfg:    disabledCfg,
					Status: dyncfg.StatusDisabled,
				})
				mgr.secretStoreDeps.SetActiveJobStores(disabledCfg.FullName(), "success:nginx", []string{secretstore.StoreKey(secretstore.KindVault, "vault_prod")})

				removeFn := dyncfg.NewFunction(functions.Function{
					UID:  "ss-remove-blocked",
					Args: []string{mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")), string(dyncfg.CommandRemove)},
				})
				mgr.dyncfgSecretStoreSeqExec(removeFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-remove-blocked", &resp)
				assert.Equal(t, float64(409), resp["status"])
				assert.Equal(t, "The specified secretstore 'vault:vault_prod' is used by jobs (success:mysql, success:nginx).", resp["errorMessage"])

				_, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.True(t, ok)
				_, ok = mustSecretStoreService(t, mgr).GetStatus(secretstore.StoreKey(secretstore.KindVault, "vault_prod"))
				assert.True(t, ok)
			},
		},
		"userconfig returns yaml from payload": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				userconfigFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-userconfig",
					ContentType: "application/json",
					Payload:     mustJSON(t, testVaultConfig()),
					Args: []string{
						mgr.dyncfgSecretStoreTemplateID(secretstore.KindVault),
						string(dyncfg.CommandUserconfig),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(userconfigFn)

				re := regexp.MustCompile(`(?s)FUNCTION_RESULT_BEGIN ss-userconfig [^\n]+\n(.*?)\nFUNCTION_RESULT_END`)
				match := re.FindStringSubmatch(out.String())
				require.Len(t, match, 2)
				var parsed map[string]any
				require.NoError(t, yaml.Unmarshal([]byte(match[1]), &parsed))
				_, ok := parsed["name"]
				assert.False(t, ok)
				_, ok = parsed["kind"]
				assert.False(t, ok)
			},
		},
		"test rejects wrapped config payload": {
			run: func(t *testing.T, mgr *Manager, out *bytes.Buffer) {
				seedSecretStore(t, mgr, secretstore.KindVault, "vault_prod", testVaultConfig(), dyncfg.StatusAccepted)

				testFn := dyncfg.NewFunction(functions.Function{
					UID:         "ss-test-wrapped-config-payload",
					ContentType: "application/json",
					Payload: mustJSON(t, map[string]any{
						"config": testVaultConfigTokenFile(),
					}),
					Args: []string{
						mgr.dyncfgSecretStoreID(secretstore.StoreKey(secretstore.KindVault, "vault_prod")),
						string(dyncfg.CommandTest),
					},
				})
				mgr.dyncfgSecretStoreSeqExec(testFn)

				var resp map[string]any
				mustDecodeFunctionPayload(t, out.String(), "ss-test-wrapped-config-payload", &resp)
				assert.Equal(t, float64(400), resp["status"])
				assert.Contains(t, resp["errorMessage"], "mode")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, out := newDyncfgSecretStoreTestManager()
			tc.run(t, mgr, out)
		})
	}
}

func TestSecretStoreConfigFromPayload(t *testing.T) {
	tests := map[string]struct {
		fn              functions.Function
		name            string
		kind            secretstore.StoreKind
		wantErrContains string
		assertConfig    func(t *testing.T, cfg secretstore.Config)
	}{
		"json direct config payload": {
			fn: functions.Function{
				ContentType: "application/json",
				Payload:     mustJSON(t, testVaultConfig()),
			},
			name: "vault_prod",
			kind: secretstore.KindVault,
			assertConfig: func(t *testing.T, cfg secretstore.Config) {
				require.NotNil(t, cfg)
				assert.Equal(t, "vault_prod", cfg.Name())
				assert.Equal(t, secretstore.KindVault, cfg.Kind())
			},
		},
		"yaml direct config payload": {
			fn: functions.Function{
				Payload: []byte("mode: token\nmode_token:\n  token: vault-token\naddr: https://vault.example\n"),
			},
			name: "vault_prod",
			kind: secretstore.KindVault,
			assertConfig: func(t *testing.T, cfg secretstore.Config) {
				require.NotNil(t, cfg)
				assert.Equal(t, "vault_prod", cfg.Name())
				assert.Equal(t, secretstore.KindVault, cfg.Kind())
			},
		},
		"missing payload": {
			fn: functions.Function{
				ContentType: "application/json",
			},
			name:            "vault_prod",
			kind:            secretstore.KindVault,
			wantErrContains: "missing configuration payload",
		},
		"wrapped config payload becomes invalid raw config": {
			fn: functions.Function{
				ContentType: "application/json",
				Payload: mustJSON(t, map[string]any{
					"config": testVaultConfig(),
				}),
			},
			name: "vault_prod",
			kind: secretstore.KindVault,
			assertConfig: func(t *testing.T, cfg secretstore.Config) {
				require.NotNil(t, cfg)
				assert.Equal(t, "vault_prod", cfg.Name())
				assert.Equal(t, secretstore.KindVault, cfg.Kind())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mgr, _ := newDyncfgSecretStoreTestManager()
			cfg, err := mgr.secretStoreConfigFromPayload(dyncfg.NewFunction(tc.fn), tc.name, tc.kind)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			if tc.assertConfig != nil {
				tc.assertConfig(t, cfg)
			}
		})
	}
}

func TestNew_InitializesSecretStoreController(t *testing.T) {
	tests := map[string]struct{}{
		"new manager initializes secretstore controller and service": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			mgr := New(Config{PluginName: testPluginName})

			require.NotNil(t, mgr.secretsCtl)
			require.NotNil(t, mgr.secretsCtl.Service())
			assert.Equal(t, mgr.secretsCtl.Prefix(), mgr.dyncfgSecretStorePrefixValue())
		})
	}
}

func newDyncfgSecretStoreTestManager() (*Manager, *bytes.Buffer) {
	return newDyncfgSecretStoreTestManagerWithService(nil)
}

func newDyncfgSecretStoreTestManagerWithService(secretStoreSvc secretstore.Service) (*Manager, *bytes.Buffer) {
	var out bytes.Buffer

	mgr := New(Config{
		PluginName:         testPluginName,
		SecretStoreService: secretStoreSvc,
	})
	mgr.ctx = context.Background()
	mgr.modules = prepareMockRegistry()
	mgr.fileStatus = newFileStatus()
	mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&out))))

	return mgr, &out
}

func mustSecretStoreService(t *testing.T, mgr *Manager) secretstore.Service {
	t.Helper()
	require.NotNil(t, mgr.secretsCtl)
	svc := mgr.secretsCtl.Service()
	require.NotNil(t, svc)
	return svc
}

func testVaultConfig() map[string]any {
	return map[string]any{
		"mode": "token",
		"mode_token": map[string]any{
			"token": "vault-token",
		},
		"addr": "https://vault.example",
	}
}

func testVaultConfigTokenFile() map[string]any {
	return map[string]any{
		"mode": "token_file",
		"mode_token_file": map[string]any{
			"path": "/var/lib/netdata/vault.token",
		},
		"addr": "https://vault.example",
	}
}

func newSecretStoreFromConfig(t *testing.T, svc secretstore.Service, kind secretstore.StoreKind, name string, cfg map[string]any) secretstore.Config {
	t.Helper()
	_ = svc
	return newSecretStoreConfigWithSource(t, kind, name, cfg, confgroup.TypeDyncfg, confgroup.TypeDyncfg)
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

func seedSecretStore(t *testing.T, mgr *Manager, kind secretstore.StoreKind, name string, cfg map[string]any, status dyncfg.Status) secretstore.Config {
	t.Helper()

	switch status {
	case dyncfg.StatusAccepted:
		raw := newSecretStoreFromConfig(t, mustSecretStoreService(t, mgr), kind, name, cfg)
		entry, changed, err := mgr.rememberSecretStoreConfig(raw)
		require.NoError(t, err)
		require.True(t, changed)
		require.NotNil(t, entry)
		return entry.Cfg
	case dyncfg.StatusRunning, dyncfg.StatusFailed:
		fn := dyncfg.NewFunction(functions.Function{
			UID:         "seed-" + string(kind) + "-" + name + "-" + status.String(),
			ContentType: "application/json",
			Payload:     mustJSON(t, cfg),
			Args: []string{
				mgr.dyncfgSecretStoreTemplateID(kind),
				string(dyncfg.CommandAdd),
				name,
			},
		})
		mgr.dyncfgSecretStoreSeqExec(fn)

		entry, ok := mgr.lookupSecretStoreEntry(secretstore.StoreKey(kind, name))
		require.True(t, ok)
		require.Equal(t, status, entry.Status)
		return entry.Cfg
	default:
		t.Fatalf("unsupported secretstore seed status %q", status)
		return nil
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	bs, err := json.Marshal(v)
	require.NoError(t, err)
	return bs
}

func mustDecodeFunctionPayload(t *testing.T, output, uid string, dst any) {
	t.Helper()

	re := regexp.MustCompile(`(?s)FUNCTION_RESULT_BEGIN ` + regexp.QuoteMeta(uid) + ` [^\n]+\n(.*?)\nFUNCTION_RESULT_END`)
	match := re.FindStringSubmatch(output)
	require.Len(t, match, 2, "function result for uid '%s' not found in output:\n%s", uid, output)
	require.NoError(t, json.Unmarshal([]byte(match[1]), dst))
}
