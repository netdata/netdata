// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCollectorReplayPcapThroughListenerToJournal(t *testing.T) {
	requireJournalctl(t)
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	port := freeUDPPort(t)
	c := New()
	c.SetJobName("e2e")
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}
	c.Versions = []string{"v2c"}
	c.Communities = []string{"public"}

	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(t.Context()) })

	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	require.Len(t, packets, 1)

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = conn.Write(packets[0].payload)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		if err := c.trapWriter.Flush(); err != nil {
			return false
		}
		out := runJournalctlAllowEmpty(t, c.journalDir, "TRAP_CATEGORY=state_change")
		return strings.Contains(out, "SNMPv2-MIB::coldStart")
	}, 5*time.Second, 50*time.Millisecond)
}
