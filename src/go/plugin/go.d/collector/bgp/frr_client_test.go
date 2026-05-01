// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFRRClient_ReusesSocketConnection(t *testing.T) {
	server := newFRRReplayServer(t, map[string][]byte{
		"show bgp vrf all ipv4 summary json": []byte(`{"ipv4Unicast":{"routerId":"192.0.2.254"}}`),
		"show bgp vrf all neighbors json":    []byte(`{}`),
	})

	client := &frrClient{
		socketPath: server.socketPath,
		timeout:    time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	data, err := client.Summary("ipv4", "unicast")
	require.NoError(t, err)
	assert.JSONEq(t, `{"ipv4Unicast":{"routerId":"192.0.2.254"}}`, string(data))

	data, err = client.Neighbors()
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))

	assert.Equal(t, []string{"enable", "show bgp vrf all ipv4 summary json", "show bgp vrf all neighbors json"}, server.commands())
	assert.Equal(t, 1, server.connectionCount())
	server.assertNoError(t)
}

func TestFRRClient_ReconnectsAfterServerClose(t *testing.T) {
	server := newFRRReplayServerClosing(t, map[string][]byte{
		"show bgp vrf all ipv4 summary json": []byte(`{"ipv4Unicast":{"routerId":"192.0.2.254"}}`),
	})

	client := &frrClient{
		socketPath: server.socketPath,
		timeout:    time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	for i := 0; i < 2; i++ {
		data, err := client.Summary("ipv4", "unicast")
		require.NoError(t, err)
		assert.JSONEq(t, `{"ipv4Unicast":{"routerId":"192.0.2.254"}}`, string(data))
	}

	assert.Equal(t, []string{"enable", "show bgp vrf all ipv4 summary json", "enable", "show bgp vrf all ipv4 summary json"}, server.commands())
	assert.Equal(t, 2, server.connectionCount())
	server.assertNoError(t)
}

func TestFRRClient_EnablesEachSocketOnce(t *testing.T) {
	bgpServer := newFRRReplayServer(t, map[string][]byte{
		"show bgp vrf all ipv4 summary json": []byte(`{"ipv4Unicast":{"routerId":"192.0.2.254"}}`),
	})
	zebraServer := newFRRReplayServer(t, map[string][]byte{
		"show evpn vni json": []byte(`{"vniCount":1}`),
	})

	client := &frrClient{
		socketPath:      bgpServer.socketPath,
		zebraSocketPath: zebraServer.socketPath,
		timeout:         time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	data, err := client.Summary("ipv4", "unicast")
	require.NoError(t, err)
	assert.JSONEq(t, `{"ipv4Unicast":{"routerId":"192.0.2.254"}}`, string(data))

	data, err = client.EVPNVNI()
	require.NoError(t, err)
	assert.JSONEq(t, `{"vniCount":1}`, string(data))

	data, err = client.Summary("ipv4", "unicast")
	require.NoError(t, err)
	assert.JSONEq(t, `{"ipv4Unicast":{"routerId":"192.0.2.254"}}`, string(data))

	assert.Equal(t, []string{"enable", "show bgp vrf all ipv4 summary json", "show bgp vrf all ipv4 summary json"}, bgpServer.commands())
	assert.Equal(t, []string{"enable", "show evpn vni json"}, zebraServer.commands())
	assert.Equal(t, 1, bgpServer.connectionCount())
	assert.Equal(t, 1, zebraServer.connectionCount())
	bgpServer.assertNoError(t)
	zebraServer.assertNoError(t)
}
