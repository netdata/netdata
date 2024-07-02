// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type tcpServer struct {
	addr        string
	server      net.Listener
	rowsNumResp int
}

func (t *tcpServer) Run() (err error) {
	t.server, err = net.Listen("tcp", t.addr)
	if err != nil {
		return
	}
	return t.handleConnections()
}

func (t *tcpServer) Close() (err error) {
	return t.server.Close()
}

func (t *tcpServer) handleConnections() (err error) {
	for {
		conn, err := t.server.Accept()
		if err != nil || conn == nil {
			return errors.New("could not accept connection")
		}
		t.handleConnection(conn)
	}
}

func (t *tcpServer) handleConnection(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(time.Millisecond * 100))

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	_, err := rw.ReadString('\n')
	if err != nil {
		_, _ = rw.WriteString("failed to read input")
		_ = rw.Flush()
	} else {
		resp := strings.Repeat("pong\n", t.rowsNumResp)
		_, _ = rw.WriteString(resp)
		_ = rw.Flush()
	}
}

type udpServer struct {
	addr        string
	conn        *net.UDPConn
	rowsNumResp int
}

func (u *udpServer) Run() (err error) {
	addr, err := net.ResolveUDPAddr("udp", u.addr)
	if err != nil {
		return err
	}
	u.conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return
	}
	u.handleConnections()
	return nil
}

func (u *udpServer) Close() (err error) {
	return u.conn.Close()
}

func (u *udpServer) handleConnections() {
	for {
		var buf [2048]byte
		_, addr, _ := u.conn.ReadFromUDP(buf[0:])
		resp := strings.Repeat("pong\n", u.rowsNumResp)
		_, _ = u.conn.WriteToUDP([]byte(resp), addr)
	}
}

type unixServer struct {
	addr        string
	conn        *net.UnixListener
	rowsNumResp int
}

func (u *unixServer) Run() (err error) {
	_, _ = os.CreateTemp("/tmp", "testSocketFD")
	addr, err := net.ResolveUnixAddr("unix", u.addr)
	if err != nil {
		return err
	}
	u.conn, err = net.ListenUnix("unix", addr)
	if err != nil {
		return
	}
	go u.handleConnections()
	return nil
}

func (u *unixServer) Close() (err error) {
	_ = os.Remove(testUnixServerAddress)
	return u.conn.Close()
}

func (u *unixServer) handleConnections() {
	var conn net.Conn
	var err error
	conn, err = u.conn.AcceptUnix()
	if err != nil {
		panic(fmt.Errorf("could not accept connection: %v", err))
	}
	u.handleConnection(conn)
}

func (u *unixServer) handleConnection(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(time.Second))

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	_, err := rw.ReadString('\n')
	if err != nil {
		_, _ = rw.WriteString("failed to read input")
		_ = rw.Flush()
	} else {
		resp := strings.Repeat("pong\n", u.rowsNumResp)
		_, _ = rw.WriteString(resp)
		_ = rw.Flush()
	}
}
