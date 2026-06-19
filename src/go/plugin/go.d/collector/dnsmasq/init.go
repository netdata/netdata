// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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

func (c *Collector) initCharts() (*collectorapi.Charts, error) {
	return cacheCharts.Copy(), nil
}

func isProtocolValid(protocol string) bool {
	return slices.Contains(validProtocols, protocol)
}

var validProtocols = []string{
	"udp",
	"tcp",
	"tcp-tls",
}
