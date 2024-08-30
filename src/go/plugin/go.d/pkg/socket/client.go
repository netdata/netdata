// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"time"
)

// New returns a new pointer to a socket client given the socket
// type (IP, TCP, UDP, UNIX), a network address (IP/domain:port),
// a timeout and a TLS config. It supports both IPv4 and IPv6 address
// and reuses connection where possible.
func New(config Config) *Socket {
	return &Socket{
		Config: config,
		conn:   nil,
	}
}

// Socket is the implementation of a socket client.
type Socket struct {
	Config
	conn net.Conn
}

// Connect connects to the Socket address on the named network.
// If the address is a domain name it will also perform the DNS resolution.
// Address like :80 will attempt to connect to the localhost.
// The config timeout and TLS config will be used.
func (s *Socket) Connect() error {
	network, address := networkType(s.Address)
	var conn net.Conn
	var err error

	if s.TLSConf == nil {
		conn, err = net.DialTimeout(network, address, s.ConnectTimeout)
	} else {
		var d net.Dialer
		d.Timeout = s.ConnectTimeout
		conn, err = tls.DialWithDialer(&d, network, address, s.TLSConf)
	}
	if err != nil {
		return err
	}

	s.conn = conn

	return nil
}

// Disconnect closes the connection.
// Any in-flight commands will be cancelled and return errors.
func (s *Socket) Disconnect() (err error) {
	if s.conn != nil {
		err = s.conn.Close()
		s.conn = nil
	}
	return err
}

// Command writes the command string to the connection and passed the
// response bytes line by line to the process function. It uses the
// timeout value from the Socket config and returns read, write and
// timeout errors if any. If a timeout occurs during the processing
// of the responses this function will stop processing and return a
// timeout error.
func (s *Socket) Command(command string, process Processor) error {
	if s.conn == nil {
		return errors.New("cannot send command on nil connection")
	}
	if err := write(command, s.conn, s.WriteTimeout); err != nil {
		return err
	}
	return read(s.conn, process, s.ReadTimeout)
}

func write(command string, writer net.Conn, timeout time.Duration) error {
	if writer == nil {
		return errors.New("attempt to write on nil connection")
	}
	if err := writer.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	_, err := writer.Write([]byte(command))
	return err
}

func read(reader net.Conn, process Processor, timeout time.Duration) error {
	if process == nil {
		return errors.New("process func is nil")
	}
	if reader == nil {
		return errors.New("attempt to read on nil connection")
	}
	if err := reader.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() && process(scanner.Bytes()) {
	}
	return scanner.Err()
}
