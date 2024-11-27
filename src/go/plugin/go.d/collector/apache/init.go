// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	"errors"
	"net/http"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	if !strings.HasSuffix(c.URL, "?auto") {
		return errors.New("invalid URL, should ends in '?auto'")
	}
	return nil
}

func (c *Collector) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(c.ClientConfig)
}
