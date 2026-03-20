// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreInit(t *testing.T) {
	tests := map[string]struct {
		cfg             Config
		wantErrContains string
	}{
		"service principal": {
			cfg: Config{
				Mode: cloudauth.AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
					TenantID:     "tenant-id",
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				},
			},
		},
		"default": {
			cfg: Config{
				Mode: cloudauth.AzureADAuthModeDefault,
			},
		},
		"service principal validation": {
			cfg: Config{
				Mode: cloudauth.AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
					TenantID: "tenant-id",
					ClientID: "client-id",
				},
			},
			wantErrContains: "mode_service_principal.client_secret is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &store{
				Config: tc.cfg,
				provider: &provider{
					apiClient:  &http.Client{},
					imdsClient: &http.Client{},
				},
			}

			err := s.init(context.Background())
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, s.published)
			assert.NotNil(t, s.published.tokenProvider)
		})
	}
}

func TestStoreAuthTimeout(t *testing.T) {
	tests := map[string]struct {
		mode        string
		apiTimeout  time.Duration
		imdsTimeout time.Duration
		want        time.Duration
	}{
		"service principal uses api client timeout": {
			mode:       cloudauth.AzureADAuthModeServicePrincipal,
			apiTimeout: 10 * time.Second,
			want:       10 * time.Second,
		},
		"managed identity uses imds timeout": {
			mode:        cloudauth.AzureADAuthModeManagedIdentity,
			imdsTimeout: 2 * time.Second,
			want:        2 * time.Second,
		},
		"default prefers api client timeout": {
			mode:        cloudauth.AzureADAuthModeDefault,
			apiTimeout:  10 * time.Second,
			imdsTimeout: 2 * time.Second,
			want:        10 * time.Second,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := &store{
				Config: Config{Mode: tc.mode},
				provider: &provider{
					apiClient:  &http.Client{Timeout: tc.apiTimeout},
					imdsClient: &http.Client{Timeout: tc.imdsTimeout},
				},
			}

			assert.Equal(t, tc.want, s.authTimeout())
		})
	}
}

func TestCredentialWithTimeout(t *testing.T) {
	tests := map[string]struct {
		timeout        time.Duration
		getToken       func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error)
		wantErr        bool
		wantErrIs      error
		wantToken      string
		wantMaxElapsed time.Duration
	}{
		"zero timeout passes through": {
			timeout: 0,
			getToken: func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
				return azcore.AccessToken{Token: "ok"}, nil
			},
			wantToken: "ok",
		},
		"timeout cancels token request": {
			timeout: 20 * time.Millisecond,
			getToken: func(ctx context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
				<-ctx.Done()
				return azcore.AccessToken{}, ctx.Err()
			},
			wantErr:        true,
			wantErrIs:      context.DeadlineExceeded,
			wantMaxElapsed: 250 * time.Millisecond,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cred := credentialWithTimeout{
				cred:    fakeTokenCredential{getToken: tc.getToken},
				timeout: tc.timeout,
			}

			start := time.Now()
			token, err := cred.GetToken(context.Background(), policy.TokenRequestOptions{Scopes: []string{azureKeyVaultScope}})
			elapsed := time.Since(start)

			if tc.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErrIs)
				assert.Less(t, elapsed, tc.wantMaxElapsed)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantToken, token.Token)
		})
	}
}

func TestDefaultCredentialTransportRouting(t *testing.T) {
	tests := map[string]struct {
		url              string
		wantDefaultCalls int
		wantNoProxyCalls int
	}{
		"aad host uses default transport": {
			url:              "https://login.microsoftonline.com/tenant/oauth2/v2.0/token",
			wantDefaultCalls: 1,
		},
		"imds uses no proxy transport": {
			url:              "http://169.254.169.254/metadata/identity/oauth2/token",
			wantNoProxyCalls: 1,
		},
		"localhost managed identity endpoint uses no proxy transport": {
			url:              "http://localhost/msi/token",
			wantNoProxyCalls: 1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var defaultCalls int
			var noProxyCalls int

			s := &store{
				Config: Config{Mode: cloudauth.AzureADAuthModeDefault},
				provider: &provider{
					apiClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						defaultCalls++
						return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
					})},
					imdsClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						noProxyCalls++
						return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
					})},
				},
			}

			transport, ok := s.credentialOptions().ClientOptions.Transport.(routingTransportAdapter)
			require.True(t, ok)

			req, err := http.NewRequest(http.MethodGet, tc.url, nil)
			require.NoError(t, err)

			_, err = transport.Do(req)
			require.NoError(t, err)

			assert.Equal(t, tc.wantDefaultCalls, defaultCalls)
			assert.Equal(t, tc.wantNoProxyCalls, noProxyCalls)
		})
	}
}
