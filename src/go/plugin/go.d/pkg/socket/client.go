// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"
)

// Processor is a callback function passed to the Socket.Command method.
// It processes each response line received from the server.
type Processor func([]byte) (bool, error)

// Client defines an interface for socket clients, abstracting the underlying implementation.
// Implementations should provide connections for various socket types such as TCP, UDP, or Unix domain sockets.
type Client interface {
	Connect() error
	Disconnect() error
	Command(command string, process Processor) error
}

// ConnectAndRead establishes a connection using the given configuration,
// executes the provided processor function on the incoming response lines,
// and ensures the connection is properly closed after use.
func ConnectAndRead(cfg Config, process Processor) error {
	sock := New(cfg)

	if err := sock.Connect(); err != nil {
		return err
	}

	defer func() { _ = sock.Disconnect() }()

	return sock.read(process)
}

// New creates and returns a new Socket instance configured with the provided settings.
// The socket supports multiple types (TCP, UDP, UNIX), addresses (IPv4, IPv6, domain names),
// and optional TLS encryption. Connections are reused where possible.
func New(cfg Config) *Socket {
	return &Socket{Config: cfg}
}

// Socket is a concrete implementation of the Client interface, managing a network connection
// based on the specified configuration (address, type, timeout, and optional TLS settings).
type Socket struct {
	Config
	conn net.Conn
}

// Config encapsulates the settings required to establish a network connection.
type Config struct {
	Address      string
	Timeout      time.Duration
	TLSConf      *tls.Config
	MaxReadLines int64
}

// Connect establishes a connection to the specified address using the configuration details.
func (s *Socket) Connect() error {
	conn, err := s.dial()
	if err != nil {
		return fmt.Errorf("socket.Connect: %w", err)
	}

	s.conn = conn

	return nil
}

// Disconnect terminates the active connection if one exists.
func (s *Socket) Disconnect() error {
	if s.conn == nil {
		return nil
	}
	err := s.conn.Close()
	s.conn = nil
	return err
}

// Command sends a command string to the connected server and processes its response line by line
// using the provided Processor function. This method respects the timeout configuration
// for write and read operations. If a timeout or processing error occurs, it stops and returns the error.
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
		return errors.New("write: nil connection")
	}

	if err := s.conn.SetWriteDeadline(s.deadline()); err != nil {
		return err
	}

	_, err := s.conn.Write([]byte(command))

	return err
}

func (s *Socket) read(process Processor) error {
	if process == nil {
		return errors.New("read: process func is nil")
	}
	if s.conn == nil {
		return errors.New("read: nil connection")
	}

	if err := s.conn.SetReadDeadline(s.deadline()); err != nil {
		return err
	}

	sc := bufio.NewScanner(s.conn)

	var n int64
	limit := s.MaxReadLines

	for sc.Scan() {
		more, err := process(sc.Bytes())
		if err != nil {
			return err
		}
		if n++; limit > 0 && n > limit {
			return fmt.Errorf("read line limit exceeded (%d", limit)
		}
		if !more {
			break
		}
	}

	return sc.Err()
}

func (s *Socket) dial() (net.Conn, error) {
	network, address := parseAddress(s.Address)

	var d net.Dialer
	d.Timeout = s.timeout()

	if s.TLSConf != nil {
		return tls.DialWithDialer(&d, network, address, s.TLSConf)
	}
	return d.DialContext(context.Background(), network, address)
}

func (s *Socket) deadline() time.Time {
	return time.Now().Add(s.timeout())
}

func (s *Socket) timeout() time.Duration {
	if s.Timeout == 0 {
		return time.Second
	}
	return s.Timeout
}
