// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dataBIRDProtocolsAllAccessDenied []byte

func TestBIRDClient_ProtocolsAll(t *testing.T) {
	server := newBIRDReplayServer(t, dataBIRDProtocolsAllMultichannel)

	client := &birdClient{
		socketPath: server.socketPath,
		timeout:    time.Second,
	}

	data, err := client.ProtocolsAll()
	require.NoError(t, err)
	assert.Equal(t, stripBIRDTerminalReply(dataBIRDProtocolsAllMultichannel), data)
	assert.Equal(t, []string{"show protocols all"}, server.commands())
	server.assertNoError(t)
}

func TestBIRDClient_ProtocolsAllAccessDenied(t *testing.T) {
	server := newBIRDReplayServer(t, dataBIRDProtocolsAllAccessDenied)

	client := &birdClient{
		socketPath: server.socketPath,
		timeout:    time.Second,
	}

	_, err := client.ProtocolsAll()
	require.Error(t, err)
	assert.ErrorIs(t, err, fs.ErrPermission)
	assert.Equal(t, []string{"show protocols all"}, server.commands())
	server.assertNoError(t)
}

type birdReplayServer struct {
	socketPath string
	listener   net.Listener
	response   []byte
	keepAlive  bool

	mu   sync.Mutex
	seen []string

	connections int

	errCh chan error
}

func newBIRDReplayServer(t *testing.T, response []byte) *birdReplayServer {
	return newBIRDReplayServerWithOptions(t, response, false)
}

func newBIRDReplayServerPersistent(t *testing.T, response []byte) *birdReplayServer {
	return newBIRDReplayServerWithOptions(t, response, true)
}

func newBIRDReplayServerWithOptions(t *testing.T, response []byte, keepAlive bool) *birdReplayServer {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "bird.ctl")
	ln, err := net.Listen("unix", socketPath)
	require.NoError(t, err)

	srv := &birdReplayServer{
		socketPath: socketPath,
		listener:   ln,
		response:   response,
		keepAlive:  keepAlive,
		errCh:      make(chan error, 1),
	}

	go srv.serve()

	t.Cleanup(func() {
		_ = ln.Close()
		srv.assertNoError(t)
	})

	return srv
}

func (s *birdReplayServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			s.recordErr(err)
			return
		}
		s.recordConnection()
		go s.handleConn(conn)
	}
}

func (s *birdReplayServer) handleConn(conn net.Conn) {
	defer conn.Close()

	if _, err := conn.Write([]byte("0001 BIRD 2.18 ready.\n")); err != nil {
		s.recordErr(err)
		return
	}

	for {
		command, err := readLine(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			s.recordErr(err)
			return
		}
		s.recordCommand(command)

		if _, err := conn.Write(s.response); err != nil {
			s.recordErr(err)
			return
		}
		if !s.keepAlive {
			return
		}
	}
}

func (s *birdReplayServer) recordCommand(command string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seen = append(s.seen, command)
}

func (s *birdReplayServer) commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.seen...)
}

func (s *birdReplayServer) recordConnection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections++
}

func (s *birdReplayServer) connectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connections
}

func (s *birdReplayServer) recordErr(err error) {
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *birdReplayServer) assertNoError(t *testing.T) {
	t.Helper()

	select {
	case err := <-s.errCh:
		require.NoError(t, err)
	default:
	}
}

func readLine(conn net.Conn) (string, error) {
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func stripBIRDTerminalReply(data []byte) []byte {
	return bytes.TrimSuffix(data, []byte("\n0000 OK\n"))
}

func TestBIRDClient_ProtocolsAllReusesConnection(t *testing.T) {
	server := newBIRDReplayServerPersistent(t, dataBIRDProtocolsAllMultichannel)

	client := &birdClient{
		socketPath: server.socketPath,
		timeout:    time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	for i := 0; i < 2; i++ {
		data, err := client.ProtocolsAll()
		require.NoError(t, err)
		assert.Equal(t, stripBIRDTerminalReply(dataBIRDProtocolsAllMultichannel), data)
	}

	assert.Equal(t, []string{"show protocols all", "show protocols all"}, server.commands())
	assert.Equal(t, 1, server.connectionCount())
	server.assertNoError(t)
}

func TestBIRDClient_ProtocolsAllReconnectsAfterServerClose(t *testing.T) {
	server := newBIRDReplayServer(t, dataBIRDProtocolsAllMultichannel)

	client := &birdClient{
		socketPath: server.socketPath,
		timeout:    time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	for i := 0; i < 2; i++ {
		data, err := client.ProtocolsAll()
		require.NoError(t, err)
		assert.Equal(t, stripBIRDTerminalReply(dataBIRDProtocolsAllMultichannel), data)
	}

	assert.Equal(t, []string{"show protocols all", "show protocols all"}, server.commands())
	assert.Equal(t, 2, server.connectionCount())
	server.assertNoError(t)
}
