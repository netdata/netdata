// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (c *Collector) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.ClientConfig)
}
