// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
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

func TestSecretValue_CustomEndpointUsesSignedHostHeader(t *testing.T) {
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
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tc.wantHost, r.Host)
				w.Header().Set("Content-Type", "application/x-amz-json-1.1")
				_, _ = w.Write([]byte(`{"SecretString":"value"}`))
			}))
			defer srv.Close()

			store := &publishedStore{
				provider: &provider{
					apiClient: srv.Client(),
					endpoint:  srv.URL + "/",
					now: func() time.Time {
						return time.Date(2026, time.March, 18, 12, 0, 0, 0, time.UTC)
					},
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

func TestPublishedStoreResolve_LogsDetailedResolution(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKID")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "SECRET")

	store := &publishedStore{
		provider: &provider{
			apiClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(`{"SecretString":"{\"password\":\"secret-value\"}"}`)),
					Header:     make(http.Header),
				}, nil
			})},
			endpoint: "https://example.test/",
			now: func() time.Time {
				return time.Date(2026, time.March, 18, 12, 0, 0, 0, time.UTC)
			},
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
