// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSecretsManagerHost(t *testing.T) {
	tests := map[string]struct {
		region   string
		wantHost string
	}{
		"standard partition": {
			region:   "us-east-1",
			wantHost: "secretsmanager.us-east-1.amazonaws.com",
		},
		"china partition": {
			region:   "cn-north-1",
			wantHost: "secretsmanager.cn-north-1.amazonaws.com.cn",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantHost, secretsManagerHost(tc.region))
		})
	}
}

func TestSecretValue_UsesSignedHostHeader(t *testing.T) {
	tests := map[string]struct {
		region   string
		wantHost string
	}{
		"standard partition": {
			region:   "us-east-1",
			wantHost: "secretsmanager.us-east-1.amazonaws.com",
		},
		"china partition": {
			region:   "cn-north-1",
			wantHost: "secretsmanager.cn-north-1.amazonaws.com.cn",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := &publishedStore{
				runtime: &runtime{
					apiClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
						assert.Equal(t, tc.wantHost, r.Host)
						return &http.Response{
							StatusCode: http.StatusOK,
							Body:       io.NopCloser(bytes.NewBufferString(`{"SecretString":"value"}`)),
							Header:     make(http.Header),
						}, nil
					})},
				},
			}

			value, err := store.secretValue(context.Background(), &credentials{
				accessKeyID:     "AKID",
				secretAccessKey: "SECRET",
			}, tc.region, "db/password", "${store:aws-sm:aws_prod:db/password}")
			require.NoError(t, err)
			assert.Equal(t, "value", value)
		})
	}
}

func TestSecretValueResponseHandling(t *testing.T) {
	readErr := errors.New("response read failed")
	exactValue := strings.Repeat("x", responseBodyLimit-len(`{"SecretString":""}`))
	tests := map[string]struct {
		status          int
		body            string
		reader          io.Reader
		wantValueLen    int
		wantErrContains string
		wantTooLarge    bool
	}{
		"response at limit succeeds": {
			status:       http.StatusOK,
			body:         `{"SecretString":"` + exactValue + `"}`,
			wantValueLen: len(exactValue),
		},
		"response above limit fails": {
			status:          http.StatusOK,
			body:            strings.Repeat("x", responseBodyLimit+1),
			wantErrContains: "reading AWS Secrets Manager response",
			wantTooLarge:    true,
		},
		"HTTP status wins over oversized diagnostic body": {
			status:          http.StatusForbidden,
			body:            strings.Repeat("x", responseBodyLimit+1),
			wantErrContains: "AWS Secrets Manager returned HTTP 403",
		},
		"read failure fails": {
			status:          http.StatusOK,
			reader:          errorReader{err: readErr},
			wantErrContains: "reading AWS Secrets Manager response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reader := tc.reader
			if reader == nil {
				reader = strings.NewReader(tc.body)
			}
			store := &publishedStore{
				runtime: &runtime{
					apiClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						return &http.Response{
							StatusCode: tc.status,
							Body:       io.NopCloser(reader),
							Header:     make(http.Header),
						}, nil
					})},
				},
			}

			value, err := store.secretValue(context.Background(), &credentials{
				accessKeyID:     "AKID",
				secretAccessKey: "SECRET",
			}, "us-east-1", "db/password", "${store:aws-sm:aws_prod:db/password}")

			if tc.wantErrContains == "" {
				require.NoError(t, err)
				assert.Len(t, value, tc.wantValueLen)
				return
			}
			require.ErrorContains(t, err, tc.wantErrContains)
			if tc.wantTooLarge {
				require.ErrorIs(t, err, httpx.ErrResponseTooLarge)
				return
			}
			require.NotErrorIs(t, err, httpx.ErrResponseTooLarge)
		})
	}
}

func TestPublishedStoreResolve_LogsDetailedResolution(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")

	store := &publishedStore{
		runtime: &runtime{
			apiClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "secretsmanager.us-east-1.amazonaws.com", req.Host)
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"SecretString":"{\"password\":\"secret-value\"}"}`)),
					Header:     make(http.Header),
				}, nil
			})},
		},
		mode:        "env",
		regionValue: "us-east-1",
	}

	out := captureLoggerOutput(t, func(log *logger.Logger) {
		ctx := logger.ContextWithLogger(context.Background(), log)
		value, err := store.Resolve(ctx, secretstore.ResolveRequest{
			StoreKey: "aws-sm:aws_prod",
			Operand:  "db/password#password",
			Original: "${store:aws-sm:aws_prod:db/password#password}",
		})
		require.NoError(t, err)
		assert.Equal(t, "secret-value", value)
	})

	assert.Contains(t, out, "resolved secret via aws-sm secretstore 'aws-sm:aws_prod' secret 'db/password' key 'password'")
	assert.NotContains(t, out, "secret-value")
}

func captureLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	fn(logger.NewWithWriter(&buf))
	return buf.String()
}
