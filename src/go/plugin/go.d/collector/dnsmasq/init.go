// SPDX-License-Identifier: GPL-3.0-or-later

package dnsmasq

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

func (d *Dnsmasq) validateConfig() error {
	if d.Address == "" {
		return errors.New("'address' parameter not set")
	}
	if !isProtocolValid(d.Protocol) {
		return fmt.Errorf("'protocol' (%s) is not valid, expected one of %v", d.Protocol, validProtocols)
	}
	return nil
}

func (d *Dnsmasq) initDNSClient() (dnsClient, error) {
	return d.newDNSClient(d.Protocol, d.Timeout.Duration()), nil
}

func (d *Dnsmasq) initCharts() (*module.Charts, error) {
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
