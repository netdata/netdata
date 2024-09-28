// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"time"
)

// Processor function passed to the Socket.Command function.
// It is passed by the caller to process a command's response line by line.
type Processor func([]byte) bool

// Client is the interface that wraps the basic socket client operations
// and hides the implementation details from the users.
// Implementations should return TCP, UDP or Unix ready sockets.
type Client interface {
	Connect() error
	Disconnect() error
	Command(command string, process Processor) error
}

func ConnectAndRead(cfg Config, process Processor) error {
	sock := New(cfg)

	if err := sock.Connect(); err != nil {
		return err
	}

	defer func() { _ = sock.Disconnect() }()

	return sock.read(process)
}

// New returns a new pointer to a socket client given the socket
// type (IP, TCP, UDP, UNIX), a network address (IP/domain:port),
// a timeout and a TLS config. It supports both IPv4 and IPv6 address
// and reuses connection where possible.
func New(cfg Config) *Socket {
	return &Socket{Config: cfg}
}

// Socket is the implementation of a socket client.
type Socket struct {
	Config
	conn net.Conn
}

// Config holds the network ip v4 or v6 address, port,
// Socket type(ip, tcp, udp, unix), timeout and TLS configuration for a Socket
type Config struct {
	Address string
	Timeout time.Duration
	TLSConf *tls.Config
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
		conn, err = net.DialTimeout(network, address, s.timeout())
	} else {
		var d net.Dialer
		d.Timeout = s.timeout()
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

	if err := s.write(command); err != nil {
		return err
	}

	return s.read(process)
}

func (s *Socket) write(command string) error {
	if s.conn == nil {
		return errors.New("attempt to write on nil connection")
	}

	if err := s.conn.SetWriteDeadline(time.Now().Add(s.timeout())); err != nil {
		return err
	}

	_, err := s.conn.Write([]byte(command))

	return err
}

func (s *Socket) read(process Processor) error {
	if process == nil {
		return errors.New("process func is nil")
	}

	if s.conn == nil {
		return errors.New("attempt to read on nil connection")
	}

	if err := s.conn.SetReadDeadline(time.Now().Add(s.timeout())); err != nil {
		return err
	}

	sc := bufio.NewScanner(s.conn)

	for sc.Scan() && process(sc.Bytes()) {
	}

	return sc.Err()
}

func (s *Socket) timeout() time.Duration {
	if s.Timeout == 0 {
		return time.Second
	}
	return s.Timeout
}
