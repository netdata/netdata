// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

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
	l, err := newListener("listener-read-error", ListenConfig{
		Endpoints: []EndpointConfig{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	})
	require.NoError(t, err)
	t.Cleanup(l.close)

	metrics := &perJobMetrics{}
	l.metrics = metrics

	l.wg.Add(1)
	go l.readLoop(l.endpoints[0], func(_ []byte, _ net.IP, _ *net.UDPConn, _ *net.UDPAddr) {})

	require.NoError(t, l.endpoints[0].conn.Close())
	require.Eventually(t, func() bool {
		return atomic.LoadUint64(&metrics.errors.listenerReadFailed) > 0
	}, time.Second, 10*time.Millisecond)
}

func TestNewListenerAppliesReceiveBuffer(t *testing.T) {
	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })

	var got []int
	setUDPReadBuffer = func(_ *net.UDPConn, bytes int) error {
		got = append(got, bytes)
		return nil
	}

	l, err := newListener("listener-buffer", ListenConfig{
		Endpoints: []EndpointConfig{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: 123456,
	})
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

	l, err := newListener("listener-buffer-zero", ListenConfig{
		Endpoints: []EndpointConfig{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	})
	require.NoError(t, err)
	t.Cleanup(l.close)

	assert.False(t, called)
}

func TestNewListenerFailsWhenReceiveBufferCannotBeSet(t *testing.T) {
	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })

	setUDPReadBuffer = func(_ *net.UDPConn, _ int) error {
		return errors.New("boom")
	}

	l, err := newListener("listener-buffer-error", ListenConfig{
		Endpoints: []EndpointConfig{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
		ReceiveBuffer: 123456,
	})
	require.Error(t, err)
	assert.Nil(t, l)
	assert.Contains(t, err.Error(), "set receive buffer")
	assert.Contains(t, err.Error(), "boom")
}
