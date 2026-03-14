// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFileConfigs(t *testing.T) {
	t.Run("loads user and stock roots with filename defaults", func(t *testing.T) {
		base := t.TempDir()
		userRoot := filepath.Join(base, "etc", "netdata", "go.d")
		stockRoot := filepath.Join(base, "usr", "lib", "netdata", "conf.d", "go.d")

		mustWriteSecretStoreConfigFile(t, filepath.Join(userRoot, "ss", "vault.conf"), `
jobs:
  - name: vault_prod
    mode: token
    mode_token:
      token: vault-token
    addr: https://vault.example
`)
		mustWriteSecretStoreConfigFile(t, filepath.Join(userRoot, "ss", "azure-kv.yaml"), `
jobs:
  - name: azure_prod
    mode: managed_identity
`)
		mustWriteSecretStoreConfigFile(t, filepath.Join(stockRoot, "ss", "aws-sm.conf"), `
jobs:
  - name: aws_prod
    auth_mode: env
    region: us-east-1
`)
		mustWriteSecretStoreConfigFile(t, filepath.Join(stockRoot, "ss", "gcp-sm.yml"), `
jobs:
  - name: gcp_prod
    mode: metadata
`)

		cfgs, errs := secretstore.LoadFileConfigs([]string{userRoot, stockRoot})
		require.Empty(t, errs)
		require.Len(t, cfgs, 4)

		assert.Equal(t, secretstore.KindAzureKV, cfgs[0].Kind())
		assert.Equal(t, confgroup.TypeUser, cfgs[0].SourceType())
		assert.Contains(t, cfgs[0].Source(), "file="+filepath.Join(userRoot, "ss", "azure-kv.yaml"))

		assert.Equal(t, secretstore.KindVault, cfgs[1].Kind())
		assert.Equal(t, confgroup.TypeUser, cfgs[1].SourceType())

		assert.Equal(t, secretstore.KindAWSSM, cfgs[2].Kind())
		assert.Equal(t, confgroup.TypeStock, cfgs[2].SourceType())

		assert.Equal(t, secretstore.KindGCPSM, cfgs[3].Kind())
		assert.Equal(t, confgroup.TypeStock, cfgs[3].SourceType())
	})

	t.Run("keeps explicit kind and allows unknown file stem", func(t *testing.T) {
		base := t.TempDir()
		userRoot := filepath.Join(base, "etc", "netdata", "go.d")

		mustWriteSecretStoreConfigFile(t, filepath.Join(userRoot, "ss", "custom.conf"), `
jobs:
  - name: explicit_kind
    kind: vault
    mode: token
    mode_token:
      token: vault-token
    addr: https://vault.example
  - name: inferred_kind
    mode: token
    mode_token:
      token: custom-token
    addr: https://vault.example
`)

		cfgs, errs := secretstore.LoadFileConfigs([]string{userRoot})
		require.Empty(t, errs)
		require.Len(t, cfgs, 2)
		assert.Equal(t, secretstore.KindVault, cfgs[0].Kind())
		assert.Equal(t, secretstore.StoreKind("custom"), cfgs[1].Kind())
	})

	t.Run("skips malformed files and non-keyable jobs", func(t *testing.T) {
		base := t.TempDir()
		userRoot := filepath.Join(base, "etc", "netdata", "go.d")

		mustWriteSecretStoreConfigFile(t, filepath.Join(userRoot, "ss", "vault.conf"), `
jobs:
  - mode: token
    mode_token:
      token: missing-name
    addr: https://vault.example
  - name: vault_prod
    mode: token
    mode_token:
      token: vault-token
    addr: https://vault.example
`)
		mustWriteSecretStoreConfigFile(t, filepath.Join(userRoot, "ss", "gcp-sm.conf"), `
jobs:
  - name: gcp_prod
    mode: [
`)

		cfgs, errs := secretstore.LoadFileConfigs([]string{userRoot})
		require.Len(t, cfgs, 1)
		assert.Equal(t, "vault_prod", cfgs[0].Name())
		assert.Len(t, errs, 2)
		var messages []string
		for _, err := range errs {
			messages = append(messages, err.Error())
		}
		joined := strings.Join(messages, "\n")
		assert.Contains(t, joined, "store name is required")
		assert.Contains(t, joined, "secretstore file config")
	})
}

func mustWriteSecretStoreConfigFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
