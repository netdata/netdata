// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const (
	vaultTokenLookupPath = "auth/token/lookup-self"

	publicErrToken          = "the configured Vault token is unavailable"
	publicErrEndpoint       = "the configured Vault endpoint is unavailable"
	publicErrAuthentication = "the configured Vault authentication check failed"
)

var (
	errAuthenticationResponseTooLarge = errors.New("Vault authentication response exceeds size limit")
	errAuthenticationTokenInvalid     = errors.New("Vault rejected the configured token")
)

func (s *store) Test(ctx context.Context) error {
	if s == nil || ctx == nil || s.published == nil || s.runtime == nil {
		return errors.New("invalid Vault operational test")
	}
	defer s.runtime.httpClientInsecure.CloseIdleConnections()

	addr, err := s.published.address()
	if err != nil {
		return dyncfg.NewPublicError(publicErrEndpoint, err)
	}
	token, err := s.published.token()
	if err != nil {
		return dyncfg.NewPublicError(publicErrToken, err)
	}
	req, err := s.published.newRequest(ctx, addr, vaultTokenLookupPath, token)
	if err != nil {
		return dyncfg.NewPublicError(publicErrEndpoint, err)
	}
	// Keep the GET bodyless on the wire while making it non-replayable to
	// net/http; a replay could consume another use from a limited-use token.
	req.Body = io.NopCloser(strings.NewReader(""))

	resp, err := s.published.client().Do(req)
	if err != nil {
		return dyncfg.NewPublicError(
			publicErrEndpoint,
			fmt.Errorf("Vault authentication request failed: %w", err),
		)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusForbidden {
		permissionOnly, err := isPermissionOnlyDenied(resp.Body)
		if err != nil {
			return dyncfg.NewPublicError(publicErrAuthentication, err)
		}
		if permissionOnly {
			return dyncfg.ErrTestUnsupported
		}
	}
	if resp.StatusCode != http.StatusOK {
		return dyncfg.NewPublicError(
			publicErrAuthentication,
			fmt.Errorf("Vault authentication check returned HTTP %d", resp.StatusCode),
		)
	}
	return nil
}

func isPermissionOnlyDenied(r io.Reader) (bool, error) {
	body, err := io.ReadAll(io.LimitReader(r, responseBodyLimit+1))
	if err != nil {
		return false, fmt.Errorf("reading Vault authentication response: %w", err)
	}
	if len(body) > responseBodyLimit {
		return false, errAuthenticationResponseTooLarge
	}
	var response struct {
		Errors []string `json:"errors"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return false, fmt.Errorf("parsing Vault authentication response: %w", err)
	}
	var permissionDenied bool
	for _, message := range response.Errors {
		semanticErrors, ok := parseVaultErrors(message)
		if !ok {
			return false, nil
		}
		for _, err := range semanticErrors {
			switch err {
			case "invalid token":
				return false, errAuthenticationTokenInvalid
			case "permission denied":
				permissionDenied = true
			default:
				return false, nil
			}
		}
	}
	return permissionDenied, nil
}

func parseVaultErrors(message string) ([]string, bool) {
	message = strings.TrimSpace(message)
	if message == "" {
		return nil, false
	}
	if !strings.Contains(message, "\n") {
		return []string{message}, true
	}

	lines := strings.Split(message, "\n")
	countText, suffix, ok := strings.Cut(lines[0], " ")
	if !ok {
		return nil, false
	}
	count, err := strconv.Atoi(countText)
	if err != nil || count <= 0 {
		return nil, false
	}
	if count == 1 {
		if suffix != "error occurred:" {
			return nil, false
		}
	} else if suffix != "errors occurred:" {
		return nil, false
	}
	if count > len(lines)-1 {
		return nil, false
	}

	semanticErrors := make([]string, 0, count)
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "* ") {
			return nil, false
		}
		message := strings.TrimSpace(strings.TrimPrefix(line, "* "))
		if message == "" {
			return nil, false
		}
		semanticErrors = append(semanticErrors, message)
	}
	return semanticErrors, len(semanticErrors) == count
}
