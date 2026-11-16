// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenReleasesBoundSocketsOnFailure(t *testing.T) {
	free := freeAddress(t)
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()

	_, err = Listen(t.Context(), Config{
		Endpoints: []Endpoint{
			{Network: "udp", Address: free, Name: "first"},
			{Network: "tcp", Address: free, Name: "second"},
			{Network: "tcp", Address: occupied.Addr().String(), Name: "third"},
		},
		Stats: &Stats{},
	})
	require.ErrorContains(t, err, "third: ")
	requireFree(t, "udp", free)
	requireFree(t, "tcp", free)
}

func TestPermanentListenerLossFailsServer(t *testing.T) {
	for name, lose := range map[string]func(*Server) error{
		"udp": func(s *Server) error { return s.udp[0].Close() },
		"tcp": func(s *Server) error { return s.tcp[0].Close() },
	} {
		t.Run(name, func(t *testing.T) {
			f := start(t, nil)
			client := f.dialTCP(t)
			_, err := client.Write([]byte("held:10|g\n"))
			require.NoError(t, err)
			f.waitFraming(t, framing{
				records: []string{"held:10|g"},
			})

			require.NoError(t, lose(f.s))
			select {
			case <-f.s.Failed():
			case <-time.After(waitFor):
				t.Fatal("listener loss did not fail the server")
			}
			f.close()
			require.Error(t, f.s.Err())
			assert.Contains(t, f.s.Err().Error(), name+" listener")
			require.NoError(t, client.SetReadDeadline(time.Now().Add(waitFor)))
			_, err = client.Read(make([]byte, 1))
			require.Error(t, err, "peers are closed with the failed server")
			requireFree(t, "udp", f.addr)
			requireFree(t, "tcp", f.addr)
		})
	}
}

func TestReaderPanicFailsOnlyTheServer(t *testing.T) {
	f := start(t, func(cfg *Config) {
		cfg.Ingest = func(record string, _ time.Time) {
			var none []int
			_ = none[len(record)] // A runtime error panic from the ingest function.
		}
	})
	conn := f.dialTCP(t)
	_, err := conn.Write([]byte("x:1|c\n"))
	require.NoError(t, err)
	select {
	case <-f.s.Failed():
	case <-time.After(waitFor):
		t.Fatal("reader panic did not fail the server")
	}
	f.close()
	require.ErrorContains(t, f.s.Err(), "tcp listener "+f.addr+" client: receiver panic")
	var cause runtime.Error
	require.ErrorAs(t, f.s.Err(), &cause, "an error panic value stays in the chain")
	requireFree(t, "udp", f.addr)
	requireFree(t, "tcp", f.addr)
}

type temporaryFailingListener struct {
	net.Listener
	failures atomic.Int32
}

func (l *temporaryFailingListener) Accept() (net.Conn, error) {
	if l.failures.Add(-1) >= 0 {
		return nil, &net.OpError{
			Op:  "accept",
			Net: "tcp",
			Err: os.NewSyscallError("accept", syscall.EMFILE),
		}
	}
	return l.Listener.Accept()
}

type temporaryFailingPacketConn struct {
	net.PacketConn
	failures atomic.Int32
}

func (c *temporaryFailingPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	if c.failures.Add(-1) >= 0 {
		return 0, nil, &net.OpError{
			Op:  "read",
			Net: "udp",
			Err: os.NewSyscallError("recvfrom", syscall.EMFILE),
		}
	}
	return c.PacketConn.ReadFrom(p)
}

func TestTemporaryListenerErrorsKeepServing(t *testing.T) {
	f := bind(t, nil)
	udp := &temporaryFailingPacketConn{
		PacketConn: f.s.udp[0],
	}
	udp.failures.Store(2)
	tcp := &temporaryFailingListener{
		Listener: f.s.tcp[0],
	}
	tcp.failures.Store(2)
	f.s.udp[0], f.s.tcp[0] = udp, tcp
	f.s.Start()

	client := f.dialTCP(t)
	_, err := client.Write([]byte("tcp:1|c\n"))
	require.NoError(t, err)
	f.waitFraming(t, framing{
		records: []string{"tcp:1|c"},
	})
	f.sendUDP(t, "udp:1|c")
	f.waitFraming(t, framing{
		records: []string{"tcp:1|c", "udp:1|c"},
	})
	select {
	case <-f.s.Failed():
		t.Fatalf("temporary errors failed the server: %v", f.s.Err())
	default:
	}
	f.close()
	require.NoError(t, f.s.Err())
}
