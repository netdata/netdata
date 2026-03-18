// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

func (s *publishedStore) Resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	return s.resolve(ctx, req)
}

func (s *publishedStore) resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	vaultName, secretName, ok := splitOperand(req.Operand)
	if !ok {
		return "", fmt.Errorf("resolving secret '%s': store '%s': operand must be in format 'vault-name/secret-name'", req.Original, req.StoreKey)
	}
	if !reAzureSafeName.MatchString(vaultName) {
		return "", fmt.Errorf("resolving secret '%s': store '%s': invalid vault name '%s'", req.Original, req.StoreKey, vaultName)
	}
	if !reAzureSafeName.MatchString(secretName) {
		return "", fmt.Errorf("resolving secret '%s': store '%s': invalid secret name '%s'", req.Original, req.StoreKey, secretName)
	}

	token, err := s.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("https://%s.vault.azure.net/secrets/%s?api-version=7.4", vaultName, secretName), nil)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': creating request: %w", req.Original, req.StoreKey, err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := s.provider.apiClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': request failed: %w", req.Original, req.StoreKey, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': reading response: %w", req.Original, req.StoreKey, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': store '%s': Azure Key Vault returned HTTP %d: %s", req.Original, req.StoreKey, resp.StatusCode, httpx.TruncateBody(body))
	}

	var result struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': parsing response: %w", req.Original, req.StoreKey, err)
	}
	if result.Value == "" {
		return "", fmt.Errorf("resolving secret '%s': store '%s': Azure Key Vault returned empty secret value", req.Original, req.StoreKey)
	}
	return result.Value, nil
}

func splitOperand(operand string) (string, string, bool) {
	vaultName, secretName, ok := strings.Cut(operand, "/")
	return vaultName, secretName, ok && vaultName != "" && secretName != ""
}

func (s *publishedStore) accessToken(ctx context.Context) (string, error) {
	switch s.mode {
	case "client":
		if s.clientTenantID == "" {
			return "", fmt.Errorf("mode_client.tenant_id is required")
		}
		if s.clientID == "" {
			return "", fmt.Errorf("mode_client.client_id is required")
		}
		if s.clientSecret == "" {
			return "", fmt.Errorf("mode_client.client_secret is required")
		}
		return s.clientCredentialsToken(ctx, s.clientTenantID, s.clientID, s.clientSecret)
	case "managed_identity":
		var clientID string
		if s.managedIdentityClientID != "" {
			clientID = s.managedIdentityClientID
		}
		return s.managedIdentityToken(ctx, clientID)
	default:
		return "", fmt.Errorf("mode '%s' is invalid for azure-kv", s.mode)
	}
}

func (s *publishedStore) clientCredentialsToken(ctx context.Context, tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := s.provider.loginEndpointURL
	if tokenURL == "" {
		tokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	}
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"scope":         {"https://vault.azure.net/.default"},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("creating client credentials token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.provider.apiClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("client credentials token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading client credentials token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("client credentials token request returned HTTP %d: %s", resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing client credentials token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("client credentials token response missing access_token")
	}
	return result.AccessToken, nil
}

func (s *publishedStore) managedIdentityToken(ctx context.Context, clientID string) (string, error) {
	reqURL := "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=https://vault.azure.net"
	if clientID != "" {
		reqURL += "&client_id=" + url.QueryEscape(clientID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("creating managed identity token request: %w", err)
	}
	req.Header.Set("Metadata", "true")
	resp, err := s.provider.imdsClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("managed identity token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading managed identity token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("managed identity token request returned HTTP %d: %s", resp.StatusCode, httpx.TruncateBody(body))
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parsing managed identity token response: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("managed identity token response missing access_token")
	}
	return result.AccessToken, nil
}
