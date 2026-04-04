// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bytes"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type frrClient struct {
	socketPath      string
	zebraSocketPath string
	timeout         time.Duration

	mu    sync.Mutex
	conns map[string]*frrConn
}

type frrConn struct {
	conn    net.Conn
	enabled bool
}

func (c *frrClient) Summary(afi, safi string) ([]byte, error) {
	cmd, err := buildFRRSummaryCommand(afi, safi)
	if err != nil {
		return nil, err
	}
	return c.execAt(c.socketPath, cmd)
}

func (c *frrClient) Neighbors() ([]byte, error) {
	return c.execAt(c.socketPath, "show bgp vrf all neighbors json")
}

func (c *frrClient) RPKICacheServers() ([]byte, error) {
	return c.execAt(c.socketPath, "show rpki cache-server json")
}

func (c *frrClient) RPKICacheConnections() ([]byte, error) {
	return c.execAt(c.socketPath, "show rpki cache-connection json")
}

func (c *frrClient) RPKIPrefixCount() ([]byte, error) {
	return c.execAt(c.socketPath, "show rpki prefix-count json")
}

func (c *frrClient) EVPNVNI() ([]byte, error) {
	return c.execAt(c.zebraSocketPath, "show evpn vni json")
}

func (c *frrClient) PeerRoutes(vrf, afi, safi, neighbor string) ([]byte, error) {
	return c.execAt(c.socketPath, buildFRRPeerCommand(vrf, afi, safi, neighbor, "routes"))
}

func (c *frrClient) PeerAdvertisedRoutes(vrf, afi, safi, neighbor string) ([]byte, error) {
	return c.execAt(c.socketPath, buildFRRPeerCommand(vrf, afi, safi, neighbor, "advertised-routes"))
}

func (c *frrClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var firstErr error
	for socketPath, state := range c.conns {
		if state == nil || state.conn == nil {
			continue
		}
		if err := state.conn.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(c.conns, socketPath)
	}

	return firstErr
}

func (c *frrClient) execAt(socketPath, cmd string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		state, err := c.conn(socketPath)
		if err != nil {
			return nil, err
		}

		data, err := c.execWithConn(state, cmd)
		if err == nil {
			return data, nil
		}

		c.closeConn(socketPath)
		if attempt == 1 {
			return nil, err
		}
	}

	return nil, fmt.Errorf("unexpected retry loop exit in execAt")
}

func (c *frrClient) conn(socketPath string) (*frrConn, error) {
	if c.conns == nil {
		c.conns = make(map[string]*frrConn)
	}

	if state := c.conns[socketPath]; state != nil && state.conn != nil {
		return state, nil
	}

	conn, err := (&net.Dialer{Timeout: c.timeout}).Dial("unix", socketPath)
	if err != nil {
		return nil, err
	}

	state := &frrConn{conn: conn}
	c.conns[socketPath] = state
	return state, nil
}

func (c *frrClient) closeConn(socketPath string) {
	state := c.conns[socketPath]
	if state == nil || state.conn == nil {
		return
	}
	_ = state.conn.Close()
	delete(c.conns, socketPath)
}

func (c *frrClient) execWithConn(state *frrConn, cmd string) ([]byte, error) {
	if err := state.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	if !state.enabled {
		if _, err := state.conn.Write([]byte("enable\x00")); err != nil {
			return nil, err
		}
		if _, err := readFRRResponse(state.conn, buf); err != nil {
			return nil, err
		}
		state.enabled = true
	}

	if _, err := state.conn.Write([]byte(cmd + "\x00")); err != nil {
		return nil, err
	}

	return readFRRResponse(state.conn, buf)
}

func readFRRResponse(conn net.Conn, buf []byte) ([]byte, error) {
	var response bytes.Buffer
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return response.Bytes(), err
		}

		response.Write(buf[:n])
		if n > 0 && buf[n-1] == 0 {
			return bytes.TrimRight(response.Bytes(), "\x00"), nil
		}
	}
}

func buildFRRSummaryCommand(afi, safi string) (string, error) {
	afi = strings.ToLower(strings.TrimSpace(afi))
	safi = strings.ToLower(strings.TrimSpace(safi))

	switch afi {
	case "ipv4", "ipv6":
		switch safi {
		case "", "unicast":
			return fmt.Sprintf("show bgp vrf all %s summary json", afi), nil
		default:
			return "", fmt.Errorf("unsupported safi %q for afi %q", safi, afi)
		}
	case "l2vpn":
		if safi != "evpn" {
			return "", fmt.Errorf("unsupported safi %q for afi %q", safi, afi)
		}
		return "show bgp vrf all l2vpn evpn summary json", nil
	default:
		return "", fmt.Errorf("unsupported afi %q", afi)
	}
}

func buildFRRPeerCommand(vrf, afi, safi, neighbor, suffix string) string {
	afi = strings.ToLower(strings.TrimSpace(afi))
	safi = strings.ToLower(strings.TrimSpace(safi))
	neighbor = strings.TrimSpace(neighbor)
	suffix = strings.TrimSpace(suffix)

	if strings.EqualFold(strings.TrimSpace(vrf), "default") {
		return fmt.Sprintf("show bgp %s %s neighbors %s %s json", afi, safi, neighbor, suffix)
	}

	return fmt.Sprintf("show bgp vrf %s %s %s neighbors %s %s json", strings.TrimSpace(vrf), afi, safi, neighbor, suffix)
}
