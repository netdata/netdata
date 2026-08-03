// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxDatagramSize          = 8192
	listenerReadErrorBackoff = 100 * time.Millisecond
)

var setUDPReadBuffer = func(conn *net.UDPConn, bytes int) error {
	return conn.SetReadBuffer(bytes)
}

type listenerEndpoint struct {
	conn *net.UDPConn
	cfg  Endpoint
}

type listenerReceiveBufferWarning struct {
	endpoint  Endpoint
	requested int
	err       error
}

type listener struct {
	endpoints             []listenerEndpoint
	receiveBufferWarnings []listenerReceiveBufferWarning
	report                Reporter
	mu                    sync.Mutex
	closed                bool
	wg                    sync.WaitGroup
}

func newListener(cfg ListenConfig, report Reporter) (*listener, error) {
	l := &listener{report: report}

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
				if cfg.ReceiveBuffer != DefaultReceiveBuffer {
					conn.Close()
					closeConns(bound)
					return nil, fmt.Errorf("endpoint %d: set receive buffer for %s to %d bytes: %w", i, addr, cfg.ReceiveBuffer, err)
				}
				l.receiveBufferWarnings = append(l.receiveBufferWarnings, listenerReceiveBufferWarning{
					endpoint:  ep,
					requested: cfg.ReceiveBuffer,
					err:       err,
				})
			}
		}

		bound = append(bound, conn)
		l.endpoints = append(l.endpoints, listenerEndpoint{conn: conn, cfg: ep})
	}

	return l, nil
}

func (l *listener) start(handler func(Datagram)) {
	for i := range l.endpoints {
		ep := l.endpoints[i]
		l.wg.Add(1)
		go l.readLoop(ep, handler)
	}
}

func (l *listener) readLoop(ep listenerEndpoint, handler func(Datagram)) {
	defer l.wg.Done()

	// Keep one extra byte so oversized datagrams are classified by decodeTrap.
	buf := make([]byte, maxDatagramSize+1)
	for {
		n, peer, err := ep.conn.ReadFromUDP(buf)
		if err != nil {
			if l.isClosed() {
				return
			}
			l.reportEvent(Event{Type: EventListenerReadFailed, Endpoint: ep.cfg, Err: err})
			time.Sleep(listenerReadErrorBackoff)
			continue
		}
		var peerIP net.IP
		if peer != nil {
			peerIP = peer.IP
		}
		handler(Datagram{Data: buf[:n], PeerIP: peerIP, Conn: ep.conn, Peer: peer})
	}
}

func (l *listener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *listener) close() {
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

func (l *listener) reportEvent(event Event) {
	if l.report != nil {
		l.report(event)
	}
}
