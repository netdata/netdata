// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vcsa/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL not set")
	}
	if c.Username == "" || c.Password == "" {
		return errors.New("username or password not set")
	}
	return nil
}

func (c *Collector) initHealthClient() (*client.Client, error) {
	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}

	return client.New(httpClient, c.URL, c.Username, c.Password), nil
}
