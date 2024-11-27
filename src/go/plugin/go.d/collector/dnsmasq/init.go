// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (c *Collector) validateConfig() error {
	if c.Address == "" {
		return errors.New("'address' parameter not set")
	}
	if !isProtocolValid(c.Protocol) {
		return fmt.Errorf("'protocol' (%s) is not valid, expected one of %v", c.Protocol, validProtocols)
	}
	return nil
}

func (c *Collector) initDNSClient() (dnsClient, error) {
	return c.newDNSClient(c.Protocol, c.Timeout.Duration()), nil
}

func (c *Collector) initCharts() (*module.Charts, error) {
	return cacheCharts.Copy(), nil
}

func isProtocolValid(protocol string) bool {
	for _, v := range validProtocols {
		if protocol == v {
			return true
		}
	}
	return false
}

var validProtocols = []string{
	"udp",
	"tcp",
	"tcp-tls",
}
