// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	if len(c.Repositories) == 0 {
		return errors.New("repositories not set")
	}
	return nil
}

func (c *Collector) initApiClient() (*apiClient, error) {
	client, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}
	return newAPIClient(client, c.RequestConfig), nil
}
