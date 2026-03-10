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

func TestResolveAWSSM_PlainString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "secretsmanager.GetSecretValue", r.Header.Get("X-Amz-Target"))
		assert.Contains(t, r.Header.Get("Authorization"), "AWS4-HMAC-SHA256")

		resp := map[string]any{
			"SecretString": "my-plain-secret",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	origClient := awsHTTPClient
	awsHTTPClient = srv.Client()
	defer func() { awsHTTPClient = origClient }()

	origEndpoint := awsEndpointOverride
	awsEndpointOverride = srv.URL + "/"
	defer func() { awsEndpointOverride = origEndpoint }()

	cfg := map[string]any{
		"password": "${aws-sm:myapp/secret}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "my-plain-secret", cfg["password"])
}

func TestResolveAWSSM_JSONKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"SecretString": `{"username":"admin","password":"s3cret"}`,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	origClient := awsHTTPClient
	awsHTTPClient = srv.Client()
	defer func() { awsHTTPClient = origClient }()

	origEndpoint := awsEndpointOverride
	awsEndpointOverride = srv.URL + "/"
	defer func() { awsEndpointOverride = origEndpoint }()

	cfg := map[string]any{
		"password": "${aws-sm:myapp/secret#password}",
	}

	require.NoError(t, Resolve(cfg))
	assert.Equal(t, "s3cret", cfg["password"])
}

func TestResolveAWSSM_MissingRegion(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_DEFAULT_REGION", "")
	t.Setenv("AWS_REGION", "")

	cfg := map[string]any{
		"password": "${aws-sm:mysecret}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS region not set")
}

func TestResolveAWSSM_EmptyName(t *testing.T) {
	cfg := map[string]any{
		"password": "${aws-sm:}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "secret name is empty")
}

func TestResolveAWSSM_MissingAccessKeySecret(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_DEFAULT_REGION", "us-east-1")

	cfg := map[string]any{
		"password": "${aws-sm:mysecret}",
	}

	err := Resolve(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AWS_SECRET_ACCESS_KEY is not")
}

func TestAWSGetCredentials_EnvVars(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_SESSION_TOKEN", "TOKEN")

	creds, err := awsGetCredentials()
	require.NoError(t, err)
	assert.Equal(t, "AKID", creds.accessKeyID)
	assert.Equal(t, "SECRET", creds.secretAccessKey)
	assert.Equal(t, "TOKEN", creds.sessionToken)
}

func TestAWSGetCredentials_EnvVarsNoSession(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")
	t.Setenv("AWS_SESSION_TOKEN", "")

	creds, err := awsGetCredentials()
	require.NoError(t, err)
	assert.Equal(t, "AKID", creds.accessKeyID)
	assert.Equal(t, "SECRET", creds.secretAccessKey)
	assert.Empty(t, creds.sessionToken)
}

func TestAWSGetCredentials_ECSRelativeURI(t *testing.T) {
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
	defer func() { awsIMDSHTTPClient = origClient }()

	creds, err := awsGetCredentials()
	require.NoError(t, err)
	assert.Equal(t, "ECSAK", creds.accessKeyID)
	assert.Equal(t, "ECSSK", creds.secretAccessKey)
	assert.Equal(t, "ECSTOKEN", creds.sessionToken)
}

func TestAWSGetCredentials_IMDSFallbackAfterECSFailure(t *testing.T) {
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
	defer func() { awsIMDSHTTPClient = origClient }()

	creds, err := awsGetCredentials()
	require.NoError(t, err)
	assert.Equal(t, "IMDSAK", creds.accessKeyID)
	assert.Equal(t, "IMDSSK", creds.secretAccessKey)
	assert.Equal(t, "IMDSTOKEN", creds.sessionToken)
}

func TestAWSSigV4Sign(t *testing.T) {
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
}

func TestAWSCanonicalHeaders(t *testing.T) {
	headers := map[string]string{
		"x-amz-date":   "20230101T000000Z",
		"host":         "example.com",
		"content-type": "application/json",
	}

	canonical, signed := awsCanonicalHeaders(headers)

	assert.Contains(t, canonical, "content-type:application/json\n")
	assert.Contains(t, canonical, "host:example.com\n")
	assert.Contains(t, canonical, "x-amz-date:20230101T000000Z\n")
	assert.Equal(t, "content-type;host;x-amz-date", signed)
}

func TestAWSDeriveSigningKey(t *testing.T) {
	key := awsDeriveSigningKey("secret", "20230101", "us-east-1")
	assert.Len(t, key, 32) // HMAC-SHA256 produces 32 bytes
}

func TestAWSSHA256Hex(t *testing.T) {
	hash := awsSHA256Hex([]byte("hello"))
	assert.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", hash)
}

func TestAWSHMACSHA256(t *testing.T) {
	mac := awsHMACSHA256([]byte("key"), []byte("data"))
	assert.Len(t, mac, 32)
}
