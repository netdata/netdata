// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenerReadLoopCountsUnexpectedReadErrors(t *testing.T) {
	reported := make(chan Endpoint, 1)
	l, err := newListener(ListenConfig{
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
	l, err := newListener(ListenConfig{
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
	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })

	var got []int
	setUDPReadBuffer = func(_ *net.UDPConn, bytes int) error {
		got = append(got, bytes)
		return nil
	}

	l, err := newListener(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: 123456,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(l.close)

	assert.Equal(t, []int{123456}, got)
}

func TestNewListenerSkipsReceiveBufferWhenZero(t *testing.T) {
	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })

	called := false
	setUDPReadBuffer = func(_ *net.UDPConn, _ int) error {
		called = true
		return nil
	}

	l, err := newListener(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	}, nil)
	require.NoError(t, err)
	t.Cleanup(l.close)

	assert.False(t, called)
}

func TestNewListenerAllowsDefaultReceiveBufferFailure(t *testing.T) {
	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })

	setUDPReadBuffer = func(_ *net.UDPConn, _ int) error {
		return errors.New("boom")
	}

	l, err := newListener(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: DefaultReceiveBuffer,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(l.close)
	require.Len(t, l.endpoints, 1)
	require.Len(t, l.receiveBufferWarnings, 1)
	assert.Equal(t, "udp4", l.receiveBufferWarnings[0].endpoint.Protocol)
	assert.Equal(t, DefaultReceiveBuffer, l.receiveBufferWarnings[0].requested)
	assert.ErrorContains(t, l.receiveBufferWarnings[0].err, "boom")
}

func TestNewListenerFailsWhenReceiveBufferCannotBeSet(t *testing.T) {
	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })

	setUDPReadBuffer = func(_ *net.UDPConn, _ int) error {
		return errors.New("boom")
	}

	l, err := newListener(ListenConfig{
		Endpoints: []Endpoint{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: 123456,
	}, nil)
	require.Error(t, err)
	assert.Nil(t, l)
	assert.Contains(t, err.Error(), "set receive buffer")
	assert.Contains(t, err.Error(), "boom")
}
