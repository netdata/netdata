// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/jackc/pgx/v5"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/azureauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAADTokenCredential struct {
	getToken func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error)
}

func (f fakeAADTokenCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return f.getToken(ctx, opts)
}

func TestCollector_openAzureADConnection_NoTokenProvider(t *testing.T) {
	c := New()
	cfg, err := pgx.ParseConfig("postgres://netdata@127.0.0.1:5432/postgres")
	require.NoError(t, err)

	db, err := c.openAzureADConnection(cfg, "Postgres database")
	assert.Nil(t, db)
	assert.ErrorContains(t, err, "azure token provider is not initialized")
}

func TestCollector_azureADBeforeConnect_SetsPassword(t *testing.T) {
	cred := fakeAADTokenCredential{
		getToken: func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
			return azcore.AccessToken{
				Token:     "aad-token",
				ExpiresOn: time.Now().Add(30 * time.Minute),
			}, nil
		},
	}
	provider, err := azureauth.NewTokenProvider(cred, []string{"scope"}, time.Minute)
	require.NoError(t, err)

	c := New()
	c.azureTokenProvider = provider

	cfg, err := pgx.ParseConfig("postgres://netdata@127.0.0.1:5432/postgres")
	require.NoError(t, err)

	err = c.azureADBeforeConnect(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, "aad-token", cfg.Password)
}

func TestCollector_azureADBeforeConnect_ProviderError(t *testing.T) {
	cred := fakeAADTokenCredential{
		getToken: func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
			return azcore.AccessToken{}, errors.New("token failure")
		},
	}
	provider, err := azureauth.NewTokenProvider(cred, []string{"scope"}, time.Minute)
	require.NoError(t, err)

	c := New()
	c.azureTokenProvider = provider

	cfg, err := pgx.ParseConfig("postgres://netdata@127.0.0.1:5432/postgres")
	require.NoError(t, err)
	cfg.Password = "old-password"

	err = c.azureADBeforeConnect(context.Background(), cfg)
	assert.ErrorContains(t, err, "token failure")
	assert.Equal(t, "old-password", cfg.Password)
}
