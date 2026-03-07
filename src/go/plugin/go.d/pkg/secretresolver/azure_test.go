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

func TestAzureGetTokenClientCredentials_WithEndpointOverride(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "test-client-id", r.FormValue("client_id"))
		assert.Equal(t, "test-client-secret", r.FormValue("client_secret"))
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))

		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "test-access-token",
		})
	}))
	defer tokenSrv.Close()

	origClient := azureHTTPClient
	azureHTTPClient = tokenSrv.Client()
	defer func() { azureHTTPClient = origClient }()

	origLoginEndpoint := azureLoginEndpointOverride
	azureLoginEndpointOverride = tokenSrv.URL
	defer func() { azureLoginEndpointOverride = origLoginEndpoint }()

	token, err := azureGetTokenClientCredentials("test-tenant", "test-client-id", "test-client-secret")
	require.NoError(t, err)
	assert.Equal(t, "test-access-token", token)
}

func TestResolveAzureKV_InvalidRef(t *testing.T) {
	cfg := map[string]any{
		"password": "${azure-kv:just-vault-name}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'vault-name/secret-name'")
}

func TestResolveAzureKV_EmptyVaultName(t *testing.T) {
	cfg := map[string]any{
		"password": "${azure-kv:/secret-name}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'vault-name/secret-name'")
}

func TestResolveAzureKV_EmptySecretName(t *testing.T) {
	cfg := map[string]any{
		"password": "${azure-kv:vault-name/}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'vault-name/secret-name'")
}

func TestResolveAzureKV_UnsafeVaultName(t *testing.T) {
	cfg := map[string]any{
		"password": "${azure-kv:evil.com#/secret-name}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid vault name")
}

func TestResolveAzureKV_UnsafeSecretName(t *testing.T) {
	cfg := map[string]any{
		"password": "${azure-kv:my-vault/secret?inject=true}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid secret name")
}

func TestAzureGetTokenClientCredentials_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
		assert.Equal(t, "https://vault.azure.net/.default", r.FormValue("scope"))
		assert.Equal(t, "cid", r.FormValue("client_id"))
		assert.Equal(t, "csecret", r.FormValue("client_secret"))

		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "tok123",
		})
	}))
	defer srv.Close()

	origLoginEndpoint := azureLoginEndpointOverride
	azureLoginEndpointOverride = srv.URL
	defer func() { azureLoginEndpointOverride = origLoginEndpoint }()

	origClient := azureHTTPClient
	azureHTTPClient = srv.Client()
	defer func() { azureHTTPClient = origClient }()

	token, err := azureGetTokenClientCredentials("test-tenant", "cid", "csecret")
	require.NoError(t, err)
	assert.Equal(t, "tok123", token)
}

func TestAzureGetTokenClientCredentials_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	origLoginEndpoint := azureLoginEndpointOverride
	azureLoginEndpointOverride = srv.URL
	defer func() { azureLoginEndpointOverride = origLoginEndpoint }()

	origClient := azureHTTPClient
	azureHTTPClient = srv.Client()
	defer func() { azureHTTPClient = origClient }()

	_, err := azureGetTokenClientCredentials("test-tenant", "cid", "csecret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 401")
}

func TestAzureGetAccessToken_NoCredentials(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "")
	t.Setenv("AZURE_CLIENT_ID", "")
	t.Setenv("AZURE_CLIENT_SECRET", "")

	// Will try managed identity IMDS which will fail outside Azure.
	_, err := azureGetAccessToken()
	assert.Error(t, err)
}
