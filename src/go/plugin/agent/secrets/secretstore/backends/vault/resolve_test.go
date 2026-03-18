// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponse(t *testing.T) {
	req := secretstore.ResolveRequest{
		StoreKey:  "vault:vault_prod",
		StoreKind: secretstore.KindVault,
		StoreName: "vault_prod",
		Original:  "${store:vault:vault_prod:secret/data/mysql#password}",
	}

	const kv2Body = `{"data":{"data":{"password":"s3cr3t"},"metadata":{"created_time":"2024-01-01T00:00:00Z","deletion_time":"","destroyed":false,"version":3}}}`

	tests := map[string]struct {
		body            string
		key             string
		want            string
		wantErrContains string
	}{
		"kv1 returns requested key": {
			body: `{"data":{"password":"s3cr3t","username":"netdata"}}`,
			key:  "password",
			want: "s3cr3t",
		},
		"kv1 with top-level data key still uses kv1 lookup": {
			body: `{"data":{"data":{"nested":"value"},"password":"s3cr3t"}}`,
			key:  "password",
			want: "s3cr3t",
		},
		"kv2 returns requested nested key": {
			body: kv2Body,
			key:  "password",
			want: "s3cr3t",
		},
		"kv2 does not leak metadata envelope field": {
			body:            kv2Body,
			key:             "metadata",
			wantErrContains: "key 'metadata' not found",
		},
		"kv2 does not leak data envelope field": {
			body:            kv2Body,
			key:             "data",
			wantErrContains: "key 'data' not found",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseResponse([]byte(tc.body), tc.key, req)
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
