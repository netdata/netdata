// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxDatagramSize              = 8192
	defaultListenerReceiveBuffer = 4 * 1024 * 1024
	maxListenerReceiveBuffer     = 256 * 1024 * 1024
	listenerReadErrorBackoff     = 100 * time.Millisecond
)

var setUDPReadBuffer = func(conn *net.UDPConn, bytes int) error {
	return conn.SetReadBuffer(bytes)
}

type listenerEndpoint struct {
	conn *net.UDPConn
	cfg  EndpointConfig
}

type Listener struct {
	jobName     string
	endpoints   []listenerEndpoint
	metrics     *perJobMetrics
	onReadError func(EndpointConfig, error)
	mu          sync.Mutex
	closed      bool
	wg          sync.WaitGroup
}

func newListener(jobName string, cfg ListenConfig) (*Listener, error) {
	l := &Listener{
		jobName: jobName,
	}

	var bound []*net.UDPConn

	for i, ep := range cfg.Endpoints {
		protocol := strings.ToLower(ep.Protocol)
		addr := net.JoinHostPort(ep.Address, strconv.Itoa(ep.Port))
		udpAddr, err := net.ResolveUDPAddr(protocol, addr)
		if err != nil {
			closeConns(bound)
			return nil, fmt.Errorf("endpoint %d: resolve %s: %w", i, addr, err)
		}

		conn, err := net.ListenUDP(protocol, udpAddr)
		if err != nil {
			closeConns(bound)
			return nil, fmt.Errorf("endpoint %d: bind %s: %w", i, addr, err)
		}
		if cfg.ReceiveBuffer > 0 {
			if err := setUDPReadBuffer(conn, cfg.ReceiveBuffer); err != nil {
				conn.Close()
				closeConns(bound)
				return nil, fmt.Errorf("endpoint %d: set receive buffer for %s to %d bytes: %w", i, addr, cfg.ReceiveBuffer, err)
			}
		}

		bound = append(bound, conn)
		l.endpoints = append(l.endpoints, listenerEndpoint{conn: conn, cfg: ep})
	}

	return l, nil
}

func (l *Listener) start(handler func([]byte, net.IP, *net.UDPConn, *net.UDPAddr)) {
	for i := range l.endpoints {
		ep := l.endpoints[i]
		l.wg.Add(1)
		go l.readLoop(ep, handler)
	}
}

func (l *Listener) readLoop(ep listenerEndpoint, handler func([]byte, net.IP, *net.UDPConn, *net.UDPAddr)) {
	defer l.wg.Done()

	// Keep one extra byte so oversized datagrams are classified by DecodeTrap.
	buf := make([]byte, maxDatagramSize+1)
	for {
		n, peer, err := ep.conn.ReadFromUDP(buf)
		if err != nil {
			if l.isClosed() {
				return
			}
			if l.metrics != nil {
				l.metrics.incError("listener_read_failed")
			}
			if l.onReadError != nil {
				l.onReadError(ep.cfg, err)
			}
			time.Sleep(listenerReadErrorBackoff)
			continue
		}
		var peerIP net.IP
		if peer != nil {
			peerIP = peer.IP
		}
		handler(buf[:n], peerIP, ep.conn, peer)
	}
}

func (l *Listener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *Listener) close() {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	for _, ep := range l.endpoints {
		ep.conn.Close()
	}
	l.mu.Unlock()
	l.wg.Wait()
}

func closeConns(conns []*net.UDPConn) {
	for _, c := range conns {
		c.Close()
	}
}

func listenerEndpointLogName(ep EndpointConfig) string {
	protocol := strings.ToLower(ep.Protocol)
	return protocol + "://" + net.JoinHostPort(ep.Address, strconv.Itoa(ep.Port))
}
