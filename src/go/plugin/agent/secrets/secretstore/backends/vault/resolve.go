// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

func (s *publishedStore) Resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	return s.resolve(ctx, req)
}

func (s *publishedStore) resolve(ctx context.Context, req secretstore.ResolveRequest) (string, error) {
	path, key, ok := strings.Cut(req.Operand, "#")
	if !ok || key == "" {
		return "", fmt.Errorf("resolving secret '%s': store '%s': operand must be in format 'path#key'", req.Original, req.StoreKey)
	}
	if path == "" {
		return "", fmt.Errorf("resolving secret '%s': store '%s': vault path is empty", req.Original, req.StoreKey)
	}
	if strings.Contains(path, "..") || strings.ContainsAny(path, "?#") {
		return "", fmt.Errorf("resolving secret '%s': store '%s': vault path contains invalid characters", req.Original, req.StoreKey)
	}

	addr, err := s.address()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	token, err := s.token()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(addr, "/")+"/v1/"+path, nil)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': %w", req.Original, req.StoreKey, err)
	}
	httpReq.Header.Set("X-Vault-Token", token)
	if ns, ok := s.namespace(); ok {
		httpReq.Header.Set("X-Vault-Namespace", ns)
	}

	client := s.runtime.httpClient
	if s.skipVerify() {
		client = s.runtime.httpClientInsecure
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': vault request failed: %w", req.Original, req.StoreKey, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': reading vault response: %w", req.Original, req.StoreKey, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolving secret '%s': store '%s': vault returned HTTP %d: %s", req.Original, req.StoreKey, resp.StatusCode, httpx.TruncateBody(body))
	}
	value, err := parseResponse(body, key, req)
	if err != nil {
		return "", err
	}
	logResolvedRequest(ctx, req, path, key)
	return value, nil
}

func logResolvedRequest(ctx context.Context, req secretstore.ResolveRequest, path, key string) {
	if log, ok := logger.LoggerFromContext(ctx); ok {
		log.Infof("resolved secret via vault secretstore '%s' path '%s' key '%s'", req.StoreKey, path, key)
	}
}

func (s *publishedStore) address() (string, error) {
	if s.addr == "" {
		return "", fmt.Errorf("addr is required")
	}
	return s.addr, nil
}

func (s *publishedStore) namespace() (string, bool) {
	if s.namespaceValue == "" {
		return "", false
	}
	return s.namespaceValue, true
}

func (s *publishedStore) skipVerify() bool {
	return s.tlsSkipVerify
}

func (s *publishedStore) token() (string, error) {
	switch s.mode {
	case "token":
		if s.tokenValue == "" {
			return "", fmt.Errorf("mode_token.token is required")
		}
		return s.tokenValue, nil
	case "token_file":
		path := s.tokenFilePath
		if path == "" {
			return "", fmt.Errorf("mode_token_file.path is required")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("cannot read token file '%s': %w", path, err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return "", fmt.Errorf("token file '%s' is empty", path)
		}
		return token, nil
	default:
		return "", fmt.Errorf("mode '%s' is invalid for vault", s.mode)
	}
}

func parseResponse(body []byte, key string, req secretstore.ResolveRequest) (string, error) {
	var resp vaultReadResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': parsing vault response: %w", req.Original, req.StoreKey, err)
	}

	var kvV2 vaultKV2ReadPayload
	if err := json.Unmarshal(resp.Data, &kvV2); err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': parsing vault response data: %w", req.Original, req.StoreKey, err)
	}
	if kvV2.isDetected() {
		data, err := unmarshalVaultDataPayload(kvV2.Data)
		if err != nil {
			return "", fmt.Errorf("resolving secret '%s': store '%s': parsing vault response data: %w", req.Original, req.StoreKey, err)
		}
		if val, ok := data[key]; ok {
			return valueToString(val)
		}
		return "", fmt.Errorf("resolving secret '%s': store '%s': key '%s' not found in vault response", req.Original, req.StoreKey, key)
	}

	payload, err := unmarshalVaultDataPayload(resp.Data)
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': store '%s': parsing vault response data: %w", req.Original, req.StoreKey, err)
	}
	if val, ok := payload[key]; ok {
		return valueToString(val)
	}

	return "", fmt.Errorf("resolving secret '%s': store '%s': key '%s' not found in vault response", req.Original, req.StoreKey, key)
}

type vaultReadResponse struct {
	Data json.RawMessage `json:"data"`
}

type vaultKV2ReadPayload struct {
	Data     json.RawMessage   `json:"data"`
	Metadata *vaultKV2Metadata `json:"metadata"`
}

type vaultKV2Metadata struct {
	CreatedTime  *string `json:"created_time"`
	DeletionTime *string `json:"deletion_time"`
	Destroyed    *bool   `json:"destroyed"`
	Version      *int    `json:"version"`
}

func (p vaultKV2ReadPayload) isDetected() bool {
	// KV v2 read responses expose secret values under data.data and include
	// version metadata alongside it. Requiring the standard metadata fields
	// avoids misclassifying KV v1 secrets that happen to have top-level
	// keys named "data" and "metadata".
	return len(p.Data) != 0 && p.Metadata != nil &&
		p.Metadata.CreatedTime != nil &&
		p.Metadata.DeletionTime != nil &&
		p.Metadata.Destroyed != nil &&
		p.Metadata.Version != nil
}

func unmarshalVaultDataPayload(data json.RawMessage) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func valueToString(val any) (string, error) {
	if s, ok := val.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(val)
	if err != nil {
		return "", fmt.Errorf("encoding vault value: %w", err)
	}
	return string(b), nil
}
