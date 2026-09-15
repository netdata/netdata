// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

import (
	"fmt"

	"context"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return fmt.Errorf("url not set")
	}
	return nil
}

func (c *Collector) initHTTPClient(ctx context.Context) (*web.HTTPClient, error) {
	return web.NewHTTPClient(ctx, c.ClientConfig, c.CredentialFiles())
}
