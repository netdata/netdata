// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	"github.com/netdata/netdata/go/go.d.plugin/modules/openvpn/client"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/socket"
)

func (o *OpenVPN) validateConfig() error {
	return nil
}

func (o *OpenVPN) initPerUserMatcher() (matcher.Matcher, error) {
	if o.PerUserStats.Empty() {
		return nil, nil
	}
	return o.PerUserStats.Parse()
}

func (o *OpenVPN) initClient() *client.Client {
	config := socket.Config{
		Address:        o.Address,
		ConnectTimeout: o.Timeout.Duration(),
		ReadTimeout:    o.Timeout.Duration(),
		WriteTimeout:   o.Timeout.Duration(),
	}
	return &client.Client{Client: socket.New(config)}
}
