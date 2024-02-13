// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"crypto/tls"
	"time"
)

// Processor function passed to the Socket.Command function.
// It is passed by the caller to process a command's response
// line by line.
type Processor func([]byte) bool

// Client is the interface that wraps the basic socket client operations
// and hides the implementation details from the users.
//
// Connect should prepare the connection.
//
// Disconnect should stop any in-flight connections.
//
// Command should send the actual data to the wire and pass
// any results to the processor function.
//
// Implementations should return TCP, UDP or Unix ready sockets.
type Client interface {
	Connect() error
	Disconnect() error
	Command(command string, process Processor) error
}

// Config holds the network ip v4 or v6 address, port,
// Socket type(ip, tcp, udp, unix), timeout and TLS configuration
// for a Socket
type Config struct {
	Address        string
	ConnectTimeout time.Duration
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	TLSConf        *tls.Config
}
