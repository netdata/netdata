// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *ClickHouse) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (c *ClickHouse) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.Client)
}
