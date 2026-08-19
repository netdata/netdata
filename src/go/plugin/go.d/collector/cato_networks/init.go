// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) initSiteSelector() error {
	expr := strings.TrimSpace(c.SiteSelector)
	if expr == "" || expr == "*" {
		c.siteMatcher = matcher.TRUE()
		return nil
	}

	m, err := matcher.NewSimplePatternsMatcher(expr)
	if err != nil {
		return fmt.Errorf("init site_selector: %w", err)
	}
	c.siteMatcher = m
	return nil
}

func (c *Collector) initClient() error {
	if c.client != nil {
		return nil
	}

	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return fmt.Errorf("init http client: %w", err)
	}
	c.httpClient = httpClient

	client, err := c.newClient(c.Config, httpClient)
	if err != nil {
		return fmt.Errorf("init Cato client: %w", err)
	}
	c.client = client
	return nil
}
