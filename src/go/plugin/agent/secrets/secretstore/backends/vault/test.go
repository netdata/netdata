// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

	resp, err := s.published.client().Do(req)
	if err != nil {
		return dyncfg.NewPublicError(
			publicErrEndpoint,
			fmt.Errorf("Vault authentication request failed: %w", err),
		)
	}
	defer func() { _ = resp.Body.Close() }()

	accepted := resp.StatusCode == http.StatusOK
	if resp.StatusCode == http.StatusForbidden {
		accepted, err = acceptsPermissionDenied(resp.Body)
		if err != nil {
			return dyncfg.NewPublicError(publicErrAuthentication, err)
		}
	}
	if !accepted {
		return dyncfg.NewPublicError(
			publicErrAuthentication,
			fmt.Errorf("Vault authentication check returned HTTP %d", resp.StatusCode),
		)
	}
	return nil
}

func acceptsPermissionDenied(r io.Reader) (bool, error) {
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
		if strings.Contains(message, "invalid token") {
			return false, errAuthenticationTokenInvalid
		}
		if strings.Contains(message, "permission denied") {
			permissionDenied = true
		}
	}
	return permissionDenied, nil
}
