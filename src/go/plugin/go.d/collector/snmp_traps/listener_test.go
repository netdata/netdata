// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
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
	reported := make(chan EndpointConfig, 1)
	l.onReadError = func(ep EndpointConfig, err error) {
		if err == nil {
			return
		}
		select {
		case reported <- ep:
		default:
		}
	}

	l.wg.Add(1)
	go l.readLoop(l.endpoints[0], func(_ []byte, _ net.IP, _ *net.UDPConn, _ *net.UDPAddr) {})

	require.NoError(t, l.endpoints[0].conn.Close())
	select {
	case ep := <-reported:
		assert.Equal(t, "udp4", ep.Protocol)
		assert.Equal(t, "127.0.0.1", ep.Address)
	case <-time.After(time.Second):
		t.Fatal("read error callback was not called")
	}
	require.Eventually(t, func() bool {
		return metrics.errors.listenerReadFailed.Load() > 0
	}, time.Second, 10*time.Millisecond)
}

func TestListenerReadLoopDoesNotReportReadErrorDuringClose(t *testing.T) {
	l, err := newListener("listener-close", ListenConfig{
		Endpoints: []EndpointConfig{{
			Protocol: "udp4",
			Address:  "127.0.0.1",
			Port:     0,
		}},
	})
	require.NoError(t, err)

	var reported atomic.Bool
	l.onReadError = func(EndpointConfig, error) {
		reported.Store(true)
	}

	l.wg.Add(1)
	go l.readLoop(l.endpoints[0], func(_ []byte, _ net.IP, _ *net.UDPConn, _ *net.UDPAddr) {})

	l.close()

	assert.False(t, reported.Load())
}

func TestCollectorLogListenerReadErrorIsRateLimited(t *testing.T) {
	var buf bytes.Buffer
	c := New()
	c.Logger = logger.NewWithWriter(&buf)
	ep := EndpointConfig{
		Protocol: "udp4",
		Address:  "127.0.0.1",
		Port:     9162,
	}

	c.logListenerReadError(ep, errors.New("boom"))
	c.logListenerReadError(ep, errors.New("again"))

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "SNMP trap listener read failed"))
	assert.Contains(t, out, "endpoint=udp4://127.0.0.1:9162")
	assert.Contains(t, out, "boom")
	assert.NotContains(t, out, "again")
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
