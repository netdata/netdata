// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataBIRDProtocolsAllMultichannel []byte
	dataBIRDProtocolsAllLegacy       []byte
	dataBIRDProtocolsAllBird3        []byte
	dataBIRDProtocolsAllAdvanced     []byte
)

func TestParseBIRDProtocolsAllMultichannel(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllMultichannel)
	require.NoError(t, err)
	require.Len(t, protocols, 2)

	edge := protocols[0]
	assert.Equal(t, "bgp_edge", edge.Name)
	assert.Equal(t, "BGP", edge.Proto)
	assert.Equal(t, "master", edge.Table)
	assert.Equal(t, "up", edge.Status)
	assert.Equal(t, "Established", edge.Info)
	assert.Equal(t, "Edge transit", edge.Description)
	assert.Equal(t, "Established", edge.BGPState)
	assert.Equal(t, "192.0.2.2", edge.PeerAddress)
	assert.Equal(t, int64(65001), edge.RemoteAS)
	assert.Equal(t, int64(64512), edge.LocalAS)
	assert.Equal(t, int64(7200), edge.UptimeSecs)
	require.Len(t, edge.Channels, 2)
	assert.Equal(t, "ipv4", edge.Channels[0].Name)
	assert.Equal(t, "master4", edge.Channels[0].Table)
	assert.Equal(t, int64(12), edge.Channels[0].Imported)
	assert.Equal(t, int64(1), edge.Channels[0].Filtered)
	assert.Equal(t, int64(34), edge.Channels[0].Exported)
	assert.Equal(t, int64(10), edge.Channels[0].Preferred)
	assert.Equal(t, int64(6), edge.Channels[0].ImportWithdraws.Received)
	assert.Equal(t, int64(18), edge.Channels[0].ExportWithdraws.Accepted)
	assert.Equal(t, "ipv6", edge.Channels[1].Name)
	assert.Equal(t, "master6", edge.Channels[1].Table)
	assert.Equal(t, int64(3), edge.Channels[1].Imported)
	assert.Equal(t, int64(2), edge.Channels[1].Filtered)
	assert.Equal(t, int64(5), edge.Channels[1].Exported)
	assert.Equal(t, int64(2), edge.Channels[1].Preferred)
	assert.Equal(t, int64(25), edge.Channels[1].ImportWithdraws.Received)
	assert.Equal(t, int64(39), edge.Channels[1].ExportWithdraws.Accepted)

	backup := protocols[1]
	assert.Equal(t, "bgp_backup", backup.Name)
	assert.Equal(t, "start", backup.Status)
	assert.Equal(t, "Active", backup.Info)
	assert.Equal(t, "Active", backup.BGPState)
	assert.Equal(t, "2001:db8::2", backup.PeerAddress)
	assert.Equal(t, int64(65002), backup.RemoteAS)
	assert.Equal(t, int64(64512), backup.LocalAS)
	assert.Equal(t, int64(300), backup.UptimeSecs)
	require.Len(t, backup.Channels, 1)
	assert.Equal(t, "backup6", backup.Channels[0].Table)
}

func TestParseBIRDProtocolsAllLegacy(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2018, 1, 1, 2, 0, 0, 0, time.UTC))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllLegacy)
	require.NoError(t, err)
	require.Len(t, protocols, 1)

	legacy := protocols[0]
	assert.Equal(t, "bgp_legacy", legacy.Name)
	assert.Equal(t, "198.51.100.1", legacy.PeerAddress)
	assert.Equal(t, int64(3600), legacy.UptimeSecs)
	require.Len(t, legacy.Channels, 1)
	assert.Equal(t, "ipv4", legacy.Channels[0].Name)
	assert.Equal(t, int64(12), legacy.Channels[0].Imported)
	assert.Equal(t, int64(1), legacy.Channels[0].Filtered)
	assert.Equal(t, int64(34), legacy.Channels[0].Exported)
	assert.Equal(t, int64(100), legacy.Channels[0].Preferred)
}

func TestParseBIRDProtocolsAllBird3(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllBird3)
	require.NoError(t, err)
	require.Len(t, protocols, 3)

	static4 := protocols[1]
	assert.Equal(t, "static4", static4.Name)
	require.Len(t, static4.Channels, 1)
	assert.Equal(t, "master4", static4.Channels[0].Table)
	assert.Equal(t, int64(1), static4.Channels[0].ImportUpdates.Received)
	assert.Equal(t, int64(1), static4.Channels[0].ImportUpdates.Accepted)
	assert.Equal(t, int64(0), static4.Channels[0].ImportWithdraws.Ignored)

	bgp4 := protocols[2]
	assert.Equal(t, "bgp4", bgp4.Name)
	assert.Equal(t, "Active", bgp4.BGPState)
	assert.Equal(t, "198.51.100.2", bgp4.PeerAddress)
	assert.Equal(t, int64(65001), bgp4.RemoteAS)
	assert.Equal(t, int64(64512), bgp4.LocalAS)
	require.Len(t, bgp4.Channels, 1)
	assert.Equal(t, "master4", bgp4.Channels[0].Table)
}

func TestParseBIRDProtocolsAllAdvancedFamilies(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllAdvanced)
	require.NoError(t, err)
	require.Len(t, protocols, 1)

	advanced := protocols[0]
	assert.Equal(t, "bgp_adv", advanced.Name)
	assert.Equal(t, "198.51.100.2", advanced.PeerAddress)
	assert.Equal(t, int64(65010), advanced.RemoteAS)
	assert.Equal(t, int64(64512), advanced.LocalAS)
	assert.Equal(t, int64(3600), advanced.UptimeSecs)
	require.Len(t, advanced.Channels, 10)

	assert.Equal(t, "ipv4 multicast", advanced.Channels[0].Name)
	assert.Equal(t, "mcast4", advanced.Channels[0].Table)
	assert.Equal(t, int64(10), advanced.Channels[0].Imported)
	assert.Equal(t, "vpn4 mpls", advanced.Channels[4].Name)
	assert.Equal(t, "vpn4tab", advanced.Channels[4].Table)
	assert.Equal(t, int64(50), advanced.Channels[4].Imported)
	assert.Equal(t, "flow6", advanced.Channels[9].Name)
	assert.Equal(t, "flow6tab", advanced.Channels[9].Table)
	assert.Equal(t, int64(100), advanced.Channels[9].Imported)
}

func TestParseBIRDChannelFamily(t *testing.T) {
	tests := []struct {
		name     string
		channel  string
		wantAFI  string
		wantSAFI string
		ok       bool
	}{
		{name: "ipv4 unicast", channel: "ipv4", wantAFI: "ipv4", wantSAFI: "unicast", ok: true},
		{name: "ipv6 unicast", channel: "ipv6", wantAFI: "ipv6", wantSAFI: "unicast", ok: true},
		{name: "ipv4 multicast", channel: "ipv4 multicast", wantAFI: "ipv4", wantSAFI: "multicast", ok: true},
		{name: "ipv6 multicast", channel: " ipv6   multicast ", wantAFI: "ipv6", wantSAFI: "multicast", ok: true},
		{name: "ipv4 multicast alias", channel: "ipv4-mc", wantAFI: "ipv4", wantSAFI: "multicast", ok: true},
		{name: "ipv6 multicast alias", channel: "ipv6-mc", wantAFI: "ipv6", wantSAFI: "multicast", ok: true},
		{name: "ipv4 labeled", channel: "ipv4 mpls", wantAFI: "ipv4", wantSAFI: "label", ok: true},
		{name: "ipv6 labeled", channel: "ipv6 mpls", wantAFI: "ipv6", wantSAFI: "label", ok: true},
		{name: "ipv4 labeled alias", channel: "ipv4-mpls", wantAFI: "ipv4", wantSAFI: "label", ok: true},
		{name: "ipv6 labeled alias", channel: "ipv6-mpls", wantAFI: "ipv6", wantSAFI: "label", ok: true},
		{name: "vpn4", channel: "vpn4 mpls", wantAFI: "ipv4", wantSAFI: "vpn", ok: true},
		{name: "vpn6", channel: "vpn6 mpls", wantAFI: "ipv6", wantSAFI: "vpn", ok: true},
		{name: "vpn4 alias", channel: "vpn4-mpls", wantAFI: "ipv4", wantSAFI: "vpn", ok: true},
		{name: "vpn6 alias", channel: "vpn6-mpls", wantAFI: "ipv6", wantSAFI: "vpn", ok: true},
		{name: "mvpn4", channel: "vpn4 multicast", wantAFI: "ipv4", wantSAFI: "multicast_vpn", ok: true},
		{name: "mvpn6", channel: "vpn6 multicast", wantAFI: "ipv6", wantSAFI: "multicast_vpn", ok: true},
		{name: "mvpn4 alias", channel: "vpn4-mc", wantAFI: "ipv4", wantSAFI: "multicast_vpn", ok: true},
		{name: "mvpn6 alias", channel: "vpn6-mc", wantAFI: "ipv6", wantSAFI: "multicast_vpn", ok: true},
		{name: "flow4", channel: "flow4", wantAFI: "ipv4", wantSAFI: "flowspec", ok: true},
		{name: "flow6", channel: "flow6", wantAFI: "ipv6", wantSAFI: "flowspec", ok: true},
		{name: "unsupported", channel: "l2vpn evpn", ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			afi, safi, ok := parseBIRDChannelFamily(tc.channel)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.wantAFI, afi)
			assert.Equal(t, tc.wantSAFI, safi)
		})
	}
}

func TestParseBIRDUptime(t *testing.T) {
	tests := []struct {
		name  string
		now   time.Time
		value string
		want  int64
	}{
		{name: "duration", now: time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local), value: "00:05:00", want: 300},
		{name: "duration milliseconds", now: time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local), value: "06:17:34.575", want: 22654},
		{name: "unix", now: time.Unix(1514772000, 0), value: "1514768400", want: 3600},
		{name: "iso", now: time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local), value: "2026-04-03 07:00:00", want: 3600},
		{name: "iso milliseconds", now: time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local), value: "2026-04-03 07:59:30.500", want: 29},
		{name: "date only", now: time.Date(2026, 4, 3, 8, 0, 0, 0, time.Local), value: "2026-04-02", want: 115200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBIRDUptime(tc.value, tc.now)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func setBIRDNowForTest(now time.Time) func() {
	prev := birdNow
	birdNow = func() time.Time { return now }
	return func() { birdNow = prev }
}
