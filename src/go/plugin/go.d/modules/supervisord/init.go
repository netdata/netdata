// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (s *Supervisord) verifyConfig() error {
	if s.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (s *Supervisord) initSupervisorClient() (supervisorClient, error) {
	u, err := url.Parse(s.URL)
	if err != nil {
		return nil, fmt.Errorf("parse 'url': %v (%s)", err, s.URL)
	}
	httpClient, err := web.NewHTTPClient(s.Client)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %v", err)
	}
	return newSupervisorRPCClient(u, httpClient)
}
