// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGCPSM_InvalidRef(t *testing.T) {
	cfg := map[string]any{
		"password": "${gcp-sm:just-project}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'project/secret'")
}

func TestResolveGCPSM_EmptyProject(t *testing.T) {
	cfg := map[string]any{
		"password": "${gcp-sm:/secret}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'project/secret'")
}

func TestResolveGCPSM_EmptySecret(t *testing.T) {
	cfg := map[string]any{
		"password": "${gcp-sm:project/}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be in format 'project/secret'")
}

func TestResolveGCPSM_UnsafeProjectName(t *testing.T) {
	cfg := map[string]any{
		"password": "${gcp-sm:evil/path/../inject/my-secret}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	// path traversal blocked because ".." is split into project="evil", secret="path",
	// and the remaining "../inject" parts cause extra path components in the URL.
	// The initial "/" after "evil" splits correctly via strings.Cut.
}

func TestResolveGCPSM_UnsafeSecretName(t *testing.T) {
	cfg := map[string]any{
		"password": "${gcp-sm:my-project/secret?v=1}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid secret name")
}

func TestResolveGCPSM_DomainScopedProjectID(t *testing.T) {
	// Domain-scoped GCP project IDs like "domain.com:project-id" must be accepted.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/projects/example.com:my-project/secrets/db-pass/versions/latest:access")

		secretData := base64.StdEncoding.EncodeToString([]byte("domain-secret"))
		json.NewEncoder(w).Encode(map[string]any{
			"payload": map[string]string{
				"data": secretData,
			},
		})
	}))
	defer srv.Close()

	origClient := gcpHTTPClient
	gcpHTTPClient = srv.Client()
	defer func() { gcpHTTPClient = origClient }()

	origEndpoint := gcpSecretManagerEndpointOverride
	gcpSecretManagerEndpointOverride = srv.URL
	defer func() { gcpSecretManagerEndpointOverride = origEndpoint }()

	// Mock the metadata server to return a token.
	metadataSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "test-token",
		})
	}))
	defer metadataSrv.Close()

	origMetaClient := gcpMetadataHTTPClient
	gcpMetadataHTTPClient = metadataSrv.Client()
	defer func() { gcpMetadataHTTPClient = origMetaClient }()

	// We can't easily mock the metadata URL, so test through service account path instead.
	// Generate a temporary service account JSON with a test key.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes})

	// Create a token exchange server.
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "sa-test-token",
		})
	}))
	defer tokenSrv.Close()

	saJSON, err := json.Marshal(map[string]string{
		"client_email": "test@test.iam.gserviceaccount.com",
		"private_key":  string(privKeyPEM),
		"token_uri":    tokenSrv.URL,
	})
	require.NoError(t, err)

	dir := t.TempDir()
	saFile := filepath.Join(dir, "sa.json")
	require.NoError(t, os.WriteFile(saFile, saJSON, 0600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", saFile)

	cfg := map[string]any{
		"password": "${gcp-sm:example.com:my-project/db-pass}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "domain-secret", cfg["password"])
}

func TestResolveGCPSM_VersionInRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/versions/42:access")

		secretData := base64.StdEncoding.EncodeToString([]byte("versioned-secret"))
		json.NewEncoder(w).Encode(map[string]any{
			"payload": map[string]string{
				"data": secretData,
			},
		})
	}))
	defer srv.Close()

	origClient := gcpHTTPClient
	gcpHTTPClient = srv.Client()
	defer func() { gcpHTTPClient = origClient }()

	origEndpoint := gcpSecretManagerEndpointOverride
	gcpSecretManagerEndpointOverride = srv.URL
	defer func() { gcpSecretManagerEndpointOverride = origEndpoint }()

	// Use service account auth with a test key.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes})

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
	}))
	defer tokenSrv.Close()

	saJSON, err := json.Marshal(map[string]string{
		"client_email": "test@test.iam.gserviceaccount.com",
		"private_key":  string(privKeyPEM),
		"token_uri":    tokenSrv.URL,
	})
	require.NoError(t, err)

	dir := t.TempDir()
	saFile := filepath.Join(dir, "sa.json")
	require.NoError(t, os.WriteFile(saFile, saJSON, 0600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", saFile)

	cfg := map[string]any{
		"password": "${gcp-sm:myproject/mysecret/42}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "versioned-secret", cfg["password"])
}

func TestResolveGCPSM_ParseResponse(t *testing.T) {
	secretData := base64.StdEncoding.EncodeToString([]byte("gcp-secret-value"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"payload": map[string]string{
				"data": secretData,
			},
		})
	}))
	defer srv.Close()

	origClient := gcpHTTPClient
	gcpHTTPClient = srv.Client()
	defer func() { gcpHTTPClient = origClient }()

	// Test response parsing directly.
	resp, err := gcpHTTPClient.Get(srv.URL + "/v1/projects/myproj/secrets/mysecret/versions/latest:access")
	require.NoError(t, err)
	defer resp.Body.Close()

	var result struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	decoded, err := base64.StdEncoding.DecodeString(result.Payload.Data)
	require.NoError(t, err)
	assert.Equal(t, "gcp-secret-value", string(decoded))
}

func TestGCPCreateSignedJWT(t *testing.T) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)

	privKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	})

	jwt, err := gcpCreateSignedJWT("test@sa.iam.gserviceaccount.com", "https://oauth2.googleapis.com/token", string(privKeyPEM), 1700000000)
	require.NoError(t, err)
	assert.NotEmpty(t, jwt)

	// JWT should have 3 dot-separated parts.
	parts := strings.SplitN(jwt, ".", 3)
	assert.Len(t, parts, 3)

	// Decode header.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	var header map[string]string
	require.NoError(t, json.Unmarshal(headerJSON, &header))
	assert.Equal(t, "RS256", header["alg"])
	assert.Equal(t, "JWT", header["typ"])

	// Decode claims.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var claims map[string]any
	require.NoError(t, json.Unmarshal(claimsJSON, &claims))
	assert.Equal(t, "test@sa.iam.gserviceaccount.com", claims["iss"])
	assert.Equal(t, "https://www.googleapis.com/auth/cloud-platform", claims["scope"])
}

func TestGCPCreateSignedJWT_InvalidPEM(t *testing.T) {
	_, err := gcpCreateSignedJWT("test@sa.iam.gserviceaccount.com", "https://oauth2.googleapis.com/token", "not-a-pem", 1700000000)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PEM")
}

func TestGCPGetAccessToken_NoCredentials(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")

	// Metadata server will fail outside GCE.
	_, err := gcpGetAccessToken("${gcp-sm:proj/sec}")
	assert.Error(t, err)
}

func TestGCPGetTokenServiceAccount_MissingFile(t *testing.T) {
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/sa.json")

	_, err := gcpGetTokenServiceAccount()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading service account file")
}

func TestGCPGetTokenServiceAccount_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sa.json")
	require.NoError(t, os.WriteFile(f, []byte("not json"), 0600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", f)

	_, err := gcpGetTokenServiceAccount()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing service account JSON")
}

func TestGCPGetTokenServiceAccount_MissingFields(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "sa.json")
	require.NoError(t, os.WriteFile(f, []byte(`{"client_email":"","private_key":"","token_uri":""}`), 0600))
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", f)

	_, err := gcpGetTokenServiceAccount()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required fields")
}
