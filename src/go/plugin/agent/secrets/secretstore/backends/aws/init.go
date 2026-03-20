// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
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

	published := &publishedStore{
		provider:    s.provider,
		mode:        s.Config.AuthMode,
		regionValue: region,
	}

	s.published = published
	return nil
}
