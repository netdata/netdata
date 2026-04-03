// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/internal/httpx"
)

func (s *store) init(_ context.Context) error {
	switch strings.TrimSpace(s.Config.AuthMode) {
	case "env", "ecs", "imds":
		s.Config.AuthMode = strings.TrimSpace(s.Config.AuthMode)
	default:
		return fmt.Errorf("auth_mode '%s' is invalid for kind '%s'", s.Config.AuthMode, secretstore.KindAWSSM)
	}

	region := strings.TrimSpace(s.Config.Region)
	if region == "" {
		return fmt.Errorf("region is required")
	}
	s.Config.Region = region

	switch {
	case s.Config.Timeout.Duration() < 0:
		return fmt.Errorf("timeout cannot be negative")
	case s.Config.Timeout.Duration() == 0:
		s.Config.Timeout = defaultTimeout
	}
	s.runtime = &runtime{
		apiClient:  httpx.APIClient(s.Config.Timeout.Duration()),
		imdsClient: httpx.NoProxyClient(s.Config.Timeout.Duration()),
	}

	published := &publishedStore{
		runtime:     s.runtime,
		mode:        s.Config.AuthMode,
		regionValue: region,
	}

	s.published = published
	return nil
}
