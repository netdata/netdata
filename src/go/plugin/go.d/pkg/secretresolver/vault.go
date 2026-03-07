// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// vaultNoRedirect prevents following redirects that could leak the Vault token.
var vaultNoRedirect = func(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

var (
	vaultHTTPClient = &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: vaultNoRedirect,
	}
	vaultHTTPClientInsecure = &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: vaultNoRedirect,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
)

func resolveVault(ref, original string) (string, error) {
	path, key, ok := strings.Cut(ref, "#")
	if !ok || key == "" {
		return "", fmt.Errorf("resolving secret '%s': vault reference must be in format 'path#key'", original)
	}
	if path == "" {
		return "", fmt.Errorf("resolving secret '%s': vault path is empty", original)
	}
	// Reject path traversal and query/fragment injection.
	if strings.Contains(path, "..") || strings.ContainsAny(path, "?#") {
		return "", fmt.Errorf("resolving secret '%s': vault path contains invalid characters", original)
	}

	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		return "", fmt.Errorf("resolving secret '%s': VAULT_ADDR environment variable is not set", original)
	}

	token, err := vaultToken()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}

	url := strings.TrimRight(addr, "/") + "/v1/" + path

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': %w", original, err)
	}
	req.Header.Set("X-Vault-Token", token)
	if ns := os.Getenv("VAULT_NAMESPACE"); ns != "" {
		req.Header.Set("X-Vault-Namespace", ns)
	}

	client := vaultHTTPClient
	if v := os.Getenv("VAULT_SKIP_VERIFY"); v == "true" || v == "1" {
		client = vaultHTTPClientInsecure
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': vault request failed: %w", original, err)
	}
	defer resp.Body.Close()

	// Cap response body to 1 MiB to prevent memory exhaustion.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': reading vault response: %w", original, err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': vault returned HTTP %d: %s", original, resp.StatusCode, truncateBody(body))
	}

	return parseVaultResponse(body, key, original)
}

func vaultToken() (string, error) {
	if token := os.Getenv("VAULT_TOKEN"); token != "" {
		return token, nil
	}

	tokenFile := os.Getenv("VAULT_TOKEN_FILE")
	if tokenFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("VAULT_TOKEN not set and cannot determine home directory: %w", err)
		}
		tokenFile = filepath.Join(home, ".vault-token")
	}

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return "", fmt.Errorf("VAULT_TOKEN not set and cannot read token file '%s': %w", tokenFile, err)
	}

	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("VAULT_TOKEN not set and token file '%s' is empty", tokenFile)
	}

	return token, nil
}

func parseVaultResponse(body []byte, key, original string) (string, error) {
	var resp struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("resolving secret '%s': parsing vault response: %w", original, err)
	}

	// Try KV v2 format first: .data.data[key]
	var kvV2 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.Data, &kvV2); err == nil && kvV2.Data != nil {
		if val, ok := kvV2.Data[key]; ok {
			return vaultValueToString(val)
		}
	}

	// Fall back to KV v1 format: .data[key]
	var kvV1 map[string]any
	if err := json.Unmarshal(resp.Data, &kvV1); err == nil {
		if val, ok := kvV1[key]; ok {
			return vaultValueToString(val)
		}
	}

	return "", fmt.Errorf("resolving secret '%s': key '%s' not found in vault response", original, key)
}

func vaultValueToString(val any) (string, error) {
	if s, ok := val.(string); ok {
		return s, nil
	}
	// For non-string values (numbers, booleans), use JSON encoding
	// to avoid Go-internal formatting like map[k:v].
	b, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("encoding vault value: %w", err)
	}
	return string(b), nil
}

func truncateBody(body []byte) string {
	const maxLen = 200
	s := strings.TrimSpace(string(body))
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
