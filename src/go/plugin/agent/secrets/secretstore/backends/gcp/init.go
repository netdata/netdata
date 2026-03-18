// SPDX-License-Identifier: GPL-3.0-or-later

package gcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

func (s *store) init(_ context.Context) error {
	published := &publishedStore{provider: s.provider}

	switch strings.TrimSpace(s.Config.Mode) {
	case "metadata":
		s.Config.Mode = "metadata"
		s.Config.ModeServiceAccountFile = nil
		published.mode = s.Config.Mode
	case "service_account_file":
		if s.Config.ModeServiceAccountFile == nil {
			return fmt.Errorf("mode_service_account_file is required when mode is 'service_account_file'")
		}
		path := strings.TrimSpace(s.Config.ModeServiceAccountFile.Path)
		if path == "" {
			return fmt.Errorf("mode_service_account_file.path is required")
		}
		s.Config.Mode = "service_account_file"
		s.Config.ModeServiceAccountFile.Path = path
		published.mode = s.Config.Mode
		published.serviceAccountFilePath = path
	default:
		return fmt.Errorf("mode '%s' is invalid for kind '%s'", s.Config.Mode, secretstore.KindGCPSM)
	}

	s.published = published
	return nil
}
