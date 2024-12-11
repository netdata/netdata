// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

func newTCPServer(addr string) *tcpServer {
	ctx, cancel := context.WithCancel(context.Background())
	_, addr = parseAddress(addr)
	return &tcpServer{
		addr:   addr,
		ctx:    ctx,
		cancel: cancel,
	}
}

type tcpServer struct {
	addr     string
	listener net.Listener
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

func (t *tcpServer) Run() error {
	var err error
	t.listener, err = net.Listen("tcp", t.addr)
	if err != nil {
		return fmt.Errorf("failed to start TCP server: %w", err)
	}
	return t.handleConnections()
}

func (t *tcpServer) Close() (err error) {
	t.cancel()
	if t.listener != nil {
		if err := t.listener.Close(); err != nil {
			return fmt.Errorf("failed to close TCP server: %w", err)
		}
	}
	t.wg.Wait()
	return nil
}

func (t *tcpServer) handleConnections() (err error) {
	for {
		select {
		case <-t.ctx.Done():
			return nil
		default:
			conn, err := t.listener.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return fmt.Errorf("could not accept connection: %v", err)
			}
			t.wg.Add(1)
			go func() {
				defer t.wg.Done()
				t.handleConnection(conn)
			}()
		}
	}
}

func (t *tcpServer) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	if _, err := rw.ReadString('\n'); err != nil {
		writeResponse(rw, fmt.Sprintf("failed to read input: %v\n", err))
	} else {
		writeResponse(rw, "pong\n")
	}
}

func newUDPServer(addr string) *udpServer {
	ctx, cancel := context.WithCancel(context.Background())
	_, addr = parseAddress(addr)
	return &udpServer{
		addr:   addr,
		ctx:    ctx,
		cancel: cancel,
	}
}

type udpServer struct {
	addr   string
	conn   *net.UDPConn
	ctx    context.Context
	cancel context.CancelFunc
}

func (u *udpServer) Run() error {
	addr, err := net.ResolveUDPAddr("udp", u.addr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	u.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to start UDP server: %w", err)
	}

	return u.handleConnections()
}

func (u *udpServer) Close() (err error) {
	u.cancel()
	if u.conn != nil {
		if err := u.conn.Close(); err != nil {
			return fmt.Errorf("failed to close UDP server: %w", err)
		}
	}
	return nil
}

func (u *udpServer) handleConnections() error {
	buffer := make([]byte, 8192)
	for {
		select {
		case <-u.ctx.Done():
			return nil
		default:
			if err := u.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				continue
			}

			_, addr, err := u.conn.ReadFromUDP(buffer[0:])
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					return fmt.Errorf("failed to read UDP packet: %w", err)
				}
				continue
			}

			if _, err := u.conn.WriteToUDP([]byte("pong\n"), addr); err != nil {
				return fmt.Errorf("failed to write UDP response: %w", err)
			}
		}
	}
}

func newUnixServer(addr string) *unixServer {
	ctx, cancel := context.WithCancel(context.Background())
	_, addr = parseAddress(addr)
	return &unixServer{
		addr:   addr,
		ctx:    ctx,
		cancel: cancel,
	}
}

type unixServer struct {
	addr     string
	listener *net.UnixListener
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

func (u *unixServer) Run() error {
	if err := os.Remove(u.addr); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clean up existing socket: %w", err)
	}

	addr, err := net.ResolveUnixAddr("unix", u.addr)
	if err != nil {
		return fmt.Errorf("failed to resolve Unix address: %w", err)
	}

	u.listener, err = net.ListenUnix("unix", addr)
	if err != nil {
		return fmt.Errorf("failed to start Unix server: %w", err)
	}

	return u.handleConnections()
}

func (u *unixServer) Close() error {
	u.cancel()

	if u.listener != nil {
		if err := u.listener.Close(); err != nil {
			return fmt.Errorf("failed to close Unix server: %w", err)
		}
	}

	u.wg.Wait()
	_ = os.Remove(u.addr)

	return nil
}

func (u *unixServer) handleConnections() error {
	for {
		select {
		case <-u.ctx.Done():
			return nil
		default:
			if err := u.listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
				continue
			}

			conn, err := u.listener.AcceptUnix()
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					return err
				}
				continue
			}

			u.wg.Add(1)
			go func() {
				defer u.wg.Done()
				u.handleConnection(conn)
			}()
		}
	}
}

func (u *unixServer) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		return
	}

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	if _, err := rw.ReadString('\n'); err != nil {
		writeResponse(rw, fmt.Sprintf("failed to read input: %v\n", err))
	} else {
		writeResponse(rw, "pong\n")
	}

}

func writeResponse(rw *bufio.ReadWriter, response string) {
	_, _ = rw.WriteString(response)
	_ = rw.Flush()
}
