// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

func TestPublishedStoreResolve_LogsDetailedResolution(t *testing.T) {
	s := &publishedStore{
		provider: &provider{
			httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data":{"password":"secret-value"}}`)),
					Header:     make(http.Header),
				}, nil
			})},
			httpClientInsecure: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data":{"password":"secret-value"}}`)),
					Header:     make(http.Header),
				}, nil
			})},
		},
		mode:       "token",
		tokenValue: "vault-token",
		addr:       "https://vault.example",
	}

	out := captureLoggerOutput(t, func(log *logger.Logger) {
		ctx := logger.ContextWithLogger(context.Background(), log)
		value, err := s.Resolve(ctx, secretstore.ResolveRequest{
			StoreKey: "vault:vault_prod",
			Operand:  "secret/data/mysql#password",
			Original: "${store:vault:vault_prod:secret/data/mysql#password}",
		})
		require.NoError(t, err)
		assert.Equal(t, "secret-value", value)
	})

	assert.Contains(t, out, "resolved secret via vault secretstore 'vault:vault_prod' path 'secret/data/mysql' key 'password'")
	assert.NotContains(t, out, "secret-value")
}

func captureLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	fn(logger.NewWithWriter(&buf))
	return buf.String()
}
