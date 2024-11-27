// SPDX-License-Identifier: GPL-3.0-or-later

package portcheck

import (
	"errors"
	"fmt"
	"net"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	udpPortCheckStateOpenFiltered = "open_filtered"
	udpPortCheckStateClosed       = "closed"
)

type udpPort struct {
	number         int
	status         string
	statusChangeTs time.Time

	err error
}

func (c *Collector) checkUDPPort(port *udpPort) {
	port.err = nil

	timeout := time.Duration(max(float64(100*time.Millisecond), float64(c.Timeout.Duration())*0.7))
	addr := c.address(port.number)

	open, err := c.scanUDP(addr, timeout)
	if err != nil {
		c.Warningf("UDP port check failed for '%s': %v", addr, err)
		port.err = err
		return
	}

	state := udpPortCheckStateOpenFiltered
	if !open {
		state = udpPortCheckStateClosed
	}

	c.setUDPPortCheckState(port, state)
}

func (c *Collector) setUDPPortCheckState(port *udpPort, state string) {
	if port.status != state {
		port.status = state
		port.statusChangeTs = time.Now()
	} else if port.statusChangeTs.IsZero() {
		port.statusChangeTs = time.Now()
	}
}

func scanUDPPort(address string, timeout time.Duration) (bool, error) {
	// With this scan type, we send 0-byte UDP packets to the port on the target system.
	// Receipt of an ICMP Destination Unreachable message signifies the port is closed;
	// otherwise it is assumed open (timeout).
	// This is equivalent to "close"/"open/filtered" states reported by nmap.

	raddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return false, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	network, icmpNetwork, icmpProto := getUDPNetworkParams(raddr.IP)

	udpConn, err := net.DialUDP(network, nil, raddr)
	if err != nil {
		return false, fmt.Errorf("failed to open UDP connection to '%s': %w", raddr.String(), err)
	}
	defer func() { _ = udpConn.Close() }()

	icmpConn, err := icmp.ListenPacket(icmpNetwork, "")
	if err != nil {
		return false, fmt.Errorf("failed to listen for ICMP packets: %w", err)
	}
	defer func() { _ = icmpConn.Close() }()

	if _, err = udpConn.Write([]byte{}); err != nil {
		return false, fmt.Errorf("failed to send UDP packet: %w", err)
	}

	return readICMPResponse(icmpConn, udpConn, icmpProto, timeout)
}

func readICMPResponse(icmpConn *icmp.PacketConn, udpConn *net.UDPConn, icmpProto int, timeout time.Duration) (bool, error) {
	buff := make([]byte, 1500)

	if err := icmpConn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return false, fmt.Errorf("failed to set read deadline on ICMP connection: %w", err)
	}

	localPort := uint16(udpConn.LocalAddr().(*net.UDPAddr).Port)

	for {
		n, _, err := icmpConn.ReadFrom(buff)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return false, fmt.Errorf("ICMP connection closed unexpectedly")
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return true, nil // Timeout means no ICMP response, assume port is open
			}
			return false, fmt.Errorf("failed to read ICMP packet: %w", err)
		}

		if n == 0 {
			continue
		}

		msg, err := icmp.ParseMessage(icmpProto, buff[:n])
		if err != nil {
			return false, fmt.Errorf("failed to parse ICMP message: %w", err)
		}

		if msg.Type != ipv4.ICMPTypeDestinationUnreachable && msg.Type != ipv6.ICMPTypeDestinationUnreachable {
			continue
		}

		body, ok := msg.Body.(*icmp.DstUnreach)
		if !ok {
			continue
		}

		srcPort, err := extractSourcePort(msg.Type, body.Data)
		if err != nil {
			return false, err
		}

		if srcPort == localPort {
			return false, nil // Received ICMP Destination Unreachable, port is closed
		}
	}
}

func getUDPNetworkParams(ip net.IP) (network, icmpNetwork string, icmpProto int) {
	if ip.To4() != nil {
		return "udp4", "ip4:icmp", 1
	}
	return "udp6", "ip6:ipv6-icmp", 58
}

func extractSourcePort(msgType icmp.Type, data []byte) (uint16, error) {
	const udpHeaderLen = 8
	var headerLen, minLen int

	switch msgType {
	case ipv4.ICMPTypeDestinationUnreachable:
		headerLen, minLen = ipv4.HeaderLen, ipv4.HeaderLen+udpHeaderLen
	case ipv6.ICMPTypeDestinationUnreachable:
		headerLen, minLen = ipv6.HeaderLen, ipv6.HeaderLen+udpHeaderLen
	default:
		return 0, fmt.Errorf("unexpected ICMP message type: %v", msgType)
	}

	if len(data) < minLen {
		return 0, fmt.Errorf("ICMP message too short: want %d got %d", minLen, len(data))
	}

	return (uint16(data[headerLen]) << udpHeaderLen) | uint16(data[headerLen+1]), nil
}
