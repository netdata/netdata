// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/traptest"
	"github.com/stretchr/testify/require"
)

func TestCollectorReplayPcapThroughListenerToJournal(t *testing.T) {
	requireJournalctl(t)
	manager := setMinimalProfileDir(t)
	withTestCacheDir(t)

	port := freeUDPPort(t)
	c := newTestSNMPTrapsCollectorWithCatalog(manager)
	c.Name = "e2e"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}
	c.Versions = []string{"v2c"}
	c.Communities = []string{"public"}

	require.NoError(t, c.Init(t.Context()))
	t.Cleanup(func() { c.Cleanup(t.Context()) })

	packets := traptest.ReadPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	require.Len(t, packets, 1)
	packet := packets[0]

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = conn.Write(packet.Payload)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		out := runJournalctlAllowEmpty(t, journal.Root(c.Name), "TRAP_CATEGORY=state_change")
		return strings.Contains(out, "SNMPv2-MIB::coldStart")
	}, 5*time.Second, 50*time.Millisecond)
}
