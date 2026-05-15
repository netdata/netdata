// SPDX-License-Identifier: GPL-3.0-or-later

package vault

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

func (s *store) init(_ context.Context) error {
	switch {
	case s.Config.Timeout.Duration() < 0:
		return fmt.Errorf("timeout cannot be negative")
	case s.Config.Timeout.Duration() == 0:
		s.Config.Timeout = defaultTimeout
	}
	s.runtime = &runtime{
		httpClient:         httpx.VaultClient(s.Config.Timeout.Duration()),
		httpClientInsecure: httpx.VaultInsecureClient(s.Config.Timeout.Duration()),
	}

	published := &publishedStore{runtime: s.runtime}

	switch strings.TrimSpace(s.Config.Mode) {
	case "token":
		if s.Config.ModeToken == nil {
			return fmt.Errorf("mode_token is required when mode is 'token'")
		}
		token := strings.TrimSpace(s.Config.ModeToken.Token)
		if token == "" {
			return fmt.Errorf("mode_token.token is required")
		}
		s.Config.Mode = "token"
		s.Config.ModeToken.Token = token
		s.Config.ModeTokenFile = nil
		published.mode = s.Config.Mode
		published.tokenValue = token
	case "token_file":
		if s.Config.ModeTokenFile == nil {
			return fmt.Errorf("mode_token_file is required when mode is 'token_file'")
		}
		path := strings.TrimSpace(s.Config.ModeTokenFile.Path)
		if path == "" {
			return fmt.Errorf("mode_token_file.path is required")
		}
		s.Config.Mode = "token_file"
		s.Config.ModeTokenFile.Path = path
		s.Config.ModeToken = nil
		published.mode = s.Config.Mode
		published.tokenFilePath = path
	default:
		return fmt.Errorf("mode '%s' is invalid for kind '%s'", s.Config.Mode, secretstore.KindVault)
	}

	addr := strings.TrimSpace(s.Config.Addr)
	if addr == "" {
		return fmt.Errorf("addr is required")
	}
	s.Config.Addr = addr
	published.addr = addr

	s.Config.Namespace = strings.TrimSpace(s.Config.Namespace)
	published.namespaceValue = s.Config.Namespace
	published.tlsSkipVerify = s.Config.TLSSkipVerify

	s.published = published
	return nil
}
