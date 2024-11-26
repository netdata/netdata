// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) validateConfig() error {
	if c.URL == "" {
		return errors.New("url not set")
	}

	return nil
}

func (c *Collector) initPermitPluginMatcher() (matcher.Matcher, error) {
	if c.PermitPlugin == "" {
		return matcher.TRUE(), nil
	}

	return matcher.NewSimplePatternsMatcher(c.PermitPlugin)
}

func (c *Collector) initApiClient() (*apiClient, error) {
	client, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return nil, err
	}

	return newAPIClient(client, c.RequestConfig), nil
}
