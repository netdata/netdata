// SPDX-License-Identifier: GPL-3.0-or-later

package azureauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type fakeTokenCredential struct {
	getToken func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error)
}

func (f fakeTokenCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return f.getToken(ctx, opts)
}

func TestNewTokenProviderValidation(t *testing.T) {
	_, err := NewTokenProvider(nil, []string{"scope"}, time.Minute)
	if err == nil {
		t.Fatalf("expected error for nil credential")
	}

	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			return azcore.AccessToken{}, nil
		},
	}
	_, err = NewTokenProvider(cred, nil, time.Minute)
	if err == nil {
		t.Fatalf("expected error for empty scopes")
	}
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
	if err != nil {
		t.Fatalf("NewTokenProvider() unexpected error: %v", err)
	}
	p.now = func() time.Time { return baseNow }

	token, expiry, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() unexpected error: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("unexpected token: %q", token)
	}
	if !expiry.Equal(baseNow.Add(30 * time.Minute)) {
		t.Fatalf("unexpected expiry: %v", expiry)
	}

	token, _, err = p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() unexpected error on second call: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("unexpected token on second call: %q", token)
	}
	if calls != 1 {
		t.Fatalf("expected one credential call, got %d", calls)
	}
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
	if err != nil {
		t.Fatalf("NewTokenProvider() unexpected error: %v", err)
	}
	p.now = func() time.Time { return currentNow }

	first, _, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() first call unexpected error: %v", err)
	}

	// Past expiry-refresh boundary: now + margin >= cached expiry.
	currentNow = baseNow.Add(6 * time.Minute)
	second, _, err := p.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() second call unexpected error: %v", err)
	}

	if first == second {
		t.Fatalf("expected refreshed token, got same token %q", second)
	}
	if calls != 2 {
		t.Fatalf("expected two credential calls, got %d", calls)
	}
}

func TestTokenProviderReturnsCredentialError(t *testing.T) {
	cred := fakeTokenCredential{
		getToken: func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			return azcore.AccessToken{}, errors.New("boom")
		},
	}

	p, err := NewTokenProvider(cred, []string{"scope"}, time.Minute)
	if err != nil {
		t.Fatalf("NewTokenProvider() unexpected error: %v", err)
	}

	_, _, err = p.Token(context.Background())
	if err == nil {
		t.Fatalf("expected error")
	}
}
