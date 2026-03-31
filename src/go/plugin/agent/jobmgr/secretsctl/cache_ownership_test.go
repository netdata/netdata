// SPDX-License-Identifier: GPL-3.0-or-later

package secretsctl

import (
	"bytes"
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestControllerRememberDiscoveredConfig_InvalidDoesNotEnterCaches(t *testing.T) {
	ctl, _, _ := newVaultControllerTestSubject()

	raw := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{}, "/etc/netdata/secretstores.yaml", confgroup.TypeUser)

	entry, changed, err := ctl.RememberDiscoveredConfig(raw)
	require.Error(t, err)
	assert.False(t, changed)
	assert.Equal(t, Entry{}, entry)
	assert.Zero(t, ctl.seen.Count())
	assert.Zero(t, ctl.exposed.Count())
}

func TestControllerRememberDiscoveredConfig_PreservesUnknownFields(t *testing.T) {
	ctl, _, _ := newVaultControllerTestSubject()

	cfg := vaultModeTokenConfig()
	cfg["ui_note"] = "kept"
	cfg["mode_token"].(map[string]any)["extra"] = "kept"

	raw := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", cfg, "/etc/netdata/secretstores.yaml", confgroup.TypeUser)
	entry, changed, err := ctl.RememberDiscoveredConfig(raw)
	require.NoError(t, err)
	require.True(t, changed)

	assert.Equal(t, "kept", entry.Cfg["ui_note"])

	modeToken := entry.Cfg["mode_token"].(map[string]any)
	assert.Equal(t, "kept", modeToken["extra"])
}

func TestControllerSeqExec_FileDefinedConfigBecomesDyncfgOverride(t *testing.T) {
	ctl, out, _ := newVaultControllerTestSubject()
	key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")

	fileCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", vaultModeTokenConfig(), "file=/etc/netdata/go.d/ss/vault.conf", confgroup.TypeUser)
	require.NoError(t, ctl.Service().Add(context.Background(), fileCfg))
	ctl.seen.Add(fileCfg)
	ctl.exposed.Add(&dyncfg.Entry[secretstore.Config]{
		Cfg:    fileCfg,
		Status: dyncfg.StatusRunning,
	})

	updateFn := dyncfg.NewFunction(functions.Function{
		UID:         "ss-file-update",
		ContentType: "application/json",
		Payload:     mustJSON(t, vaultModeTokenFileConfig()),
		Args: []string{
			ctl.configID(key),
			string(dyncfg.CommandUpdate),
		},
	})
	ctl.SeqExec(updateFn)

	var updateResp map[string]any
	mustDecodeFunctionPayload(t, out.String(), "ss-file-update", &updateResp)
	assert.Equal(t, float64(200), updateResp["status"])

	entry, ok := ctl.Lookup(key)
	require.True(t, ok)
	assert.Equal(t, dyncfg.StatusRunning, entry.Status)
	assert.Equal(t, confgroup.TypeDyncfg, entry.Cfg.SourceType())
	assert.Equal(t, confgroup.TypeDyncfg, entry.Cfg.Source())
	assert.Equal(t, "token_file", entry.Cfg["mode"])

	removeFn := dyncfg.NewFunction(functions.Function{
		UID:  "ss-file-remove",
		Args: []string{ctl.configID(key), string(dyncfg.CommandRemove)},
	})
	ctl.SeqExec(removeFn)

	var removeResp map[string]any
	mustDecodeFunctionPayload(t, out.String(), "ss-file-remove", &removeResp)
	assert.Equal(t, float64(200), removeResp["status"])

	_, ok = ctl.Lookup(key)
	assert.False(t, ok)

	seenUser, ok := ctl.seen.LookupByUID(fileCfg.UID())
	require.True(t, ok)
	assert.Equal(t, fileCfg.UID(), seenUser.UID())
	assert.Equal(t, 1, ctl.seen.Count())
	assert.Zero(t, ctl.exposed.Count())
}

func TestControllerRemoveDiscoveredConfig_DoesNotRevealLowerPrioritySeenConfig(t *testing.T) {
	ctl, _, _ := newVaultControllerTestSubject()
	key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")

	userCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", vaultModeTokenConfig(), "/etc/netdata/secretstores.yaml", confgroup.TypeUser)
	entry, changed, err := ctl.RememberDiscoveredConfig(userCfg)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, userCfg.UID(), entry.Cfg.UID())

	dyncfgCfg := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", vaultModeTokenFileConfig(), confgroup.TypeDyncfg, confgroup.TypeDyncfg)
	entry, changed, err = ctl.RememberDiscoveredConfig(dyncfgCfg)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, dyncfgCfg.UID(), entry.Cfg.UID())

	removed, ok := ctl.RemoveDiscoveredConfig(dyncfgCfg)
	require.True(t, ok)
	assert.Equal(t, dyncfgCfg.UID(), removed.Cfg.UID())

	_, ok = ctl.Lookup(key)
	assert.False(t, ok)

	seenUser, ok := ctl.seen.LookupByUID(userCfg.UID())
	require.True(t, ok)
	assert.Equal(t, userCfg.UID(), seenUser.UID())
	assert.Equal(t, 1, ctl.seen.Count())
	assert.Zero(t, ctl.exposed.Count())
}

func TestControllerSeqExec_RemoveFailedUnpublishedStoreDoesNotRestartDependents(t *testing.T) {
	ctl, out, seams := newVaultControllerTestSubject()
	key := secretstore.StoreKey(secretstore.KindVault, "vault_prod")

	raw := newSecretStoreConfigWithSource(t, secretstore.KindVault, "vault_prod", map[string]any{"mode": "token"}, confgroup.TypeDyncfg, confgroup.TypeDyncfg)
	ctl.seen.Add(raw)
	ctl.exposed.Add(&dyncfg.Entry[secretstore.Config]{
		Cfg:    raw,
		Status: dyncfg.StatusFailed,
	})

	removeFn := dyncfg.NewFunction(functions.Function{
		UID:  "ss-remove-no-restart",
		Args: []string{ctl.configID(key), string(dyncfg.CommandRemove)},
	})
	ctl.SeqExec(removeFn)

	var resp map[string]any
	mustDecodeFunctionPayload(t, out.String(), "ss-remove-no-restart", &resp)
	assert.Equal(t, float64(200), resp["status"])
	assert.Equal(t, "", resp["message"])
	assert.Empty(t, seams.restartCalls)
}

func newVaultControllerTestSubject() (*Controller, *bytes.Buffer, *controllerSeams) {
	return newControllerTestSubjectWithOptions(Options{
		Service: secretstore.NewService(backends.Creators()...),
	})
}

func vaultModeTokenConfig() map[string]any {
	return map[string]any{
		"mode": "token",
		"mode_token": map[string]any{
			"token": "vault-token",
		},
		"addr": "https://vault.example",
	}
}

func vaultModeTokenFileConfig() map[string]any {
	return map[string]any{
		"mode": "token_file",
		"mode_token_file": map[string]any{
			"path": "/var/lib/netdata/vault.token",
		},
		"addr": "https://vault.example",
	}
}
