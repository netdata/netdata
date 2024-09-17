// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (pc *PortCheck) collect() (map[string]int64, error) {
	wg := &sync.WaitGroup{}

	for _, port := range pc.tcpPorts {
		wg.Add(1)
		port := port
		go func() { defer wg.Done(); pc.checkTCPPort(port) }()
	}
	for _, port := range pc.udpPorts {
		wg.Add(1)
		port := port
		go func() { defer wg.Done(); pc.checkUDPPort(port) }()
	}

	wg.Wait()

	// FIXME: in state time calculation

	mx := make(map[string]int64)

	for _, p := range pc.tcpPorts {
		if !pc.seenTcpPorts[p.number] {
			pc.seenTcpPorts[p.number] = true
			pc.addTCPPortCharts(p)
		}

		px := fmt.Sprintf("tcp_port_%d_", p.number)

		mx[px+"current_state_duration"] = int64(p.inState)
		mx[px+"latency"] = int64(p.latency)
		mx[px+tcpPortCheckStateSuccess] = 0
		mx[px+tcpPortCheckStateTimeout] = 0
		mx[px+tcpPortCheckStateFailed] = 0
		mx[px+p.state] = 1
	}

	if pc.doUdpPorts {
		for _, p := range pc.udpPorts {
			if p.err != nil {
				if isListenOpNotPermittedError(p.err) {
					pc.doUdpPorts = false
					break
				}
				continue
			}

			if !pc.seenUdpPorts[p.number] {
				pc.seenUdpPorts[p.number] = true
				pc.addUDPPortCharts(p)
			}

			px := fmt.Sprintf("udp_port_%d_", p.number)

			mx[px+"current_status_duration"] = int64(p.inState)
			mx[px+udpPortCheckStateOpenFiltered] = 0
			mx[px+udpPortCheckStateClosed] = 0
			mx[px+p.state] = 1
		}
	}

	return mx, nil
}

func (pc *PortCheck) address(port int) string {
	// net.JoinHostPort expects literal IPv6 address, it adds []
	host := strings.Trim(pc.Host, "[]")
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func durationToMs(duration time.Duration) int {
	return int(duration) / (int(time.Millisecond) / int(time.Nanosecond))
}

func isListenOpNotPermittedError(err error) bool {
	// icmp.ListenPacket failed (socket: operation not permitted)
	var opErr *net.OpError
	return errors.As(err, &opErr) &&
		opErr.Op == "listen" &&
		strings.Contains(opErr.Error(), "operation not permitted")
}
