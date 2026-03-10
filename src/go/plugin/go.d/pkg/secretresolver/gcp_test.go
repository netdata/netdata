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

func writeTestGCPServiceAccountFile(t *testing.T, tokenURI string) string {
	t.Helper()

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)

	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes})

	saJSON, err := json.Marshal(map[string]string{
		"client_email": "test@test.iam.gserviceaccount.com",
		"private_key":  string(privKeyPEM),
		"token_uri":    tokenURI,
	})
	require.NoError(t, err)

	dir := t.TempDir()
	saFile := filepath.Join(dir, "sa.json")
	require.NoError(t, os.WriteFile(saFile, saJSON, 0600))

	return saFile
}

func TestResolveGCPSM_Validation(t *testing.T) {
	tests := map[string]struct {
		ref             string
		wantErrContains string
	}{
		"invalid ref": {
			ref:             "${gcp-sm:just-project}",
			wantErrContains: "must be in format 'project/secret'",
		},
		"empty project": {
			ref:             "${gcp-sm:/secret}",
			wantErrContains: "must be in format 'project/secret'",
		},
		"empty secret": {
			ref:             "${gcp-sm:project/}",
			wantErrContains: "must be in format 'project/secret'",
		},
		"unsafe project name": {
			ref:             "${gcp-sm:evil/path/../inject/my-secret}",
			wantErrContains: "invalid version",
		},
		"unsafe secret name": {
			ref:             "${gcp-sm:my-project/secret?v=1}",
			wantErrContains: "invalid secret name",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolver := New()
			cfg := map[string]any{"password": tc.ref}
			err := resolver.Resolve(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErrContains)
		})
	}
}

func TestResolveGCPSM(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, resolver *Resolver)
	}{
		"domain scoped project id": {
			run: func(t *testing.T, resolver *Resolver) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Contains(t, r.URL.Path, "/v1/projects/example.com:my-project/secrets/db-pass/versions/latest:access")

					secretData := base64.StdEncoding.EncodeToString([]byte("domain-secret"))
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"payload": map[string]string{"data": secretData},
					}))
				}))
				t.Cleanup(srv.Close)

				tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"access_token": "sa-test-token"}))
				}))
				t.Cleanup(tokenSrv.Close)

				saFile := writeTestGCPServiceAccountFile(t, tokenSrv.URL)
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", saFile)

				resolver.gcpHTTPClient = srv.Client()
				resolver.gcpSecretManagerEndpoint = srv.URL

				cfg := map[string]any{"password": "${gcp-sm:example.com:my-project/db-pass}"}
				require.NoError(t, resolver.Resolve(cfg))
				assert.Equal(t, "domain-secret", cfg["password"])
			},
		},
		"version in reference": {
			run: func(t *testing.T, resolver *Resolver) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					assert.Contains(t, r.URL.Path, "/versions/42:access")

					secretData := base64.StdEncoding.EncodeToString([]byte("versioned-secret"))
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"payload": map[string]string{"data": secretData},
					}))
				}))
				t.Cleanup(srv.Close)

				tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]string{"access_token": "token"}))
				}))
				t.Cleanup(tokenSrv.Close)

				saFile := writeTestGCPServiceAccountFile(t, tokenSrv.URL)
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", saFile)

				resolver.gcpHTTPClient = srv.Client()
				resolver.gcpSecretManagerEndpoint = srv.URL

				cfg := map[string]any{"password": "${gcp-sm:myproject/mysecret/42}"}
				require.NoError(t, resolver.Resolve(cfg))
				assert.Equal(t, "versioned-secret", cfg["password"])
			},
		},
		"parse response": {
			run: func(t *testing.T, resolver *Resolver) {
				secretData := base64.StdEncoding.EncodeToString([]byte("gcp-secret-value"))

				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"payload": map[string]string{"data": secretData},
					}))
				}))
				t.Cleanup(srv.Close)

				resolver.gcpHTTPClient = srv.Client()

				resp, err := resolver.gcpHTTPClient.Get(srv.URL + "/v1/projects/myproj/secrets/mysecret/versions/latest:access")
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
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolver := New()
			tc.run(t, resolver)
		})
	}
}

func TestGCPCreateSignedJWT(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"success": {
			run: func(t *testing.T) {
				privKey, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)

				privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
				require.NoError(t, err)

				privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes})

				jwt, err := gcpCreateSignedJWT("test@sa.iam.gserviceaccount.com", "https://oauth2.googleapis.com/token", string(privKeyPEM), 1700000000)
				require.NoError(t, err)
				assert.NotEmpty(t, jwt)

				parts := strings.SplitN(jwt, ".", 3)
				assert.Len(t, parts, 3)

				headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
				require.NoError(t, err)
				var header map[string]string
				require.NoError(t, json.Unmarshal(headerJSON, &header))
				assert.Equal(t, "RS256", header["alg"])
				assert.Equal(t, "JWT", header["typ"])

				claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
				require.NoError(t, err)
				var claims map[string]any
				require.NoError(t, json.Unmarshal(claimsJSON, &claims))
				assert.Equal(t, "test@sa.iam.gserviceaccount.com", claims["iss"])
				assert.Equal(t, "https://www.googleapis.com/auth/cloud-platform", claims["scope"])
			},
		},
		"invalid pem": {
			run: func(t *testing.T) {
				_, err := gcpCreateSignedJWT("test@sa.iam.gserviceaccount.com", "https://oauth2.googleapis.com/token", "not-a-pem", 1700000000)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "failed to decode PEM")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func TestGCPAuthPaths(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T, resolver *Resolver)
	}{
		"get access token no credentials": {
			run: func(t *testing.T, resolver *Resolver) {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")

				_, err := resolver.gcpGetAccessToken("${gcp-sm:proj/sec}")
				assert.Error(t, err)
			},
		},
		"service account missing file": {
			run: func(t *testing.T, resolver *Resolver) {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/nonexistent/sa.json")

				_, err := resolver.gcpGetTokenServiceAccount()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "reading service account file")
			},
		},
		"service account invalid json": {
			run: func(t *testing.T, resolver *Resolver) {
				dir := t.TempDir()
				f := filepath.Join(dir, "sa.json")
				require.NoError(t, os.WriteFile(f, []byte("not json"), 0600))
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", f)

				_, err := resolver.gcpGetTokenServiceAccount()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "parsing service account JSON")
			},
		},
		"service account missing fields": {
			run: func(t *testing.T, resolver *Resolver) {
				dir := t.TempDir()
				f := filepath.Join(dir, "sa.json")
				require.NoError(t, os.WriteFile(f, []byte(`{"client_email":"","private_key":"","token_uri":""}`), 0600))
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", f)

				_, err := resolver.gcpGetTokenServiceAccount()
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "missing required fields")
			},
		},
		"metadata token success": {
			run: func(t *testing.T, resolver *Resolver) {
				resolver.gcpMetadataHTTPClient = &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						assert.Equal(t, "GET", req.Method)
						assert.Equal(t, "metadata.google.internal", req.URL.Host)
						assert.Equal(t, "/computeMetadata/v1/instance/service-accounts/default/token", req.URL.Path)
						assert.Equal(t, "Google", req.Header.Get("Metadata-Flavor"))
						return newHTTPResponse(http.StatusOK, `{"access_token":"metadata-token"}`), nil
					}),
				}

				token, err := resolver.gcpGetTokenMetadata()
				require.NoError(t, err)
				assert.Equal(t, "metadata-token", token)
			},
		},
		"access token metadata fallback success": {
			run: func(t *testing.T, resolver *Resolver) {
				t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")

				resolver.gcpMetadataHTTPClient = &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						assert.Equal(t, "Google", req.Header.Get("Metadata-Flavor"))
						return newHTTPResponse(http.StatusOK, `{"access_token":"metadata-token"}`), nil
					}),
				}

				token, err := resolver.gcpGetAccessToken("${gcp-sm:project/secret}")
				require.NoError(t, err)
				assert.Equal(t, "metadata-token", token)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolver := New()
			tc.run(t, resolver)
		})
	}
}
