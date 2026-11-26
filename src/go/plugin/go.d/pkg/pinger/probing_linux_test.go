// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package pinger

import (
	"context"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Linux refuses a send to a broadcast address with EACCES until the socket
// has SO_BROADCAST. pro-bing sets the flag and logs the retry, so a nil
// pro-bing logger crashed the whole process here. Only unprivileged sockets
// lack the flag: Go sets it on the raw sockets it creates.
func TestProbe_UnprivilegedBroadcastDestination(t *testing.T) {
	client, err := New(Config{
		Probe: ProbeConfig{
			Network:  "ip",
			Packets:  1,
			Interval: confopt.Duration(10 * time.Millisecond),
			Timeout:  200 * time.Millisecond,
		},
	}, logger.NewWithWriter(nil))
	require.NoError(t, err)

	sample, err := client.Probe(context.Background(), "127.255.255.255")
	if icmpSocketNotPermitted(err) {
		t.Skipf("unprivileged ICMP sockets are not permitted (net.ipv4.ping_group_range): %v", err)
	}
	require.NoError(t, err)
	assert.Equal(t, int64(1), sample.PacketsSent)
}

func icmpSocketNotPermitted(err error) bool {
	var sysErr *os.SyscallError
	return errors.As(err, &sysErr) && sysErr.Syscall == "socket" &&
		(errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM))
}
