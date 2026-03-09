// SPDX-License-Identifier: GPL-3.0-or-later

package azureauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTokenCredential struct {
	getToken func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error)
}

func (f fakeTokenCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return f.getToken(ctx, opts)
}

func TestNewTokenProviderValidation(t *testing.T) {
	_, err := NewTokenProvider(nil, []string{"scope"}, time.Minute)
	require.Error(t, err)

	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			return azcore.AccessToken{}, nil
		},
	}
	_, err = NewTokenProvider(cred, nil, time.Minute)
	require.Error(t, err)
}

func TestTokenProviderCachesToken(t *testing.T) {
	baseNow := time.Date(2026, time.March, 6, 10, 0, 0, 0, time.UTC)
	calls := 0
	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			calls++
			return azcore.AccessToken{
				Token:     "token-1",
				ExpiresOn: baseNow.Add(30 * time.Minute),
			}, nil
		},
	}

	p, err := NewTokenProvider(cred, []string{"scope"}, 5*time.Minute)
	require.NoError(t, err)
	p.now = func() time.Time { return baseNow }

	token, expiry, err := p.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token-1", token)
	assert.Equal(t, baseNow.Add(30*time.Minute), expiry)

	token, _, err = p.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token-1", token)
	assert.Equal(t, 1, calls)
}

func TestTokenProviderRefreshesNearExpiry(t *testing.T) {
	baseNow := time.Date(2026, time.March, 6, 10, 0, 0, 0, time.UTC)
	currentNow := baseNow
	calls := 0

	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			calls++
			return azcore.AccessToken{
				Token:     "token-" + string(rune('0'+calls)),
				ExpiresOn: currentNow.Add(10 * time.Minute),
			}, nil
		},
	}

	p, err := NewTokenProvider(cred, []string{"scope"}, 5*time.Minute)
	require.NoError(t, err)
	p.now = func() time.Time { return currentNow }

	first, _, err := p.Token(context.Background())
	require.NoError(t, err)

	// Past expiry-refresh boundary: now + margin >= cached expiry.
	currentNow = baseNow.Add(6 * time.Minute)
	second, _, err := p.Token(context.Background())
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.Equal(t, 2, calls)
}

func TestTokenProviderReturnsCredentialError(t *testing.T) {
	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			return azcore.AccessToken{}, errors.New("boom")
		},
	}

	p, err := NewTokenProvider(cred, []string{"scope"}, time.Minute)
	require.NoError(t, err)

	_, _, err = p.Token(context.Background())
	require.Error(t, err)
}

func TestTokenProviderFallsBackOnRefreshFailure(t *testing.T) {
	baseNow := time.Date(2026, time.March, 6, 10, 0, 0, 0, time.UTC)
	currentNow := baseNow
	calls := 0

	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			calls++
			if calls == 1 {
				return azcore.AccessToken{
					Token:     "good-token",
					ExpiresOn: baseNow.Add(10 * time.Minute),
				}, nil
			}
			return azcore.AccessToken{}, errors.New("transient failure")
		},
	}

	p, err := NewTokenProvider(cred, []string{"scope"}, 5*time.Minute)
	require.NoError(t, err)
	p.now = func() time.Time { return currentNow }

	// First call succeeds
	token, _, err := p.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "good-token", token)

	// Advance into refresh margin but before expiry
	currentNow = baseNow.Add(6 * time.Minute)
	token, _, err = p.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "good-token", token)

	// Advance past expiry — should error since no valid cache
	currentNow = baseNow.Add(11 * time.Minute)
	_, _, err = p.Token(context.Background())
	require.Error(t, err)
}

func TestTokenProviderDoesNotFallbackToExpiredTokenAfterSlowRefreshFailure(t *testing.T) {
	baseNow := time.Date(2026, time.March, 6, 10, 0, 0, 0, time.UTC)
	currentNow := baseNow
	calls := 0

	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			calls++
			if calls == 1 {
				return azcore.AccessToken{
					Token:     "good-token",
					ExpiresOn: baseNow.Add(10 * time.Minute),
				}, nil
			}

			// Simulate a slow refresh attempt that crosses token expiry before failing.
			currentNow = baseNow.Add(11 * time.Minute)
			return azcore.AccessToken{}, errors.New("slow refresh failure")
		},
	}

	p, err := NewTokenProvider(cred, []string{"scope"}, 5*time.Minute)
	require.NoError(t, err)
	p.now = func() time.Time { return currentNow }

	// Seed cache.
	token, _, err := p.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "good-token", token)

	// Enter refresh window while token is still valid.
	currentNow = baseNow.Add(6 * time.Minute)

	// Refresh fails after cache has expired in wall-clock time.
	_, _, err = p.Token(context.Background())
	require.Error(t, err)
	assert.Equal(t, 2, calls)
}
