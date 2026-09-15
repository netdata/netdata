// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	"errors"
	"strings"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}
	if !strings.HasSuffix(c.URL, "?auto") {
		c.Warningf("missing '?auto' parameter - needed for machine-readable output")
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*web.HTTPClient, error) {
	return web.NewHTTPClient(ctx, c.ClientConfig, c.CredentialFiles())
}
