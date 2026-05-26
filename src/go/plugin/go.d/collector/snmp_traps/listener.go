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

const maxDatagramSize = 8192
const listenerReadErrorBackoff = 100 * time.Millisecond

type listenerEndpoint struct {
	conn *net.UDPConn
	cfg  EndpointConfig
}

type Listener struct {
	jobName   string
	endpoints []listenerEndpoint
	mu        sync.Mutex
	closed    bool
	wg        sync.WaitGroup
}

func newListener(jobName string, endpoints []EndpointConfig) (*Listener, error) {
	l := &Listener{
		jobName: jobName,
	}

	var bound []*net.UDPConn

	for i, ep := range endpoints {
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
