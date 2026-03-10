// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVault(t *testing.T) {
	tests := map[string]struct {
		buildCfg        func(t *testing.T, resolver *Resolver) map[string]any
		wantErrContains string
		assertCfg       func(t *testing.T, cfg map[string]any)
	}{
		"kv v2": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "/v1/secret/data/myapp", r.URL.Path)
					assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

					resp := map[string]any{
						"data": map[string]any{
							"data": map[string]any{"password": "s3cret"},
						},
					}
					require.NoError(t, json.NewEncoder(w).Encode(resp))
				}))
				t.Cleanup(srv.Close)

				t.Setenv("VAULT_ADDR", srv.URL)
				t.Setenv("VAULT_TOKEN", "test-token")
				resolver.vaultHTTPClient = srv.Client()

				return map[string]any{"password": "${vault:secret/data/myapp#password}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "s3cret", cfg["password"])
			},
		},
		"kv v1": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := map[string]any{
						"data": map[string]any{"password": "v1secret"},
					}
					require.NoError(t, json.NewEncoder(w).Encode(resp))
				}))
				t.Cleanup(srv.Close)

				t.Setenv("VAULT_ADDR", srv.URL)
				t.Setenv("VAULT_TOKEN", "test-token")
				resolver.vaultHTTPClient = srv.Client()

				return map[string]any{"password": "${vault:secret/myapp#password}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "v1secret", cfg["password"])
			},
		},
		"namespace": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "engineering", r.Header.Get("X-Vault-Namespace"))

					resp := map[string]any{
						"data": map[string]any{
							"data": map[string]any{"api_key": "nskey"},
						},
					}
					require.NoError(t, json.NewEncoder(w).Encode(resp))
				}))
				t.Cleanup(srv.Close)

				t.Setenv("VAULT_ADDR", srv.URL)
				t.Setenv("VAULT_TOKEN", "test-token")
				t.Setenv("VAULT_NAMESPACE", "engineering")
				resolver.vaultHTTPClient = srv.Client()

				return map[string]any{"key": "${vault:secret/data/app#api_key}"}
			},
			assertCfg: func(t *testing.T, cfg map[string]any) {
				assert.Equal(t, "nskey", cfg["key"])
			},
		},
		"missing key": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					resp := map[string]any{
						"data": map[string]any{
							"data": map[string]any{"other": "value"},
						},
					}
					require.NoError(t, json.NewEncoder(w).Encode(resp))
				}))
				t.Cleanup(srv.Close)

				t.Setenv("VAULT_ADDR", srv.URL)
				t.Setenv("VAULT_TOKEN", "test-token")
				resolver.vaultHTTPClient = srv.Client()

				return map[string]any{"password": "${vault:secret/data/myapp#nonexistent}"}
			},
			wantErrContains: "key 'nonexistent' not found",
		},
		"missing vault addr": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				t.Setenv("VAULT_ADDR", "")
				t.Setenv("VAULT_TOKEN", "test-token")
				return map[string]any{"password": "${vault:secret/data/myapp#password}"}
			},
			wantErrContains: "VAULT_ADDR",
		},
		"missing token": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				t.Setenv("VAULT_ADDR", "http://localhost:8200")
				t.Setenv("VAULT_TOKEN", "")
				t.Setenv("VAULT_TOKEN_FILE", "/nonexistent/token/file")
				return map[string]any{"password": "${vault:secret/data/myapp#password}"}
			},
			wantErrContains: "cannot read token file",
		},
		"path traversal": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				return map[string]any{"password": "${vault:secret/../sys/seal#key}"}
			},
			wantErrContains: "invalid characters",
		},
		"query injection": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				return map[string]any{"password": "${vault:secret/data/myapp?list=true#key}"}
			},
			wantErrContains: "invalid characters",
		},
		"missing hash key": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				t.Setenv("VAULT_ADDR", "http://localhost:8200")
				t.Setenv("VAULT_TOKEN", "test-token")
				return map[string]any{"password": "${vault:secret/data/myapp}"}
			},
			wantErrContains: "must be in format 'path#key'",
		},
		"http error": {
			buildCfg: func(t *testing.T, resolver *Resolver) map[string]any {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
				}))
				t.Cleanup(srv.Close)

				t.Setenv("VAULT_ADDR", srv.URL)
				t.Setenv("VAULT_TOKEN", "bad-token")
				resolver.vaultHTTPClient = srv.Client()

				return map[string]any{"password": "${vault:secret/data/myapp#password}"}
			},
			wantErrContains: "HTTP 403",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolver := New()
			cfg := tc.buildCfg(t, resolver)
			err := resolver.Resolve(cfg)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			if tc.assertCfg != nil {
				tc.assertCfg(t, cfg)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := map[string]struct {
		body    []byte
		wantLen int
		want    string
	}{
		"short body": {
			body:    []byte("short message"),
			wantLen: len("short message"),
			want:    "short message",
		},
		"long body": {
			body:    []byte(strings.Repeat("a", 300)),
			wantLen: 203,
			want:    strings.Repeat("a", 200) + "...",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := truncateBody(tc.body)
			assert.Equal(t, tc.want, result)
			assert.Len(t, result, tc.wantLen)
		})
	}
}
