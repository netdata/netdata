// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"net"
	"time"
)

type dialTCPFunc func(network, address string, timeout time.Duration) (net.Conn, error)

func (c *Collector) validateConfig() error {
	if c.Host == "" {
		return errors.New("missing required parameter: 'host' must be specified")
	}
	if len(c.Ports) == 0 && len(c.UDPPorts) == 0 {
		return errors.New("missing required parameters: at least one of 'ports' (TCP) or 'udp_ports' (UDP) must be specified")
	}
	return nil
}

func (c *Collector) initPorts() (tcpPorts []*tcpPort, udpPorts []*udpPort) {
	for _, p := range c.Ports {
		tcpPorts = append(tcpPorts, &tcpPort{number: p})
	}
	for _, p := range c.UDPPorts {
		udpPorts = append(udpPorts, &udpPort{number: p})
	}

	return tcpPorts, udpPorts
}
