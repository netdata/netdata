// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/safefile"
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
		runtime: &runtime{
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

func TestPublishedStoreResolveUsesConfiguredRequestPath(t *testing.T) {
	tests := map[string]struct {
		skipVerify bool
		namespace  string
	}{
		"secure client without namespace": {},
		"insecure client with namespace": {
			skipVerify: true,
			namespace:  "team-a",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var request *http.Request
			selected := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				request = req.Clone(req.Context())
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"data":{"password":"secret-value"}}`)),
					Header:     make(http.Header),
				}, nil
			})
			unexpected := roundTripFunc(func(*http.Request) (*http.Response, error) {
				require.FailNow(t, "test failed", "unexpected Vault HTTP client was selected")
				return nil, nil
			})
			secure, insecure := http.RoundTripper(selected), http.RoundTripper(unexpected)
			if tc.skipVerify {
				secure, insecure = insecure, secure
			}
			s := &publishedStore{
				runtime: &runtime{
					httpClient:         &http.Client{Transport: secure},
					httpClientInsecure: &http.Client{Transport: insecure},
				},
				mode:           "token",
				tokenValue:     "synthetic-token",
				addr:           "https://vault.example/",
				namespaceValue: tc.namespace,
				tlsSkipVerify:  tc.skipVerify,
			}

			value, err := s.Resolve(t.Context(), secretstore.ResolveRequest{
				StoreKey: "vault:vault_prod",
				Operand:  "secret/data/mysql#password",
				Original: "${store:vault:vault_prod:secret/data/mysql#password}",
			})

			require.NoError(t, err)
			assert.Equal(t, "secret-value", value)
			require.NotNil(t, request)
			assert.Equal(t, http.MethodGet, request.Method)
			assert.Equal(t, "https://vault.example/v1/secret/data/mysql", request.URL.String())
			assert.Equal(t, "synthetic-token", request.Header.Get("X-Vault-Token"))
			assert.Equal(t, tc.namespace, request.Header.Get("X-Vault-Namespace"))
		})
	}
}

func TestPublishedStoreResolveUsesSafeTokenFile(t *testing.T) {
	tests := map[string]struct {
		write       func(t *testing.T, path string)
		wantToken   string
		wantErr     error
		wantRequest bool
	}{
		"reads regular token file": {
			write: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(path, []byte("  file-token\n"), 0o600))
			},
			wantToken:   "file-token",
			wantRequest: true,
		},
		"rejects oversized token file": {
			write: func(t *testing.T, path string) {
				require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("x"), int(safefile.MaxSize)+1), 0o600))
			},
			wantErr: safefile.ErrTooLarge,
		},
		"rejects non-regular token path": {
			write: func(t *testing.T, path string) {
				require.NoError(t, os.Mkdir(path, 0o700))
			},
			wantErr: safefile.ErrNotRegular,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vault.token")
			tc.write(t, path)
			var gotToken string
			var requested bool
			s := &publishedStore{
				runtime: &runtime{
					httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						requested = true
						gotToken = req.Header.Get("X-Vault-Token")
						return &http.Response{
							StatusCode: http.StatusOK,
							Body: io.NopCloser(
								bytes.NewBufferString(`{"data":{"password":"secret-value"}}`),
							),
							Header: make(http.Header),
						}, nil
					})},
					httpClientInsecure: &http.Client{},
				},
				mode:          "token_file",
				tokenFilePath: path,
				addr:          "https://vault.example",
			}

			_, err := s.Resolve(t.Context(), secretstore.ResolveRequest{
				StoreKey: "vault:vault_prod",
				Operand:  "secret/data/mysql#password",
				Original: "${store:vault:vault_prod:secret/data/mysql#password}",
			})

			assert.Equal(t, tc.wantRequest, requested)
			assert.Equal(t, tc.wantToken, gotToken)
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
				require.ErrorIs(t, err, safefile.ErrFile)
			}
		})
	}
}

func captureLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	fn(logger.NewWithWriter(&buf))
	return buf.String()
}
