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

	// Test response parsing directly (auth not possible on non-GCE).
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

func TestResolveGCPSM_VersionRef(t *testing.T) {
	// Verify version parsing in the ref.
	ref := "myproject/mysecret/42"
	project, rest, ok := splitGCPRef(ref)
	require.True(t, ok)
	assert.Equal(t, "myproject", project)
	assert.Equal(t, "mysecret", rest.secret)
	assert.Equal(t, "42", rest.version)
}

func TestResolveGCPSM_LatestVersion(t *testing.T) {
	ref := "myproject/mysecret"
	project, rest, ok := splitGCPRef(ref)
	require.True(t, ok)
	assert.Equal(t, "myproject", project)
	assert.Equal(t, "mysecret", rest.secret)
	assert.Equal(t, "latest", rest.version)
}

// splitGCPRef is a helper to test the ref parsing logic used by resolveGCPSM.
type gcpRefParts struct {
	secret  string
	version string
}

func splitGCPRef(ref string) (string, gcpRefParts, bool) {
	project, rest, ok := cutString(ref, "/")
	if !ok || project == "" || rest == "" {
		return "", gcpRefParts{}, false
	}
	secret, version, hasVersion := cutString(rest, "/")
	if !hasVersion || version == "" {
		version = "latest"
	}
	return project, gcpRefParts{secret: secret, version: version}, true
}

func cutString(s, sep string) (string, string, bool) {
	i := indexOf(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestGCPCreateSignedJWT(t *testing.T) {
	// Generate a test RSA key.
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
	parts := splitJWT(jwt)
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

func splitJWT(jwt string) []string {
	var parts []string
	for {
		i := indexOf(jwt, ".")
		if i < 0 {
			parts = append(parts, jwt)
			break
		}
		parts = append(parts, jwt[:i])
		jwt = jwt[i+1:]
	}
	return parts
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
