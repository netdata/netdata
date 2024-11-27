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

func (c *Collector) collect() (map[string]int64, error) {
	wg := &sync.WaitGroup{}

	for _, port := range c.tcpPorts {
		wg.Add(1)
		port := port
		go func() { defer wg.Done(); c.checkTCPPort(port) }()
	}

	if c.doUdpPorts {
		for _, port := range c.udpPorts {
			wg.Add(1)
			port := port
			go func() { defer wg.Done(); c.checkUDPPort(port) }()
		}
	}

	wg.Wait()

	mx := make(map[string]int64)

	now := time.Now()

	for _, p := range c.tcpPorts {
		if !c.seenTcpPorts[p.number] {
			c.seenTcpPorts[p.number] = true
			c.addTCPPortCharts(p)
		}

		px := fmt.Sprintf("tcp_port_%d_", p.number)

		mx[px+"current_state_duration"] = int64(now.Sub(p.statusChangeTs).Seconds())
		mx[px+"latency"] = int64(p.latency)
		mx[px+tcpPortCheckStateSuccess] = 0
		mx[px+tcpPortCheckStateTimeout] = 0
		mx[px+tcpPortCheckStateFailed] = 0
		mx[px+p.status] = 1
	}

	if c.doUdpPorts {
		for _, p := range c.udpPorts {
			if p.err != nil {
				if isListenOpNotPermittedError(p.err) {
					c.doUdpPorts = false
					break
				}
				continue
			}

			if !c.seenUdpPorts[p.number] {
				c.seenUdpPorts[p.number] = true
				c.addUDPPortCharts(p)
			}

			px := fmt.Sprintf("udp_port_%d_", p.number)

			mx[px+"current_status_duration"] = int64(now.Sub(p.statusChangeTs).Seconds())
			mx[px+udpPortCheckStateOpenFiltered] = 0
			mx[px+udpPortCheckStateClosed] = 0
			mx[px+p.status] = 1
		}
	}

	return mx, nil
}

func (c *Collector) address(port int) string {
	// net.JoinHostPort expects literal IPv6 address, it adds []
	host := strings.Trim(c.Host, "[]")
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
