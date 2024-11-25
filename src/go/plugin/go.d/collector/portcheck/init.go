// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"net"
	"time"
)

type dialTCPFunc func(network, address string, timeout time.Duration) (net.Conn, error)

func (pc *PortCheck) validateConfig() error {
	if pc.Host == "" {
		return errors.New("missing required parameter: 'host' must be specified")
	}
	if len(pc.Ports) == 0 && len(pc.UDPPorts) == 0 {
		return errors.New("missing required parameters: at least one of 'ports' (TCP) or 'udp_ports' (UDP) must be specified")
	}
	return nil
}

func (pc *PortCheck) initPorts() (tcpPorts []*tcpPort, udpPorts []*udpPort) {
	for _, p := range pc.Ports {
		tcpPorts = append(tcpPorts, &tcpPort{number: p})
	}
	for _, p := range pc.UDPPorts {
		udpPorts = append(udpPorts, &udpPort{number: p})
	}

	return tcpPorts, udpPorts
}
