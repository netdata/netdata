// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
