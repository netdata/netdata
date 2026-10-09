// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ipmibmc "github.com/bougou/go-ipmi/pkg/bmc"
	"github.com/bougou/go-ipmi/pkg/hal/mock"
	"github.com/bougou/go-ipmi/pkg/handlers"
	"github.com/bougou/go-ipmi/pkg/server"
	ipmitransport "github.com/bougou/go-ipmi/pkg/transport"
	"github.com/bougou/go-ipmi/pkg/transport/udp"
	"github.com/bougou/go-ipmi/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteReaderLifecycle(t *testing.T) {
	for _, driver := range []string{"lan", "lanplus"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newRemoteFixture(t)
			reader := fixture.reader(t, driver)

			require.NoError(t, reader.Check(context.Background()))
			assert.Equal(t, 0, fixture.sessions(), "Check must close its probe session")

			for range 2 {
				snapshot, err := reader.Collect(context.Background(), true)
				require.NoError(t, err)
				assertRemoteSnapshot(t, snapshot)
				assert.Equal(t, 1, fixture.sessions(), "successful collection retains one session")
			}
			assert.Equal(t, int32(2), fixture.sessionSetups.Load(), "one Check session and one reused collection session")

			fixture.malformedReading.Store(true)
			snapshot, err := reader.Collect(context.Background(), true)
			require.Error(t, err)
			assert.Nil(t, snapshot)
			assert.Equal(t, 0, fixture.sessions(), "a wire decoding failure closes the session")

			fixture.malformedReading.Store(false)
			snapshot, err = reader.Collect(context.Background(), true)
			require.NoError(t, err)
			assertRemoteSnapshot(t, snapshot)
			assert.Equal(t, 1, fixture.sessions())
			assert.Equal(t, int32(3), fixture.sessionSetups.Load(), "the next collection establishes a new session")

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			require.NoError(t, reader.Close(ctx))
			assert.Equal(t, 0, fixture.sessions(), "canceled cleanup still releases the remote session")
			require.NoError(t, reader.Close(ctx))
		})
	}
}

func TestRemoteReaderCanceledCollection(t *testing.T) {
	for _, driver := range []string{"lan", "lanplus"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newRemoteFixture(t)
			reader := fixture.reader(t, driver)
			_, err := reader.Collect(context.Background(), true)
			require.NoError(t, err)

			fixture.blockReading.Store(true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			collected := make(chan error, 1)
			go func() {
				_, err := reader.Collect(ctx, true)
				collected <- err
			}()
			// Join the collector even if an assertion fails before cancellation.
			joined := false
			defer func() {
				cancel()
				fixture.release()
				if !joined {
					<-collected
				}
			}()
			select {
			case <-fixture.readingStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("sensor request did not reach the server")
			}
			cancel()
			// The simulator serializes commands per session. Release the
			// canceled reading so it can handle the following Close Session.
			fixture.release()
			<-fixture.readingFinished
			select {
			case err = <-collected:
				joined = true
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("collection did not finish after cancellation")
			}
			assert.Equal(t, 0, fixture.sessions(), "cancellation must close the established remote session")
			fixture.blockReading.Store(false)
			snapshot, err := reader.Collect(context.Background(), true)
			require.NoError(t, err)
			assertRemoteSnapshot(t, snapshot)
		})
	}
}

func TestRemoteReaderFailedSessionSetup(t *testing.T) {
	for _, driver := range []string{"lan", "lanplus"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newRemoteFixture(t)
			reader := fixture.reader(t, driver)
			// Reject the final privilege command after session activation, so
			// failed-Connect cleanup must release a real server-side session.
			fixture.rejectPrivilege.Store(true)
			require.Error(t, reader.Check(context.Background()))
			assert.Equal(t, 0, fixture.sessions())

			fixture.rejectPrivilege.Store(false)
			require.NoError(t, reader.Check(context.Background()))
			assert.Equal(t, 0, fixture.sessions())
		})
	}
}

func TestRemoteReaderCloseDeadline(t *testing.T) {
	for _, driver := range []string{"lan", "lanplus"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newRemoteFixture(t)
			reader := fixture.readerWithTimeout(t, driver, 10*time.Second)
			_, err := reader.Collect(context.Background(), true)
			require.NoError(t, err)

			// Lose the Close Session reply after the BMC handles it. The
			// detached cleanup budget must override the longer command timeout.
			fixture.dropCloseReply.Store(true)
			started := time.Now()
			err = reader.Close(context.Background())
			elapsed := time.Since(started)
			require.Error(t, err)
			assert.GreaterOrEqual(t, elapsed, 1500*time.Millisecond)
			assert.Less(t, elapsed, 5*time.Second)
			assert.Equal(t, int32(1), fixture.conn.dropped.Load())
			assert.Equal(t, 0, fixture.sessions())
			require.NoError(t, reader.Close(context.Background()))

			snapshot, err := reader.Collect(context.Background(), true)
			require.NoError(t, err)
			assertRemoteSnapshot(t, snapshot)
			assert.Equal(t, 1, fixture.sessions())
		})
	}
}

func TestRemoteReaderCommandTimeout(t *testing.T) {
	for _, driver := range []string{"lan", "lanplus"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newRemoteFixture(t)
			reader := fixture.readerWithTimeout(t, driver, 250*time.Millisecond)
			_, err := reader.Collect(context.Background(), true)
			require.NoError(t, err)

			fixture.dropReadingReply.Store(true)
			started := time.Now()
			snapshot, err := reader.Collect(context.Background(), true)
			elapsed := time.Since(started)
			require.Error(t, err)
			var timeout net.Error
			require.ErrorAs(t, err, &timeout)
			assert.True(t, timeout.Timeout())
			assert.Nil(t, snapshot)
			assert.Less(t, elapsed, 2*time.Second)
			assert.Equal(t, int32(1), fixture.conn.dropped.Load())
			assert.Equal(t, 0, fixture.sessions(), "command timeout releases the failed session")

			snapshot, err = reader.Collect(context.Background(), true)
			require.NoError(t, err)
			assertRemoteSnapshot(t, snapshot)
			assert.Equal(t, 1, fixture.sessions())
		})
	}
}

func assertRemoteSnapshot(t *testing.T, snapshot *Snapshot) {
	t.Helper()
	require.NotNil(t, snapshot)
	require.Len(t, snapshot.Sensors, 1)
	reading := snapshot.Sensors[0]
	assert.Equal(t, "CPU Temp", reading.Name)
	assert.Equal(t, UnitCelsius, reading.Unit)
	assert.Equal(t, StateNominal, reading.State)
	require.NotNil(t, reading.Value)
	assert.Equal(t, float64(42), *reading.Value)
	require.NotNil(t, snapshot.SELEntries)
	assert.Equal(t, 7, *snapshot.SELEntries)
	assert.Empty(t, snapshot.Warnings)
}

type remoteFixture struct {
	bmc              *ipmibmc.BMC
	conn             *dropRemoteReplyConn
	dropCloseReply   atomic.Bool
	dropReadingReply atomic.Bool
	port             int
	malformedReading atomic.Bool
	blockReading     atomic.Bool
	rejectPrivilege  atomic.Bool
	sessionSetups    atomic.Int32
	readingStarted   chan struct{}
	readingFinished  chan struct{}
	releaseReading   chan struct{}
	releaseOnce      sync.Once
}

func newRemoteFixture(t *testing.T) *remoteFixture {
	t.Helper()
	f := &remoteFixture{
		readingStarted:  make(chan struct{}, 1),
		readingFinished: make(chan struct{}, 1),
		releaseReading:  make(chan struct{}),
	}
	hardware := mock.New()
	require.NoError(t, hardware.Storage().SDR().Write(context.Background(), 1, fullRecord(1, 10, "CPU Temp")))
	f.bmc = ipmibmc.New(ipmibmc.DeviceInfo{DeviceID: 32, IPMIVersion: 0x20}, [16]byte{}, hardware)
	user, err := f.bmc.Users.Add(2, "monitor")
	require.NoError(t, err)
	user.SetPassword([]byte("password"))
	user.Enabled = true
	user.ChannelAccess[1] = ipmibmc.UserChannelAccess{
		MaxPrivilege: ipmibmc.PrivilegeLevelUser,
		Enabled:      true,
	}

	standard := handlers.NewRegistry()
	handlers.RegisterAllHandlers(standard)
	registry := handlers.NewRegistry()
	handlers.RegisterAllHandlers(registry)
	registry.RegisterFunc(types.CommandCloseSession, func(ctx context.Context, hctx *handlers.HandlerContext, data []byte) ([]byte, types.CompletionCode, error) {
		if f.dropCloseReply.Swap(false) {
			f.conn.dropNext.Store(true)
		}
		command := types.CommandCloseSession
		return standard.Dispatch(ctx, hctx, uint8(command.NetFn), command.ID, data)
	})
	registry.RegisterFunc(types.CommandSetSessionPrivilegeLevel, func(ctx context.Context, hctx *handlers.HandlerContext, data []byte) ([]byte, types.CompletionCode, error) {
		f.sessionSetups.Add(1)
		if f.rejectPrivilege.Load() {
			return nil, types.CodeInsufficientPrivilege, nil
		}
		command := types.CommandSetSessionPrivilegeLevel
		return standard.Dispatch(ctx, hctx, uint8(command.NetFn), command.ID, data)
	})
	// The reference server implements SDR storage, but its hardware simulator
	// leaves sensor readings and SEL information to custom command handlers.
	registry.RegisterFunc(types.CommandGetSensorReading, func(_ context.Context, _ *handlers.HandlerContext, data []byte) ([]byte, types.CompletionCode, error) {
		if len(data) != 1 || data[0] != 10 {
			return nil, types.CodeParameterOutOfRange, nil
		}
		if f.blockReading.Load() {
			f.readingStarted <- struct{}{}
			<-f.releaseReading
			defer func() { f.readingFinished <- struct{}{} }()
		}
		if f.dropReadingReply.Swap(false) {
			f.conn.dropNext.Store(true)
		}
		if f.malformedReading.Load() {
			return []byte{42}, types.CodeOK, nil
		}
		return []byte{42, 0xc0, 0, 0}, types.CodeOK, nil
	})
	registry.RegisterFunc(types.CommandGetSELInfo, func(context.Context, *handlers.HandlerContext, []byte) ([]byte, types.CompletionCode, error) {
		data := make([]byte, 14)
		data[0] = 0x51
		binary.LittleEndian.PutUint16(data[1:3], 7)
		return data, types.CodeOK, nil
	})
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	f.port = conn.LocalAddr().(*net.UDPAddr).Port
	f.conn = &dropRemoteReplyConn{PacketConn: udp.Wrap(conn)}
	srv := server.NewServer(f.bmc, f.conn, server.WithHandlerRegistry(registry))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	t.Cleanup(func() {
		f.release()
		cancel()
		assert.NoError(t, srv.Close())
		if err := <-done; err != nil {
			assert.ErrorIs(t, err, context.Canceled)
		}
	})
	return f
}

func (f *remoteFixture) reader(t *testing.T, driver string) *Reader {
	t.Helper()
	return f.readerWithTimeout(t, driver, 2*time.Second)
}

func (f *remoteFixture) readerWithTimeout(t *testing.T, driver string, timeout time.Duration) *Reader {
	t.Helper()
	reader := New(Config{
		Driver:         driver,
		Hostname:       "127.0.0.1",
		Port:           f.port,
		Username:       "monitor",
		Password:       "password",
		PrivilegeLevel: "user",
		Timeout:        timeout,
	})
	t.Cleanup(func() { assert.NoError(t, reader.Close(context.Background())) })
	return reader
}

func (f *remoteFixture) sessions() int {
	return f.bmc.Sessions.Count() + f.bmc.V15Sessions.Count()
}

func (f *remoteFixture) release() {
	f.releaseOnce.Do(func() { close(f.releaseReading) })
}

// Drop replies at the server socket after handlers finish, so a lost response
// does not hold the simulator's per-session lock and prevent session cleanup.
type dropRemoteReplyConn struct {
	ipmitransport.PacketConn
	dropNext atomic.Bool
	dropped  atomic.Int32
}

func (c *dropRemoteReplyConn) WriteTo(data []byte, addr net.Addr) (int, error) {
	if c.dropNext.Swap(false) {
		c.dropped.Add(1)
		return len(data), nil
	}
	return c.PacketConn.WriteTo(data, addr)
}
