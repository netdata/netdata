// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
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

	resp, err := s.runtime.apiClient.Do(httpReq)
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
	logResolvedRequest(ctx, req, vaultName, secretName)
	return result.Value, nil
}

func logResolvedRequest(ctx context.Context, req secretstore.ResolveRequest, vaultName, secretName string) {
	if log, ok := logger.LoggerFromContext(ctx); ok {
		log.Infof("resolved secret via azure-kv secretstore '%s' vault '%s' secret '%s'", req.StoreKey, vaultName, secretName)
	}
}

func splitOperand(operand string) (string, string, bool) {
	vaultName, secretName, ok := strings.Cut(operand, "/")
	return vaultName, secretName, ok && vaultName != "" && secretName != ""
}

func (s *publishedStore) accessToken(ctx context.Context) (string, error) {
	if s.tokenProvider == nil {
		return "", fmt.Errorf("azure token provider is not initialized")
	}

	token, _, err := s.tokenProvider.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("acquiring Azure Key Vault access token: %w", err)
	}
	return token, nil
}
