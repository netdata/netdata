// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
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

func TestRunAcquiresOnlyAfterPreparation(t *testing.T) {
	addr := freeAddress(t)
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	f := prepareFixture(t, c)
	// Init and Check validated and prepared everything without binding.
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() {
		done <- c.Run(ctx, func() {
			// Readiness follows acquisition of every required listener, before traffic.
			for _, protocol := range []string{protocolUDP, protocolTCP} {
				var err error
				if protocol == protocolUDP {
					_, err = net.ListenPacket(protocol, addr)
				} else {
					_, err = net.Listen(protocol, addr)
				}
				assert.Error(t, err, "%s must be bound at readiness", protocol)
			}
			close(ready)
		})
	}()
	select {
	case <-ready:
	case <-time.After(waitFor):
		t.Fatal("not ready")
	}
	f.cycle.BeginCycle()
	require.NoError(t, c.Collect(context.Background()))
	f.cycle.AbortCycle()

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err, "requested stop is a normal return")
	case <-time.After(waitFor):
		t.Fatal("Run did not return")
	}
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
}

func TestRunStartupFailureReleasesAcquiredListeners(t *testing.T) {
	free := freeAddress(t)
	occupied, err := net.Listen(protocolTCP, "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()

	c := New()
	c.Listeners = []ListenerConfig{
		{Protocol: protocolUDP, Address: free},
		{Protocol: protocolTCP, Address: free},
		{Protocol: protocolTCP, Address: occupied.Addr().String()},
	}
	c.profileDirs = nil
	prepareFixture(t, c)
	readyCalled := false
	err = c.Run(context.Background(), func() { readyCalled = true })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listeners[2]")
	assert.False(t, readyCalled)
	requireFree(t, protocolUDP, free)
	requireFree(t, protocolTCP, free)
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
}

func TestConfigurationTestDuringIncumbentReception(t *testing.T) {
	incumbent := startRuntime(t, nil)
	incumbent.sendUDP(t, "before:1|c")
	incumbent.waitCounts(t, receiverCounts{
		accepted: 1,
	})

	// DynCfg test constructs, configures, runs Init and Check, then Cleanup; never Run.
	probe := New()
	probe.Config = incumbent.c.Config
	probe.profileDirs = nil
	require.NoError(t, probe.Init(context.Background()))
	require.NoError(t, probe.Check(context.Background()))
	probe.Cleanup(context.Background())

	incumbent.sendUDP(t, "after:1|c")
	incumbent.waitCounts(t, receiverCounts{
		accepted: 2,
	})
	incumbent.collect(t, false, false)
	value(t, incumbent.c, "c.total.before", 1, nil)
	value(t, incumbent.c, "c.total.after", 1, nil)
}

func TestCancellationClosesClientsAndJoinsReaders(t *testing.T) {
	f := startRuntime(t, nil)
	conn := f.dialTCP(t)
	_, err := conn.Write([]byte("open:1|c\npartial"))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 1,
	})

	require.NoError(t, f.stop(t))
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(waitFor)))
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err, "shutdown closes established clients")
	assert.Zero(t, f.c.diagnostics.tcpConnections.Load())
	requireFree(t, protocolUDP, f.addr)
	requireFree(t, protocolTCP, f.addr)
	require.ErrorIs(t, f.c.Collect(context.Background()), rejectUnavailable)
}

func TestPermanentListenerLossStopsReceiver(t *testing.T) {
	for name, lose := range map[string]func(*server) error{
		"udp": func(s *server) error { return s.udp[0].Close() },
		"tcp": func(s *server) error { return s.tcp[0].Close() },
	} {
		t.Run(name, func(t *testing.T) {
			addr := freeAddress(t)
			c := New()
			c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
			c.profileDirs = nil
			f := prepareFixture(t, c)
			s, err := c.listen(context.Background())
			require.NoError(t, err)
			done := make(chan error, 1)
			ready := make(chan struct{})
			go func() { done <- c.serve(context.Background(), s, func() { close(ready) }) }()
			<-ready
			client, err := net.Dial(protocolTCP, addr)
			require.NoError(t, err)
			defer client.Close()
			_, err = client.Write([]byte("held:10|g\n"))
			require.NoError(t, err)
			f.waitCounts(t, receiverCounts{
				accepted: 1,
			})

			require.NoError(t, lose(s))
			select {
			case err := <-done:
				require.Error(t, err)
				assert.Contains(t, err.Error(), name+" listener")
			case <-time.After(waitFor):
				t.Fatal("listener loss did not end the receiver")
			}
			// No held gauge or empty interval can be published after failure.
			f.cycle.BeginCycle()
			require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
			f.cycle.AbortCycle()
			require.NoError(t, client.SetReadDeadline(time.Now().Add(waitFor)))
			_, err = client.Read(make([]byte, 1))
			require.Error(t, err, "peers are closed with the failed receiver")
			requireFree(t, protocolUDP, addr)
			requireFree(t, protocolTCP, addr)
		})
	}
}

func TestReaderPanicFailsOnlyTheReceiver(t *testing.T) {
	addr := freeAddress(t)
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	prepareFixture(t, c)
	s, err := c.listen(context.Background())
	require.NoError(t, err)
	s.receiver = nil // Any reader panic takes this path; a nil receiver is a deterministic trigger.
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- c.serve(context.Background(), s, func() { close(ready) }) }()
	<-ready
	conn, err := net.Dial(protocolTCP, addr)
	require.NoError(t, err)
	defer conn.Close()
	_, err = conn.Write([]byte("x:1|c\n"))
	require.NoError(t, err)
	select {
	case err := <-done:
		require.ErrorContains(t, err, "tcp listener "+addr+" client: receiver panic")
		var cause runtime.Error
		require.ErrorAs(t, err, &cause, "an error panic value stays in the chain")
	case <-time.After(waitFor):
		t.Fatal("reader panic did not fail the receiver")
	}
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
	requireFree(t, protocolUDP, addr)
	requireFree(t, protocolTCP, addr)
}

type temporaryFailingListener struct {
	net.Listener
	failures atomic.Int32
}

func (l *temporaryFailingListener) Accept() (net.Conn, error) {
	if l.failures.Add(-1) >= 0 {
		return nil, &net.OpError{
			Op:  "accept",
			Net: protocolTCP,
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
			Net: protocolUDP,
			Err: os.NewSyscallError("recvfrom", syscall.EMFILE),
		}
	}
	return c.PacketConn.ReadFrom(p)
}

func TestTemporaryListenerErrorsKeepServing(t *testing.T) {
	addr := freeAddress(t)
	c := New()
	c.Listeners = []ListenerConfig{{Protocol: protocolUDP, Address: addr}, {Protocol: protocolTCP, Address: addr}}
	c.profileDirs = nil
	f := prepareFixture(t, c)
	s, err := c.listen(context.Background())
	require.NoError(t, err)
	udp := &temporaryFailingPacketConn{
		PacketConn: s.udp[0],
	}
	udp.failures.Store(2)
	tcp := &temporaryFailingListener{
		Listener: s.tcp[0],
	}
	tcp.failures.Store(2)
	s.udp[0], s.tcp[0] = udp, tcp
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- c.serve(ctx, s, func() { close(ready) }) }()
	<-ready

	client, err := net.Dial(protocolTCP, addr)
	require.NoError(t, err)
	defer client.Close()
	_, err = client.Write([]byte("tcp:1|c\n"))
	require.NoError(t, err)
	sender, err := net.Dial(protocolUDP, addr)
	require.NoError(t, err)
	defer sender.Close()
	_, err = sender.Write([]byte("udp:1|c"))
	require.NoError(t, err)
	f.waitCounts(t, receiverCounts{
		accepted: 2,
	})
	select {
	case err := <-done:
		t.Fatalf("temporary errors ended the receiver: %v", err)
	default:
	}
	cancel()
	require.NoError(t, <-done)
}
