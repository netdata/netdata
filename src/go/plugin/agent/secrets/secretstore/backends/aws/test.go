// SPDX-License-Identifier: GPL-3.0-or-later

package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

const publicErrCredentials = "AWS credentials are unavailable"

func (s *store) Test(ctx context.Context) error {
	if s == nil || ctx == nil || s.runtime == nil || s.runtime.imdsClient == nil ||
		s.published == nil || s.published.runtime == nil || s.published.runtime.imdsClient == nil {
		return errors.New("invalid AWS operational test")
	}
	defer s.runtime.imdsClient.CloseIdleConnections()

	if _, err := s.published.credentials(ctx); err != nil {
		return dyncfg.NewPublicError(publicErrCredentials, fmt.Errorf("AWS credential acquisition failed: %w", err))
	}
	return nil
}
