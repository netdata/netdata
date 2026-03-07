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

func TestResolveVault_KVv2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/secret/data/myapp", r.URL.Path)
		assert.Equal(t, "test-token", r.Header.Get("X-Vault-Token"))

		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"password": "s3cret",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	// Override the HTTP client to use the test server's client.
	origClient := vaultHTTPClient
	vaultHTTPClient = srv.Client()
	defer func() { vaultHTTPClient = origClient }()

	cfg := map[string]any{
		"password": "${vault:secret/data/myapp#password}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "s3cret", cfg["password"])
}

func TestResolveVault_KVv1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"password": "v1secret",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	origClient := vaultHTTPClient
	vaultHTTPClient = srv.Client()
	defer func() { vaultHTTPClient = origClient }()

	cfg := map[string]any{
		"password": "${vault:secret/myapp#password}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "v1secret", cfg["password"])
}

func TestResolveVault_Namespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "engineering", r.Header.Get("X-Vault-Namespace"))

		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"api_key": "nskey",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")
	t.Setenv("VAULT_NAMESPACE", "engineering")

	origClient := vaultHTTPClient
	vaultHTTPClient = srv.Client()
	defer func() { vaultHTTPClient = origClient }()

	cfg := map[string]any{
		"key": "${vault:secret/data/app#api_key}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "nskey", cfg["key"])
}

func TestResolveVault_MissingKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"data": map[string]any{
				"data": map[string]any{
					"other": "value",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "test-token")

	origClient := vaultHTTPClient
	vaultHTTPClient = srv.Client()
	defer func() { vaultHTTPClient = origClient }()

	cfg := map[string]any{
		"password": "${vault:secret/data/myapp#nonexistent}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "key 'nonexistent' not found")
}

func TestResolveVault_NoAddr(t *testing.T) {
	t.Setenv("VAULT_ADDR", "")
	t.Setenv("VAULT_TOKEN", "test-token")

	cfg := map[string]any{
		"password": "${vault:secret/data/myapp#password}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VAULT_ADDR")
}

func TestResolveVault_NoToken(t *testing.T) {
	t.Setenv("VAULT_ADDR", "http://localhost:8200")
	t.Setenv("VAULT_TOKEN", "")
	t.Setenv("VAULT_TOKEN_FILE", "/nonexistent/token/file")

	cfg := map[string]any{
		"password": "${vault:secret/data/myapp#password}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot read token file")
}

func TestResolveVault_PathTraversal(t *testing.T) {
	cfg := map[string]any{
		"password": "${vault:secret/../sys/seal#key}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestResolveVault_QueryInjection(t *testing.T) {
	cfg := map[string]any{
		"password": "${vault:secret/data/myapp?list=true#key}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid characters")
}

func TestResolveVault_MissingHashKey(t *testing.T) {
	t.Setenv("VAULT_ADDR", "http://localhost:8200")
	t.Setenv("VAULT_TOKEN", "test-token")

	cfg := map[string]any{
		"password": "${vault:secret/data/myapp}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'path#key'")
}

func TestResolveVault_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"errors":["permission denied"]}`))
	}))
	defer srv.Close()

	t.Setenv("VAULT_ADDR", srv.URL)
	t.Setenv("VAULT_TOKEN", "bad-token")

	origClient := vaultHTTPClient
	vaultHTTPClient = srv.Client()
	defer func() { vaultHTTPClient = origClient }()

	cfg := map[string]any{
		"password": "${vault:secret/data/myapp#password}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
}

func TestTruncateBody(t *testing.T) {
	short := "short message"
	assert.Equal(t, short, truncateBody([]byte(short)))

	long := make([]byte, 300)
	for i := range long {
		long[i] = 'a'
	}
	result := truncateBody(long)
	assert.Len(t, result, 203) // 200 + "..."
	assert.True(t, len(result) <= 203)
}
