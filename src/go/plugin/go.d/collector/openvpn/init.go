// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/openvpn/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

func (c *Collector) validateConfig() error {
	return nil
}

func (c *Collector) initPerUserMatcher() (matcher.Matcher, error) {
	if c.PerUserStats.Empty() {
		return nil, nil
	}
	return c.PerUserStats.Parse()
}

func (c *Collector) initClient() *client.Client {
	config := socket.Config{
		Address: c.Address,
		Timeout: c.Timeout.Duration(),
	}
	return &client.Client{Client: socket.New(config)}
}
