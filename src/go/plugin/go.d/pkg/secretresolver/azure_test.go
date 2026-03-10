// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAzureKV_Validation(t *testing.T) {
	tests := map[string]struct {
		ref             string
		wantErrContains string
	}{
		"invalid ref": {
			ref:             "${azure-kv:just-vault-name}",
			wantErrContains: "must be in format 'vault-name/secret-name'",
		},
		"empty vault name": {
			ref:             "${azure-kv:/secret-name}",
			wantErrContains: "must be in format 'vault-name/secret-name'",
		},
		"empty secret name": {
			ref:             "${azure-kv:vault-name/}",
			wantErrContains: "must be in format 'vault-name/secret-name'",
		},
		"unsafe vault name": {
			ref:             "${azure-kv:evil.com#/secret-name}",
			wantErrContains: "invalid vault name",
		},
		"unsafe secret name": {
			ref:             "${azure-kv:my-vault/secret?inject=true}",
			wantErrContains: "invalid secret name",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := map[string]any{"password": tc.ref}
			err := Resolve(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}
}

func TestAzureGetTokenClientCredentials(t *testing.T) {
	tests := map[string]struct {
		tenantID        string
		clientID        string
		clientSecret    string
		handler         http.HandlerFunc
		wantToken       string
		wantErrContains string
	}{
		"with endpoint override": {
			tenantID:     "test-tenant",
			clientID:     "test-client-id",
			clientSecret: "test-client-secret",
			handler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				require.NoError(t, r.ParseForm())
				assert.Equal(t, "test-client-id", r.FormValue("client_id"))
				assert.Equal(t, "test-client-secret", r.FormValue("client_secret"))
				assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
				require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"access_token": "test-access-token"}))
			},
			wantToken: "test-access-token",
		},
		"success": {
			tenantID:     "test-tenant",
			clientID:     "cid",
			clientSecret: "csecret",
			handler: func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, r.ParseForm())
				assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
				assert.Equal(t, "https://vault.azure.net/.default", r.FormValue("scope"))
				assert.Equal(t, "cid", r.FormValue("client_id"))
				assert.Equal(t, "csecret", r.FormValue("client_secret"))
				require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"access_token": "tok123"}))
			},
			wantToken: "tok123",
		},
		"http error": {
			tenantID:     "test-tenant",
			clientID:     "cid",
			clientSecret: "csecret",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			},
			wantErrContains: "HTTP 401",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)

			origClient := azureHTTPClient
			azureHTTPClient = srv.Client()
			t.Cleanup(func() { azureHTTPClient = origClient })

			origLoginEndpoint := azureLoginEndpointOverride
			azureLoginEndpointOverride = srv.URL
			t.Cleanup(func() { azureLoginEndpointOverride = origLoginEndpoint })

			token, err := azureGetTokenClientCredentials(tc.tenantID, tc.clientID, tc.clientSecret)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantToken, token)
		})
	}
}

func TestAzureAccessTokenPaths(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"no credentials": {
			run: func(t *testing.T) {
				t.Setenv("AZURE_TENANT_ID", "")
				t.Setenv("AZURE_CLIENT_ID", "")
				t.Setenv("AZURE_CLIENT_SECRET", "")

				_, err := azureGetAccessToken()
				assert.Error(t, err)
			},
		},
		"managed identity success": {
			run: func(t *testing.T) {
				t.Setenv("AZURE_CLIENT_ID", "user-assigned-client-id")

				origClient := azureIMDSHTTPClient
				azureIMDSHTTPClient = &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						assert.Equal(t, "GET", req.Method)
						assert.Equal(t, "169.254.169.254", req.URL.Host)
						assert.Equal(t, "/metadata/identity/oauth2/token", req.URL.Path)
						assert.Equal(t, "true", req.Header.Get("Metadata"))
						assert.Equal(t, "2018-02-01", req.URL.Query().Get("api-version"))
						assert.Equal(t, "https://vault.azure.net", req.URL.Query().Get("resource"))
						assert.Equal(t, "user-assigned-client-id", req.URL.Query().Get("client_id"))
						return newHTTPResponse(http.StatusOK, `{"access_token":"mi-token"}`), nil
					}),
				}
				t.Cleanup(func() { azureIMDSHTTPClient = origClient })

				token, err := azureGetTokenManagedIdentity()
				require.NoError(t, err)
				assert.Equal(t, "mi-token", token)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}
