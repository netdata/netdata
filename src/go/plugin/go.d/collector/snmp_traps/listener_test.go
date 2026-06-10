// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestListenerReadLoopCountsUnexpectedReadErrors(t *testing.T) {
	l, err := newListener("listener-read-error", []EndpointConfig{{
		Protocol: "udp4",
		Address:  "127.0.0.1",
		Port:     0,
	}})
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
