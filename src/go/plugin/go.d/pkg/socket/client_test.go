// SPDX-License-Identifier: GPL-3.0-or-later

package socket

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testServerAddress     = "127.0.0.1:9999"
	testUdpServerAddress  = "udp://127.0.0.1:9999"
	testUnixServerAddress = "/tmp/testSocketFD"
	defaultTimeout        = 100 * time.Millisecond
)

var tcpConfig = Config{
	Address:        testServerAddress,
	ConnectTimeout: defaultTimeout,
	ReadTimeout:    defaultTimeout,
	WriteTimeout:   defaultTimeout,
	TLSConf:        nil,
}

var udpConfig = Config{
	Address:        testUdpServerAddress,
	ConnectTimeout: defaultTimeout,
	ReadTimeout:    defaultTimeout,
	WriteTimeout:   defaultTimeout,
	TLSConf:        nil,
}

var unixConfig = Config{
	Address:        testUnixServerAddress,
	ConnectTimeout: defaultTimeout,
	ReadTimeout:    defaultTimeout,
	WriteTimeout:   defaultTimeout,
	TLSConf:        nil,
}

var tcpTlsConfig = Config{
	Address:        testServerAddress,
	ConnectTimeout: defaultTimeout,
	ReadTimeout:    defaultTimeout,
	WriteTimeout:   defaultTimeout,
	TLSConf:        &tls.Config{},
}

func Test_clientCommand(t *testing.T) {
	srv := &tcpServer{addr: testServerAddress, rowsNumResp: 1}
	go func() { _ = srv.Run(); defer func() { _ = srv.Close() }() }()

	time.Sleep(time.Millisecond * 100)
	sock := New(tcpConfig)
	require.NoError(t, sock.Connect())
	err := sock.Command("ping\n", func(bytes []byte) bool {
		assert.Equal(t, "pong", string(bytes))
		return true
	})
	require.NoError(t, sock.Disconnect())
	require.NoError(t, err)
}

func Test_clientTimeout(t *testing.T) {
	srv := &tcpServer{addr: testServerAddress, rowsNumResp: 1}
	go func() { _ = srv.Run() }()

	time.Sleep(time.Millisecond * 100)
	sock := New(tcpConfig)
	require.NoError(t, sock.Connect())
	sock.ReadTimeout = 0
	sock.ReadTimeout = 0
	err := sock.Command("ping\n", func(bytes []byte) bool {
		assert.Equal(t, "pong", string(bytes))
		return true
	})
	require.Error(t, err)
}

func Test_clientIncompleteSSL(t *testing.T) {
	srv := &tcpServer{addr: testServerAddress, rowsNumResp: 1}
	go func() { _ = srv.Run() }()

	time.Sleep(time.Millisecond * 100)
	sock := New(tcpTlsConfig)
	err := sock.Connect()
	require.Error(t, err)
}

func Test_clientCommandStopProcessing(t *testing.T) {
	srv := &tcpServer{addr: testServerAddress, rowsNumResp: 2}
	go func() { _ = srv.Run() }()

	time.Sleep(time.Millisecond * 100)
	sock := New(tcpConfig)
	require.NoError(t, sock.Connect())
	err := sock.Command("ping\n", func(bytes []byte) bool {
		assert.Equal(t, "pong", string(bytes))
		return false
	})
	require.NoError(t, sock.Disconnect())
	require.NoError(t, err)
}

func Test_clientUDPCommand(t *testing.T) {
	srv := &udpServer{addr: testServerAddress, rowsNumResp: 1}
	go func() { _ = srv.Run(); defer func() { _ = srv.Close() }() }()

	time.Sleep(time.Millisecond * 100)
	sock := New(udpConfig)
	require.NoError(t, sock.Connect())
	err := sock.Command("ping\n", func(bytes []byte) bool {
		assert.Equal(t, "pong", string(bytes))
		return false
	})
	require.NoError(t, sock.Disconnect())
	require.NoError(t, err)
}

func Test_clientTCPAddress(t *testing.T) {
	srv := &tcpServer{addr: testServerAddress, rowsNumResp: 1}
	go func() { _ = srv.Run() }()
	time.Sleep(time.Millisecond * 100)

	sock := New(tcpConfig)
	require.NoError(t, sock.Connect())

	tcpConfig.Address = "tcp://" + tcpConfig.Address
	sock = New(tcpConfig)
	require.NoError(t, sock.Connect())
}

func Test_clientUnixCommand(t *testing.T) {
	srv := &unixServer{addr: testUnixServerAddress, rowsNumResp: 1}
	// cleanup previous file descriptors
	_ = srv.Close()
	go func() { _ = srv.Run() }()

	time.Sleep(time.Millisecond * 200)
	sock := New(unixConfig)
	require.NoError(t, sock.Connect())
	err := sock.Command("ping\n", func(bytes []byte) bool {
		assert.Equal(t, "pong", string(bytes))
		return false
	})
	require.NoError(t, err)
	require.NoError(t, sock.Disconnect())
}

func Test_clientEmptyProcessFunc(t *testing.T) {
	srv := &tcpServer{addr: testServerAddress, rowsNumResp: 1}
	go func() { _ = srv.Run() }()

	time.Sleep(time.Millisecond * 100)
	sock := New(tcpConfig)
	require.NoError(t, sock.Connect())
	err := sock.Command("ping\n", nil)
	require.Error(t, err, "nil process func should return an error")
}
