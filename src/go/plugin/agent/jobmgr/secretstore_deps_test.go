// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"bytes"
	"context"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func TestExtractSecretStoreKeys(t *testing.T) {
	tests := map[string]struct {
		cfg  map[string]any
		want []string
	}{
		"single store reference": {
			cfg: map[string]any{
				"password": "${store:aws-sm:aws_prod:db/password}",
			},
			want: []string{"aws-sm:aws_prod"},
		},
		"multiple refs with non-store refs ignored": {
			cfg: map[string]any{
				"dsn": "postgres://${env:USER}:${store:vault:vault_prod:secret/data/db#password}@host/${store:aws-sm:aws_prod:db#pw}",
			},
			want: []string{"aws-sm:aws_prod", "vault:vault_prod"},
		},
		"nested structures and internal keys": {
			cfg: map[string]any{
				"__source__": "${store:vault:ignored:foo}",
				"outer": map[string]any{
					"items": []any{
						"${store:gcp-sm:gcp_prod:project/secret/latest}",
						map[any]any{"x": "${store:azure-kv:az_prod:vault/secret}"},
					},
				},
			},
			want: []string{"azure-kv:az_prod", "gcp-sm:gcp_prod"},
		},
		"invalid store refs are ignored": {
			cfg: map[string]any{
				"a": "${store::bad:secret}",
				"b": "${store:bad-id:thing:path}",
				"c": "${store:vault:good_1:path}",
			},
			want: []string{"vault:good_1"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := prepareUserCfg("mod", "job")
			maps.Copy(cfg, tc.cfg)
			got := extractSecretStoreKeys(cfg)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSecretStoreDepsImpacted(t *testing.T) {
	tests := map[string]struct {
		run         func(d *secretStoreDeps)
		storeKey    string
		wantExposed []secretstore.JobRef
		wantRunning []secretstore.JobRef
	}{
		"set active and running": {
			run: func(d *secretStoreDeps) {
				d.SetActiveJobStores("mysql_prod", "mysql:prod", []string{"vault:vault_prod"})
				d.setRunning("mysql_prod", true)
			},
			storeKey: "vault:vault_prod",
			wantExposed: []secretstore.JobRef{
				{ID: "mysql_prod", Display: "mysql:prod"},
			},
			wantRunning: []secretstore.JobRef{
				{ID: "mysql_prod", Display: "mysql:prod"},
			},
		},
		"replace active stores keeps running alignment": {
			run: func(d *secretStoreDeps) {
				d.SetActiveJobStores("mysql_prod", "mysql:prod", []string{"vault:vault_prod"})
				d.setRunning("mysql_prod", true)
				d.SetActiveJobStores("mysql_prod", "mysql:prod", []string{"aws-sm:aws_prod"})
			},
			storeKey: "aws-sm:aws_prod",
			wantExposed: []secretstore.JobRef{
				{ID: "mysql_prod", Display: "mysql:prod"},
			},
			wantRunning: []secretstore.JobRef{
				{ID: "mysql_prod", Display: "mysql:prod"},
			},
		},
		"remove active job clears impacted": {
			run: func(d *secretStoreDeps) {
				d.SetActiveJobStores("mysql_prod", "mysql:prod", []string{"vault:vault_prod"})
				d.setRunning("mysql_prod", true)
				d.RemoveActiveJob("mysql_prod")
			},
			storeKey:    "vault:vault_prod",
			wantExposed: nil,
			wantRunning: nil,
		},
		"non-running job appears only in exposed": {
			run: func(d *secretStoreDeps) {
				d.SetActiveJobStores("redis_prod", "redis:prod", []string{"gcp-sm:gcp_prod"})
				d.setRunning("redis_prod", false)
			},
			storeKey: "gcp-sm:gcp_prod",
			wantExposed: []secretstore.JobRef{
				{ID: "redis_prod", Display: "redis:prod"},
			},
			wantRunning: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			deps := newSecretStoreDeps()
			tc.run(deps)

			exposed, running := deps.Impacted(tc.storeKey)
			assert.Equal(t, tc.wantExposed, exposed)
			assert.Equal(t, tc.wantRunning, running)
		})
	}
}

func TestDyncfgTestDoesNotMutateSecretStoreDeps(t *testing.T) {
	tests := map[string]struct{}{
		"command test leaves dependency index unchanged": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			mgr := newCollectorTestManager()
			mgr.ctx = context.Background()
			mgr.SetDyncfgResponder(dyncfg.NewResponder(netdataapi.New(safewriter.New(&buf))))

			cfg := prepareDyncfgCfg("success", "job")
			cfg["password"] = "${store:vault:vault_prod:secret/data/mysql#password}"
			mgr.syncSecretStoreDepsForConfig(cfg)
			mgr.secretStoreDeps.setRunning(cfg.FullName(), true)

			beforeExposed, beforeRunning := mgr.secretStoreDeps.Impacted("vault:vault_prod")
			require.Len(t, beforeExposed, 1)
			require.Len(t, beforeRunning, 1)

			fn := dyncfg.NewFunction(functions.Function{
				UID:         "secretstore-deps-test",
				ContentType: "application/json",
				Payload:     mustMarshalCollectorConfigPayload(t, prepareDyncfgCfg("success", "job")),
				Args:        []string{mgr.dyncfgModID("success"), string(dyncfg.CommandTest), "job"},
			})

			mgr.dyncfgCollectorSeqExec(fn)
			mgr.cmdTestWG.Wait()

			afterExposed, afterRunning := mgr.secretStoreDeps.Impacted("vault:vault_prod")
			assert.Equal(t, beforeExposed, afterExposed)
			assert.Equal(t, beforeRunning, afterRunning)
		})
	}
}

func TestSecretStoreDepsNoStateLeakOnRemoveThenStop(t *testing.T) {
	tests := map[string]struct{}{
		"setRunning false on missing state is no-op": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			deps := newSecretStoreDeps()

			deps.SetActiveJobStores("mysql_prod", "mysql:prod", []string{"vault:vault_prod"})
			deps.setRunning("mysql_prod", true)
			deps.RemoveActiveJob("mysql_prod")
			deps.setRunning("mysql_prod", false)

			assert.Empty(t, deps.jobs)
			exposed, running := deps.Impacted("vault:vault_prod")
			assert.Empty(t, exposed)
			assert.Empty(t, running)
		})
	}
}
