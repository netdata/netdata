// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"context"
	"errors"
	"net"
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
	for name, tc := range map[string]struct {
		listeners func(free, occupied string) []ListenerConfig
		wantErr   string
	}{
		"later listener fails": {
			listeners: func(free, occupied string) []ListenerConfig {
				return []ListenerConfig{
					{Protocol: protocolUDP, Address: free},
					{Protocol: protocolTCP, Address: free},
					{Protocol: protocolTCP, Address: occupied},
				}
			},
			wantErr: "listeners[2]",
		},
		"both fails after its udp socket bound": {
			listeners: func(free, occupied string) []ListenerConfig {
				return []ListenerConfig{
					{Protocol: protocolBoth, Address: free},
					{Protocol: protocolBoth, Address: occupied},
				}
			},
			wantErr: "listeners[1]",
		},
	} {
		t.Run(name, func(t *testing.T) {
			free := freeAddress(t)
			occupied, err := net.Listen(protocolTCP, "127.0.0.1:0")
			require.NoError(t, err)
			defer occupied.Close()
			requireFree(t, protocolUDP, occupied.Addr().String())

			c := New()
			c.Listeners = tc.listeners(free, occupied.Addr().String())
			c.profileDirs = nil
			prepareFixture(t, c)
			readyCalled := false
			err = c.Run(context.Background(), func() { readyCalled = true })
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.False(t, readyCalled)
			requireFree(t, protocolUDP, free)
			requireFree(t, protocolTCP, free)
			requireFree(t, protocolUDP, occupied.Addr().String())
			require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
		})
	}
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
	assert.Zero(t, f.c.diagnostics.transport.TCPConnections.Load())
	requireFree(t, protocolUDP, f.addr)
	requireFree(t, protocolTCP, f.addr)
	require.ErrorIs(t, f.c.Collect(context.Background()), rejectUnavailable)
}

// fakeServer lets a test fail the server and observe what the collector does
// before closing it.
type fakeServer struct {
	failed  chan struct{}
	err     error
	started bool
	onClose func()
}

func (s *fakeServer) Start()                  { s.started = true }
func (s *fakeServer) Failed() <-chan struct{} { return s.failed }
func (s *fakeServer) Close()                  { s.onClose() }
func (s *fakeServer) Err() error              { return s.err }

func TestServerFailureStopsAdmissionBeforeClose(t *testing.T) {
	c := New()
	c.Listeners = testListeners
	c.profileDirs = nil
	f := prepareFixture(t, c)
	lost := errors.New("udp listener lost")
	var ingestAtClose error
	s := &fakeServer{
		failed: make(chan struct{}),
		err:    lost,
	}
	s.onClose = func() { ingestAtClose = c.receiver.ingest("late:1|c", time.Now()) }
	done := make(chan error, 1)
	ready := make(chan struct{})
	go func() { done <- c.serve(context.Background(), s, func() { close(ready) }) }()
	<-ready
	require.NoError(t, c.receiver.ingest("held:10|g", time.Now()))

	close(s.failed)
	select {
	case err := <-done:
		require.ErrorIs(t, err, lost, "Run reports the server failure")
	case <-time.After(waitFor):
		t.Fatal("server failure did not end the receiver")
	}
	assert.True(t, s.started)
	require.ErrorIs(t, ingestAtClose, rejectUnavailable, "admission stops before sockets close")
	// No held gauge or empty interval can be published after failure.
	f.cycle.BeginCycle()
	require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
	f.cycle.AbortCycle()
}
