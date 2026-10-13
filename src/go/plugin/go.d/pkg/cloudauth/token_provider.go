// SPDX-License-Identifier: GPL-3.0-or-later

package cloudauth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

const (
	DefaultTokenRefreshMargin = 5 * time.Minute
)

type TokenProvider struct {
	mu            sync.Mutex
	cred          azcore.TokenCredential
	scopes        []string
	refreshMargin time.Duration
	now           func() time.Time

	cachedToken  string
	cachedExpiry time.Time
}

func NewTokenProvider(cred azcore.TokenCredential, scopes []string, refreshMargin time.Duration) (*TokenProvider, error) {
	if cred == nil {
		return nil, errors.New("token credential is nil")
	}
	if len(scopes) == 0 {
		return nil, errors.New("token scopes are required")
	}
	if slices.Contains(scopes, "") {
		return nil, errors.New("token scopes contain an empty value")
	}
	if refreshMargin <= 0 {
		refreshMargin = DefaultTokenRefreshMargin
	}

	return &TokenProvider{
		cred:          cred,
		scopes:        scopes,
		refreshMargin: refreshMargin,
		now:           time.Now,
	}, nil
}

func (p *TokenProvider) Token(ctx context.Context) (string, time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if p.cachedToken != "" && now.Add(p.refreshMargin).Before(p.cachedExpiry) {
		return p.cachedToken, p.cachedExpiry, nil
	}

	token, err := p.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: p.scopes})
	if err != nil {
		// Fall back to cached token if it hasn't expired yet
		// Re-evaluate time after refresh attempt, because GetToken may block.
		fallbackNow := p.now()
		if p.cachedToken != "" && fallbackNow.Before(p.cachedExpiry) {
			return p.cachedToken, p.cachedExpiry, nil
		}
		return "", time.Time{}, err
	}
	if token.Token == "" {
		return "", time.Time{}, fmt.Errorf("received empty token for scopes %v", p.scopes)
	}

	p.cachedToken = token.Token
	p.cachedExpiry = token.ExpiresOn

	return p.cachedToken, p.cachedExpiry, nil
}
