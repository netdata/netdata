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

func TestResolveAWSSM(t *testing.T) {
	setupAWSServer := func(t *testing.T, handler http.HandlerFunc) {
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)

		origClient := awsHTTPClient
		awsHTTPClient = srv.Client()
		t.Cleanup(func() { awsHTTPClient = origClient })

		origEndpoint := awsEndpointOverride
		awsEndpointOverride = srv.URL + "/"
		t.Cleanup(func() { awsEndpointOverride = origEndpoint })
	}

	tests := map[string]struct {
		buildCfg        func(t *testing.T) map[string]any
		setup           func(t *testing.T)
		wantErrContains string
		wantValue       string
	}{
		"plain string": {
			setup: func(t *testing.T) {
				setupAWSServer(t, func(w http.ResponseWriter, r *http.Request) {
					assert.Equal(t, "secretsmanager.GetSecretValue", r.Header.Get("X-Amz-Target"))
					assert.Contains(t, r.Header.Get("Authorization"), "AWS4-HMAC-SHA256")
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"SecretString": "my-plain-secret"}))
				})

				t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
				t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
			},
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${aws-sm:myapp/secret}"}
			},
			wantValue: "my-plain-secret",
		},
		"json key": {
			setup: func(t *testing.T) {
				setupAWSServer(t, func(w http.ResponseWriter, r *http.Request) {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"SecretString": `{"username":"admin","password":"s3cret"}`,
					}))
				})

				t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
				t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
			},
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${aws-sm:myapp/secret#password}"}
			},
			wantValue: "s3cret",
		},
		"missing region": {
			setup: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
				t.Setenv("AWS_DEFAULT_REGION", "")
				t.Setenv("AWS_REGION", "")
			},
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${aws-sm:mysecret}"}
			},
			wantErrContains: "AWS region not set",
		},
		"empty name": {
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${aws-sm:}"}
			},
			wantErrContains: "secret name is empty",
		},
		"missing access key secret": {
			setup: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "")
				t.Setenv("AWS_DEFAULT_REGION", "us-east-1")
			},
			buildCfg: func(t *testing.T) map[string]any {
				return map[string]any{"password": "${aws-sm:mysecret}"}
			},
			wantErrContains: "AWS_SECRET_ACCESS_KEY is not",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(t)
			}

			cfg := tc.buildCfg(t)
			err := Resolve(cfg)

			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, cfg["password"])
		})
	}
}

func TestAWSGetCredentials(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"env vars with session token": {
			run: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
				t.Setenv("AWS_SESSION_TOKEN", "TOKEN")

				creds, err := awsGetCredentials()
				require.NoError(t, err)
				assert.Equal(t, "AKID", creds.accessKeyID)
				assert.Equal(t, "SECRET", creds.secretAccessKey)
				assert.Equal(t, "TOKEN", creds.sessionToken)
			},
		},
		"env vars without session token": {
			run: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
				t.Setenv("AWS_SESSION_TOKEN", "")

				creds, err := awsGetCredentials()
				require.NoError(t, err)
				assert.Equal(t, "AKID", creds.accessKeyID)
				assert.Equal(t, "SECRET", creds.secretAccessKey)
				assert.Empty(t, creds.sessionToken)
			},
		},
		"ecs relative uri": {
			run: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "")
				t.Setenv("AWS_SESSION_TOKEN", "")
				t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/abc")

				origClient := awsIMDSHTTPClient
				awsIMDSHTTPClient = &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						assert.Equal(t, "GET", req.Method)
						assert.Equal(t, "169.254.170.2", req.URL.Host)
						assert.Equal(t, "/v2/credentials/abc", req.URL.Path)
						return newHTTPResponse(http.StatusOK, `{"AccessKeyId":"ECSAK","SecretAccessKey":"ECSSK","Token":"ECSTOKEN"}`), nil
					}),
				}
				t.Cleanup(func() { awsIMDSHTTPClient = origClient })

				creds, err := awsGetCredentials()
				require.NoError(t, err)
				assert.Equal(t, "ECSAK", creds.accessKeyID)
				assert.Equal(t, "ECSSK", creds.secretAccessKey)
				assert.Equal(t, "ECSTOKEN", creds.sessionToken)
			},
		},
		"imds fallback after ecs failure": {
			run: func(t *testing.T) {
				t.Setenv("AWS_ACCESS_KEY_ID", "")
				t.Setenv("AWS_SECRET_ACCESS_KEY", "")
				t.Setenv("AWS_SESSION_TOKEN", "")
				t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "/v2/credentials/fail")

				origClient := awsIMDSHTTPClient
				awsIMDSHTTPClient = &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						switch {
						case req.URL.Host == "169.254.170.2":
							assert.Equal(t, "GET", req.Method)
							assert.Equal(t, "/v2/credentials/fail", req.URL.Path)
							return newHTTPResponse(http.StatusInternalServerError, `{"error":"temporary"}`), nil
						case req.Method == http.MethodPut && req.URL.Host == "169.254.169.254" && req.URL.Path == "/latest/api/token":
							assert.Equal(t, "21600", req.Header.Get("X-aws-ec2-metadata-token-ttl-seconds"))
							return newHTTPResponse(http.StatusOK, "imds-token"), nil
						case req.Method == http.MethodGet && req.URL.Host == "169.254.169.254" && req.URL.Path == "/latest/meta-data/iam/security-credentials/":
							assert.Equal(t, "imds-token", req.Header.Get("X-aws-ec2-metadata-token"))
							return newHTTPResponse(http.StatusOK, "my-role"), nil
						case req.Method == http.MethodGet && req.URL.Host == "169.254.169.254" && req.URL.Path == "/latest/meta-data/iam/security-credentials/my-role":
							assert.Equal(t, "imds-token", req.Header.Get("X-aws-ec2-metadata-token"))
							return newHTTPResponse(http.StatusOK, `{"AccessKeyId":"IMDSAK","SecretAccessKey":"IMDSSK","Token":"IMDSTOKEN"}`), nil
						default:
							return newHTTPResponse(http.StatusNotFound, `{"error":"unexpected"}`), nil
						}
					}),
				}
				t.Cleanup(func() { awsIMDSHTTPClient = origClient })

				creds, err := awsGetCredentials()
				require.NoError(t, err)
				assert.Equal(t, "IMDSAK", creds.accessKeyID)
				assert.Equal(t, "IMDSSK", creds.secretAccessKey)
				assert.Equal(t, "IMDSTOKEN", creds.sessionToken)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func TestAWSSigV4Sign(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"authorization header structure": {
			run: func(t *testing.T) {
				creds := &awsCredentials{
					accessKeyID:     "AKIDEXAMPLE",
					secretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				}

				headers := map[string]string{
					"host":         "secretsmanager.us-east-1.amazonaws.com",
					"x-amz-date":   "20230101T000000Z",
					"x-amz-target": "secretsmanager.GetSecretValue",
					"content-type": "application/x-amz-json-1.1",
				}

				auth := awsSigV4Sign("POST", "/", "", headers, `{"SecretId":"test"}`, creds, "us-east-1", "20230101", "20230101T000000Z")

				assert.Contains(t, auth, "AWS4-HMAC-SHA256")
				assert.Contains(t, auth, "Credential=AKIDEXAMPLE/20230101/us-east-1/secretsmanager/aws4_request")
				assert.Contains(t, auth, "SignedHeaders=")
				assert.Contains(t, auth, "Signature=")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func TestAWSCanonicalHeaders(t *testing.T) {
	tests := map[string]struct {
		headers      map[string]string
		wantContains []string
		wantSigned   string
	}{
		"sorts and canonicalizes": {
			headers: map[string]string{
				"x-amz-date":   "20230101T000000Z",
				"host":         "example.com",
				"content-type": "application/json",
			},
			wantContains: []string{
				"content-type:application/json\n",
				"host:example.com\n",
				"x-amz-date:20230101T000000Z\n",
			},
			wantSigned: "content-type;host;x-amz-date",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			canonical, signed := awsCanonicalHeaders(tc.headers)
			for _, want := range tc.wantContains {
				assert.Contains(t, canonical, want)
			}
			assert.Equal(t, tc.wantSigned, signed)
		})
	}
}

func TestAWSCryptoHelpers(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"derive signing key length": {
			run: func(t *testing.T) {
				key := awsDeriveSigningKey("secret", "20230101", "us-east-1")
				assert.Len(t, key, 32)
			},
		},
		"sha256 hex": {
			run: func(t *testing.T) {
				hash := awsSHA256Hex([]byte("hello"))
				assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", hash)
			},
		},
		"hmac sha256 length": {
			run: func(t *testing.T) {
				mac := awsHMACSHA256([]byte("key"), []byte("data"))
				assert.Len(t, mac, 32)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tc.run(t)
		})
	}
}
