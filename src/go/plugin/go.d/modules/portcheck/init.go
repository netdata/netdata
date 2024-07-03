// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"net"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

type dialFunc func(network, address string, timeout time.Duration) (net.Conn, error)

type port struct {
	number  int
	state   checkState
	inState int
	latency int
}

func (pc *PortCheck) validateConfig() error {
	if pc.Host == "" {
		return errors.New("'host' parameter not set")
	}
	if len(pc.Ports) == 0 {
		return errors.New("'ports' parameter not set")
	}
	return nil
}

func (pc *PortCheck) initCharts() (*module.Charts, error) {
	charts := module.Charts{}

	for _, port := range pc.Ports {
		if err := charts.Add(*newPortCharts(pc.Host, port)...); err != nil {
			return nil, err
		}
	}

	return &charts, nil
}

func (pc *PortCheck) initPorts() (ports []*port) {
	for _, p := range pc.Ports {
		ports = append(ports, &port{number: p})
	}
	return ports
}
