// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"time"
)

const (
	tcpPortCheckStateSuccess = "success"
	tcpPortCheckStateTimeout = "timeout"
	tcpPortCheckStateFailed  = "failed"
)

type tcpPort struct {
	number         int
	status         string
	statusChangeTs time.Time
	latency        int
}

func (c *Collector) checkTCPPort(port *tcpPort) {
	start := time.Now()

	addr := c.address(port.number)
	conn, err := c.dialTCP("tcp", addr, c.Timeout.Duration())

	dur := time.Since(start)

	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	if err != nil {
		if v, ok := err.(interface{ Timeout() bool }); ok && v.Timeout() {
			c.setTcpPortCheckState(port, tcpPortCheckStateTimeout)
		} else {
			c.setTcpPortCheckState(port, tcpPortCheckStateFailed)
		}
		return
	}

	c.setTcpPortCheckState(port, tcpPortCheckStateSuccess)
	port.latency = durationToMs(dur)
}

func (c *Collector) setTcpPortCheckState(port *tcpPort, state string) {
	if port.status != state {
		port.status = state
		port.statusChangeTs = time.Now()
	} else if port.statusChangeTs.IsZero() {
		port.statusChangeTs = time.Now()
	}
}
