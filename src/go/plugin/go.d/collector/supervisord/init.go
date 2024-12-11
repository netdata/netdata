// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) verifyConfig() error {
	if c.URL == "" {
		return errors.New("'url' not set")
	}
	return nil
}

func (c *Collector) initSupervisorClient() (supervisorClient, error) {
	u, err := url.Parse(c.URL)
	if err != nil {
		return nil, fmt.Errorf("parse 'url': %v (%s)", err, c.URL)
	}
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("create HTTP client: %v", err)
	}
	return newSupervisorRPCClient(u, httpClient)
}
