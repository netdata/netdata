// SPDX-License-Identifier: GPL-3.0-or-later

package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

func (s *store) init(_ context.Context) error {
	published := &publishedStore{provider: s.provider}

	switch strings.TrimSpace(s.Config.Mode) {
	case "client":
		if s.Config.ModeClient == nil {
			return fmt.Errorf("mode_client is required when mode is 'client'")
		}
		tenantID := strings.TrimSpace(s.Config.ModeClient.TenantID)
		if tenantID == "" {
			return fmt.Errorf("mode_client.tenant_id is required")
		}
		clientID := strings.TrimSpace(s.Config.ModeClient.ClientID)
		if clientID == "" {
			return fmt.Errorf("mode_client.client_id is required")
		}
		clientSecret := strings.TrimSpace(s.Config.ModeClient.ClientSecret)
		if clientSecret == "" {
			return fmt.Errorf("mode_client.client_secret is required")
		}
		s.Config.Mode = "client"
		s.Config.ModeClient.TenantID = tenantID
		s.Config.ModeClient.ClientID = clientID
		s.Config.ModeClient.ClientSecret = clientSecret
		s.Config.ModeManagedIdentity = nil
		published.mode = s.Config.Mode
		published.clientTenantID = tenantID
		published.clientID = clientID
		published.clientSecret = clientSecret
	case "managed_identity":
		s.Config.Mode = "managed_identity"
		s.Config.ModeClient = nil
		published.mode = s.Config.Mode
		if s.Config.ModeManagedIdentity != nil {
			clientID := strings.TrimSpace(s.Config.ModeManagedIdentity.ClientID)
			if clientID != "" {
				s.Config.ModeManagedIdentity.ClientID = clientID
				published.managedIdentityClientID = clientID
			} else {
				s.Config.ModeManagedIdentity = nil
			}
		}
	default:
		return fmt.Errorf("mode '%s' is invalid for kind '%s'", s.Config.Mode, secretstore.KindAzureKV)
	}

	s.published = published
	return nil
}
