// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCollectorIngestsNetFlowV5(t *testing.T) {
	c := New()
	c.Address = "127.0.0.1:0"
	c.UpdateEvery = 1
	c.Aggregation.BucketSeconds = 1
	c.Aggregation.MaxBuckets = 2
	c.Aggregation.MaxKeys = 100

	require.NoError(t, c.Init(context.Background()))
	defer c.Cleanup(context.Background())

	addr := c.conn.LocalAddr().(*net.UDPAddr)
	conn, err := net.DialUDP("udp", nil, addr)
	require.NoError(t, err)
	defer conn.Close()

	payload := buildNetFlowV5Packet()
	_, err = conn.Write(payload)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	snapshot := c.aggregator.Snapshot("agent-1")
	require.NotEmpty(t, snapshot.Buckets)
	require.Greater(t, snapshot.Summaries["total_bytes"].(uint64), uint64(0))
}
