// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTokenCredential struct {
	getToken func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error)
}

func (f fakeTokenCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return f.getToken(ctx, opts)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPublishedStoreResolve(t *testing.T) {
	errTokenFailure := errors.New("token failure")

	tests := map[string]struct {
		operand         string
		transport       roundTripFunc
		getToken        func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error)
		wantValue       string
		wantErrContains string
		wantErrIs       error
	}{
		"key vault secret": {
			operand: "my-vault/my-secret",
			transport: func(req *http.Request) (*http.Response, error) {
				assert.Equal(t, "https", req.URL.Scheme)
				assert.Equal(t, "my-vault.vault.azure.net", req.URL.Host)
				assert.Equal(t, "/secrets/my-secret", req.URL.Path)
				assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))

				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"value":"secret-value"}`)),
					Header:     make(http.Header),
				}, nil
			},
			getToken: func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
				return azcore.AccessToken{
					Token:     "test-token",
					ExpiresOn: time.Now().Add(30 * time.Minute),
				}, nil
			},
			wantValue: "secret-value",
		},
		"token acquisition failure": {
			operand: "my-vault/my-secret",
			getToken: func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
				return azcore.AccessToken{}, errTokenFailure
			},
			wantErrContains: "acquiring Azure Key Vault access token",
			wantErrIs:       errTokenFailure,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tokenProvider, err := cloudauth.NewTokenProvider(
				fakeTokenCredential{
					getToken: tc.getToken,
				},
				[]string{azureKeyVaultScope},
				time.Minute,
			)
			require.NoError(t, err)

			s := &publishedStore{
				provider: &provider{
					apiClient: &http.Client{Transport: tc.transport},
				},
				tokenProvider: tokenProvider,
			}

			value, err := s.Resolve(context.Background(), secretstore.ResolveRequest{
				StoreKey: "azure-kv:azure_prod",
				Operand:  tc.operand,
				Original: "${store:azure-kv:azure_prod:my-vault/my-secret}",
			})
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				assert.ErrorIs(t, err, tc.wantErrIs)
				assert.Empty(t, value)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantValue, value)
		})
	}
}

func TestPublishedStoreResolve_LogsDetailedResolution(t *testing.T) {
	tokenProvider, err := cloudauth.NewTokenProvider(
		fakeTokenCredential{
			getToken: func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
				return azcore.AccessToken{
					Token:     "test-token",
					ExpiresOn: time.Now().Add(30 * time.Minute),
				}, nil
			},
		},
		[]string{azureKeyVaultScope},
		time.Minute,
	)
	require.NoError(t, err)

	s := &publishedStore{
		provider: &provider{
			apiClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`{"value":"secret-value"}`)),
					Header:     make(http.Header),
				}, nil
			})},
		},
		tokenProvider: tokenProvider,
	}

	out := captureLoggerOutput(t, func(log *logger.Logger) {
		ctx := logger.ContextWithLogger(context.Background(), log)
		value, err := s.Resolve(ctx, secretstore.ResolveRequest{
			StoreKey: "azure-kv:azure_prod",
			Operand:  "my-vault/my-secret",
			Original: "${store:azure-kv:azure_prod:my-vault/my-secret}",
		})
		require.NoError(t, err)
		assert.Equal(t, "secret-value", value)
	})

	assert.Contains(t, out, "resolved secret via azure-kv secretstore 'azure-kv:azure_prod' vault 'my-vault' secret 'my-secret'")
	assert.NotContains(t, out, "secret-value")
}

func captureLoggerOutput(t *testing.T, fn func(log *logger.Logger)) string {
	t.Helper()

	var buf bytes.Buffer
	fn(logger.NewWithWriter(&buf))
	return buf.String()
}
