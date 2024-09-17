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
	number  int
	state   string
	inState int
	latency int
}

func (pc *PortCheck) checkTCPPort(port *tcpPort) {
	start := time.Now()

	addr := pc.address(port.number)
	conn, err := pc.dialTCP("tcp", addr, pc.Timeout.Duration())

	dur := time.Since(start)

	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()

	if err != nil {
		if v, ok := err.(interface{ Timeout() bool }); ok && v.Timeout() {
			pc.setTcpPortCheckState(port, tcpPortCheckStateTimeout)
		} else {
			pc.setTcpPortCheckState(port, tcpPortCheckStateFailed)
		}
		return
	}

	pc.setTcpPortCheckState(port, tcpPortCheckStateSuccess)
	port.latency = durationToMs(dur)
}

func (pc *PortCheck) setTcpPortCheckState(port *tcpPort, state string) {
	if port.state == state {
		port.inState += pc.UpdateEvery
	} else {
		port.inState = pc.UpdateEvery
		port.state = state
	}
}
