// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReceiverBindStartDeliverClose(t *testing.T) {
	listen := ListenConfig{Endpoints: []Endpoint{{Protocol: "udp", Address: "127.0.0.1", Port: 0}}}

	recv := New(NewPolicy(PolicyConfig{
		Listen:      listen,
		Versions:    []string{"v2c"},
		Communities: []string{"public"},
	}), nil)
	bindEvents, err := recv.Bind()
	require.NoError(t, err)
	assert.Empty(t, bindEvents)
	t.Cleanup(recv.Close)
	require.Len(t, recv.listener.endpoints, 1)
	boundAddr, ok := recv.listener.endpoints[0].conn.LocalAddr().(*net.UDPAddr)
	require.True(t, ok)
	port := boundAddr.Port

	results := make(chan Result, 1)
	recv.Start(func(datagram Datagram) {
		results <- recv.Process(datagram)
	})

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Write(buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1"))
	require.NoError(t, err)

	select {
	case result := <-results:
		require.NotNil(t, result.PDU)
		assert.Equal(t, model.PduTypeTrap, result.PDU.PduType)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for receiver delivery")
	}

	recv.Close()
	rebound, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	require.NoError(t, err, "receiver did not release bound endpoint")
	require.NoError(t, rebound.Close())
}

func TestListenerReadLoopCountsUnexpectedReadErrors(t *testing.T) {
	reported := make(chan Endpoint, 1)
	l, _, err := newListener(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	}, func(event Event) {
		if event.Type == EventListenerReadFailed && event.Err != nil {
			select {
			case reported <- event.Endpoint:
			default:
			}
		}
	})
	require.NoError(t, err)
	t.Cleanup(l.close)

	l.wg.Add(1)
	go l.readLoop(l.endpoints[0], func(Datagram) {})

	require.NoError(t, l.endpoints[0].conn.Close())
	select {
	case ep := <-reported:
		assert.Equal(t, "udp4", ep.Protocol)
		assert.Equal(t, "127.0.0.1", ep.Address)
	case <-time.After(time.Second):
		t.Fatal("read error callback was not called")
	}
}

func TestListenerReadLoopDoesNotReportReadErrorDuringClose(t *testing.T) {
	var reported atomic.Bool
	l, _, err := newListener(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	}, func(event Event) {
		if event.Type == EventListenerReadFailed {
			reported.Store(true)
		}
	})
	require.NoError(t, err)

	l.wg.Add(1)
	go l.readLoop(l.endpoints[0], func(Datagram) {})

	l.close()

	assert.False(t, reported.Load())
}

func TestNewListenerAppliesReceiveBuffer(t *testing.T) {
	var got []int
	setReadBuffer := func(_ *net.UDPConn, bytes int) error {
		got = append(got, bytes)
		return nil
	}

	l, bindEvents, err := newListenerWithReadBuffer(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: 123456,
	}, nil, setReadBuffer)
	require.NoError(t, err)
	t.Cleanup(l.close)

	assert.Equal(t, []int{123456}, got)
	assert.Empty(t, bindEvents)
}

func TestNewListenerSkipsReceiveBufferWhenZero(t *testing.T) {
	called := false
	setReadBuffer := func(_ *net.UDPConn, _ int) error {
		called = true
		return nil
	}

	l, bindEvents, err := newListenerWithReadBuffer(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	}, nil, setReadBuffer)
	require.NoError(t, err)
	t.Cleanup(l.close)

	assert.False(t, called)
	assert.Empty(t, bindEvents)
}

func TestNewListenerReturnsDefaultReceiveBufferFailure(t *testing.T) {
	setReadBuffer := func(_ *net.UDPConn, _ int) error {
		return errors.New("boom")
	}

	l, bindEvents, err := newListenerWithReadBuffer(ListenConfig{
		Endpoints:     []Endpoint{{Protocol: "udp4", Address: "127.0.0.1", Port: 0}},
		ReceiveBuffer: DefaultReceiveBuffer,
	}, nil, setReadBuffer)
	require.NoError(t, err)
	t.Cleanup(l.close)
	require.Len(t, l.endpoints, 1)
	require.Len(t, bindEvents, 1)
	assert.Equal(t, EventListenerBufferDegraded, bindEvents[0].Type)
	assert.Equal(t, "udp4", bindEvents[0].Endpoint.Protocol)
	assert.Equal(t, DefaultReceiveBuffer, bindEvents[0].Requested)
	assert.ErrorContains(t, bindEvents[0].Err, "boom")
}

func TestNewListenerFailsWhenReceiveBufferCannotBeSet(t *testing.T) {
	setReadBuffer := func(_ *net.UDPConn, _ int) error {
		return errors.New("boom")
	}

	l, bindEvents, err := newListenerWithReadBuffer(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: 123456,
	}, nil, setReadBuffer)
	require.Error(t, err)
	assert.Nil(t, l)
	assert.Empty(t, bindEvents)
	assert.Contains(t, err.Error(), "set receive buffer")
	assert.Contains(t, err.Error(), "boom")
}
